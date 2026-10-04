package api_test

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

	"halo/internal/api"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/store"
	"halo/internal/testdb"
)

func setup(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	st := testdb.New(t)
	public, _ := url.Parse("http://localhost:3200")
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	mux := http.NewServeMux()
	if err := api.Register(mux, httpx.Deps{Config: config.Config{PublicURL: public, Dev: true}, Store: st, Auth: authn}); err != nil {
		t.Fatal(err)
	}
	return st, authn.Resolve(authn.SameOrigin(mux))
}

func person(t *testing.T, st *store.Store, email string, roles ...string) (store.User, string) {
	t.Helper()
	u, err := st.CreateUser(context.Background(), store.NewUser{Email: email, Name: strings.Split(email, "@")[0], Status: "active", Roles: roles})
	if err != nil {
		t.Fatal(err)
	}
	return u, session(t, st, u.ID)
}

func session(t *testing.T, st *store.Store, userID string) string {
	t.Helper()
	_, token, err := st.CreateSession(context.Background(), store.NewSession{UserID: userID, Method: "passkey"})
	if err != nil {
		t.Fatal(err)
	}
	return token
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

type created struct {
	Application  store.Application `json:"application"`
	ClientSecret *string           `json:"clientSecret"`
}

func TestRoleEnforcement(t *testing.T) {
	st, h := setup(t)
	_, auditor := person(t, st, "audrey@example.com", "auditor")
	_, appAdmin := person(t, st, "abe@example.com", "app_admin")
	nobody, nobodyToken := person(t, st, "nora@example.com")

	expect(t, call(t, h, auditor, "GET", "/api/v1/users", nil), 200)
	expect(t, call(t, h, auditor, "GET", "/api/v1/audit", nil), 200)
	rec := call(t, h, auditor, "POST", "/api/v1/groups", map[string]any{"name": "Ops", "kind": "assigned"})
	expect(t, rec, 403)
	if !strings.Contains(rec.Body.String(), "ERR_FORBIDDEN") {
		t.Fatalf("forbidden code missing: %s", rec.Body)
	}
	expect(t, call(t, h, auditor, "POST", "/api/v1/users/"+nobody.ID+"/suspend", nil), 403)
	expect(t, call(t, h, appAdmin, "POST", "/api/v1/users", map[string]any{"email": "x@example.com", "name": "X"}), 403)
	expect(t, call(t, h, appAdmin, "PUT", "/api/v1/users/"+nobody.ID+"/roles", map[string]any{"roles": []string{"auditor"}}), 403)
	expect(t, call(t, h, nobodyToken, "GET", "/api/v1/users", nil), 403)
	expect(t, call(t, h, "", "GET", "/api/v1/users", nil), 401)
	expect(t, call(t, h, auditor, "GET", "/api/v1/users/usr_missing", nil), 404)
}

func TestInviteReturnsWorkingEnrollLink(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "uma@example.com", "user_admin")
	ops, err := st.CreateGroup(ctx, store.NewGroup{Name: "Ops", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}

	rec := call(t, h, admin, "POST", "/api/v1/users", map[string]any{"email": "sam@example.com", "name": "Sam Lee", "groupIds": []string{ops.ID}})
	expect(t, rec, 201)
	out := decode[struct {
		User      store.User `json:"user"`
		EnrollURL string     `json:"enrollUrl"`
		ExpiresAt time.Time  `json:"expiresAt"`
	}](t, rec)
	prefix := "http://localhost:3200/enroll?token="
	if !strings.HasPrefix(out.EnrollURL, prefix) || out.ExpiresAt.Before(time.Now()) {
		t.Fatalf("enroll link: %+v", out)
	}
	got, purpose, err := st.PeekEnrollmentToken(ctx, strings.TrimPrefix(out.EnrollURL, prefix))
	if err != nil || got.ID != out.User.ID || purpose != "invite" || got.Status != "invited" || !slices.Contains(got.GroupIDs, ops.ID) {
		t.Fatalf("token does not resolve to the invited user: %+v %s %v", got, purpose, err)
	}

	expect(t, call(t, h, admin, "POST", "/api/v1/users", map[string]any{"email": "SAM@example.com", "name": "Sam again"}), 409)
	expect(t, call(t, h, admin, "POST", "/api/v1/users", map[string]any{"email": "Sam <sam2@example.com>", "name": "Sam"}), 422)
	expect(t, call(t, h, admin, "POST", "/api/v1/users", map[string]any{"email": "not-an-email", "name": "Sam"}), 422)

	audit, err := st.ListAudit(ctx, 1)
	if err != nil || len(audit) != 1 || audit[0].Action != "user.invite" || audit[0].TargetID != out.User.ID || audit[0].IP == "" {
		t.Fatalf("audit: %+v %v", audit, err)
	}
}

func TestSuspendRevokesSessions(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	admin, adminToken := person(t, st, "uma@example.com", "user_admin")
	target, targetToken := person(t, st, "tom@example.com")
	session(t, st, target.ID)
	if err := st.TouchSignIn(ctx, target.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	newcomer, _ := person(t, st, "nia@example.com")
	globalAdmin, _ := person(t, st, "gia@example.com", "global_admin")

	expect(t, call(t, h, targetToken, "GET", "/api/v1/users", nil), 403)
	rec := call(t, h, adminToken, "POST", "/api/v1/users/"+target.ID+"/suspend", nil)
	expect(t, rec, 200)
	if u := decode[store.User](t, rec); u.Status != "suspended" {
		t.Fatalf("status: %s", u.Status)
	}
	if sessions, _ := st.ListSessions(ctx, target.ID); len(sessions) != 0 {
		t.Fatalf("sessions survived suspension: %d", len(sessions))
	}
	if _, err := st.SessionByToken(ctx, targetToken); err != store.ErrNotFound {
		t.Fatalf("revoked token still resolves: %v", err)
	}

	expect(t, call(t, h, adminToken, "POST", "/api/v1/users/"+admin.ID+"/suspend", nil), 422)
	expect(t, call(t, h, adminToken, "POST", "/api/v1/users/"+globalAdmin.ID+"/suspend", nil), 403)

	rec = call(t, h, adminToken, "POST", "/api/v1/users/"+target.ID+"/restore", nil)
	expect(t, rec, 200)
	if u := decode[store.User](t, rec); u.Status != "active" {
		t.Fatalf("restore: %s", u.Status)
	}

	expect(t, call(t, h, adminToken, "POST", "/api/v1/users/"+newcomer.ID+"/suspend", nil), 200)
	rec = call(t, h, adminToken, "POST", "/api/v1/users/"+newcomer.ID+"/restore", nil)
	expect(t, rec, 200)
	if u := decode[store.User](t, rec); u.Status != "invited" {
		t.Fatalf("restoring someone who never enrolled: %s", u.Status)
	}
}

func TestApplicationCreateAndRedirectValidation(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "abe@example.com", "app_admin")

	rec := call(t, h, admin, "POST", "/api/v1/applications", map[string]any{"name": "Grafana", "protocol": "oidc", "type": "web", "redirectUris": []string{"https://grafana.example.com/login/generic_oauth"}})
	expect(t, rec, 201)
	out := decode[created](t, rec)
	if out.ClientSecret == nil || !strings.HasPrefix(*out.ClientSecret, "hls_") || len(out.Application.Credentials) != 1 || out.Application.Credentials[0].Label != "Initial" {
		t.Fatalf("create: %+v", out)
	}
	if _, err := st.VerifyClientSecret(ctx, out.Application.ClientID, *out.ClientSecret); err != nil {
		t.Fatalf("returned secret does not verify: %v", err)
	}
	get := call(t, h, admin, "GET", "/api/v1/applications/"+out.Application.ID, nil)
	expect(t, get, 200)
	if strings.Contains(get.Body.String(), *out.ClientSecret) {
		t.Fatal("secret is readable after creation")
	}

	rec = call(t, h, admin, "POST", "/api/v1/applications", map[string]any{"name": "Console", "protocol": "oidc", "type": "spa", "redirectUris": []string{"http://localhost:5173/callback"}})
	expect(t, rec, 201)
	if spa := decode[created](t, rec); spa.ClientSecret != nil || len(spa.Application.Credentials) != 0 {
		t.Fatalf("public client got a secret: %+v", spa)
	}

	for _, body := range []map[string]any{
		{"name": "Bad", "protocol": "oidc", "type": "web", "redirectUris": []string{"http://grafana.example.com/cb"}},
		{"name": "Bad", "protocol": "oidc", "type": "web", "redirectUris": []string{"https://grafana.example.com/cb#frag"}},
		{"name": "Bad", "protocol": "oidc", "type": "web", "redirectUris": []string{"/relative"}},
		{"name": "Bad", "protocol": "oidc", "type": "native", "redirectUris": []string{"myapp:/callback"}},
		{"name": "Bad", "protocol": "oidc", "type": "web", "redirectUris": []string{}},
		{"name": "Bad", "protocol": "saml", "type": "web", "redirectUris": []string{"https://x.example.com/acs"}},
	} {
		expect(t, call(t, h, admin, "POST", "/api/v1/applications", body), 422)
	}
	rec = call(t, h, admin, "POST", "/api/v1/applications", map[string]any{"name": "Bad", "protocol": "ldap", "type": "web"})
	expect(t, rec, 422)
	for _, protocol := range []string{`\"oidc\"`, `\"oauth\"`, `\"saml\"`} {
		if !strings.Contains(rec.Body.String(), protocol) {
			t.Fatalf("protocol list missing %s: %s", protocol, rec.Body)
		}
	}
}

func TestSecretRotationKeepsOldSecretFor24Hours(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "abe@example.com", "app_admin")
	app := decode[created](t, call(t, h, admin, "POST", "/api/v1/applications", map[string]any{"name": "Billing", "protocol": "oauth", "type": "service"}))
	old := app.Application.Credentials[0]

	rec := call(t, h, admin, "POST", "/api/v1/applications/"+app.Application.ID+"/secrets", nil)
	expect(t, rec, 201)
	rotated := decode[struct {
		Credential   store.Credential `json:"credential"`
		ClientSecret string           `json:"clientSecret"`
	}](t, rec)
	for _, value := range []string{*app.ClientSecret, rotated.ClientSecret} {
		if _, err := st.VerifyClientSecret(ctx, app.Application.ClientID, value); err != nil {
			t.Fatalf("secret must work during the grace period: %v", err)
		}
	}
	reloaded, err := st.GetApplication(ctx, app.Application.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range reloaded.Credentials {
		left := time.Until(c.ExpiresAt)
		if c.ID == old.ID && (left > 24*time.Hour || left < 23*time.Hour) {
			t.Fatalf("old secret expires in %s, want 24h", left)
		}
		if c.ID == rotated.Credential.ID && left < 300*24*time.Hour {
			t.Fatalf("new secret expires too soon: %s", left)
		}
	}

	expect(t, call(t, h, admin, "DELETE", "/api/v1/applications/"+app.Application.ID+"/secrets/"+old.ID, nil), 204)
	if _, err := st.VerifyClientSecret(ctx, app.Application.ClientID, *app.ClientSecret); err != store.ErrNotFound {
		t.Fatalf("revoked secret still verifies: %v", err)
	}
}

func TestDynamicGroupMembershipIsRefused(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "uma@example.com", "user_admin")
	member, _ := person(t, st, "tom@example.com")

	rec := call(t, h, admin, "POST", "/api/v1/groups", map[string]any{"name": "Engineering", "kind": "dynamic", "rule": "department = Engineering"})
	expect(t, rec, 422)
	if !strings.Contains(rec.Body.String(), `user.department == \"Engineering\"`) {
		t.Fatalf("rule error should show the syntax: %s", rec.Body)
	}
	rec = call(t, h, admin, "POST", "/api/v1/groups", map[string]any{"name": "Engineering", "kind": "dynamic", "rule": `user.department == "Engineering"`})
	expect(t, rec, 201)
	dynamic := decode[store.Group](t, rec)
	expect(t, call(t, h, admin, "POST", "/api/v1/groups/"+dynamic.ID+"/members", map[string]any{"userId": member.ID}), 409)
	expect(t, call(t, h, admin, "DELETE", "/api/v1/groups/"+dynamic.ID+"/members/"+member.ID, nil), 409)

	ops := decode[store.Group](t, call(t, h, admin, "POST", "/api/v1/groups", map[string]any{"name": "Ops", "kind": "assigned"}))
	expect(t, call(t, h, admin, "POST", "/api/v1/groups/"+ops.ID+"/members", map[string]any{"userId": member.ID}), 204)
	if u, _ := st.GetUser(ctx, member.ID); !slices.Contains(u.GroupIDs, ops.ID) {
		t.Fatalf("member not added: %v", u.GroupIDs)
	}
	expect(t, call(t, h, admin, "DELETE", "/api/v1/groups/"+ops.ID+"/members/"+member.ID, nil), 204)
	expect(t, call(t, h, admin, "DELETE", "/api/v1/groups/"+ops.ID+"/members/"+member.ID, nil), 404)
}

func TestOverviewShape(t *testing.T) {
	st, h := setup(t)
	_, token := person(t, st, "gia@example.com", "global_admin")
	rec := call(t, h, token, "GET", "/api/v1/overview", nil)
	expect(t, rec, 200)
	raw := decode[map[string]json.RawMessage](t, rec)
	for _, key := range []string{"weakAdmins", "expiringCredentials", "highRiskSignIns", "recentSignIns", "failureRates"} {
		if v := string(raw[key]); !strings.HasPrefix(v, "[") {
			t.Fatalf("%s must be an array, got %q", key, v)
		}
	}
	counts := decode[struct {
		WeakAdmins []struct{ ID, Name string } `json:"weakAdmins"`
		Counts     map[string]int              `json:"counts"`
	}](t, rec)
	if len(counts.WeakAdmins) != 1 || counts.Counts["users"] != 1 || counts.Counts["activeUsers"] != 1 || counts.Counts["applications"] != 0 {
		t.Fatalf("overview: %s", rec.Body)
	}
	if body := call(t, h, token, "GET", "/api/v1/groups", nil).Body.String(); strings.TrimSpace(body) != "[]" {
		t.Fatalf("empty list must be [], got %s", body)
	}
}

func TestCrossSitePostRejected(t *testing.T) {
	st, h := setup(t)
	_, token := person(t, st, "gia@example.com", "global_admin")
	req := httptest.NewRequest("POST", "/api/v1/groups", strings.NewReader(`{"name":"Ops","kind":"assigned"}`))
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: token})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	expect(t, rec, 403)
	if !strings.Contains(rec.Body.String(), "ERR_CROSS_SITE") {
		t.Fatalf("code: %s", rec.Body)
	}
	if groups, _ := st.ListGroups(context.Background()); len(groups) != 0 {
		t.Fatal("cross-site request created a group")
	}
}

func TestSeedDemo(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	if err := st.SeedDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.SeedDemo(ctx); err == nil || !strings.Contains(err.Error(), "docker compose -f compose.dev.yml down -v") {
		t.Fatalf("second seed must refuse with reset instructions, got %v", err)
	}
	users, _ := st.ListUsers(ctx)
	groups, _ := st.ListGroups(ctx)
	apps, _ := st.ListApplications(ctx)
	if len(users) != 68 || len(groups) != 14 || len(apps) != 12 {
		t.Fatalf("seeded %d users, %d groups, %d applications", len(users), len(groups), len(apps))
	}
	if _, err := st.GetUserByEmail(ctx, "farah.castillo@example.com"); err != nil {
		t.Fatalf("generated users must match the prototype: %v", err)
	}
	luna, err := st.GetUserByEmail(ctx, "luna@example.com")
	if err != nil || !luna.HasRole("global_admin") || luna.Strength != "phishing-resistant" || len(luna.Methods) != 4 {
		t.Fatalf("luna: %+v %v", luna, err)
	}

	rec := call(t, h, session(t, st, luna.ID), "GET", "/api/v1/overview", nil)
	expect(t, rec, 200)
	out := decode[struct {
		WeakAdmins []struct {
			Name string `json:"name"`
		} `json:"weakAdmins"`
		Expiring []struct {
			AppName  string `json:"appName"`
			DaysLeft int    `json:"daysLeft"`
		} `json:"expiringCredentials"`
		HighRisk []store.SignInEvent `json:"highRiskSignIns"`
		Recent   []store.SignInEvent `json:"recentSignIns"`
	}](t, rec)
	if len(out.WeakAdmins) != 3 || len(out.Expiring) != 2 || out.Expiring[0].AppName != "Grafana" || out.Expiring[0].DaysLeft != 9 || out.Expiring[1].AppName != "Proxmox VE" || out.Expiring[1].DaysLeft != 21 {
		t.Fatalf("overview: %s", rec.Body)
	}
	if len(out.HighRisk) != 1 || out.HighRisk[0].Location != "Unknown · Tor exit node" || len(out.Recent) != 8 {
		t.Fatalf("sign-ins: %+v", out.HighRisk)
	}
	if sessions, _ := st.ListSessions(ctx, ""); len(sessions) < 10 {
		t.Fatalf("expected sessions from recent sign-ins, got %d", len(sessions))
	}
}
