package policy_test

import (
	"bytes"
	"cmp"
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
	"halo/internal/mail"
	"halo/internal/policy"
	"halo/internal/secret"
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

func TestMobileSignInAdvertised(t *testing.T) {
	st, h := server(t)
	ctx := context.Background()
	mobile := func() bool {
		return decode[struct{ MobileSignIn bool }](t, call(t, h, "", "GET", "/api/v1/auth/methods", nil)).MobileSignIn
	}
	if mobile() {
		t.Fatal("mobile sign-in without the built-in app")
	}
	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Halo Mobile", Protocol: "oidc", Type: "native", ClientID: store.MobileClientID, RedirectURIs: []string{store.MobileRedirectURI}})
	if err != nil {
		t.Fatal(err)
	}
	if !mobile() {
		t.Fatal("mobile sign-in with the built-in app")
	}
	if err := st.SetApplicationStatus(ctx, app.ID, "disabled", false); err != nil {
		t.Fatal(err)
	}
	if mobile() {
		t.Fatal("mobile sign-in with the built-in app disabled")
	}
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
	expect(t, call(t, h, token, "POST", "/api/v1/policies", body("Managed", "report", access.Block, store.PolicyConditions{AllUsers: true, AllApps: true, Compliance: "managed"})), 422)

	rec := call(t, h, token, "POST", "/api/v1/policies", body("Admins need passkeys", "report", access.RequirePhishingResistant, store.PolicyConditions{GroupIDs: []string{admins.ID, admins.ID}, AllApps: true, AppIDs: []string{"halo"}}))
	expect(t, rec, 201)
	first := decode[store.AccessPolicy](t, rec)
	if first.Priority != 1 || !slices.Equal(first.Conditions.GroupIDs, []string{admins.ID}) || len(first.Conditions.AppIDs) != 0 || first.Conditions.Network != "any" || first.Conditions.Device != "any" || first.Conditions.Compliance != "any" {
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
	if c := audit[0].Changes; len(c) != 1 || c[0].Field != "mode" || string(c[0].Before) != `"report"` || string(c[0].After) != `"enforce"` {
		t.Fatalf("policy changes: %+v", c)
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

	expect(t, call(t, h, token, "POST", "/api/v1/policies", body("Office only", "report", access.Block, store.PolicyConditions{AllUsers: true, AllApps: true, Network: "not-in", ZoneIDs: []string{office.ID}})), 201)
	inUse := call(t, h, token, "DELETE", "/api/v1/network-zones/"+office.ID, nil)
	expect(t, inUse, 409)
	if !strings.Contains(inUse.Body.String(), "Office only") {
		t.Fatalf("conflict should name the policy: %s", inUse.Body)
	}
}

func TestLockoutGuard(t *testing.T) {
	st, h := server(t)
	_, token := admin(t, st, "sec@example.com", "security_admin")
	admin(t, st, "gia@example.com", "global_admin")
	blockAll := body("Block everyone", "enforce", access.Block, everyone)

	refused := call(t, h, token, "POST", "/api/v1/policies", blockAll)
	expect(t, refused, 409)
	if !strings.Contains(refused.Body.String(), "ERR_LOCKOUT") {
		t.Fatalf("a policy that blocks every global administrator must be refused: %s", refused.Body)
	}
	if saved, _ := st.ListAccessPolicies(context.Background()); len(saved) != 0 {
		t.Fatalf("a refused policy must not be saved: %+v", saved)
	}
	rec := call(t, h, token, "POST", "/api/v1/policies", body("Block everyone", "report", access.Block, everyone))
	expect(t, rec, 201)
	trial := decode[store.AccessPolicy](t, rec)
	expect(t, call(t, h, token, "PUT", "/api/v1/policies/"+trial.ID, blockAll), 409)
	expect(t, call(t, h, token, "PUT", "/api/v1/policies/order", map[string]any{"ids": []string{trial.ID}}), 204)

	compliantOnly := body("Compliant devices only", "enforce", access.Block, store.PolicyConditions{AllUsers: true, AllApps: true, Compliance: "not-compliant"})
	expect(t, call(t, h, token, "POST", "/api/v1/policies", compliantOnly), 409)

	erin := member(t, st, "erin@example.com")
	emergency(t, st, erin.ID)
	expect(t, call(t, h, token, "PUT", "/api/v1/policies/"+trial.ID, blockAll), 200)
	expect(t, call(t, h, token, "POST", "/api/v1/policies", compliantOnly), 201)
	passkeys := call(t, h, token, "POST", "/api/v1/policies", body("Passkeys everywhere", "enforce", access.RequirePhishingResistant, everyone))
	expect(t, passkeys, 409)
	if !strings.Contains(passkeys.Body.String(), "ERR_LOCKOUT") {
		t.Fatalf("an emergency access account without a passkey can't get past a passkey policy: %s", passkeys.Body)
	}
}

func TestLockoutGuardCoversMethodsAndZones(t *testing.T) {
	st, h := server(t)
	_, token := admin(t, st, "sec@example.com", "security_admin")
	admin(t, st, "gia@example.com", "global_admin")
	expect(t, call(t, h, token, "POST", "/api/v1/policies", body("Passkeys everywhere", "enforce", access.RequirePhishingResistant, everyone)), 201)
	expect(t, call(t, h, token, "PUT", "/api/v1/methods/security-key", map[string]any{"enabled": false}), 204)
	passkeysOff := call(t, h, token, "PUT", "/api/v1/methods/passkey", map[string]any{"enabled": false})
	expect(t, passkeysOff, 409)
	if !strings.Contains(passkeysOff.Body.String(), "ERR_LOCKOUT") {
		t.Fatalf("turning off the last phishing-resistant method under a passkey policy must be refused: %s", passkeysOff.Body)
	}
	methods, _ := st.ListMethodSettings(context.Background())
	if !slices.ContainsFunc(methods, func(m store.MethodSetting) bool { return m.Method == "passkey" && m.Enabled }) {
		t.Fatalf("a refused method change must not be saved: %+v", methods)
	}

	st, h = server(t)
	_, token = admin(t, st, "sec@example.com", "security_admin")
	rec := call(t, h, token, "POST", "/api/v1/network-zones", map[string]any{"name": "Office", "kind": "trusted", "cidrs": []string{"192.0.2.0/24"}})
	expect(t, rec, 201)
	office := decode[store.NetworkZone](t, rec)
	expect(t, call(t, h, token, "POST", "/api/v1/policies", body("Office only", "enforce", access.Block, store.PolicyConditions{AllUsers: true, AllApps: true, Network: "not-in", ZoneIDs: []string{office.ID}})), 201)
	moved := call(t, h, token, "PUT", "/api/v1/network-zones/"+office.ID, map[string]any{"name": "Office", "kind": "trusted", "cidrs": []string{"198.51.100.0/24"}})
	expect(t, moved, 409)
	if !strings.Contains(moved.Body.String(), "ERR_LOCKOUT") {
		t.Fatalf("moving the only allowed network away from every administrator must be refused: %s", moved.Body)
	}
	if z, _ := st.ListNetworkZones(context.Background()); !slices.Equal(z[0].CIDRs, []string{"192.0.2.0/24"}) {
		t.Fatalf("a refused zone change must not be saved: %+v", z)
	}
}

func TestLockoutGuardReadsTheActorsDeviceCompliance(t *testing.T) {
	st, h := server(t)
	ctx := context.Background()
	sec, token := admin(t, st, "sec@example.com", "security_admin")
	compliantOnly := body("Compliant devices only", "enforce", access.Block, store.PolicyConditions{AllUsers: true, AllApps: true, Compliance: "not-compliant"})
	expect(t, call(t, h, token, "POST", "/api/v1/policies", compliantOnly), 409)

	laptop := strings.Repeat("l", 43)
	if _, err := st.TouchDevice(ctx, sec.ID, secret.Hash(laptop), firefox, "198.51.100.7", true); err != nil {
		t.Fatal(err)
	}
	sourceID, err := st.CreateComplianceSource(ctx, store.ComplianceSource{Name: "Intune", Kind: "intune", TenantID: "contoso", ClientID: "halo", MaxAgeMinutes: 60}, "secret")
	if err != nil {
		t.Fatal(err)
	}
	src, err := st.GetComplianceSource(ctx, sourceID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReportCompliance(ctx, src, []store.ComplianceRecord{{ExternalID: "mdm-1", Name: "SEC-LAPTOP", State: "compliant"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	devices, err := st.ListDevices(ctx, sec.ID)
	if err != nil || len(devices) != 1 {
		t.Fatalf("devices: %+v %v", devices, err)
	}
	external := "mdm-1"
	if err := st.LinkDeviceCompliance(ctx, devices[0].ID, &src.ID, &external); err != nil {
		t.Fatal(err)
	}

	b, _ := json.Marshal(compliantOnly)
	req := httptest.NewRequest("POST", "/api/v1/policies", bytes.NewReader(b))
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: token})
	req.AddCookie(&http.Cookie{Name: httpx.DeviceCookie, Value: laptop})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	expect(t, rec, 201)
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

	rule(t, st, "Compliant devices only", "enforce", access.Block, store.PolicyConditions{AllUsers: true, AllApps: true, Compliance: "not-compliant"})
	for compliance, effect := range map[string]string{"": access.Block, "unknown": access.Block, "non-compliant": access.Block, "compliant": access.Allow} {
		got := decode[simulated](t, call(t, h, token, "POST", "/api/v1/policies/simulate", map[string]any{"userId": ada.ID, "appId": "halo", "ip": "198.51.100.7", "method": "passkey", "deviceTrust": "trusted", "deviceCompliance": compliance}))
		if got.Effect != effect || effect == access.Block && got.Reason != "Blocks sign-ins on devices that aren't compliant. The simulated device is "+cmp.Or(compliance, "unknown")+"." {
			t.Fatalf("simulated %q device: %+v", compliance, got)
		}
	}
	expect(t, call(t, h, token, "POST", "/api/v1/policies/simulate", map[string]any{"userId": ada.ID, "appId": "halo", "ip": "198.51.100.7", "method": "passkey", "deviceTrust": "trusted", "deviceCompliance": "managed"}), 422)
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
	if len(methods) != 8 || methods[0].Method != "passkey" || methods[5].Method != "password" || methods[5].Enabled || methods[6].Method != "certificate" || methods[6].Enabled || methods[7].Method != "kerberos" || methods[7].Enabled || totpRow.Users != 1 || totpRow.Exclusive != 1 {
		t.Fatalf("methods: %+v", methods)
	}
	for _, kind := range []string{"passkey", "security-key", "totp"} {
		expect(t, call(t, h, token, "PUT", "/api/v1/methods/"+kind, map[string]any{"enabled": false}), 204)
	}
	expect(t, call(t, h, token, "PUT", "/api/v1/methods/magic-link", map[string]any{"enabled": false}), 409)
	enabled := decode[struct {
		Methods     []string
		EmailSignIn string
	}](t, call(t, h, "", "GET", "/api/v1/auth/methods", nil))
	if !slices.Equal(enabled.Methods, []string{"magic-link", "recovery-codes"}) || enabled.EmailSignIn != "link" {
		t.Fatalf("enabled methods: %+v", enabled)
	}
	expect(t, call(t, h, token, "PUT", "/api/v1/methods/kerberos", map[string]any{"enabled": true}), 204)
	offered := func() []string {
		return decode[struct{ Methods []string }](t, call(t, h, "", "GET", "/api/v1/auth/methods", nil)).Methods
	}
	if got := offered(); slices.Contains(got, "kerberos") {
		t.Fatalf("Windows sign-in without a keytab: %v", got)
	}
	if err := st.SaveKerberosSource(ctx, store.KerberosSource{ServicePrincipal: "HTTP/auth.example.com@CORP.EXAMPLE.COM", Realms: []store.KerberosRealm{}, ZoneIDs: []string{}, Keytab: []byte("keytab")}); err != nil {
		t.Fatal(err)
	}
	if got := offered(); !slices.Contains(got, "kerberos") {
		t.Fatalf("Windows sign-in with a keytab: %v", got)
	}
	settings, _ := st.Settings(ctx)
	settings.FeaturesOff = []string{"kerberos"}
	if err := st.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	st.ForgetSettings()
	if got := offered(); slices.Contains(got, "kerberos") {
		t.Fatalf("Windows sign-in with the feature off: %v", got)
	}
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
	box := &outbox{}
	mux := http.NewServeMux()
	if err := auth.Register(mux, httpx.Deps{Config: config.Config{PublicURL: public, Dev: true}, Store: st, Auth: authn, Policy: policy.NewEngine(st), Mailer: box}); err != nil {
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

	gia, _ := admin(t, st, "gia@example.com", "global_admin")
	erinSecret := totpUser(t, st, "erin@example.com", contractors.ID)
	erin, _ := st.GetUserByEmail(ctx, "erin@example.com")
	emergency(t, st, erin.ID)
	if status, code := signIn("erin@example.com", erinSecret); status != http.StatusOK {
		t.Fatalf("an emergency access account must get past a blocking policy: %d %s", status, code)
	}
	audit, _ := st.ListAudit(ctx, 100)
	if !slices.ContainsFunc(audit, func(e store.AuditEvent) bool { return e.Action == "emergency_access.sign_in" && e.TargetID == erin.ID }) {
		t.Fatalf("an emergency sign-in must be audited: %+v", audit)
	}
	if len(box.sent) != 1 || box.sent[0].To != gia.Email || !strings.Contains(box.sent[0].Subject, "signed in with an emergency access account") {
		t.Fatalf("the other global administrators must be emailed: %+v", box.sent)
	}
	hooks, err := st.PollWebhookEvents(ctx, time.Now().Add(time.Second), 100)
	if err != nil || !slices.ContainsFunc(hooks, func(e store.WebhookEvent) bool { return e.Type == "emergency_access.sign_in" }) {
		t.Fatalf("an emergency sign-in must reach webhooks and notification rules: %+v %v", hooks, err)
	}
}

func TestHaloPolicyAppliesToSessionsFromApplicationSignIns(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	public, _ := url.Parse("http://localhost:3200")
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	mux := http.NewServeMux()
	if err := auth.Register(mux, httpx.Deps{Config: config.Config{PublicURL: public, Dev: true}, Store: st, Auth: authn, Policy: policy.NewEngine(st), Mailer: &outbox{}}); err != nil {
		t.Fatal(err)
	}
	mux.HandleFunc("GET /oauth2/authorize/callback", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := httpx.CurrentUser(r); !ok {
			w.WriteHeader(http.StatusUnauthorized)
		}
	})
	h := authn.Resolve(mux)

	admins, staff := group(t, st, "Admins"), group(t, st, "Staff")
	ada := totpUser(t, st, "ada@example.com", admins.ID)
	fred := totpUser(t, st, "fred@example.com", staff.ID)
	rule(t, st, "Admins need passkeys for Halo", "enforce", access.RequirePhishingResistant, store.PolicyConditions{GroupIDs: []string{admins.ID}, AppIDs: []string{"halo"}})
	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://grafana.example.com/cb"}, GroupIDs: []string{admins.ID, staff.ID}})
	if err != nil {
		t.Fatal(err)
	}

	signIn := func(email, secret, request string) string {
		t.Helper()
		if _, err := st.CreateAuthRequest(ctx, store.AuthRequest{ID: request, ClientID: app.ClientID, RedirectURI: app.RedirectURIs[0], Scopes: []string{"openid"}, ResponseType: "code"}); err != nil {
			t.Fatal(err)
		}
		code, _ := totp.GenerateCode(secret, time.Now())
		rec := call(t, h, "", "POST", "/api/v1/auth/totp", map[string]string{"email": email, "code": code, "authRequest": request})
		expect(t, rec, 200)
		for _, c := range rec.Result().Cookies() {
			if c.Name == httpx.SessionCookie && c.Value != "" {
				return c.Value
			}
		}
		t.Fatalf("no session cookie for %s: %s", email, rec.Body)
		return ""
	}

	adaSession := signIn("ada@example.com", ada, "are_ada")
	expect(t, call(t, h, adaSession, "GET", "/oauth2/authorize/callback?id=are_ada", nil), 200)
	expect(t, call(t, h, adaSession, "GET", "/api/v1/me", nil), 401)
	fredSession := signIn("fred@example.com", fred, "are_fred")
	expect(t, call(t, h, fredSession, "GET", "/api/v1/me", nil), 200)
}

type outbox struct{ sent []mail.Message }

func (o *outbox) Send(_ context.Context, m mail.Message) error {
	o.sent = append(o.sent, m)
	return nil
}

func (o *outbox) Configured() bool { return true }
