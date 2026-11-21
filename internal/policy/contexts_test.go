package policy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestContextPoliciesCollectEveryEnforcedRequirement(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	eng := policy.NewEngine(st)
	ada := member(t, st, "ada@example.com")
	strong, err := st.CreateAuthContext(ctx, store.AuthContext{ID: "strong", Name: "Strong sign-in", Methods: []string{"passkey", "security-key"}})
	if err != nil {
		t.Fatal(err)
	}
	recent, err := st.CreateAuthContext(ctx, store.AuthContext{ID: "recent", Name: "Recent sign-in", MaxAge: 300})
	if err != nil {
		t.Fatal(err)
	}
	rule(t, st, "Admins need passkeys", "enforce", access.RequirePhishingResistant, everyone)
	if _, err := st.CreateAccessPolicy(ctx, store.AccessPolicy{Name: "Strong for everyone", Enabled: true, Mode: "enforce", Effect: access.RequireContext, ContextID: &strong.ID, Conditions: everyone}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAccessPolicy(ctx, store.AccessPolicy{Name: "Recent (trial)", Enabled: true, Mode: "report", Effect: access.RequireContext, ContextID: &recent.ID, Conditions: everyone}); err != nil {
		t.Fatal(err)
	}

	d := evaluate(t, eng, ada, "totp", "198.51.100.7", "")
	want(t, d, access.RequirePhishingResistant, "Admins need passkeys")
	if len(d.Contexts) != 1 || d.Contexts[0].ID != strong.ID {
		t.Fatalf("an enforced context policy must not hide behind the winner, and a report-only one must not apply: %+v", d.Contexts)
	}
	if _, unmet := d.StepUp(nil, store.Session{Method: "totp"}); unmet == nil || unmet.ID != strong.ID {
		t.Fatalf("a totp session must not meet %s: %+v", strong.ID, unmet)
	}
	if acr, unmet := d.StepUp(nil, store.Session{Method: "passkey"}); unmet != nil || acr != strong.ID {
		t.Fatalf("a passkey session meets %s: %q %+v", strong.ID, acr, unmet)
	}
	requested := []store.AuthContext{recent}
	if acr, unmet := (access.Decision{}).StepUp(requested, store.Session{Method: "password"}); unmet == nil || unmet.ID != recent.ID || acr != "" {
		t.Fatalf("a session from long ago must not meet %s: %q %+v", recent.ID, acr, unmet)
	}
}

func TestAuthenticationContextsAPI(t *testing.T) {
	st, h := server(t)
	ctx := context.Background()
	_, auditor := admin(t, st, "audrey@example.com", "auditor")
	_, token := admin(t, st, "sec@example.com", "security_admin")
	strong := map[string]any{"id": "passkey-recent", "name": "Passkey in the last 5 minutes", "methods": []string{"security-key", "passkey"}, "maxAge": 300}

	expect(t, call(t, h, auditor, "POST", "/api/v1/authentication-contexts", strong), 403)
	for _, bad := range []map[string]any{
		{"id": "has space", "name": "x", "methods": []string{"passkey"}},
		{"id": "a/b", "name": "x", "methods": []string{"passkey"}},
		{"id": "ok", "name": "", "methods": []string{"passkey"}},
		{"id": "ok", "name": "x", "methods": []string{"carrier-pigeon"}},
		{"id": "ok", "name": "x", "maxAge": 30},
		{"id": "ok", "name": "x"},
	} {
		expect(t, call(t, h, token, "POST", "/api/v1/authentication-contexts", bad), 422)
	}
	created := decode[store.AuthContext](t, call(t, h, token, "POST", "/api/v1/authentication-contexts", strong))
	if created.ID != "passkey-recent" || len(created.Methods) != 2 || created.Methods[0] != "passkey" || created.MaxAge != 300 {
		t.Fatalf("created: %+v", created)
	}
	expect(t, call(t, h, token, "POST", "/api/v1/authentication-contexts", strong), 409)
	if list := decode[[]store.AuthContext](t, call(t, h, auditor, "GET", "/api/v1/authentication-contexts", nil)); len(list) != 1 {
		t.Fatalf("list: %+v", list)
	}

	expect(t, call(t, h, token, "POST", "/api/v1/policies", body("Step up", "report", access.RequireContext, everyone)), 422)
	missing := body("Step up", "report", access.RequireContext, everyone)
	missing["contextId"] = "gold"
	expect(t, call(t, h, token, "POST", "/api/v1/policies", missing), 422)
	using := body("Step up", "report", access.RequireContext, everyone)
	using["contextId"] = created.ID
	p := decode[store.AccessPolicy](t, call(t, h, token, "POST", "/api/v1/policies", using))
	if p.ContextID == nil || *p.ContextID != created.ID {
		t.Fatalf("policy context: %+v", p)
	}

	strong["name"], strong["maxAge"] = "Passkey in the last 10 minutes", 600
	updated := decode[store.AuthContext](t, call(t, h, token, "PUT", "/api/v1/authentication-contexts/"+created.ID, strong))
	if updated.MaxAge != 600 {
		t.Fatalf("updated: %+v", updated)
	}
	audit, _ := st.ListAudit(ctx, 1)
	if audit[0].Action != "authentication_context.update" || audit[0].Summary != "Renamed from “Passkey in the last 5 minutes”, changed the maximum sign-in age" {
		t.Fatalf("update audit: %+v", audit[0])
	}
	expect(t, call(t, h, token, "DELETE", "/api/v1/authentication-contexts/"+created.ID, nil), 409)
	expect(t, call(t, h, token, "DELETE", "/api/v1/policies/"+p.ID, nil), 204)
	expect(t, call(t, h, token, "DELETE", "/api/v1/authentication-contexts/"+created.ID, nil), 204)
	expect(t, call(t, h, token, "PUT", "/api/v1/authentication-contexts/"+created.ID, strong), 404)
}

func TestContextPoliciesCountForTheLockoutGuard(t *testing.T) {
	st, h := server(t)
	ctx := context.Background()
	_, token := admin(t, st, "root@example.com", "global_admin")
	keys, err := st.CreateAuthContext(ctx, store.AuthContext{ID: "keys", Name: "Security keys", Methods: []string{"security-key"}})
	if err != nil {
		t.Fatal(err)
	}
	everyoneNeedsKeys := body("Security keys for everyone", "enforce", access.RequireContext, everyone)
	everyoneNeedsKeys["contextId"] = keys.ID
	if rec := call(t, h, token, "POST", "/api/v1/policies", everyoneNeedsKeys); rec.Code != 409 || !strings.Contains(rec.Body.String(), "ERR_LOCKOUT") {
		t.Fatalf("a context no administrator can meet must trip the lockout guard: %d %s", rec.Code, rec.Body)
	}
	passkeys := map[string]any{"name": "Passkeys", "methods": []string{"passkey"}}
	expect(t, call(t, h, token, "PUT", "/api/v1/authentication-contexts/"+keys.ID, passkeys), 200)
	expect(t, call(t, h, token, "POST", "/api/v1/policies", everyoneNeedsKeys), 201)
	passkeys["methods"] = []string{"security-key"}
	if rec := call(t, h, token, "PUT", "/api/v1/authentication-contexts/"+keys.ID, passkeys); rec.Code != 409 || !strings.Contains(rec.Body.String(), "ERR_LOCKOUT") {
		t.Fatalf("narrowing a context in use must trip the lockout guard: %d %s", rec.Code, rec.Body)
	}
}

func TestSignInBelowAContextPolicyIsRefused(t *testing.T) {
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
	keys, err := st.CreateAuthContext(ctx, store.AuthContext{ID: "keys", Name: "Security keys", Methods: []string{"security-key"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAccessPolicy(ctx, store.AccessPolicy{Name: "Keys only", Enabled: true, Mode: "enforce", Effect: access.RequireContext, ContextID: &keys.ID, Conditions: everyone}); err != nil {
		t.Fatal(err)
	}
	secret := totpUser(t, st, "ada@example.com")
	code, _ := totp.GenerateCode(secret, time.Now())
	payload, _ := json.Marshal(map[string]string{"email": "ada@example.com", "code": code})
	res, err := http.Post(srv.URL+"/api/v1/auth/totp", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out struct {
		Error struct{ Code, Message string } `json:"error"`
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode != http.StatusForbidden || out.Error.Code != "ERR_STEP_UP_REQUIRED" || out.Error.Message != "This sign-in needs to meet “Security keys”. Sign in again with a security key." {
		t.Fatalf("totp sign-in under a security-key context: %d %+v", res.StatusCode, out.Error)
	}
	events, err := st.ListSignIns(ctx, store.SignInFilter{})
	if err != nil || len(events) != 1 || events[0].Result != "failure" || events[0].Reason != "The sign-in does not meet the authentication context “Security keys”." {
		t.Fatalf("refusal event: %+v %v", events, err)
	}
}
