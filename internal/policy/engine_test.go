package policy_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"halo/internal/access"
	"halo/internal/policy"
	"halo/internal/store"
	"halo/internal/testdb"
)

const firefox = "Mozilla/5.0 (X11; Linux x86_64; rv:143.0) Gecko/20100101 Firefox/143.0"

var everyone = store.PolicyConditions{AllUsers: true, AllApps: true}

func group(t *testing.T, st *store.Store, name string) store.Group {
	t.Helper()
	g, err := st.CreateGroup(context.Background(), store.NewGroup{Name: name, Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func member(t *testing.T, st *store.Store, email string, groupIDs ...string) store.User {
	t.Helper()
	u, err := st.CreateUser(context.Background(), store.NewUser{Email: email, Name: strings.Split(email, "@")[0], Status: "active", GroupIDs: groupIDs})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func rule(t *testing.T, st *store.Store, name, mode, effect string, c store.PolicyConditions) store.AccessPolicy {
	t.Helper()
	p, err := st.CreateAccessPolicy(context.Background(), store.AccessPolicy{Name: name, Enabled: true, Mode: mode, Effect: effect, Conditions: c})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func zone(t *testing.T, st *store.Store, name, kind string, cidrs ...string) store.NetworkZone {
	t.Helper()
	z, err := st.SaveNetworkZone(context.Background(), store.NetworkZone{Name: name, Kind: kind, CIDRs: cidrs})
	if err != nil {
		t.Fatal(err)
	}
	return z
}

func evaluate(t *testing.T, eng access.Engine, u store.User, method, ip, token string) access.Decision {
	t.Helper()
	d, err := eng.Evaluate(context.Background(), access.Input{User: u, Method: method, IP: ip, UserAgent: firefox, DeviceToken: token})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func want(t *testing.T, d access.Decision, effect, policyName string) {
	t.Helper()
	if d.Effect != effect || d.Policy != policyName {
		t.Fatalf("want %s by %q, got %s by %q (%s)", effect, policyName, d.Effect, d.Policy, d.Reason)
	}
}

func setEnabled(t *testing.T, st *store.Store, p store.AccessPolicy, enabled bool) {
	t.Helper()
	p.Enabled = enabled
	if _, err := st.UpdateAccessPolicy(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

func TestMostRestrictiveEnforcedPolicyWins(t *testing.T) {
	st := testdb.New(t)
	eng := policy.NewEngine(st)
	ada := member(t, st, "ada@example.com")
	allow := rule(t, st, "Allow everyone", "enforce", access.Allow, everyone)
	block := rule(t, st, "Block everyone", "enforce", access.Block, everyone)
	require := rule(t, st, "Require passkeys", "enforce", access.RequirePhishingResistant, everyone)

	d := evaluate(t, eng, ada, "totp", "198.51.100.7", "")
	want(t, d, access.Block, block.Name)
	if d.Reason != "Blocks every sign-in." || d.EventID == "" || d.Risk != "none" {
		t.Fatalf("decision details: %+v", d)
	}
	setEnabled(t, st, block, false)
	want(t, evaluate(t, eng, ada, "totp", "198.51.100.7", ""), access.RequirePhishingResistant, require.Name)
	setEnabled(t, st, require, false)
	want(t, evaluate(t, eng, ada, "totp", "198.51.100.7", ""), access.Allow, allow.Name)
	setEnabled(t, st, allow, false)
	want(t, evaluate(t, eng, ada, "totp", "198.51.100.7", ""), access.Allow, "")
}

func TestReportOnlyRecordsButNeverBlocks(t *testing.T) {
	st := testdb.New(t)
	eng := policy.NewEngine(st)
	ada := member(t, st, "ada@example.com")
	report := rule(t, st, "Block everyone (trial)", "report", access.Block, everyone)
	require := rule(t, st, "Passkeys (trial)", "report", access.RequirePhishingResistant, everyone)

	want(t, evaluate(t, eng, ada, "totp", "198.51.100.7", ""), access.Allow, "")
	want(t, evaluate(t, eng, ada, "passkey", "198.51.100.7", ""), access.Allow, "")

	activity, err := st.PolicyActivitySince(context.Background(), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got := activity[report.ID]; got.Matched != 2 || got.Stopped != 2 {
		t.Fatalf("report-only block activity: %+v", got)
	}
	if got := activity[require.ID]; got.Matched != 2 || got.Stopped != 1 {
		t.Fatalf("report-only require activity: %+v", got)
	}
}

func TestExclusionsAlwaysWin(t *testing.T) {
	st := testdb.New(t)
	eng := policy.NewEngine(st)
	breakGlass := group(t, st, "Break glass")
	contractors := group(t, st, "Contractors")
	bob := member(t, st, "bob@example.com", breakGlass.ID, contractors.ID)
	carol := member(t, st, "carol@example.com", contractors.ID)
	dave := member(t, st, "dave@example.com", contractors.ID)
	erin := member(t, st, "erin@example.com")
	block := rule(t, st, "Block contractors", "enforce", access.Block, store.PolicyConditions{GroupIDs: []string{contractors.ID}, ExcludeGroupIDs: []string{breakGlass.ID}, ExcludeUserIDs: []string{dave.ID}, AllApps: true})

	want(t, evaluate(t, eng, bob, "passkey", "198.51.100.7", ""), access.Allow, "")
	d := evaluate(t, eng, carol, "passkey", "198.51.100.7", "")
	want(t, d, access.Block, block.Name)
	if d.Reason != "Blocks sign-ins by members of Contractors." {
		t.Fatalf("reason: %s", d.Reason)
	}
	want(t, evaluate(t, eng, dave, "passkey", "198.51.100.7", ""), access.Allow, "")
	want(t, evaluate(t, eng, erin, "passkey", "198.51.100.7", ""), access.Allow, "")
}

func TestNetworkZonesMatchIPv4AndIPv6(t *testing.T) {
	st := testdb.New(t)
	eng := policy.NewEngine(st)
	ada := member(t, st, "ada@example.com")
	lab := zone(t, st, "Lab", "trusted", "203.0.113.0/24", "2001:db8::/32")
	p := rule(t, st, "Block the lab", "enforce", access.Block, store.PolicyConditions{AllUsers: true, AllApps: true, Network: "in", ZoneIDs: []string{lab.ID}})

	for ip, effect := range map[string]string{"203.0.113.7": access.Block, "2001:db8::1": access.Block, "::ffff:203.0.113.9": access.Block, "198.51.100.1": access.Allow, "2001:db9::1": access.Allow, "not-an-ip": access.Allow} {
		if d := evaluate(t, eng, ada, "passkey", ip, ""); d.Effect != effect {
			t.Fatalf("in lab, %s: want %s, got %s", ip, effect, d.Effect)
		}
	}
	p.Conditions.Network = "not-in"
	if _, err := st.UpdateAccessPolicy(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	for ip, effect := range map[string]string{"203.0.113.7": access.Allow, "198.51.100.1": access.Block} {
		if d := evaluate(t, eng, ada, "passkey", ip, ""); d.Effect != effect {
			t.Fatalf("outside lab, %s: want %s, got %s", ip, effect, d.Effect)
		}
	}
	if d := evaluate(t, eng, ada, "passkey", "198.51.100.1", ""); d.Reason != "Blocks sign-ins from outside “Lab”." {
		t.Fatalf("reason: %s", d.Reason)
	}
}

func TestDisallowedMethodBlocks(t *testing.T) {
	st := testdb.New(t)
	eng := policy.NewEngine(st)
	ada := member(t, st, "ada@example.com")
	if err := st.SetMethodEnabled(context.Background(), "totp", false); err != nil {
		t.Fatal(err)
	}
	d := evaluate(t, eng, ada, "totp", "198.51.100.7", "")
	want(t, d, access.Block, "")
	if d.Reason != "Sign-in with authenticator apps is turned off for this organization." {
		t.Fatalf("reason: %s", d.Reason)
	}
	want(t, evaluate(t, eng, ada, "passkey", "198.51.100.7", ""), access.Allow, "")
}

func TestDeviceTrust(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	eng := policy.NewEngine(st)
	ada := member(t, st, "ada@example.com")
	laptop := strings.Repeat("l", 43)
	want(t, evaluate(t, eng, ada, "passkey", "198.51.100.7", laptop), access.Allow, "")
	devices, err := st.ListDevices(ctx, ada.ID)
	if err != nil || len(devices) != 1 || devices[0].SignIns != 1 || devices[0].Trust != "unknown" || devices[0].Name != "Firefox on Linux computer" || devices[0].LastIP != "198.51.100.7" {
		t.Fatalf("device after first sign-in: %+v %v", devices, err)
	}

	if err := st.SetDeviceTrust(ctx, devices[0].ID, "blocked"); err != nil {
		t.Fatal(err)
	}
	d := evaluate(t, eng, ada, "passkey", "198.51.100.7", laptop)
	want(t, d, access.Block, "")
	if d.Reason != "An administrator blocked this device." {
		t.Fatalf("reason: %s", d.Reason)
	}
	if again, _ := st.GetDevice(ctx, devices[0].ID); again.SignIns != 1 {
		t.Fatalf("blocked attempt counted as a sign-in: %+v", again)
	}

	if err := st.SetDeviceTrust(ctx, devices[0].ID, "trusted"); err != nil {
		t.Fatal(err)
	}
	p := rule(t, st, "Trusted devices only", "enforce", access.Block, store.PolicyConditions{AllUsers: true, AllApps: true, Device: "untrusted"})
	want(t, evaluate(t, eng, ada, "passkey", "198.51.100.7", laptop), access.Allow, "")
	d = evaluate(t, eng, ada, "passkey", "198.51.100.7", strings.Repeat("p", 43))
	want(t, d, access.Block, p.Name)
	if d.Reason != "Blocks sign-ins on devices that aren't trusted." {
		t.Fatalf("reason: %s", d.Reason)
	}
}

func riskEvents(t *testing.T, st *store.Store, userID string) []store.RiskEvent {
	t.Helper()
	all, err := st.ListRiskEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	mine := []store.RiskEvent{}
	for _, e := range all {
		if e.UserID != nil && *e.UserID == userID {
			mine = append(mine, e)
		}
	}
	return mine
}

func TestRiskSignals(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	eng := policy.NewEngine(st)

	ada := member(t, st, "ada@example.com")
	if d := evaluate(t, eng, ada, "passkey", "198.51.100.7", strings.Repeat("a", 43)); d.Risk != "none" {
		t.Fatalf("first device is not a new-device risk: %s", d.Risk)
	}
	d := evaluate(t, eng, ada, "passkey", "198.51.100.7", strings.Repeat("b", 43))
	events := riskEvents(t, st, ada.ID)
	if d.Risk != "medium" || len(events) != 1 || events[0].Type != "new-device" || events[0].Level != "medium" || events[0].DeviceID == nil || events[0].Device != "Firefox on Linux computer" {
		t.Fatalf("new device: %s %+v", d.Risk, events)
	}

	bea := member(t, st, "bea@example.com")
	for range 5 {
		if err := st.RecordSignIn(ctx, store.SignInEvent{UserID: &bea.ID, Email: bea.Email, Result: "failure", Method: "passkey", Reason: "The passkey signature could not be verified."}); err != nil {
			t.Fatal(err)
		}
	}
	d = evaluate(t, eng, bea, "totp", "198.51.100.7", "")
	if events := riskEvents(t, st, bea.ID); d.Risk != "high" || len(events) != 1 || events[0].Type != "failed-sign-ins" || events[0].Detail != "5 failed sign-ins in the last 15 minutes." {
		t.Fatalf("failure burst: %s %+v", d.Risk, events)
	}

	cy := member(t, st, "cy@example.com")
	zone(t, st, "Tor exit nodes", "risky", "185.220.101.0/24")
	block := rule(t, st, "Block high-risk sign-ins", "enforce", access.Block, store.PolicyConditions{AllUsers: true, AllApps: true, Risk: []string{"high"}})
	d = evaluate(t, eng, cy, "passkey", "185.220.101.47", "")
	want(t, d, access.Block, block.Name)
	if d.Reason != "Blocks sign-ins with high risk. This sign-in was high risk: 185.220.101.47 is in the risky network “Tor exit nodes”." {
		t.Fatalf("reason: %s", d.Reason)
	}
	events = riskEvents(t, st, cy.ID)
	if len(events) != 1 || events[0].Type != "risky-network" || events[0].SignInID != nil || events[0].Status != "open" {
		t.Fatalf("risky network events: %+v", events)
	}
	if err := st.RecordSignIn(ctx, store.SignInEvent{ID: d.EventID, UserID: &cy.ID, Email: cy.Email, Result: "failure", Method: "passkey", IP: "185.220.101.47", Risk: d.Risk, Reason: d.Reason}); err != nil {
		t.Fatal(err)
	}
	if events = riskEvents(t, st, cy.ID); events[0].SignInID == nil || *events[0].SignInID != d.EventID {
		t.Fatalf("risk event does not link to the sign-in recorded with the decision's event ID: %+v", events[0])
	}

	dee := member(t, st, "dee@example.com")
	if err := st.RecordSignIn(ctx, store.SignInEvent{UserID: &dee.ID, Email: dee.Email, Result: "success", Method: "passkey", IP: "145.53.112.10"}); err != nil {
		t.Fatal(err)
	}
	if d := evaluate(t, eng, dee, "passkey", "145.53.112.99", ""); d.Risk != "none" {
		t.Fatalf("known /24: %s", d.Risk)
	}
	if d := evaluate(t, eng, dee, "passkey", "81.2.69.1", ""); d.Risk != "low" || len(riskEvents(t, st, dee.ID)) != 0 {
		t.Fatalf("new /24 should be low risk without a risk event: %s", d.Risk)
	}
}

func TestSeedPolicies(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if err := st.SeedDemo(ctx); err != nil {
		t.Fatal(err)
	}
	policies, _ := st.ListAccessPolicies(ctx)
	zones, _ := st.ListNetworkZones(ctx)
	devices, _ := st.ListDevices(ctx, "")
	events, _ := st.ListRiskEvents(ctx)
	if len(policies) != 3 || policies[0].Name != "Block high-risk sign-ins" || len(policies[2].Conditions.AppIDs) != 2 || len(zones) != 2 || len(devices) == 0 {
		t.Fatalf("seed: %d policies, %d zones, %d devices", len(policies), len(zones), len(devices))
	}
	if len(events) != 1 || events[0].Type != "risky-network" || events[0].SignInID == nil || events[0].IP != "185.220.101.47" {
		t.Fatalf("seeded risk events: %+v", events)
	}
	activity, err := st.PolicyActivitySince(ctx, time.Now().Add(-24*time.Hour))
	if err != nil || activity[policies[1].ID].Stopped == 0 {
		t.Fatalf("report-only activity: %+v %v", activity, err)
	}
}
