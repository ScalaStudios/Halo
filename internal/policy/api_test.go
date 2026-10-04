package policy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"halo/internal/access"
	"halo/internal/auth"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/policy"
	"halo/internal/store"
	"halo/internal/testdb"
)

func server(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	st := testdb.New(t)
	public, _ := url.Parse("http://localhost:3200")
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	mux := http.NewServeMux()
	if err := policy.Register(mux, httpx.Deps{Config: config.Config{PublicURL: public, Dev: true}, Store: st, Auth: authn}); err != nil {
		t.Fatal(err)
	}
	return st, authn.Resolve(authn.SameOrigin(mux))
}

func admin(t *testing.T, st *store.Store, email string, roles ...string) (store.User, string) {
	t.Helper()
	ctx := context.Background()
	u, err := st.CreateUser(ctx, store.NewUser{Email: email, Name: strings.Split(email, "@")[0], Status: "active", Roles: roles})
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := st.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: "passkey"})
	if err != nil {
		t.Fatal(err)
	}
	return u, token
}

func call(t *testing.T, h http.Handler, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: token})
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func expect(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("want %d, got %d: %s", status, rec.Code, rec.Body)
	}
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return v
}

func body(name, mode, effect string, c store.PolicyConditions) map[string]any {
	return map[string]any{"name": name, "description": "", "enabled": true, "mode": mode, "effect": effect, "conditions": c}
}

func TestRoleEnforcement(t *testing.T) {
	st, h := server(t)
	_, auditor := admin(t, st, "audrey@example.com", "auditor")
	_, userAdmin := admin(t, st, "uma@example.com", "user_admin")
	_, security := admin(t, st, "sec@example.com", "security_admin")
	_, nobody := admin(t, st, "nora@example.com")
	create := body("Block everyone", "report", access.Block, everyone)

	for _, path := range []string{"/api/v1/policies", "/api/v1/network-zones", "/api/v1/methods", "/api/v1/devices", "/api/v1/risk-events"} {
		expect(t, call(t, h, auditor, "GET", path, nil), 200)
		expect(t, call(t, h, nobody, "GET", path, nil), 403)
		expect(t, call(t, h, "", "GET", path, nil), 401)
	}
	expect(t, call(t, h, auditor, "POST", "/api/v1/policies", create), 403)
	expect(t, call(t, h, userAdmin, "PUT", "/api/v1/methods/totp", map[string]any{"enabled": false}), 403)
	expect(t, call(t, h, auditor, "PUT", "/api/v1/devices/dev_missing", map[string]any{"trust": "trusted"}), 403)
	expect(t, call(t, h, auditor, "POST", "/api/v1/network-zones", map[string]any{"name": "Office", "kind": "trusted", "cidrs": []string{"203.0.113.0/24"}}), 403)
	expect(t, call(t, h, security, "POST", "/api/v1/policies", create), 201)
	expect(t, call(t, h, security, "PUT", "/api/v1/methods/totp", map[string]any{"enabled": false}), 204)
	expect(t, call(t, h, nobody, "GET", "/api/v1/me/devices", nil), 200)
	expect(t, call(t, h, "", "GET", "/api/v1/me/devices", nil), 401)
}

func TestPolicyLifecycle(t *testing.T) {
	st, h := server(t)
	ctx := context.Background()
	_, token := admin(t, st, "sec@example.com", "security_admin")
	admins := group(t, st, "Admins")

	missing := body("Admins", "report", access.RequirePhishingResistant, store.PolicyConditions{AllApps: true})
	expect(t, call(t, h, token, "POST", "/api/v1/policies", missing), 422)
	unknown := body("Admins", "report", access.RequirePhishingResistant, store.PolicyConditions{GroupIDs: []string{"grp_missing"}, AllApps: true})
	expect(t, call(t, h, token, "POST", "/api/v1/policies", unknown), 422)

	rec := call(t, h, token, "POST", "/api/v1/policies", body("Admins need passkeys", "report", access.RequirePhishingResistant, store.PolicyConditions{GroupIDs: []string{admins.ID, admins.ID}, AllApps: true, AppIDs: []string{"halo"}}))
	expect(t, rec, 201)
	first := decode[store.AccessPolicy](t, rec)
	if first.Priority != 1 || !slices.Equal(first.Conditions.GroupIDs, []string{admins.ID}) || len(first.Conditions.AppIDs) != 0 || first.Conditions.Network != "any" || first.Conditions.Device != "any" {
		t.Fatalf("created policy was not normalized: %+v", first)
	}
	expect(t, call(t, h, token, "POST", "/api/v1/policies", body("admins need PASSKEYS", "report", access.Block, everyone)), 409)
	second := decode[store.AccessPolicy](t, call(t, h, token, "POST", "/api/v1/policies", body("Block everyone", "report", access.Block, everyone)))

	update := body("Admins need passkeys", "enforce", access.RequirePhishingResistant, first.Conditions)
	expect(t, call(t, h, token, "PUT", "/api/v1/policies/"+first.ID, update), 200)
	audit, _ := st.ListAudit(ctx, 1)
	if audit[0].Action != "policy.update" || audit[0].Summary != "Switched to enforced" || audit[0].TargetID != first.ID {
		t.Fatalf("update audit: %+v", audit[0])
	}

	expect(t, call(t, h, token, "PUT", "/api/v1/policies/order", map[string]any{"ids": []string{second.ID}}), 422)
	expect(t, call(t, h, token, "PUT", "/api/v1/policies/order", map[string]any{"ids": []string{second.ID, first.ID}}), 204)
	list := decode[[]store.AccessPolicy](t, call(t, h, token, "GET", "/api/v1/policies", nil))
	if len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID || list[1].Mode != "enforce" {
		t.Fatalf("order after reorder: %+v", list)
	}

	expect(t, call(t, h, token, "DELETE", "/api/v1/policies/"+second.ID, nil), 204)
	expect(t, call(t, h, token, "GET", "/api/v1/policies/"+second.ID, nil), 404)
}

func TestNetworkZoneValidation(t *testing.T) {
	st, h := server(t)
	_, token := admin(t, st, "sec@example.com", "security_admin")

	bad := call(t, h, token, "POST", "/api/v1/network-zones", map[string]any{"name": "Office", "kind": "trusted", "cidrs": []string{"203.0.113.0/24", "10.0.0.300/8"}})
	expect(t, bad, 422)
	if !strings.Contains(bad.Body.String(), "10.0.0.300/8") {
		t.Fatalf("error should name the bad range: %s", bad.Body)
	}
	expect(t, call(t, h, token, "POST", "/api/v1/network-zones", map[string]any{"name": "Office", "kind": "safe", "cidrs": []string{"203.0.113.0/24"}}), 422)
	expect(t, call(t, h, token, "POST", "/api/v1/network-zones", map[string]any{"name": "Office", "kind": "trusted", "cidrs": []string{" "}}), 422)

	rec := call(t, h, token, "POST", "/api/v1/network-zones", map[string]any{"name": "Office", "kind": "trusted", "cidrs": []string{"203.0.113.77/24", "2001:DB8::/32", "198.51.100.4"}})
	expect(t, rec, 201)
	office := decode[store.NetworkZone](t, rec)
	if !slices.Equal(office.CIDRs, []string{"203.0.113.0/24", "2001:db8::/32", "198.51.100.4/32"}) {
		t.Fatalf("ranges were not normalized: %v", office.CIDRs)
	}
	expect(t, call(t, h, token, "POST", "/api/v1/network-zones", map[string]any{"name": "office", "kind": "risky", "cidrs": []string{"192.0.2.0/24"}}), 409)

	expect(t, call(t, h, token, "POST", "/api/v1/policies", body("Office only", "enforce", access.Block, store.PolicyConditions{AllUsers: true, AllApps: true, Network: "not-in", ZoneIDs: []string{office.ID}})), 201)
	inUse := call(t, h, token, "DELETE", "/api/v1/network-zones/"+office.ID, nil)
	expect(t, inUse, 409)
	if !strings.Contains(inUse.Body.String(), "Office only") {
		t.Fatalf("conflict should name the policy: %s", inUse.Body)
	}
}

func TestSimulate(t *testing.T) {
	st, h := server(t)
	_, token := admin(t, st, "audrey@example.com", "auditor")
	admins := group(t, st, "Admins")
	ada := member(t, st, "ada@example.com", admins.ID)
	zone(t, st, "Tor exit nodes", "risky", "185.220.101.0/24")
	rule(t, st, "Block high-risk sign-ins", "enforce", access.Block, store.PolicyConditions{AllUsers: true, AllApps: true, Risk: []string{"high"}})
	rule(t, st, "Admins need passkeys", "report", access.RequirePhishingResistant, store.PolicyConditions{GroupIDs: []string{admins.ID}, AllApps: true})

	type simulated struct {
		Effect, Outcome, Policy, Reason, Risk string
		Signals                               []struct{ Type, Level, Detail string }
		Policies                              []struct {
			Name, Mode, Outcome, Detail string
			Matched                     bool
		}
	}
	rec := call(t, h, token, "POST", "/api/v1/policies/simulate", map[string]any{"userId": ada.ID, "appId": "halo", "ip": "185.220.101.47", "method": "totp", "deviceTrust": "unknown"})
	expect(t, rec, 200)
	risky := decode[simulated](t, rec)
	if risky.Effect != access.Block || risky.Outcome != "block" || risky.Policy != "Block high-risk sign-ins" || risky.Risk != "high" || len(risky.Signals) != 1 || len(risky.Policies) != 2 {
		t.Fatalf("risky simulation: %+v", risky)
	}
	if p := risky.Policies[1]; !p.Matched || p.Outcome != "require" || p.Mode != "report" {
		t.Fatalf("report-only policy in simulation: %+v", p)
	}

	calm := decode[simulated](t, call(t, h, token, "POST", "/api/v1/policies/simulate", map[string]any{"userId": ada.ID, "appId": "halo", "ip": "198.51.100.7", "method": "passkey", "deviceTrust": "trusted"}))
	if calm.Effect != access.Allow || calm.Outcome != "allow" || calm.Risk != "none" || calm.Policies[0].Matched || calm.Policies[0].Detail != "The sign-in risk was none." || calm.Policies[1].Outcome != "allow" {
		t.Fatalf("calm simulation: %+v", calm)
	}

	blocked := decode[simulated](t, call(t, h, token, "POST", "/api/v1/policies/simulate", map[string]any{"userId": ada.ID, "appId": "halo", "ip": "198.51.100.7", "method": "passkey", "deviceTrust": "blocked"}))
	if blocked.Effect != access.Block || blocked.Reason != "An administrator blocked this device." {
		t.Fatalf("blocked device simulation: %+v", blocked)
	}

	expect(t, call(t, h, token, "POST", "/api/v1/policies/simulate", map[string]any{"userId": ada.ID, "appId": "halo", "ip": "nope", "method": "passkey", "deviceTrust": "trusted"}), 422)
	expect(t, call(t, h, token, "POST", "/api/v1/policies/simulate", map[string]any{"userId": ada.ID, "appId": "app_missing", "ip": "198.51.100.7", "method": "passkey", "deviceTrust": "trusted"}), 422)
}

func TestMethodsAndDevicesAPI(t *testing.T) {
	st, h := server(t)
	ctx := context.Background()
	_, token := admin(t, st, "sec@example.com", "security_admin")
	totpOnly := member(t, st, "tia@example.com")
	if _, err := st.CreateTOTPSecret(ctx, totpOnly.ID, "Authenticator app", st.Sealer.Seal([]byte("JBSWY3DPEHPK3PXP"))); err != nil {
		t.Fatal(err)
	}
	secrets, _ := st.ListTOTPSecrets(ctx, totpOnly.ID)
	if _, err := st.ConfirmTOTPSecret(ctx, secrets[0].ID, "Authenticator app", 0); err != nil {
		t.Fatal(err)
	}

	type method struct {
		Method           string
		Enabled          bool
		Users, Exclusive int
	}
	methods := decode[[]method](t, call(t, h, token, "GET", "/api/v1/methods", nil))
	totpRow := methods[slices.IndexFunc(methods, func(m method) bool { return m.Method == "totp" })]
	if len(methods) != 5 || methods[0].Method != "passkey" || totpRow.Users != 1 || totpRow.Exclusive != 1 {
		t.Fatalf("methods: %+v", methods)
	}
	for _, kind := range []string{"passkey", "security-key", "totp"} {
		expect(t, call(t, h, token, "PUT", "/api/v1/methods/"+kind, map[string]any{"enabled": false}), 204)
	}
	expect(t, call(t, h, token, "PUT", "/api/v1/methods/magic-link", map[string]any{"enabled": false}), 409)
	expect(t, call(t, h, token, "PUT", "/api/v1/methods/sms", map[string]any{"enabled": false}), 404)
	federated := call(t, h, token, "PUT", "/api/v1/methods/federated", map[string]any{"enabled": false})
	expect(t, federated, 422)
	if !strings.Contains(federated.Body.String(), "per identity provider") {
		t.Fatalf("federated toggle: %s", federated.Body)
	}
	for _, rec := range []*httptest.ResponseRecorder{
		call(t, h, token, "POST", "/api/v1/policies", body("Texts", "report", access.Block, store.PolicyConditions{AllUsers: true, AllApps: true, Methods: []string{"sms"}})),
		call(t, h, token, "POST", "/api/v1/policies/simulate", map[string]any{"userId": totpOnly.ID, "appId": "halo", "ip": "198.51.100.7", "method": "sms", "deviceTrust": "unknown"}),
	} {
		expect(t, rec, 422)
		for _, kind := range store.MethodKinds {
			if !strings.Contains(rec.Body.String(), kind) {
				t.Fatalf("method list missing %s: %s", kind, rec.Body)
			}
		}
	}

	eng := policy.NewEngine(st)
	evaluate(t, eng, totpOnly, "magic-link", "198.51.100.7", strings.Repeat("t", 43))
	devices := decode[[]store.Device](t, call(t, h, token, "GET", "/api/v1/devices", nil))
	if len(devices) != 1 || devices[0].UserID != totpOnly.ID {
		t.Fatalf("devices: %+v", devices)
	}
	expect(t, call(t, h, token, "PUT", "/api/v1/devices/"+devices[0].ID, map[string]any{"trust": "sideways"}), 422)
	rec := call(t, h, token, "PUT", "/api/v1/devices/"+devices[0].ID, map[string]any{"trust": "blocked"})
	expect(t, rec, 200)
	if decode[store.Device](t, rec).Trust != "blocked" {
		t.Fatalf("trust not updated: %s", rec.Body)
	}
	audit, _ := st.ListAudit(ctx, 1)
	if audit[0].Action != "device.trust" || audit[0].Summary != "Blocked Firefox on Linux computer" || audit[0].TargetID != totpOnly.ID {
		t.Fatalf("device audit: %+v", audit[0])
	}
}

func TestRiskEventStatus(t *testing.T) {
	st, h := server(t)
	_, token := admin(t, st, "sec@example.com", "security_admin")
	cy := member(t, st, "cy@example.com")
	zone(t, st, "Tor exit nodes", "risky", "185.220.101.0/24")
	evaluate(t, policy.NewEngine(st), cy, "passkey", "185.220.101.47", "")
	events := decode[[]store.RiskEvent](t, call(t, h, token, "GET", "/api/v1/risk-events", nil))
	if len(events) != 1 || events[0].Status != "open" {
		t.Fatalf("risk events: %+v", events)
	}
	rec := call(t, h, token, "POST", "/api/v1/risk-events/"+events[0].ID+"/dismiss", nil)
	expect(t, rec, 200)
	if e := decode[store.RiskEvent](t, rec); e.Status != "dismissed" || e.ResolvedBy == nil || e.ResolvedAt == nil {
		t.Fatalf("dismissed event: %+v", e)
	}
	expect(t, call(t, h, token, "POST", "/api/v1/risk-events/rsk_missing/resolve", nil), 404)
}

func totpUser(t *testing.T, st *store.Store, email string, groupIDs ...string) string {
	t.Helper()
	ctx := context.Background()
	u := member(t, st, email, groupIDs...)
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "Halo", AccountName: email})
	if err != nil {
		t.Fatal(err)
	}
	secretID, err := st.CreateTOTPSecret(ctx, u.ID, "Authenticator app", st.Sealer.Seal([]byte(key.Secret())))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ConfirmTOTPSecret(ctx, secretID, "Authenticator app", 0); err != nil {
		t.Fatal(err)
	}
	return key.Secret()
}

func TestAuthSignInThroughEngine(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	public, _ := url.Parse("http://localhost:3200")
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	mux := http.NewServeMux()
	if err := auth.Register(mux, httpx.Deps{Config: config.Config{PublicURL: public, Dev: true}, Store: st, Auth: authn, Policy: policy.NewEngine(st)}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(authn.Resolve(mux))
	t.Cleanup(srv.Close)

	contractors := group(t, st, "Contractors")
	admins := group(t, st, "Admins")
	casey := totpUser(t, st, "casey@example.com", contractors.ID)
	ada := totpUser(t, st, "ada@example.com", admins.ID)
	fred := totpUser(t, st, "fred@example.com")
	block := rule(t, st, "Block contractors", "enforce", access.Block, store.PolicyConditions{GroupIDs: []string{contractors.ID}, AllApps: true})
	rule(t, st, "Admins need passkeys", "enforce", access.RequirePhishingResistant, store.PolicyConditions{GroupIDs: []string{admins.ID}, AllApps: true})
	trial := rule(t, st, "Block everyone (trial)", "report", access.Block, everyone)

	signIn := func(email, secret string) (int, string) {
		code, _ := totp.GenerateCode(secret, time.Now())
		payload, _ := json.Marshal(map[string]string{"email": email, "code": code})
		res, err := http.Post(srv.URL+"/api/v1/auth/totp", "application/json", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out struct {
			Error struct{ Code string } `json:"error"`
		}
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out.Error.Code
	}

	if status, code := signIn("casey@example.com", casey); status != http.StatusForbidden || code != "ERR_BLOCKED_BY_POLICY" {
		t.Fatalf("block policy: %d %s", status, code)
	}
	if status, code := signIn("ada@example.com", ada); status != http.StatusForbidden || code != "ERR_STRONGER_AUTH_REQUIRED" {
		t.Fatalf("require phishing-resistant: %d %s", status, code)
	}
	if status, code := signIn("fred@example.com", fred); status != http.StatusOK {
		t.Fatalf("report-only policy must not block: %d %s", status, code)
	}

	events, err := st.ListSignIns(ctx, store.SignInFilter{})
	if err != nil {
		t.Fatal(err)
	}
	reasons := map[string]string{}
	for _, e := range events {
		reasons[e.Email] = e.Result + ": " + e.Reason
	}
	if reasons["casey@example.com"] != "failure: Policy “"+block.Name+"”: Blocks sign-ins by members of Contractors." || reasons["fred@example.com"] != "success: " {
		t.Fatalf("sign-in events: %v", reasons)
	}
	activity, err := st.PolicyActivitySince(ctx, time.Now().Add(-time.Hour))
	if err != nil || activity[trial.ID].Matched != 3 || activity[block.ID].Stopped != 1 {
		t.Fatalf("activity: %+v %v", activity, err)
	}
	devices, _ := st.ListDevices(ctx, "")
	if len(devices) != 3 {
		t.Fatalf("each sign-in should register its device: %+v", devices)
	}
}
