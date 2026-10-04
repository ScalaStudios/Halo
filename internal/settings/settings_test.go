package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"halo/internal/auth"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/store"
	"halo/internal/testdb"
)

type env struct {
	t     *testing.T
	st    *store.Store
	authn *httpx.Auth
	h     http.Handler
}

func setup(t *testing.T) *env {
	t.Helper()
	st := testdb.New(t)
	public, _ := url.Parse("http://localhost:3200")
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	d := httpx.Deps{Config: config.Config{Organization: "Fernway Systems", PublicURL: public, Dev: true}, Store: st, Auth: authn}
	mux := http.NewServeMux()
	if err := auth.Register(mux, d); err != nil {
		t.Fatal(err)
	}
	if err := Register(mux, d); err != nil {
		t.Fatal(err)
	}
	mux.Handle("GET /api/v1/organization", Organization(d))
	return &env{t: t, st: st, authn: authn, h: authn.Resolve(authn.SameOrigin(mux))}
}

func (e *env) person(email string, roles ...string) string {
	e.t.Helper()
	ctx := context.Background()
	u, err := e.st.GetUserByEmail(ctx, email)
	if err != nil {
		u, err = e.st.CreateUser(ctx, store.NewUser{Email: email, Name: strings.Split(email, "@")[0], Status: "active", Roles: roles})
	}
	if err != nil {
		e.t.Fatal(err)
	}
	_, token, err := e.st.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: "passkey"})
	if err != nil {
		e.t.Fatal(err)
	}
	return token
}

func (e *env) call(token, method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	var reader io.Reader
	if raw, ok := body.([]byte); ok {
		reader = bytes.NewReader(raw)
	} else if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: token})
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e *env) expect(rec *httptest.ResponseRecorder, status int) *httptest.ResponseRecorder {
	e.t.Helper()
	if rec.Code != status {
		e.t.Fatalf("status %d, want %d: %s", rec.Code, status, rec.Body.String())
	}
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return v
}

type settingsResponse struct {
	Settings store.Settings `json:"settings"`
}

func TestSettingsValidationCacheAndRoles(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	admin := e.person("gia@example.com", "global_admin")
	security := e.person("sam@example.com", "security_admin")
	auditor := e.person("aud@example.com", "auditor")
	users := e.person("uma@example.com", "user_admin")

	got := decode[settingsResponse](t, e.expect(e.call(auditor, "GET", "/api/v1/settings", nil), 200))
	if got.Settings != store.DefaultSettings {
		t.Fatalf("defaults: %+v", got.Settings)
	}
	for _, body := range []map[string]any{
		{"sessionHours": 0}, {"sessionHours": 73}, {"lockoutThreshold": 2}, {"lockoutMinutes": 4}, {"inviteDays": 31},
		{"accessTokenMinutes": 1441}, {"refreshTokenHours": 0}, {"contactEmail": "IT <it@example.com>"}, {"signInMessage": strings.Repeat("x", 501)},
	} {
		e.expect(e.call(admin, "PATCH", "/api/v1/settings", body), 422)
	}
	e.expect(e.call(admin, "PATCH", "/api/v1/settings", map[string]any{"bogus": 1}), 400)

	e.expect(e.call(auditor, "PATCH", "/api/v1/settings", map[string]any{"sessionHours": 8}), 403)
	e.expect(e.call(users, "PATCH", "/api/v1/settings", map[string]any{"sessionHours": 8}), 403)
	e.expect(e.call(security, "PATCH", "/api/v1/settings", map[string]any{"organizationName": "Acme"}), 403)
	got = decode[settingsResponse](t, e.expect(e.call(security, "PATCH", "/api/v1/settings", map[string]any{"sessionHours": 8, "lockoutMinutes": 30}), 200))
	if got.Settings.SessionHours != 8 || got.Settings.LockoutMinutes != 30 || got.Settings.LockoutThreshold != 5 {
		t.Fatalf("patched: %+v", got.Settings)
	}
	audit, _ := e.st.ListAudit(ctx, 10)
	if len(audit) != 1 || audit[0].Action != "settings.update" || audit[0].Summary != "Changed the session lifetime, lockout window" {
		t.Fatalf("audit: %+v", audit)
	}

	org := decode[Org](t, e.expect(e.call("", "GET", "/api/v1/organization", nil), 200))
	if org.Name != "Fernway Systems" || org.LogoURL != nil {
		t.Fatalf("fallback organization: %+v", org)
	}
	e.expect(e.call(admin, "PATCH", "/api/v1/settings", map[string]any{"organizationName": "  Acme  ", "contactEmail": "help@acme.test", "signInMessage": "Use your Acme passkey."}), 200)
	org = decode[Org](t, e.expect(e.call("", "GET", "/api/v1/organization", nil), 200))
	if org.Name != "Acme" || org.ContactEmail != "help@acme.test" || org.SignInMessage != "Use your Acme passkey." {
		t.Fatalf("stored organization: %+v", org)
	}

	changed := got.Settings
	changed.SessionHours = 3
	if err := e.st.SaveSettings(ctx, changed); err != nil {
		t.Fatal(err)
	}
	if s, _ := e.st.Settings(ctx); s.SessionHours != 8 {
		t.Fatalf("cache should still hold 8 hours, got %d", s.SessionHours)
	}
	e.st.ForgetSettings()
	if s, _ := e.st.Settings(ctx); s.SessionHours != 3 {
		t.Fatalf("after forgetting the cache: %d hours", s.SessionHours)
	}
}

func TestSessionLifetimeAppliesToSessionsAndCookies(t *testing.T) {
	e := setup(t)
	admin := e.person("gia@example.com", "global_admin")
	e.expect(e.call(admin, "PATCH", "/api/v1/settings", map[string]any{"sessionHours": 2}), 200)

	u, _ := e.st.GetUserByEmail(context.Background(), "gia@example.com")
	sess, _, err := e.st.CreateSession(context.Background(), store.NewSession{UserID: u.ID, Method: "passkey"})
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Until(sess.ExpiresAt); d < 2*time.Hour-time.Minute || d > 2*time.Hour {
		t.Fatalf("session expires in %s, want 2h", d)
	}
	rec := httptest.NewRecorder()
	e.authn.SetSession(rec, "token")
	cookie := rec.Result().Cookies()[0]
	if d := time.Until(cookie.Expires); d < 2*time.Hour-time.Minute || d > 2*time.Hour {
		t.Fatalf("cookie expires in %s, want 2h", d)
	}
}

func TestLockoutThresholdApplied(t *testing.T) {
	e := setup(t)
	admin := e.person("gia@example.com", "global_admin")
	e.person("ada@example.com")
	e.expect(e.call(admin, "PATCH", "/api/v1/settings", map[string]any{"lockoutThreshold": 3, "lockoutMinutes": 20}), 200)
	signIn := map[string]any{"email": "ada@example.com", "code": "000000"}
	for range 3 {
		e.expect(e.call("", "POST", "/api/v1/auth/totp", signIn), 401)
	}
	rec := e.expect(e.call("", "POST", "/api/v1/auth/totp", signIn), 429)
	if !strings.Contains(rec.Body.String(), "Wait 20 minutes") {
		t.Fatalf("lockout message: %s", rec.Body.String())
	}
}

func TestDomainVerification(t *testing.T) {
	e := setup(t)
	admin := e.person("gia@example.com", "global_admin")
	security := e.person("sam@example.com", "security_admin")
	records, original := map[string][]string{}, lookupTXT
	lookupTXT = func(_ context.Context, host string) ([]string, error) { return records[host], nil }
	t.Cleanup(func() { lookupTXT = original })

	e.expect(e.call(security, "POST", "/api/v1/domains", map[string]any{"name": "example.com"}), 403)
	e.expect(e.call(admin, "POST", "/api/v1/domains", map[string]any{"name": "https://example.com/login"}), 422)
	d := decode[store.Domain](t, e.expect(e.call(admin, "POST", "/api/v1/domains", map[string]any{"name": " Example.COM. "}), 201))
	if d.Name != "example.com" || d.Token == "" || d.VerifiedAt != nil {
		t.Fatalf("added: %+v", d)
	}
	e.expect(e.call(admin, "POST", "/api/v1/domains", map[string]any{"name": "example.com"}), 409)

	e.expect(e.call(admin, "POST", "/api/v1/domains/"+d.ID+"/verify", nil), 422)
	records["example.com"] = []string{"v=spf1 -all", "halo-verification=wrong"}
	e.expect(e.call(admin, "POST", "/api/v1/domains/"+d.ID+"/verify", nil), 422)
	records["example.com"] = append(records["example.com"], "halo-verification="+d.Token)
	verified := decode[store.Domain](t, e.expect(e.call(admin, "POST", "/api/v1/domains/"+d.ID+"/verify", nil), 200))
	if verified.VerifiedAt == nil {
		t.Fatalf("not verified: %+v", verified)
	}
	list := decode[[]store.Domain](t, e.expect(e.call(security, "GET", "/api/v1/domains", nil), 200))
	if len(list) != 1 || list[0].VerifiedAt == nil {
		t.Fatalf("list: %+v", list)
	}
	e.expect(e.call(admin, "DELETE", "/api/v1/domains/"+d.ID, nil), 204)
	e.expect(e.call(admin, "DELETE", "/api/v1/domains/"+d.ID, nil), 404)
}

func TestLogoUpload(t *testing.T) {
	e := setup(t)
	admin := e.person("gia@example.com", "global_admin")
	security := e.person("sam@example.com", "security_admin")
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)

	e.expect(e.call(security, "PUT", "/api/v1/branding/logo", png), 403)
	e.expect(e.call(admin, "PUT", "/api/v1/branding/logo", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)), 422)
	e.expect(e.call(admin, "PUT", "/api/v1/branding/logo", append(png, make([]byte, 256<<10)...)), 413)
	org := decode[Org](t, e.expect(e.call(admin, "PUT", "/api/v1/branding/logo", png), 200))
	if org.LogoURL == nil {
		t.Fatal("no logo URL after upload")
	}
	rec := e.expect(e.call("", "GET", "/api/v1/branding/logo", nil), 200)
	if rec.Header().Get("Content-Type") != "image/png" || !bytes.Equal(rec.Body.Bytes(), png) {
		t.Fatalf("served logo: %s", rec.Header().Get("Content-Type"))
	}
	e.expect(e.call(admin, "DELETE", "/api/v1/branding/logo", nil), 204)
	e.expect(e.call("", "GET", "/api/v1/branding/logo", nil), 404)
}

func TestExportExcludesSecrets(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if err := e.st.SeedDemo(ctx); err != nil {
		t.Fatal(err)
	}
	admin := e.person("luna@example.com")
	security := e.person("sam@example.com", "security_admin")
	apps, _ := e.st.ListApplications(ctx)
	_, clientSecret, err := e.st.AddClientSecret(ctx, apps[0].ID, "Export test", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, webhookSecret, err := e.st.CreateWebhook(ctx, store.NewWebhook{URL: "https://hooks.example.com", Events: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}

	e.expect(e.call(security, "GET", "/api/v1/export", nil), 403)
	rec := e.expect(e.call(admin, "GET", "/api/v1/export", nil), 200)
	body := rec.Body.String()
	var export map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &export); err != nil {
		t.Fatalf("export is not valid JSON: %v", err)
	}
	for _, section := range []string{"exportedAt", "organization", "settings", "users", "groups", "memberships", "roles", "applications", "policies", "accessPackages", "accessReviews"} {
		if export[section] == nil {
			t.Errorf("export has no %s", section)
		}
	}
	if users := export["users"].([]any); len(users) < 10 {
		t.Errorf("export has %d users", len(users))
	}
	for _, value := range []string{clientSecret, webhookSecret, admin, security} {
		if strings.Contains(body, value) {
			t.Errorf("export contains the secret %q", value)
		}
	}
	forbidden := regexp.MustCompile(`(?i)hash|sealed|secret|password|private`)
	var walk func(any)
	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			for key, child := range v {
				if forbidden.MatchString(key) {
					t.Errorf("export has a %q field", key)
				}
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(export)
	audit, _ := e.st.ListAudit(ctx, 1)
	if len(audit) != 1 || audit[0].Action != "organization.export" {
		t.Fatalf("export was not audited: %+v", audit)
	}
}
