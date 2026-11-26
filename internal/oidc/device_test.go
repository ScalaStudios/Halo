package oidc_test

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
	"time"

	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"halo/internal/access"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/metrics"
	halooidc "halo/internal/oidc"
	"halo/internal/secret"
	"halo/internal/store"
	"halo/internal/testdb"
)

type infra struct {
	t     *testing.T
	st    *store.Store
	srv   *httptest.Server
	admin string
	user  store.User
	token string
}

func newInfra(t *testing.T, policy access.Engine) *infra {
	t.Helper()
	ctx := context.Background()
	h := &infra{t: t, st: testdb.New(t)}
	admin, err := h.st.CreateUser(ctx, store.NewUser{Email: "grace@example.com", Name: "Grace Hopper", Status: "active", Roles: []string{"global_admin"}})
	h.must(err)
	_, h.admin, err = h.st.CreateSession(ctx, store.NewSession{UserID: admin.ID, Method: "passkey"})
	h.must(err)
	h.user, err = h.st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active"})
	h.must(err)
	_, h.token, err = h.st.CreateSession(ctx, store.NewSession{UserID: h.user.ID, Method: "totp"})
	h.must(err)

	var handler http.Handler
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
	t.Cleanup(h.srv.Close)
	issuer, _ := url.Parse(h.srv.URL)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	authn := &httpx.Auth{Store: h.st, Public: issuer, Dev: true, AccessTokens: halooidc.AccessTokenResolver(h.st)}
	deps := httpx.Deps{Config: config.Config{PublicURL: issuer, SecretKey: key, Dev: true}, Store: h.st, Auth: authn, Policy: policy}
	provider, err := halooidc.New(deps)
	h.must(err)
	api := http.NewServeMux()
	h.must(halooidc.Register(api, deps))
	api.Handle("GET /api/v1/me/probe", httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		u, _, err := httpx.RequireUser(r)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]string{"id": u.ID})
	}))
	mux := http.NewServeMux()
	mux.Handle("/api/", authn.Resolve(authn.SameOrigin(api)))
	mux.Handle("/oauth2/", authn.Resolve(provider))
	mux.Handle("/.well-known/", provider)
	handler = mux
	return h
}

func (h *infra) must(err error) {
	h.t.Helper()
	if err != nil {
		h.t.Fatal(err)
	}
}

func cookie(token string) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: token}) }
}

func bearerToken(token string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
}

func (h *infra) send(method, path string, body any, opts ...func(*http.Request)) result {
	h.t.Helper()
	var reader io.Reader
	contentType := ""
	switch b := body.(type) {
	case nil:
	case url.Values:
		reader, contentType = strings.NewReader(b.Encode()), "application/x-www-form-urlencoded"
	default:
		data, err := json.Marshal(b)
		h.must(err)
		reader, contentType = bytes.NewReader(data), "application/json"
	}
	req, err := http.NewRequest(method, h.srv.URL+path, reader)
	h.must(err)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for _, opt := range opts {
		opt(req)
	}
	resp, err := http.DefaultClient.Do(req)
	h.must(err)
	defer resp.Body.Close()
	r := result{status: resp.StatusCode, location: resp.Header.Get("Location")}
	_ = json.NewDecoder(resp.Body).Decode(&r.body)
	return r
}

func (h *infra) startDevice(scope string) (string, string) {
	h.t.Helper()
	r := h.send(http.MethodPost, "/oauth2/device_authorization", url.Values{"client_id": {halooidc.CLIClientID}, "scope": {scope}}, func(r *http.Request) { r.Header.Set("User-Agent", "Halo CLI (linux/amd64)") })
	deviceCode, _ := r.body["device_code"].(string)
	userCode, _ := r.body["user_code"].(string)
	if r.status != http.StatusOK || deviceCode == "" || r.body["verification_uri_complete"] != h.srv.URL+"/device?user_code="+userCode || r.body["interval"] != float64(5) {
		h.t.Fatalf("device authorization: %d %v", r.status, r.body)
	}
	return deviceCode, userCode
}

func (h *infra) poll(deviceCode string) result {
	h.t.Helper()
	return h.send(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {string(oidc.GrantTypeDeviceCode)}, "device_code": {deviceCode}, "client_id": {halooidc.CLIClientID}})
}

func TestDeviceFlow(t *testing.T) {
	h := newInfra(t, nil)
	ctx := context.Background()

	deviceCode, userCode := h.startDevice("openid profile email offline_access")
	if r := h.poll(deviceCode); r.status != http.StatusBadRequest || r.body["error"] != "authorization_pending" {
		t.Fatalf("poll before approval: %d %v", r.status, r.body)
	}
	if r := h.send(http.MethodGet, "/api/v1/device/"+userCode, nil); r.status != http.StatusUnauthorized {
		t.Fatalf("lookup without a session: %d %v", r.status, r.body)
	}
	typed := strings.ToLower(strings.ReplaceAll(userCode, "-", " "))
	shown := h.send(http.MethodGet, "/api/v1/device/"+url.PathEscape(typed), nil, cookie(h.token))
	if shown.status != http.StatusOK || shown.body["application"] != "Halo CLI" || shown.body["userCode"] != userCode || shown.body["ip"] != "127.0.0.1" || shown.body["userAgent"] != "Halo CLI (linux/amd64)" {
		t.Fatalf("lookup: %d %v", shown.status, shown.body)
	}
	if r := h.send(http.MethodPost, "/api/v1/device/"+userCode+"/approve", nil, cookie(h.token), func(r *http.Request) { r.Header.Set("Origin", "https://evil.example.com") }); r.status != http.StatusForbidden {
		t.Fatalf("cross-site approval: %d %v", r.status, r.body)
	}
	if r := h.send(http.MethodPost, "/api/v1/device/"+userCode+"/approve", nil, cookie(h.token)); r.status != http.StatusOK || r.body["approved"] != true {
		t.Fatalf("approve: %d %v", r.status, r.body)
	}
	if r := h.send(http.MethodPost, "/api/v1/device/"+userCode+"/deny", nil, cookie(h.token)); r.status != http.StatusNotFound {
		t.Fatalf("deciding twice: %d %v", r.status, r.body)
	}

	tokens := h.poll(deviceCode)
	access, _ := tokens.body["access_token"].(string)
	refresh, _ := tokens.body["refresh_token"].(string)
	if tokens.status != http.StatusOK || strings.Count(access, ".") != 2 || refresh == "" || tokens.body["id_token"] == nil {
		t.Fatalf("token after approval: %d %v", tokens.status, tokens.body)
	}
	if r := h.poll(deviceCode); r.status != http.StatusBadRequest || r.body["error"] != "access_denied" {
		t.Fatalf("device code reuse: %d %v", r.status, r.body)
	}
	events, err := h.st.ListSignIns(ctx, store.SignInFilter{UserID: h.user.ID})
	h.must(err)
	if len(events) != 1 || events[0].Result != "success" || events[0].Method != "totp" {
		t.Fatalf("sign-in events: %+v", events)
	}

	if r := h.send(http.MethodGet, "/oauth2/userinfo", nil, bearerToken(access)); r.status != http.StatusOK || r.body["sub"] != h.user.ID || r.body["email"] != h.user.Email {
		t.Fatalf("userinfo for an unassigned user through the CLI: %d %v", r.status, r.body)
	}
	if r := h.send(http.MethodGet, "/api/v1/me/probe", nil, bearerToken(access)); r.status != http.StatusOK || r.body["id"] != h.user.ID {
		t.Fatalf("CLI token on /api/v1/me: %d %v", r.status, r.body)
	}
	resolve := halooidc.AccessTokenResolver(h.st)
	if _, err := resolve(ctx, access[:len(access)-4]+"AAAA"); err == nil {
		t.Fatal("a token with a broken signature must be refused")
	}

	refreshed := h.send(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {halooidc.CLIClientID}})
	next, _ := refreshed.body["access_token"].(string)
	if refreshed.status != http.StatusOK || next == "" {
		t.Fatalf("refresh: %d %v", refreshed.status, refreshed.body)
	}
	if r := h.send(http.MethodPost, "/oauth2/revoke", url.Values{"token": {refreshed.body["refresh_token"].(string)}, "token_type_hint": {"refresh_token"}, "client_id": {halooidc.CLIClientID}}); r.status != http.StatusOK {
		t.Fatalf("revoke: %d %v", r.status, r.body)
	}
	if r := h.send(http.MethodGet, "/api/v1/me/probe", nil, bearerToken(next)); r.status != http.StatusUnauthorized {
		t.Fatalf("revoked CLI token on /api/v1/me: %d %v", r.status, r.body)
	}

	deniedCode, deniedUser := h.startDevice("openid")
	if r := h.send(http.MethodPost, "/api/v1/device/"+deniedUser+"/deny", nil, cookie(h.token)); r.status != http.StatusOK || r.body["approved"] != false {
		t.Fatalf("deny: %d %v", r.status, r.body)
	}
	if r := h.poll(deniedCode); r.status != http.StatusBadRequest || r.body["error"] != "access_denied" {
		t.Fatalf("poll after deny: %d %v", r.status, r.body)
	}

	expired := "expired-device-code"
	h.must(h.st.CreateDeviceAuthorization(ctx, secret.Hash(expired), store.DeviceAuthorization{UserCode: "BBBB-CCCC", ClientID: halooidc.CLIClientID, Scopes: []string{"openid"}, ExpiresAt: time.Now().Add(-time.Second)}))
	if r := h.poll(expired); r.status != http.StatusBadRequest || r.body["error"] != "expired_token" {
		t.Fatalf("poll an expired code: %d %v", r.status, r.body)
	}
	if r := h.send(http.MethodPost, "/api/v1/device/BBBB-CCCC/approve", nil, cookie(h.token)); r.status != http.StatusNotFound || r.body["error"].(map[string]any)["code"] != "ERR_DEVICE_CODE" {
		t.Fatalf("approve an expired code: %d %v", r.status, r.body)
	}

	cli, err := h.st.GetApplicationByClientID(ctx, halooidc.CLIClientID)
	h.must(err)
	if cli.Type != "native" || cli.Status != "active" {
		t.Fatalf("built-in CLI application: %+v", cli)
	}
}

func TestDeviceApprovalPolicy(t *testing.T) {
	h := newInfra(t, policyFunc(func(in access.Input) access.Decision {
		if in.App != nil && in.App.ClientID == halooidc.CLIClientID {
			return access.Decision{Effect: access.RequirePhishingResistant, Policy: "Strong CLI sign-in", Reason: "Requires a passkey."}
		}
		return access.Decision{Effect: access.Allow}
	}))
	_, userCode := h.startDevice("openid")
	if r := h.send(http.MethodPost, "/api/v1/device/"+userCode+"/approve", nil, cookie(h.token)); r.status != http.StatusForbidden || r.body["error"].(map[string]any)["code"] != "ERR_STRONGER_AUTH_REQUIRED" {
		t.Fatalf("totp session against a passkey policy: %d %v", r.status, r.body)
	}
	if r := h.send(http.MethodPost, "/api/v1/device/"+userCode+"/approve", nil, cookie(h.admin)); r.status != http.StatusOK {
		t.Fatalf("passkey session: %d %v", r.status, r.body)
	}
	events, err := h.st.ListSignIns(context.Background(), store.SignInFilter{UserID: h.user.ID})
	h.must(err)
	if len(events) != 1 || events[0].Result != "failure" || !strings.Contains(events[0].Reason, "Strong CLI sign-in") {
		t.Fatalf("refusal event: %+v", events)
	}
}

func TestDeviceApprovalContext(t *testing.T) {
	h := newInfra(t, policyFunc(func(access.Input) access.Decision {
		return access.Decision{Effect: access.RequireContext, Contexts: []store.AuthContext{{ID: "passkey", Name: "Passkey", Methods: []string{"passkey"}}}}
	}))
	_, userCode := h.startDevice("openid")
	if r := h.send(http.MethodPost, "/api/v1/device/"+userCode+"/approve", nil, cookie(h.token)); r.status != http.StatusForbidden || r.body["error"].(map[string]any)["code"] != "ERR_STEP_UP_REQUIRED" {
		t.Fatalf("totp session against a passkey context: %d %v", r.status, r.body)
	}
	if r := h.send(http.MethodPost, "/api/v1/device/"+userCode+"/approve", nil, cookie(h.admin)); r.status != http.StatusOK {
		t.Fatalf("passkey session: %d %v", r.status, r.body)
	}
}

func TestDeviceSessionLimit(t *testing.T) {
	h := newInfra(t, nil)
	ctx := context.Background()
	org := func(change func(*store.Settings)) {
		s, err := h.st.Settings(ctx)
		h.must(err)
		change(&s)
		h.must(h.st.SaveSettings(ctx, s))
		h.st.ForgetSettings()
	}
	approve := func(userCode string) result {
		return h.send(http.MethodPost, "/api/v1/device/"+userCode+"/approve", nil, cookie(h.token))
	}
	login := func() {
		deviceCode, userCode := h.startDevice("openid offline_access")
		if r := approve(userCode); r.status != http.StatusOK {
			t.Fatalf("approve: %d %v", r.status, r.body)
		}
		if r := h.poll(deviceCode); r.status != http.StatusOK {
			t.Fatalf("token: %d %v", r.status, r.body)
		}
	}

	org(func(s *store.Settings) { s.SessionLimit = 1 })
	login()
	login()

	org(func(s *store.Settings) { s.SessionLimitCLI = true })
	deviceCode, userCode := h.startDevice("openid offline_access")
	if r := approve(userCode); r.status != http.StatusForbidden || r.body["error"].(map[string]any)["code"] != "ERR_SESSION_LIMIT" {
		t.Fatalf("approve over the limit: %d %v", r.status, r.body)
	}
	org(func(s *store.Settings) { s.SessionLimitAction = "end-oldest" })
	if r := approve(userCode); r.status != http.StatusOK {
		t.Fatalf("approve after ending the oldest: %d %v", r.status, r.body)
	}
	if r := h.poll(deviceCode); r.status != http.StatusOK {
		t.Fatalf("token: %d %v", r.status, r.body)
	}

	sessions, err := h.st.ListSessions(ctx, h.user.ID)
	h.must(err)
	clients := slices.DeleteFunc(slices.Clone(sessions), func(s store.Session) bool { return s.Client == "" })
	if len(sessions) != 2 || len(clients) != 1 {
		t.Fatalf("sessions: %+v", sessions)
	}
	events, err := h.st.ListSignIns(ctx, store.SignInFilter{UserID: h.user.ID})
	h.must(err)
	results := []string{}
	for _, e := range slices.Backward(events) {
		results = append(results, e.Result+": "+e.Reason)
	}
	want := []string{"success: ", "success: ", "failure: " + store.SessionLimitReason, "ended: " + store.SessionEndedReason, "ended: " + store.SessionEndedReason, "success: "}
	if !slices.Equal(results, want) {
		t.Fatalf("sign-in log: %v", results)
	}
}

func TestDeviceApprovalTerms(t *testing.T) {
	h := newInfra(t, policyFunc(func(access.Input) access.Decision {
		return access.Decision{Effect: access.Allow, Terms: []store.Terms{{ID: "tou_1", Name: "Acceptable use", Version: 1}}}
	}))
	_, userCode := h.startDevice("openid")
	if r := h.send(http.MethodPost, "/api/v1/device/"+userCode+"/approve", nil, cookie(h.admin)); r.status != http.StatusForbidden || r.body["error"].(map[string]any)["code"] != "ERR_TERMS_REQUIRED" {
		t.Fatalf("approve before accepting the terms: %d %v", r.status, r.body)
	}
}

func TestDeviceSignInFrequency(t *testing.T) {
	h := newInfra(t, nil)
	ctx := context.Background()
	_, refresh := h.signInCLI()
	time.Sleep(1100 * time.Millisecond)
	cli, err := h.st.GetApplicationByClientID(ctx, halooidc.CLIClientID)
	h.must(err)
	cli.MaxAuthAge = 1
	h.must(h.st.UpdateApplication(ctx, cli))

	if r := h.refresh(refresh); r.status != http.StatusBadRequest || r.body["error"] != "invalid_grant" {
		t.Fatalf("refresh past the sign-in frequency: %d %v", r.status, r.body)
	}
	_, userCode := h.startDevice("openid")
	if r := h.send(http.MethodPost, "/api/v1/device/"+userCode+"/approve", nil, cookie(h.token)); r.status != http.StatusUnauthorized || r.body["error"].(map[string]any)["code"] != "ERR_REAUTH_REQUIRED" {
		t.Fatalf("approve with a session past the sign-in frequency: %d %v", r.status, r.body)
	}
	_, fresh, err := h.st.CreateSession(ctx, store.NewSession{UserID: h.user.ID, Method: "totp"})
	h.must(err)
	if r := h.send(http.MethodPost, "/api/v1/device/"+userCode+"/approve", nil, cookie(fresh)); r.status != http.StatusOK {
		t.Fatalf("approve after signing in again: %d %v", r.status, r.body)
	}
}

func TestAPIResources(t *testing.T) {
	h := newInfra(t, nil)
	ctx := context.Background()
	group, err := h.st.CreateGroup(ctx, store.NewGroup{Name: "Finance", Kind: "assigned"})
	h.must(err)
	h.must(h.st.AddGroupMember(ctx, group.ID, h.user.ID))
	const redirect = "https://invoices.example.com/callback"
	web, err := h.st.CreateApplication(ctx, store.NewApplication{Name: "Invoices", Protocol: "oidc", Type: "web", RedirectURIs: []string{redirect}, GroupIDs: []string{group.ID}, Scopes: []string{"openid", "profile", "email", "groups", "offline_access"}})
	h.must(err)
	_, webSecret, err := h.st.AddClientSecret(ctx, web.ID, "Test", time.Hour)
	h.must(err)
	worker, err := h.st.CreateApplication(ctx, store.NewApplication{Name: "Invoice worker", Protocol: "oauth", Type: "service", Scopes: []string{"jobs:run"}})
	h.must(err)
	_, workerSecret, err := h.st.AddClientSecret(ctx, worker.ID, "Test", time.Hour)
	h.must(err)

	billing := map[string]any{"name": "Billing API", "identifier": "https://billing.example.com", "accessTokenTtl": 300,
		"scopes": []map[string]string{{"name": "billing:read", "description": "Read invoices."}, {"name": "billing:write", "description": "Void invoices."}}}
	if r := h.send(http.MethodPost, "/api/v1/api-resources", billing); r.status != http.StatusUnauthorized {
		t.Fatalf("create without a session: %d", r.status)
	}
	if r := h.send(http.MethodPost, "/api/v1/api-resources", billing, cookie(h.token)); r.status != http.StatusForbidden {
		t.Fatalf("create as a user without a role: %d", r.status)
	}
	created := h.send(http.MethodPost, "/api/v1/api-resources", billing, cookie(h.admin))
	resourceID, _ := created.body["id"].(string)
	if created.status != http.StatusCreated || resourceID == "" || created.body["accessTokenAlg"] != "RS256" {
		t.Fatalf("create: %d %v", created.status, created.body)
	}
	for _, bad := range []map[string]any{
		{"name": "Reports", "identifier": "https://reports.example.com", "accessTokenTtl": 300, "scopes": []map[string]string{{"name": "billing:read"}}},
		{"name": "Reports", "identifier": "https://billing.example.com", "accessTokenTtl": 300, "scopes": []map[string]string{{"name": "reports:read"}}},
	} {
		if r := h.send(http.MethodPost, "/api/v1/api-resources", bad, cookie(h.admin)); r.status != http.StatusConflict {
			t.Fatalf("duplicate scope or identifier: %d %v", r.status, r.body)
		}
	}
	if r := h.send(http.MethodPost, "/api/v1/api-resources", map[string]any{"name": "Reports", "identifier": "https://reports.example.com", "accessTokenTtl": 300, "scopes": []map[string]string{{"name": "openid"}}}, cookie(h.admin)); r.status != http.StatusUnprocessableEntity {
		t.Fatalf("reserved scope: %d %v", r.status, r.body)
	}
	billing["accessTokenAlg"] = "ES256"
	if r := h.send(http.MethodPut, "/api/v1/api-resources/"+resourceID, billing, cookie(h.admin)); r.status != http.StatusOK || r.body["accessTokenAlg"] != "ES256" {
		t.Fatalf("switch to ES256: %d %v", r.status, r.body)
	}
	delete(billing, "accessTokenAlg")
	if r := h.send(http.MethodPut, "/api/v1/api-resources/"+resourceID, billing, cookie(h.admin)); r.status != http.StatusOK || r.body["accessTokenAlg"] != "ES256" {
		t.Fatalf("an update without accessTokenAlg keeps the algorithm: %d %v", r.status, r.body)
	}
	billing["accessTokenAlg"] = "RS256"
	h.send(http.MethodPut, "/api/v1/api-resources/"+resourceID, billing, cookie(h.admin))
	if r := h.send(http.MethodPost, "/api/v1/api-resources", map[string]any{"name": "Reports", "identifier": "https://reports.example.com", "accessTokenTtl": 300, "accessTokenAlg": "HS256", "scopes": []map[string]string{{"name": "reports:read"}}}, cookie(h.admin)); r.status != http.StatusUnprocessableEntity {
		t.Fatalf("HS256 access tokens: %d %v", r.status, r.body)
	}
	if r := h.send(http.MethodPut, "/api/v1/api-resources/"+resourceID+"/grants/"+worker.ID, map[string]any{"scopes": []string{"billing:delete"}}, cookie(h.admin)); r.status != http.StatusUnprocessableEntity {
		t.Fatalf("grant an unknown scope: %d %v", r.status, r.body)
	}
	for _, app := range []store.Application{worker, web} {
		if r := h.send(http.MethodPut, "/api/v1/api-resources/"+resourceID+"/grants/"+app.ID, map[string]any{"scopes": []string{"billing:read"}}, cookie(h.admin)); r.status != http.StatusOK {
			t.Fatalf("grant: %d %v", r.status, r.body)
		}
	}

	verifier := op.NewAccessTokenVerifier(h.srv.URL, rp.NewRemoteKeySet(http.DefaultClient, h.srv.URL+"/oauth2/keys"))
	verify := func(token string) *oidc.AccessTokenClaims {
		t.Helper()
		claims, err := op.VerifyAccessToken[*oidc.AccessTokenClaims](ctx, token, verifier)
		h.must(err)
		return claims
	}
	basic := func(id, secret string) func(*http.Request) {
		return func(r *http.Request) { r.SetBasicAuth(id, secret) }
	}

	issuedBefore := metrics.TokensIssued.Value("client_credentials")
	cc := h.send(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {"client_credentials"}, "scope": {"billing:read billing:write jobs:run"}}, basic(worker.ClientID, workerSecret))
	if got := metrics.TokensIssued.Value("client_credentials"); got != issuedBefore+1 {
		t.Fatalf("client credentials token counter went from %v to %v", issuedBefore, got)
	}
	claims := verify(cc.body["access_token"].(string))
	if cc.status != http.StatusOK || !slices.Equal(claims.Audience, []string{"https://billing.example.com"}) || claims.Claims["scope"] != "billing:read jobs:run" || claims.ClientID != worker.ClientID {
		t.Fatalf("client credentials for a granted scope: %d %v %+v", cc.status, cc.body, claims)
	}
	if lifetime := claims.Expiration.AsTime().Sub(claims.IssuedAt.AsTime()); lifetime > 300*time.Second {
		t.Fatalf("resource access token lifetime %s, want at most 5 minutes", lifetime)
	}
	own := h.send(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {"client_credentials"}, "scope": {"billing:write jobs:run"}}, basic(worker.ClientID, workerSecret))
	if claims := verify(own.body["access_token"].(string)); !slices.Equal(claims.Audience, []string{worker.ClientID}) || claims.Claims["scope"] != "jobs:run" {
		t.Fatalf("ungranted scope must not reach the API: %+v", claims)
	}
	byResource := h.send(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {"client_credentials"}, "resource": {"https://billing.example.com"}}, basic(worker.ClientID, workerSecret))
	if claims := verify(byResource.body["access_token"].(string)); !slices.Equal(claims.Audience, []string{"https://billing.example.com"}) || claims.Claims["scope"] != "billing:read" {
		t.Fatalf("resource parameter: %+v", claims)
	}
	if r := h.send(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {"client_credentials"}, "resource": {"https://payroll.example.com"}}, basic(worker.ClientID, workerSecret)); r.status != http.StatusBadRequest || r.body["error"] != "invalid_target" {
		t.Fatalf("ungranted resource: %d %v", r.status, r.body)
	}

	const codeVerifier = "Zs8rHNw0qX3cE1vYp7LkT2mB9dF4gJ6hA5uW0oQiRtUe"
	q := url.Values{"client_id": {web.ClientID}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {"openid email billing:read billing:write"}, "state": {"s"}, "nonce": {"n"},
		"code_challenge": {oidc.NewSHACodeChallenge(codeVerifier)}, "code_challenge_method": {"S256"}}
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noFollow.Get(h.srv.URL + "/oauth2/authorize?" + q.Encode())
	h.must(err)
	resp.Body.Close()
	requestID := strings.TrimPrefix(resp.Header.Get("Location"), "/sign-in?authRequest=")
	sess, sessionToken, err := h.st.CreateSession(ctx, store.NewSession{UserID: h.user.ID, Method: "passkey"})
	h.must(err)
	h.must(h.st.CompleteAuthRequest(ctx, requestID, h.user.ID, sess.ID, []string{"hwk", "mfa"}, time.Now(), ""))
	req, err := http.NewRequest(http.MethodGet, h.srv.URL+store.AuthorizeCallbackPath(requestID), nil)
	h.must(err)
	cookie(sessionToken)(req)
	resp, err = noFollow.Do(req)
	h.must(err)
	resp.Body.Close()
	location, err := url.Parse(resp.Header.Get("Location"))
	h.must(err)
	exchanged := h.send(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {"authorization_code"}, "code": {location.Query().Get("code")}, "redirect_uri": {redirect}, "code_verifier": {codeVerifier}}, basic(web.ClientID, webSecret))
	claims = verify(exchanged.body["access_token"].(string))
	if exchanged.status != http.StatusOK || claims.Subject != h.user.ID || !slices.Equal(claims.Audience, []string{"https://billing.example.com"}) || !strings.Contains(claims.Claims["scope"].(string), "billing:read") || strings.Contains(claims.Claims["scope"].(string), "billing:write") {
		t.Fatalf("authorization code with a granted resource scope: %d %v %+v", exchanged.status, exchanged.body, claims)
	}
	if r := h.send(http.MethodGet, "/oauth2/userinfo", nil, bearerToken(exchanged.body["access_token"].(string))); r.status != http.StatusOK || r.body["sub"] != h.user.ID {
		t.Fatalf("userinfo with a resource token: %d %v", r.status, r.body)
	}
	if r := h.send(http.MethodGet, "/api/v1/me/probe", nil, bearerToken(exchanged.body["access_token"].(string))); r.status != http.StatusUnauthorized {
		t.Fatalf("a token for another application must not reach /api/v1/me: %d %v", r.status, r.body)
	}

	if r := h.send(http.MethodDelete, "/api/v1/api-resources/"+resourceID, nil, cookie(h.admin)); r.status != http.StatusNoContent {
		t.Fatalf("delete: %d %v", r.status, r.body)
	}
	after := h.send(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {"client_credentials"}, "scope": {"billing:read jobs:run"}}, basic(worker.ClientID, workerSecret))
	if claims := verify(after.body["access_token"].(string)); !slices.Equal(claims.Audience, []string{worker.ClientID}) || claims.Claims["scope"] != "jobs:run" {
		t.Fatalf("after deleting the API: %+v", claims)
	}
}

func TestCLIClientRefusesAuthorizationCode(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	cli, err := h.st.GetApplicationByClientID(ctx, halooidc.CLIClientID)
	if err != nil {
		t.Fatal(err)
	}
	cli.RedirectURIs = []string{callback}
	if err := h.st.UpdateApplication(ctx, cli); err != nil {
		t.Fatal(err)
	}
	_, session := h.session(h.user, "passkey")
	q := url.Values{"client_id": {halooidc.CLIClientID}, "redirect_uri": {callback}, "response_type": {"code"}, "scope": {"openid offline_access"}, "state": {"st-1"},
		"code_challenge": {oidc.NewSHACodeChallenge(pkceSecret)}, "code_challenge_method": {"S256"}}
	status, location := h.get("/oauth2/authorize?"+q.Encode(), session)
	if location != nil && location.Path == "/oauth2/authorize/callback" {
		status, location = h.get(location.String(), session)
	}
	if status != http.StatusFound || location.Query().Get("code") != "" || location.Query().Get("error") != "unauthorized_client" {
		t.Fatalf("the CLI client must not take authorization requests: %d %v", status, location)
	}
}
