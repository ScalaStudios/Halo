package api_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"halo/internal/config"
	"halo/internal/server"
	"halo/internal/store"
	"halo/internal/testdb"
)

const toolCallback = "https://tool.example.com/callback"

func registrationStack(t *testing.T, mode string) (*store.Store, http.Handler) {
	t.Helper()
	st := testdb.New(t)
	ctx := context.Background()
	if mode != "" {
		settings, err := st.CurrentSettings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		settings.ClientRegistration = mode
		if err := st.SaveSettings(ctx, settings); err != nil {
			t.Fatal(err)
		}
		st.ForgetSettings()
	}
	public, _ := url.Parse("http://localhost:3200")
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	app, err := server.New(config.Config{Organization: "Test", PublicURL: public, SecretKey: key, Dev: true}, st)
	if err != nil {
		t.Fatal(err)
	}
	return st, app.Handler
}

func oauth(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader = http.NoBody
	if raw, ok := body.(string); ok {
		reader = strings.NewReader(raw)
	} else if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func oauthError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var body struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != status || body.Error != code || body.Description == "" {
		t.Fatalf("want %d %s, got %d: %s", status, code, rec.Code, rec.Body)
	}
}

type registration struct {
	ClientID                string   `json:"client_id"`
	ClientSecret            string   `json:"client_secret"`
	ClientSecretExpiresAt   int64    `json:"client_secret_expires_at"`
	RegistrationAccessToken string   `json:"registration_access_token"`
	RegistrationClientURI   string   `json:"registration_client_uri"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope"`
}

func tokenError(t *testing.T, h http.Handler, clientID, secret string) string {
	t.Helper()
	form := url.Values{"grant_type": {"client_credentials"}, "scope": {"openid"}}
	req := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, secret)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return body.Error
}

func discovery(t *testing.T, h http.Handler) map[string]any {
	t.Helper()
	rec := oauth(t, h, http.MethodGet, "/.well-known/openid-configuration", "", nil)
	expect(t, rec, http.StatusOK)
	return decode[map[string]any](t, rec)
}

func latestAudit(t *testing.T, st *store.Store) store.AuditEvent {
	t.Helper()
	events, err := st.ListAudit(context.Background(), 1)
	if err != nil || len(events) != 1 {
		t.Fatalf("audit: %v %v", events, err)
	}
	return events[0]
}

func TestClientRegistrationIsOffByDefault(t *testing.T) {
	st, h := registrationStack(t, "")
	if _, ok := discovery(t, h)["registration_endpoint"]; ok {
		t.Fatal("discovery advertises registration while it is off")
	}
	oauthError(t, oauth(t, h, http.MethodPost, "/oauth2/register", "", map[string]any{"redirect_uris": []string{toolCallback}}), http.StatusNotFound, "invalid_request")

	_, security := person(t, st, "sec@example.com", "security_admin")
	expect(t, call(t, h, security, http.MethodPatch, "/api/v1/settings", map[string]any{"clientRegistration": "everyone"}), http.StatusUnprocessableEntity)
	expect(t, call(t, h, security, http.MethodPatch, "/api/v1/settings", map[string]any{"clientRegistration": "open"}), http.StatusOK)
	if got := discovery(t, h)["registration_endpoint"]; got != "http://localhost:3200/oauth2/register" {
		t.Fatalf("registration_endpoint = %v", got)
	}
}

func TestOpenRegistrationGetsNoAdministratorSettings(t *testing.T) {
	st, h := registrationStack(t, "open")
	ctx := context.Background()
	rec := oauth(t, h, http.MethodPost, "/oauth2/register", "", map[string]any{
		"client_name":   "Notes tool",
		"redirect_uris": []string{toolCallback},
		"grant_types":   []string{"authorization_code", "refresh_token"},
		"scope":         "openid email groups offline_access billing:read",
		"logo_uri":      "https://tool.example.com/logo.png",
		"groupIds":      []string{"grp_x"},
		"tokenPolicy":   map[string]any{"exchange": true},
	})
	expect(t, rec, http.StatusCreated)
	reg := decode[registration](t, rec)
	if reg.ClientSecret == "" || reg.ClientSecretExpiresAt == 0 || !strings.HasPrefix(reg.RegistrationAccessToken, "hlr_") || reg.TokenEndpointAuthMethod != "client_secret_basic" {
		t.Fatalf("incomplete registration response: %s", rec.Body)
	}
	if reg.Scope != "openid email" || reg.RegistrationClientURI != "http://localhost:3200/oauth2/register/"+reg.ClientID {
		t.Fatalf("scope %q, registration_client_uri %q", reg.Scope, reg.RegistrationClientURI)
	}
	app, err := st.GetApplicationByClientID(ctx, reg.ClientID)
	if err != nil {
		t.Fatal(err)
	}
	if !app.RegisteredDynamically || app.AwaitingApproval || app.Status != "active" || app.Type != "web" || app.Protocol != "oidc" {
		t.Fatalf("unexpected application: %+v", app)
	}
	if len(app.GroupIDs) != 0 || app.Owner != nil || app.TokenPolicy.Exchange || slices.Contains(app.Scopes, "groups") {
		t.Fatalf("registered client got administrator settings: %+v", app)
	}
	if e := latestAudit(t, st); e.Action != "application.register" || e.ActorID != nil || e.TargetID != app.ID {
		t.Fatalf("audit: %+v", e)
	}

	public := decode[registration](t, oauth(t, h, http.MethodPost, "/oauth2/register", "", map[string]any{"redirect_uris": []string{"http://127.0.0.1:8123/cb"}, "token_endpoint_auth_method": "none", "application_type": "native"}))
	if public.ClientSecret != "" || public.ClientName != "127.0.0.1:8123" {
		t.Fatalf("public client: %+v", public)
	}
	if native, _ := st.GetApplicationByClientID(ctx, public.ClientID); native.Type != "native" || len(native.Credentials) != 0 {
		t.Fatalf("native client: %+v", native)
	}
}

func TestRegistrationRejectsUnsafeMetadata(t *testing.T) {
	st, h := registrationStack(t, "open")
	ctx := context.Background()
	before, _ := st.ListApplications(ctx)
	valid := func(change map[string]any) map[string]any {
		body := map[string]any{"client_name": "Tool", "redirect_uris": []string{toolCallback}}
		for k, v := range change {
			body[k] = v
		}
		return body
	}
	cases := []struct {
		name string
		body any
		code string
	}{
		{"javascript redirect", valid(map[string]any{"redirect_uris": []string{"javascript:alert(1)"}}), "invalid_redirect_uri"},
		{"data redirect", valid(map[string]any{"redirect_uris": []string{"data:text/html,<script>alert(1)</script>"}}), "invalid_redirect_uri"},
		{"plain http redirect", valid(map[string]any{"redirect_uris": []string{"http://tool.example.com/callback"}}), "invalid_redirect_uri"},
		{"fragment redirect", valid(map[string]any{"redirect_uris": []string{toolCallback + "#x"}}), "invalid_redirect_uri"},
		{"no redirect", valid(map[string]any{"redirect_uris": []string{}}), "invalid_redirect_uri"},
		{"client credentials", valid(map[string]any{"grant_types": []string{"authorization_code", "client_credentials"}}), "invalid_client_metadata"},
		{"implicit", valid(map[string]any{"grant_types": []string{"implicit"}}), "invalid_client_metadata"},
		{"token exchange", valid(map[string]any{"grant_types": []string{"authorization_code", "urn:ietf:params:oauth:grant-type:token-exchange"}}), "invalid_client_metadata"},
		{"device code", valid(map[string]any{"grant_types": []string{"urn:ietf:params:oauth:grant-type:device_code"}}), "invalid_client_metadata"},
		{"token response", valid(map[string]any{"response_types": []string{"token"}}), "invalid_client_metadata"},
		{"native with secret", valid(map[string]any{"application_type": "native"}), "invalid_client_metadata"},
		{"private key jwt", valid(map[string]any{"token_endpoint_auth_method": "private_key_jwt"}), "invalid_client_metadata"},
		{"javascript logout", valid(map[string]any{"post_logout_redirect_uris": []string{"javascript:alert(1)"}}), "invalid_client_metadata"},
		{"javascript client uri", valid(map[string]any{"client_uri": "javascript:alert(1)"}), "invalid_client_metadata"},
		{"multi-line name", valid(map[string]any{"client_name": "Tool\nPayroll"}), "invalid_client_metadata"},
		{"long name", valid(map[string]any{"client_name": strings.Repeat("n", 201)}), "invalid_client_metadata"},
		{"oversize metadata", valid(map[string]any{"software_statement": strings.Repeat("x", 17<<10)}), "invalid_client_metadata"},
		{"not json", "redirect_uris=https://tool.example.com/callback", "invalid_client_metadata"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			oauthError(t, oauth(t, h, http.MethodPost, "/oauth2/register", "", c.body), http.StatusBadRequest, c.code)
		})
	}
	if after, _ := st.ListApplications(ctx); len(after) != len(before) {
		t.Fatalf("rejected registrations created %d applications", len(after)-len(before))
	}
	for range 20 - len(cases) {
		expect(t, oauth(t, h, http.MethodPost, "/oauth2/register", "", valid(nil)), http.StatusCreated)
	}
	oauthError(t, oauth(t, h, http.MethodPost, "/oauth2/register", "", valid(nil)), http.StatusTooManyRequests, "temporarily_unavailable")
}

func TestRegistrationRequiresAnInitialAccessToken(t *testing.T) {
	st, h := registrationStack(t, "token")
	ctx := context.Background()
	key := func(name string, scopes ...string) (store.ServiceAccount, string) {
		account, err := st.CreateServiceAccount(ctx, store.NewServiceAccount{Name: name, Roles: []string{"app_admin"}})
		if err != nil {
			t.Fatal(err)
		}
		_, value, err := st.CreateAPIKey(ctx, account.ID, name, scopes, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return account, value
	}
	_, apiKey := key("Terraform", "api")
	broker, registrationKey := key("Tool broker", "registration")
	body := map[string]any{"client_name": "Tool", "redirect_uris": []string{toolCallback}}

	oauthError(t, oauth(t, h, http.MethodPost, "/oauth2/register", "", body), http.StatusUnauthorized, "invalid_token")
	oauthError(t, oauth(t, h, http.MethodPost, "/oauth2/register", "hlk_not-a-real-key", body), http.StatusUnauthorized, "invalid_token")
	rec := oauth(t, h, http.MethodPost, "/oauth2/register", apiKey, body)
	oauthError(t, rec, http.StatusUnauthorized, "invalid_token")
	if !strings.Contains(rec.Header().Get("WWW-Authenticate"), "invalid_token") {
		t.Fatalf("missing WWW-Authenticate: %v", rec.Header())
	}
	expect(t, oauth(t, h, http.MethodPost, "/oauth2/register", registrationKey, body), http.StatusCreated)
	if e := latestAudit(t, st); e.Action != "application.register" || e.ActorID == nil || *e.ActorID != broker.ID {
		t.Fatalf("audit actor must be the key's service account: %+v", e)
	}
	expect(t, oauth(t, h, http.MethodGet, "/api/v1/applications", registrationKey, nil), http.StatusUnauthorized)
}

func TestRegistrationManagement(t *testing.T) {
	st, h := registrationStack(t, "open")
	ctx := context.Background()
	first := decode[registration](t, oauth(t, h, http.MethodPost, "/oauth2/register", "", map[string]any{"client_name": "Tool", "redirect_uris": []string{toolCallback}}))
	second := decode[registration](t, oauth(t, h, http.MethodPost, "/oauth2/register", "", map[string]any{"client_name": "Other", "redirect_uris": []string{toolCallback}}))
	console, err := st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", RedirectURIs: []string{toolCallback}, Scopes: []string{"openid"}})
	if err != nil {
		t.Fatal(err)
	}
	path := "/oauth2/register/" + first.ClientID

	read := oauth(t, h, http.MethodGet, path, first.RegistrationAccessToken, nil)
	expect(t, read, http.StatusOK)
	if got := decode[registration](t, read); got.ClientSecret != "" || got.RegistrationAccessToken != "" || got.ClientName != "Tool" {
		t.Fatalf("read leaked secrets or lost metadata: %s", read.Body)
	}
	oauthError(t, oauth(t, h, http.MethodGet, path, "", nil), http.StatusUnauthorized, "invalid_token")
	oauthError(t, oauth(t, h, http.MethodGet, path, second.RegistrationAccessToken, nil), http.StatusUnauthorized, "invalid_token")
	oauthError(t, oauth(t, h, http.MethodGet, "/oauth2/register/"+console.ClientID, first.RegistrationAccessToken, nil), http.StatusUnauthorized, "invalid_token")
	oauthError(t, oauth(t, h, http.MethodDelete, "/oauth2/register/halo-cli", first.RegistrationAccessToken, nil), http.StatusUnauthorized, "invalid_token")

	update := map[string]any{"client_id": first.ClientID, "client_name": "Tool v2", "redirect_uris": []string{"https://tool.example.com/v2/callback"}, "scope": "openid profile groups"}
	oauthError(t, oauth(t, h, http.MethodPut, "/oauth2/register/"+second.ClientID, second.RegistrationAccessToken, update), http.StatusBadRequest, "invalid_request")
	rec := oauth(t, h, http.MethodPut, path, first.RegistrationAccessToken, update)
	expect(t, rec, http.StatusOK)
	if got := decode[registration](t, rec); got.Scope != "openid profile" || got.ClientName != "Tool v2" || got.RedirectURIs[0] != "https://tool.example.com/v2/callback" {
		t.Fatalf("update: %s", rec.Body)
	}
	if e := latestAudit(t, st); e.Action != "application.registration.update" || e.ActorID != nil {
		t.Fatalf("audit: %+v", e)
	}
	update["token_endpoint_auth_method"] = "none"
	oauthError(t, oauth(t, h, http.MethodPut, path, first.RegistrationAccessToken, update), http.StatusBadRequest, "invalid_client_metadata")
	delete(update, "token_endpoint_auth_method")
	update["redirect_uris"] = []string{"javascript:alert(1)"}
	oauthError(t, oauth(t, h, http.MethodPut, path, first.RegistrationAccessToken, update), http.StatusBadRequest, "invalid_redirect_uri")

	group, err := st.CreateGroup(ctx, store.NewGroup{Name: "Engineering", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	other, _ := st.GetApplicationByClientID(ctx, second.ClientID)
	if err := st.SetApplicationGroups(ctx, other.ID, []string{group.ID}); err != nil {
		t.Fatal(err)
	}
	expect(t, oauth(t, h, http.MethodPut, "/oauth2/register/"+second.ClientID, second.RegistrationAccessToken, map[string]any{"client_id": second.ClientID, "client_name": "Other", "redirect_uris": []string{"https://elsewhere.example.com/callback"}}), http.StatusOK)
	if other, _ := st.GetApplicationByClientID(ctx, second.ClientID); other.Status != "disabled" || !other.AwaitingApproval {
		t.Fatalf("an assigned client that changes its registration must wait for approval: %+v", other)
	}
	if got := tokenError(t, h, second.ClientID, second.ClientSecret); got != "invalid_client" {
		t.Fatalf("requeued client at the token endpoint: %q", got)
	}

	expect(t, oauth(t, h, http.MethodDelete, path, first.RegistrationAccessToken, nil), http.StatusNoContent)
	if _, err := st.GetApplicationByClientID(ctx, first.ClientID); err == nil {
		t.Fatal("deleted client still exists")
	}
	if e := latestAudit(t, st); e.Action != "application.registration.delete" {
		t.Fatalf("audit: %+v", e)
	}
	oauthError(t, oauth(t, h, http.MethodGet, path, first.RegistrationAccessToken, nil), http.StatusUnauthorized, "invalid_token")
}

func TestApprovalQueue(t *testing.T) {
	st, h := registrationStack(t, "approval")
	ctx := context.Background()
	reg := decode[registration](t, oauth(t, h, http.MethodPost, "/oauth2/register", "", map[string]any{"client_name": "Tool", "redirect_uris": []string{toolCallback}}))
	app, _ := st.GetApplicationByClientID(ctx, reg.ClientID)
	if app.Status != "disabled" || !app.AwaitingApproval {
		t.Fatalf("queued client: %+v", app)
	}
	if got := tokenError(t, h, reg.ClientID, reg.ClientSecret); got != "invalid_client" {
		t.Fatalf("queued client at the token endpoint: %q", got)
	}

	_, admin := person(t, st, "apps@example.com", "app_admin")
	enable := "/api/v1/applications/" + app.ID + "/enable"
	expect(t, call(t, h, admin, http.MethodPost, enable, map[string]any{"redirectUris": []string{"https://elsewhere.example.com/callback"}, "postLogoutUris": []string{}}), http.StatusConflict)
	rec := call(t, h, admin, http.MethodPost, enable, map[string]any{"redirectUris": []string{toolCallback}, "postLogoutUris": []string{}})
	expect(t, rec, http.StatusOK)
	if approved := decode[store.Application](t, rec); approved.Status != "active" || approved.AwaitingApproval || !approved.RegisteredDynamically {
		t.Fatalf("approved client: %s", rec.Body)
	}
	if e := latestAudit(t, st); e.Action != "application.registration.approve" || e.ActorID == nil {
		t.Fatalf("audit: %+v", e)
	}
	if got := tokenError(t, h, reg.ClientID, reg.ClientSecret); got != "unauthorized_client" {
		t.Fatalf("approved client at the token endpoint: %q", got)
	}

	path := "/oauth2/register/" + reg.ClientID
	same := map[string]any{"client_id": reg.ClientID, "client_name": "Tool", "redirect_uris": []string{toolCallback}, "scope": "openid profile email"}
	expect(t, oauth(t, h, http.MethodPut, path, reg.RegistrationAccessToken, same), http.StatusOK)
	if app, _ := st.GetApplicationByClientID(ctx, reg.ClientID); app.Status != "active" {
		t.Fatal("an unchanged registration must not go back to the queue")
	}
	same["redirect_uris"] = []string{"https://elsewhere.example.com/callback"}
	expect(t, oauth(t, h, http.MethodPut, path, reg.RegistrationAccessToken, same), http.StatusOK)
	if app, _ := st.GetApplicationByClientID(ctx, reg.ClientID); app.Status != "disabled" || !app.AwaitingApproval {
		t.Fatalf("changed client must wait for approval again: %+v", app)
	}
	if got := tokenError(t, h, reg.ClientID, reg.ClientSecret); got != "invalid_client" {
		t.Fatalf("requeued client at the token endpoint: %q", got)
	}

	settings, _ := st.CurrentSettings(ctx)
	settings.ClientRegistration = "off"
	if err := st.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	st.ForgetSettings()
	oauthError(t, oauth(t, h, http.MethodGet, path, reg.RegistrationAccessToken, nil), http.StatusNotFound, "invalid_request")
	oauthError(t, oauth(t, h, http.MethodDelete, path, reg.RegistrationAccessToken, nil), http.StatusNotFound, "invalid_request")
}
