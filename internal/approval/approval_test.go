package approval_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"halo/internal/api"
	"halo/internal/apikeys"
	"halo/internal/approval"
	"halo/internal/auth"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/mail"
	"halo/internal/policy"
	"halo/internal/settings"
	"halo/internal/store"
	"halo/internal/testdb"
)

type outbox struct{ sent []mail.Message }

func (o *outbox) Send(_ context.Context, m mail.Message) error {
	o.sent = append(o.sent, m)
	return nil
}

func (o *outbox) Configured() bool { return true }

type env struct {
	t   *testing.T
	st  *store.Store
	h   http.Handler
	box *outbox
}

func setup(t *testing.T) *env {
	t.Helper()
	st := testdb.New(t)
	public, _ := url.Parse("http://localhost:3200")
	authn := &httpx.Auth{Store: st, Public: public, Dev: true, Keys: apikeys.Resolver(st)}
	box := &outbox{}
	d := httpx.Deps{Config: config.Config{PublicURL: public, Dev: true}, Store: st, Auth: authn, Mailer: box}
	mux := http.NewServeMux()
	for _, register := range []func(*http.ServeMux, httpx.Deps) error{auth.Register, api.Register, policy.Register, settings.Register, apikeys.Register, approval.Register} {
		if err := register(mux, d); err != nil {
			t.Fatal(err)
		}
	}
	return &env{t: t, st: st, h: authn.Resolve(authn.KeyScope("api", authn.SameOrigin(mux))), box: box}
}

func (e *env) person(email string, roles ...string) (store.User, string) {
	e.t.Helper()
	ctx := context.Background()
	u, err := e.st.CreateUser(ctx, store.NewUser{Email: email, Name: strings.Split(email, "@")[0], Status: "active", Roles: roles})
	if err != nil {
		e.t.Fatal(err)
	}
	_, token, err := e.st.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: "passkey"})
	if err != nil {
		e.t.Fatal(err)
	}
	return u, token
}

func (e *env) call(credential, method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if strings.HasPrefix(credential, "hlk_") {
		req.Header.Set("Authorization", "Bearer "+credential)
	} else if credential != "" {
		req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: credential})
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e *env) expect(rec *httptest.ResponseRecorder, status int) *httptest.ResponseRecorder {
	e.t.Helper()
	if rec.Code != status {
		e.t.Fatalf("want %d, got %d: %s", status, rec.Code, rec.Body)
	}
	return rec
}

func (e *env) audits(action, targetID string) int {
	e.t.Helper()
	events, err := e.st.ListAudit(context.Background(), 1000)
	if err != nil {
		e.t.Fatal(err)
	}
	n := 0
	for _, ev := range events {
		if ev.Action == action && ev.TargetID == targetID {
			n++
		}
	}
	return n
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return v
}

func TestGrantingGlobalAdminWaitsForAnotherAdministrator(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, gia := e.person("gia@example.com", "global_admin")
	_, ben := e.person("ben@example.com", "global_admin")
	cara, _ := e.person("cara@example.com", "auditor")
	_, key, err := e.st.CreateAPIKey(ctx, mustAccount(t, e.st).ID, "automation", []string{"api"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	e.expect(e.call(gia, "PATCH", "/api/v1/settings", map[string]any{"approvalActions": []string{"global-admin"}}), 200)

	e.expect(e.call(gia, "POST", "/api/v1/users", map[string]any{"email": "dan@example.com", "name": "Dan", "roles": []string{"global_admin"}}), 422)
	held := decode[store.ApprovalRequest](t, e.expect(e.call(gia, "PUT", "/api/v1/users/"+cara.ID+"/roles", map[string]any{"roles": []string{"global_admin"}}), 202))
	if held.Status != "pending" || held.Requester.Name != "gia" || !held.ExpiresAt.After(time.Now().Add(71*time.Hour)) {
		t.Fatalf("held request: %+v", held)
	}
	if u, _ := e.st.GetUser(ctx, cara.ID); u.HasRole("global_admin") {
		t.Fatal("the role must not be granted before approval")
	}
	if e.audits("approval.request", cara.ID) != 1 {
		t.Fatal("the request must be in the audit log")
	}

	list := decode[[]store.ApprovalRequest](t, e.expect(e.call(gia, "GET", "/api/v1/approvals", nil), 200))
	if len(list) != 1 || list[0].CanDecide {
		t.Fatalf("the requester must not be able to decide: %+v", list)
	}
	self := e.expect(e.call(gia, "POST", "/api/v1/approvals/"+held.ID+"/approve", nil), 403)
	if !strings.Contains(self.Body.String(), "ERR_SELF_APPROVAL") {
		t.Fatalf("self approval: %s", self.Body)
	}
	e.expect(e.call(key, "POST", "/api/v1/approvals/"+held.ID+"/approve", nil), 403)

	approved := decode[store.ApprovalRequest](t, e.expect(e.call(ben, "POST", "/api/v1/approvals/"+held.ID+"/approve", nil), 200))
	if approved.Status != "approved" || approved.DecidedBy == nil || approved.DecidedBy.Name != "ben" {
		t.Fatalf("approved request: %+v", approved)
	}
	e.expect(e.call(ben, "POST", "/api/v1/approvals/"+held.ID+"/approve", nil), 409)
	if u, _ := e.st.GetUser(ctx, cara.ID); !u.HasRole("global_admin") {
		t.Fatal("approval must grant the role")
	}
	if n := e.audits("role.update", cara.ID); n != 1 {
		t.Fatalf("the change must apply exactly once, applied %d times", n)
	}
	if e.audits("approval.approve", cara.ID) != 1 {
		t.Fatal("the approval must be in the audit log")
	}
}

func TestGlobalAdminEligibilityWaitsForAnotherAdministrator(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, gia := e.person("gia@example.com", "global_admin")
	_, ben := e.person("ben@example.com", "global_admin")
	cara, _ := e.person("cara@example.com")
	dan, _ := e.person("dan@example.com")
	e.expect(e.call(gia, "PATCH", "/api/v1/settings", map[string]any{"approvalActions": []string{"global-admin"}}), 200)

	e.expect(e.call(gia, "PUT", "/api/v1/users/"+dan.ID+"/eligible-roles", map[string]any{"roles": []string{"security_admin"}}), 200)
	held := decode[store.ApprovalRequest](t, e.expect(e.call(gia, "PUT", "/api/v1/users/"+cara.ID+"/eligible-roles", map[string]any{"roles": []string{"global_admin"}}), 202))
	if held.Status != "pending" || held.Action != "global-admin" {
		t.Fatalf("held request: %+v", held)
	}
	if u, _ := e.st.GetUser(ctx, cara.ID); len(u.EligibleRoles) != 0 {
		t.Fatalf("eligibility must wait for approval: %v", u.EligibleRoles)
	}

	self := e.expect(e.call(gia, "POST", "/api/v1/approvals/"+held.ID+"/approve", nil), 403)
	if !strings.Contains(self.Body.String(), "ERR_SELF_APPROVAL") {
		t.Fatalf("self approval: %s", self.Body)
	}
	e.expect(e.call(ben, "POST", "/api/v1/approvals/"+held.ID+"/approve", nil), 200)
	e.expect(e.call(ben, "POST", "/api/v1/approvals/"+held.ID+"/approve", nil), 409)
	u, _ := e.st.GetUser(ctx, cara.ID)
	if len(u.EligibleRoles) != 1 || u.EligibleRoles[0] != "global_admin" {
		t.Fatalf("approval must make cara eligible: %v", u.EligibleRoles)
	}
	if n := e.audits("role.eligibility.update", cara.ID); n != 1 {
		t.Fatalf("the change must apply exactly once, applied %d times", n)
	}

	if _, err := e.st.ActivateRole(ctx, cara.ID, "global_admin", "incident", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	e.expect(e.call(ben, "PUT", "/api/v1/users/"+cara.ID+"/roles", map[string]any{"roles": []string{"global_admin"}}), 202)
}

func mustAccount(t *testing.T, st *store.Store) store.ServiceAccount {
	t.Helper()
	a, err := st.CreateServiceAccount(context.Background(), store.NewServiceAccount{Name: "Automation", Roles: []string{"global_admin"}})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestStaleRequestIsNotApplied(t *testing.T) {
	e := setup(t)
	_, gia := e.person("gia@example.com", "global_admin")
	_, ben := e.person("ben@example.com", "global_admin")
	policyBody := func(name string, enabled bool) map[string]any {
		return map[string]any{"name": name, "enabled": enabled, "mode": "enforce", "effect": "require-phishing-resistant", "conditions": map[string]any{"allUsers": true, "allApps": true}}
	}
	created := decode[store.AccessPolicy](t, e.expect(e.call(gia, "POST", "/api/v1/policies", policyBody("Passkeys everywhere", true)), 201))
	e.expect(e.call(gia, "PATCH", "/api/v1/settings", map[string]any{"approvalActions": []string{"policy-disable"}}), 200)

	bot, err := e.st.CreateServiceAccount(context.Background(), store.NewServiceAccount{Name: "Policy bot", Roles: []string{"security_admin"}})
	if err != nil {
		t.Fatal(err)
	}
	_, botKey, err := e.st.CreateAPIKey(context.Background(), bot.ID, "policy bot", []string{"api"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	refused := e.expect(e.call(botKey, "PUT", "/api/v1/policies/"+created.ID, policyBody("Passkeys everywhere", false)), 403)
	if !strings.Contains(refused.Body.String(), "ERR_APPROVAL_REQUIRED") {
		t.Fatalf("a service account must not be able to ask for approval its owner could then give: %s", refused.Body)
	}

	held := decode[store.ApprovalRequest](t, e.expect(e.call(gia, "PUT", "/api/v1/policies/"+created.ID, policyBody("Passkeys everywhere", false)), 202))
	rejected := decode[store.ApprovalRequest](t, e.expect(e.call(gia, "DELETE", "/api/v1/policies/"+created.ID, nil), 202))
	e.expect(e.call(ben, "POST", "/api/v1/approvals/"+rejected.ID+"/reject", nil), 200)
	e.expect(e.call(ben, "POST", "/api/v1/approvals/"+rejected.ID+"/approve", nil), 409)

	e.expect(e.call(ben, "PUT", "/api/v1/policies/"+created.ID, policyBody("Passkeys for everyone", true)), 200)
	stale := e.expect(e.call(ben, "POST", "/api/v1/approvals/"+held.ID+"/approve", nil), 409)
	if !strings.Contains(stale.Body.String(), "ERR_NOT_APPLIED") || !strings.Contains(stale.Body.String(), "changed after") {
		t.Fatalf("stale approval: %s", stale.Body)
	}
	p, err := e.st.GetAccessPolicy(context.Background(), created.ID)
	if err != nil || !p.Enabled {
		t.Fatalf("a stale request must not apply: %+v %v", p, err)
	}
	after, _ := e.st.GetApprovalRequest(context.Background(), held.ID)
	if after.Status != "failed" || after.Outcome == "" {
		t.Fatalf("stale request status: %+v", after)
	}
	if e.audits("approval.fail", created.ID) != 1 || e.audits("policy.update", created.ID) != 1 {
		t.Fatal("the unapplied approval must be in the audit log, and only the direct edit applied")
	}
	if e.audits("approval.reject", created.ID) != 1 {
		t.Fatal("the rejection must be in the audit log")
	}
}

func TestExpiredRequestCannotBeApproved(t *testing.T) {
	e := setup(t)
	gia, _ := e.person("gia@example.com", "global_admin")
	_, ben := e.person("ben@example.com", "global_admin")
	expiredID, err := e.st.CreateApprovalRequest(context.Background(), store.ApprovalRequest{Action: "password-sign-in", Summary: "Turn on sign-in with passwords", TargetType: "method", TargetID: "password", TargetLabel: "Passwords",
		Method: "PUT", Path: "/api/v1/methods/password", Body: []byte(`{"enabled":true}`), State: "false", Requester: store.NamedRef{ID: gia.ID}, ExpiresAt: time.Now().Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	e.expect(e.call(ben, "POST", "/api/v1/approvals/"+expiredID+"/approve", nil), 409)
	if r, _ := e.st.GetApprovalRequest(context.Background(), expiredID); r.Status != "expired" {
		t.Fatalf("status: %s", r.Status)
	}
}

func TestEmergencyAccessSkipsApprovalWithAnAlert(t *testing.T) {
	e := setup(t)
	gia, giaToken := e.person("gia@example.com", "global_admin")
	_, ben := e.person("ben@example.com", "global_admin")
	e.expect(e.call(ben, "PATCH", "/api/v1/settings", map[string]any{"approvalActions": []string{"password-sign-in"}, "emergencyAccessIds": []string{gia.ID}}), 200)

	e.expect(e.call(giaToken, "PUT", "/api/v1/methods/password", map[string]any{"enabled": true}), 204)
	if e.audits("approval.bypass", "password") != 1 || e.audits("method.update", "password") != 1 {
		t.Fatal("the bypass and the change must be in the audit log")
	}
	if len(e.box.sent) != 1 || e.box.sent[0].To != "ben@example.com" || !strings.Contains(e.box.sent[0].Text, "Turn on sign-in with passwords") {
		t.Fatalf("alert: %+v", e.box.sent)
	}
}

func TestApprovalSettingsNeedApprovalToChange(t *testing.T) {
	e := setup(t)
	_, gia := e.person("gia@example.com", "global_admin")
	e.expect(e.call(gia, "PATCH", "/api/v1/settings", map[string]any{"approvalActions": []string{"global-admin"}}), 422)
	_, ben := e.person("ben@example.com", "global_admin")
	e.expect(e.call(gia, "PATCH", "/api/v1/settings", map[string]any{"approvalActions": []string{"unknown"}}), 422)
	e.expect(e.call(gia, "PATCH", "/api/v1/settings", map[string]any{"approvalActions": []string{"global-admin"}}), 200)

	held := decode[store.ApprovalRequest](t, e.expect(e.call(gia, "PATCH", "/api/v1/settings", map[string]any{"approvalActions": []string{}}), 202))
	if held.Summary != "Turn off approval. Emergency access accounts: none" {
		t.Fatalf("summary: %q", held.Summary)
	}
	e.st.ForgetSettings()
	if s, _ := e.st.Settings(context.Background()); len(s.ApprovalActions) != 1 {
		t.Fatalf("approval must stay on until approved: %v", s.ApprovalActions)
	}
	e.expect(e.call(ben, "POST", "/api/v1/approvals/"+held.ID+"/approve", nil), 200)
	e.st.ForgetSettings()
	if s, _ := e.st.Settings(context.Background()); len(s.ApprovalActions) != 0 {
		t.Fatalf("approval must be off once approved: %v", s.ApprovalActions)
	}
}

func TestApprovalWaitsForAConcurrentChangeAndFailsAsStale(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, gia := e.person("gia@example.com", "global_admin")
	_, ben := e.person("ben@example.com", "global_admin")
	cara, _ := e.person("cara@example.com", "auditor")
	e.expect(e.call(gia, "PATCH", "/api/v1/settings", map[string]any{"approvalActions": []string{"global-admin"}}), 200)
	held := decode[store.ApprovalRequest](t, e.expect(e.call(gia, "PUT", "/api/v1/users/"+cara.ID+"/roles", map[string]any{"roles": []string{"global_admin", "auditor"}}), 202))

	done := make(chan *httptest.ResponseRecorder, 1)
	err := e.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.SetUserRoles(ctx, cara.ID, []string{"user_admin"}, nil); err != nil {
			return err
		}
		go func() { done <- e.call(ben, "POST", "/api/v1/approvals/"+held.ID+"/approve", nil) }()
		select {
		case rec := <-done:
			t.Fatalf("the approval must wait for the concurrent change: %d %s", rec.Code, rec.Body)
		case <-time.After(300 * time.Millisecond):
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	stale := e.expect(<-done, 409)
	if !strings.Contains(stale.Body.String(), "ERR_NOT_APPLIED") || !strings.Contains(stale.Body.String(), "changed after") {
		t.Fatalf("stale approval: %s", stale.Body)
	}
	if u, _ := e.st.GetUser(ctx, cara.ID); u.HasRole("global_admin") || !u.HasRole("user_admin") || u.HasRole("auditor") {
		t.Fatalf("the approval must not apply over the concurrent change: %v", u.RoleKeys)
	}
	if after, _ := e.st.GetApprovalRequest(ctx, held.ID); after.Status != "failed" || after.DecidedBy == nil {
		t.Fatalf("stale request: %+v", after)
	}
}

func TestOversizedBodyIsNotHeldForApproval(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, gia := e.person("gia@example.com", "global_admin")
	e.person("ben@example.com", "global_admin")
	created := decode[store.AccessPolicy](t, e.expect(e.call(gia, "POST", "/api/v1/policies", map[string]any{"name": "Passkeys everywhere", "enabled": true, "mode": "enforce", "effect": "require-phishing-resistant", "conditions": map[string]any{"allUsers": true, "allApps": true}}), 201))
	e.expect(e.call(gia, "PATCH", "/api/v1/settings", map[string]any{"approvalActions": []string{"policy-disable"}}), 200)

	big := e.expect(e.call(gia, "DELETE", "/api/v1/policies/"+created.ID, strings.Repeat("a", 1<<20)), 413)
	if !strings.Contains(big.Body.String(), "ERR_TOO_LARGE") {
		t.Fatalf("oversized body: %s", big.Body)
	}
	if list, err := e.st.ListApprovalRequests(ctx); err != nil || len(list) != 0 {
		t.Fatalf("an oversized request must not be stored: %+v %v", list, err)
	}
	e.expect(e.call(gia, "DELETE", "/api/v1/policies/"+created.ID, nil), 202)
}

func TestApprovedChangesApplyToEachKindOfTarget(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, gia := e.person("gia@example.com", "global_admin")
	_, ben := e.person("ben@example.com", "global_admin")
	bot, err := e.st.CreateServiceAccount(ctx, store.NewServiceAccount{Name: "Deploy bot", Roles: []string{"auditor"}})
	if err != nil {
		t.Fatal(err)
	}
	e.expect(e.call(gia, "PATCH", "/api/v1/settings", map[string]any{"approvalActions": []string{"global-admin", "password-sign-in", "identity-providers"}}), 200)

	for _, rec := range []*httptest.ResponseRecorder{
		e.call(gia, "PUT", "/api/v1/service-accounts/"+bot.ID+"/roles", map[string]any{"roles": []string{"global_admin"}}),
		e.call(gia, "PUT", "/api/v1/methods/password", map[string]any{"enabled": true}),
		e.call(gia, "POST", "/api/v1/identity-providers", map[string]any{"kind": "github", "name": "GitHub", "clientId": "halo", "clientSecret": "secret"}),
	} {
		held := decode[store.ApprovalRequest](t, e.expect(rec, 202))
		e.expect(e.call(ben, "POST", "/api/v1/approvals/"+held.ID+"/approve", nil), 200)
	}
	if u, _ := e.st.GetUser(ctx, bot.ID); !u.HasRole("global_admin") {
		t.Fatalf("service account roles: %v", u.RoleKeys)
	}
	methods, _ := e.st.ListMethodSettings(ctx)
	for _, m := range methods {
		if m.Method == "password" && !m.Enabled {
			t.Fatal("password sign-in must be on once approved")
		}
	}
	if providers, _ := e.st.ListIdentityProviders(ctx); len(providers) != 1 || providers[0].Name != "GitHub" {
		t.Fatalf("identity providers: %+v", providers)
	}
}

func TestApprovedProviderSecretMakesTheOtherPendingSecretStale(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, gia := e.person("gia@example.com", "global_admin")
	_, ben := e.person("ben@example.com", "global_admin")
	provider := func(secret string) map[string]any {
		return map[string]any{"kind": "github", "name": "GitHub", "clientId": "halo", "clientSecret": secret, "allowedDomains": []string{"example.com"}}
	}
	created := decode[store.IdentityProvider](t, e.expect(e.call(gia, "POST", "/api/v1/identity-providers", provider("first")), 201))
	policy := decode[store.AccessPolicy](t, e.expect(e.call(gia, "POST", "/api/v1/policies", map[string]any{"name": "Passkeys everywhere", "enabled": true, "mode": "enforce", "effect": "require-phishing-resistant", "conditions": map[string]any{"allUsers": true, "allApps": true}}), 201))
	e.expect(e.call(gia, "PATCH", "/api/v1/settings", map[string]any{"approvalActions": []string{"identity-providers", "policy-disable"}}), 200)

	second := decode[store.ApprovalRequest](t, e.expect(e.call(gia, "PUT", "/api/v1/identity-providers/"+created.ID, provider("second")), 202))
	third := decode[store.ApprovalRequest](t, e.expect(e.call(gia, "PUT", "/api/v1/identity-providers/"+created.ID, provider("third")), 202))
	e.expect(e.call(ben, "POST", "/api/v1/approvals/"+second.ID+"/approve", nil), 200)
	stale := e.expect(e.call(ben, "POST", "/api/v1/approvals/"+third.ID+"/approve", nil), 409)
	if !strings.Contains(stale.Body.String(), "changed after") {
		t.Fatalf("the older secret must not replace the approved one: %s", stale.Body)
	}

	off := decode[store.ApprovalRequest](t, e.expect(e.call(gia, "PUT", "/api/v1/policies/"+policy.ID, map[string]any{"name": "Passkeys everywhere", "enabled": false, "mode": "enforce", "effect": "require-phishing-resistant", "conditions": map[string]any{"allUsers": true, "allApps": true}}), 202))
	e.expect(e.call(ben, "POST", "/api/v1/approvals/"+off.ID+"/approve", nil), 200)
	if p, err := e.st.GetAccessPolicy(ctx, policy.ID); err != nil || p.Enabled {
		t.Fatalf("the approved policy change must apply: %+v %v", p, err)
	}
}
