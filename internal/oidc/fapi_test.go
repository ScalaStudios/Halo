package oidc_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	halooidc "halo/internal/oidc"
	"halo/internal/secret"
	"halo/internal/store"
)

type dpopSigner struct {
	t   *testing.T
	key *ecdsa.PrivateKey
}

func newDPoPSigner(t *testing.T) dpopSigner {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return dpopSigner{t, key}
}

func (d dpopSigner) thumbprint() string {
	jkt, err := oidc.JWKThumbprint(&jose.JSONWebKey{Key: d.key.Public()})
	if err != nil {
		d.t.Fatal(err)
	}
	return jkt
}

func (d dpopSigner) proof(method, uri, accessToken string, edit func(map[string]any)) string {
	claims := map[string]any{"jti": secret.Token(12), "htm": method, "htu": uri, "iat": time.Now().Unix()}
	if accessToken != "" {
		hash := sha256.Sum256([]byte(accessToken))
		claims["ath"] = base64.RawURLEncoding.EncodeToString(hash[:])
	}
	typ := "dpop+jwt"
	if edit != nil {
		edit(claims)
		typ, _ = claims["typ"].(string)
		delete(claims, "typ")
	}
	if typ == "" {
		typ = "dpop+jwt"
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: d.key}, (&jose.SignerOptions{EmbedJWK: true}).WithType(jose.ContentType(typ)))
	if err != nil {
		d.t.Fatal(err)
	}
	token, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		d.t.Fatal(err)
	}
	return token
}

func header(name string, values ...string) func(*http.Request) {
	return func(r *http.Request) { r.Header[name] = values }
}

func basicAuth(clientID, secret string) func(*http.Request) {
	return func(r *http.Request) { r.SetBasicAuth(clientID, secret) }
}

func (h *harness) do(method, path string, form url.Values, opts ...func(*http.Request)) (result, http.Header) {
	h.t.Helper()
	req, _ := http.NewRequest(method, h.srv.URL+path, nil)
	if form != nil {
		req, _ = http.NewRequest(method, h.srv.URL+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, opt := range opts {
		opt(req)
	}
	res, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	r := result{status: res.StatusCode, location: res.Header.Get("Location")}
	_ = json.NewDecoder(res.Body).Decode(&r.body)
	return r, res.Header
}

func (h *harness) code(authorize result, sessionToken string) string {
	h.t.Helper()
	requestID, ok := strings.CutPrefix(authorize.location, "/sign-in?authRequest=")
	if authorize.status != http.StatusFound || !ok {
		h.t.Fatalf("authorize: want 302 to /sign-in, got %d %q %v", authorize.status, authorize.location, authorize.body)
	}
	sess, err := h.st.SessionByToken(context.Background(), sessionToken)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := h.st.CompleteAuthRequest(context.Background(), requestID, h.user.ID, sess.ID, []string{"hwk"}, time.Now(), ""); err != nil {
		h.t.Fatal(err)
	}
	cb, _ := h.do(http.MethodGet, store.AuthorizeCallbackPath(requestID), nil, cookie(sessionToken))
	location, _ := url.Parse(cb.location)
	if cb.status != http.StatusFound || !strings.HasPrefix(cb.location, callback+"?") || location.Query().Get("code") == "" || location.Query().Get("state") != "st-fapi" {
		h.t.Fatalf("callback: %d %q", cb.status, cb.location)
	}
	return location.Query().Get("code")
}

func authorization(clientID string, pkce bool) url.Values {
	q := url.Values{"client_id": {clientID}, "redirect_uri": {callback}, "response_type": {"code"}, "scope": {"openid email offline_access"}, "state": {"st-fapi"}}
	if pkce {
		q.Set("code_challenge", oidc.NewSHACodeChallenge(pkceSecret))
		q.Set("code_challenge_method", "S256")
	}
	return q
}

func payload(t *testing.T, token string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %q", token)
	}
	data, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	if err := json.Unmarshal(data, &claims); err != nil {
		t.Fatal(err)
	}
	return claims
}

func TestPushedAuthorizationRequests(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	_, session := h.session(h.user, "passkey")

	discovery, _ := h.do(http.MethodGet, "/.well-known/openid-configuration", nil)
	if discovery.body["pushed_authorization_request_endpoint"] != h.srv.URL+"/oauth2/par" || !slices.Contains(discovery.body["dpop_signing_alg_values_supported"].([]any), any("ES256")) {
		t.Fatalf("discovery: %v", discovery.body)
	}

	params := authorization(h.app.ClientID, true)
	for name, c := range map[string]struct {
		form   url.Values
		opts   []func(*http.Request)
		status int
		err    string
	}{
		"no client authentication": {params, nil, http.StatusUnauthorized, "invalid_client"},
		"wrong client secret":      {params, []func(*http.Request){basicAuth(h.app.ClientID, "wrong")}, http.StatusBadRequest, "unauthorized_client"},
		"request_uri in the body":  {url.Values{"request_uri": {"urn:x"}, "client_id": {h.app.ClientID}}, []func(*http.Request){basicAuth(h.app.ClientID, h.secret)}, http.StatusBadRequest, "invalid_request"},
		"unregistered redirect":    {url.Values{"redirect_uri": {"https://evil.example.com/cb"}, "response_type": {"code"}, "scope": {"openid"}}, []func(*http.Request){basicAuth(h.app.ClientID, h.secret)}, http.StatusBadRequest, "invalid_request"},
		"another client_id":        {url.Values{"client_id": {"hl_other"}, "redirect_uri": {callback}, "response_type": {"code"}, "scope": {"openid"}}, []func(*http.Request){basicAuth(h.app.ClientID, h.secret)}, http.StatusBadRequest, "invalid_request"},
		"oversized body":           {url.Values{"client_id": {h.app.ClientID}, "login_hint": {strings.Repeat("a", 70<<10)}}, nil, http.StatusBadRequest, "invalid_request"},
	} {
		if r, _ := h.do(http.MethodPost, "/oauth2/par", c.form, c.opts...); r.status != c.status || r.body["error"] != c.err {
			t.Errorf("%s: got %d %v", name, r.status, r.body)
		}
	}

	pushed, _ := h.do(http.MethodPost, "/oauth2/par", params, basicAuth(h.app.ClientID, h.secret))
	requestURI, _ := pushed.body["request_uri"].(string)
	if pushed.status != http.StatusCreated || !strings.HasPrefix(requestURI, "urn:ietf:params:oauth:request_uri:") || pushed.body["expires_in"] != float64(60) {
		t.Fatalf("push: %d %v", pushed.status, pushed.body)
	}
	if r, _ := h.do(http.MethodGet, "/oauth2/authorize?"+url.Values{"client_id": {"hl_other"}, "request_uri": {requestURI}}.Encode(), nil); r.status != http.StatusBadRequest || r.location != "" {
		t.Fatalf("request_uri with another client_id: %d %q", r.status, r.location)
	}
	ignored := url.Values{"client_id": {h.app.ClientID}, "request_uri": {requestURI}, "redirect_uri": {"https://evil.example.com/cb"}, "state": {"forged"}}
	authorize, _ := h.do(http.MethodGet, "/oauth2/authorize?"+ignored.Encode(), nil)
	code := h.code(authorize, session)
	if r := h.post("/oauth2/token", url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callback}, "code_verifier": {pkceSecret}}, h.app.ClientID, h.secret); r.status != http.StatusOK || r.body["token_type"] != "Bearer" {
		t.Fatalf("token exchange after a pushed request: %d %v", r.status, r.body)
	}
	if r, _ := h.do(http.MethodGet, "/oauth2/authorize?"+ignored.Encode(), nil); r.status != http.StatusBadRequest || r.location != "" {
		t.Fatalf("reused request_uri: %d %q", r.status, r.location)
	}

	form := url.Values{"client_id": {h.app.ClientID}, "client_secret": {h.secret}}
	for k, v := range params {
		form[k] = v
	}
	store.PushedRequestLifetime = -time.Second
	expired, _ := h.do(http.MethodPost, "/oauth2/par", form)
	store.PushedRequestLifetime = time.Minute
	if expired.status != http.StatusCreated {
		t.Fatalf("push with client_secret_post: %d %v", expired.status, expired.body)
	}
	if r, _ := h.do(http.MethodGet, "/oauth2/authorize?"+url.Values{"client_id": {h.app.ClientID}, "request_uri": {expired.body["request_uri"].(string)}}.Encode(), nil); r.status != http.StatusBadRequest || r.location != "" {
		t.Fatalf("expired request_uri: %d %q", r.status, r.location)
	}

	spa, err := h.st.CreateApplication(ctx, store.NewApplication{Name: "Dashboard", Protocol: "oidc", Type: "spa", RedirectURIs: []string{callback}, GroupIDs: []string{h.group.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := h.do(http.MethodPost, "/oauth2/par", authorization(spa.ClientID, false)); r.status != http.StatusBadRequest || r.body["error"] != "invalid_request" {
		t.Fatalf("public client pushing without PKCE: %d %v", r.status, r.body)
	}
	if r, _ := h.do(http.MethodPost, "/oauth2/par", authorization(spa.ClientID, true)); r.status != http.StatusCreated {
		t.Fatalf("public client pushing with PKCE: %d %v", r.status, r.body)
	}

	for _, path := range []string{"/oauth2/par", "/oauth2/token"} {
		_, headers := h.do(http.MethodOptions, path, nil, header("Origin", "https://dash.example.com"), header("Access-Control-Request-Method", "POST"), header("Access-Control-Request-Headers", "content-type,dpop"))
		if headers.Get("Access-Control-Allow-Origin") != "https://dash.example.com" || !strings.Contains(strings.ToLower(headers.Get("Access-Control-Allow-Headers")), "dpop") {
			t.Errorf("preflight %s: %v", path, headers)
		}
	}
	if _, headers := h.do(http.MethodPost, "/oauth2/par", authorization(spa.ClientID, true), header("Origin", "https://dash.example.com")); headers.Get("Access-Control-Allow-Origin") != "https://dash.example.com" {
		t.Errorf("pushed request from a browser: %v", headers)
	}
}

func TestClientPolicies(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	_, session := h.session(h.user, "passkey")

	if r, _ := h.do(http.MethodGet, "/oauth2/authorize?"+authorization(h.app.ClientID, false).Encode(), nil); r.status != http.StatusFound || !strings.HasPrefix(r.location, "/sign-in?authRequest=") {
		t.Fatalf("confidential client without PKCE before any requirement: %d %q", r.status, r.location)
	}

	strict := h.app
	strict.RequirePAR, strict.RequirePKCE = true, true
	if err := h.st.UpdateApplication(ctx, strict); err != nil {
		t.Fatal(err)
	}
	if r, _ := h.do(http.MethodGet, "/oauth2/authorize?"+authorization(h.app.ClientID, true).Encode(), nil); r.status != http.StatusFound || !strings.HasPrefix(r.location, callback+"?") || !strings.Contains(r.location, "error=invalid_request") {
		t.Fatalf("authorization request without PAR: %d %q", r.status, r.location)
	}
	if r, _ := h.do(http.MethodPost, "/oauth2/par", authorization(h.app.ClientID, false), basicAuth(h.app.ClientID, h.secret)); r.status != http.StatusBadRequest || r.body["error"] != "invalid_request" {
		t.Fatalf("pushed request without PKCE: %d %v", r.status, r.body)
	}
	pushed, _ := h.do(http.MethodPost, "/oauth2/par", authorization(h.app.ClientID, true), basicAuth(h.app.ClientID, h.secret))
	authorize, _ := h.do(http.MethodGet, "/oauth2/authorize?"+url.Values{"client_id": {h.app.ClientID}, "request_uri": {pushed.body["request_uri"].(string)}}.Encode(), nil)
	h.code(authorize, session)

	strict.RequirePAR = false
	if err := h.st.UpdateApplication(ctx, strict); err != nil {
		t.Fatal(err)
	}
	if r, _ := h.do(http.MethodGet, "/oauth2/authorize?"+authorization(h.app.ClientID, false).Encode(), nil); r.status != http.StatusFound || !strings.Contains(r.location, "error=invalid_request") {
		t.Fatalf("confidential client without PKCE once required: %d %q", r.status, r.location)
	}

	service, err := h.st.CreateApplication(ctx, store.NewApplication{Name: "Billing sync", Protocol: "oidc", Type: "service", Scopes: []string{"billing.read"}})
	if err != nil {
		t.Fatal(err)
	}
	_, serviceSecret, err := h.st.AddClientSecret(ctx, service.ID, "Test", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	grant := url.Values{"grant_type": {"client_credentials"}, "scope": {"billing.read"}}
	if r := h.post("/oauth2/token", grant, service.ClientID, serviceSecret); r.status != http.StatusOK || r.body["token_type"] != "Bearer" || payload(t, r.body["access_token"].(string))["cnf"] != nil {
		t.Fatalf("client credentials without DPoP: %d %v", r.status, r.body)
	}
	service.RequireDPoP = true
	if err := h.st.UpdateApplication(ctx, service); err != nil {
		t.Fatal(err)
	}
	if r := h.post("/oauth2/token", grant, service.ClientID, serviceSecret); r.status != http.StatusBadRequest || r.body["error"] != "invalid_dpop_proof" {
		t.Fatalf("client credentials without DPoP once required: %d %v", r.status, r.body)
	}
	key := newDPoPSigner(t)
	r, _ := h.do(http.MethodPost, "/oauth2/token", grant, basicAuth(service.ClientID, serviceSecret), header("DPoP", key.proof("POST", h.srv.URL+"/oauth2/token", "", nil)))
	access, _ := r.body["access_token"].(string)
	if r.status != http.StatusOK || r.body["token_type"] != "DPoP" {
		t.Fatalf("client credentials with DPoP: %d %v", r.status, r.body)
	}
	if cnf, _ := payload(t, access)["cnf"].(map[string]any); cnf["jkt"] != key.thumbprint() {
		t.Fatalf("JWT access token cnf: %v", payload(t, access))
	}
}

func TestDPoPBoundTokens(t *testing.T) {
	h := newHarness(t, nil)
	_, session := h.session(h.user, "passkey")
	tokenURL, userinfoURL := h.srv.URL+"/oauth2/token", h.srv.URL+"/oauth2/userinfo"
	key, other := newDPoPSigner(t), newDPoPSigner(t)

	authorize := func() string {
		r, _ := h.do(http.MethodGet, "/oauth2/authorize?"+authorization(h.app.ClientID, true).Encode(), nil)
		return h.code(r, session)
	}
	exchange := func(code string, opts ...func(*http.Request)) result {
		r, _ := h.do(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callback}, "code_verifier": {pkceSecret}}, append(opts, basicAuth(h.app.ClientID, h.secret))...)
		return r
	}
	refresh := func(token string, opts ...func(*http.Request)) result {
		r, _ := h.do(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {token}}, append(opts, basicAuth(h.app.ClientID, h.secret))...)
		return r
	}

	plain := exchange(authorize())
	if plain.status != http.StatusOK || plain.body["token_type"] != "Bearer" || payload(t, plain.body["id_token"].(string))["cnf"] != nil {
		t.Fatalf("token exchange without DPoP: %d %v", plain.status, plain.body)
	}
	if r := h.post("/oauth2/introspect", url.Values{"token": {plain.body["access_token"].(string)}}, h.app.ClientID, h.secret); r.body["active"] != true || r.body["token_type"] != "Bearer" || r.body["cnf"] != nil {
		t.Fatalf("introspection of a bearer token: %v", r.body)
	}
	if r, _ := h.do(http.MethodGet, "/oauth2/userinfo", nil, bearerToken(plain.body["access_token"].(string))); r.status != http.StatusOK {
		t.Fatalf("userinfo with a bearer token: %d", r.status)
	}
	if r := refresh(plain.body["refresh_token"].(string), header("DPoP", key.proof("POST", tokenURL, "", nil))); r.status != http.StatusBadRequest || r.body["error"] != "invalid_grant" || !strings.Contains(r.body["error_description"].(string), "issued without a DPoP proof") {
		t.Fatalf("refresh of an unbound token with a proof: %d %v", r.status, r.body)
	}

	code := authorize()
	for name, proof := range map[string]func(*http.Request){
		"wrong htu":   header("DPoP", key.proof("POST", h.srv.URL+"/oauth2/introspect", "", nil)),
		"wrong htm":   header("DPoP", key.proof("GET", tokenURL, "", nil)),
		"stale iat":   header("DPoP", key.proof("POST", tokenURL, "", func(c map[string]any) { c["iat"] = time.Now().Add(-5 * time.Minute).Unix() })),
		"future iat":  header("DPoP", key.proof("POST", tokenURL, "", func(c map[string]any) { c["iat"] = time.Now().Add(5 * time.Minute).Unix() })),
		"wrong typ":   header("DPoP", key.proof("POST", tokenURL, "", func(c map[string]any) { c["typ"] = "JWT" })),
		"no jti":      header("DPoP", key.proof("POST", tokenURL, "", func(c map[string]any) { delete(c, "jti") })),
		"two proofs":  header("DPoP", key.proof("POST", tokenURL, "", nil), key.proof("POST", tokenURL, "", nil)),
		"not a proof": header("DPoP", "eyJhbGciOiJub25lIn0.e30."),
	} {
		if r := exchange(code, proof); r.status != http.StatusBadRequest || r.body["error"] != "invalid_dpop_proof" {
			t.Errorf("%s: got %d %v", name, r.status, r.body)
		}
	}
	first := key.proof("POST", tokenURL, "", nil)
	bound := exchange(code, header("DPoP", first))
	access, refreshToken := bound.body["access_token"].(string), bound.body["refresh_token"].(string)
	if bound.status != http.StatusOK || bound.body["token_type"] != "DPoP" || payload(t, bound.body["id_token"].(string))["cnf"] != nil {
		t.Fatalf("token exchange with DPoP: %d %v", bound.status, bound.body)
	}
	if r := refresh(refreshToken, header("DPoP", first)); r.status != http.StatusBadRequest || r.body["error"] != "invalid_dpop_proof" {
		t.Fatalf("replayed proof: %d %v", r.status, r.body)
	}

	introspection := h.post("/oauth2/introspect", url.Values{"token": {access}}, h.app.ClientID, h.secret)
	if cnf, _ := introspection.body["cnf"].(map[string]any); introspection.body["active"] != true || introspection.body["token_type"] != "DPoP" || cnf["jkt"] != key.thumbprint() {
		t.Fatalf("introspection of a DPoP token: %v", introspection.body)
	}

	dpop := func(token string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set("Authorization", "DPoP "+token) }
	}
	if r, _ := h.do(http.MethodGet, "/oauth2/userinfo", nil, bearerToken(access)); r.status == http.StatusOK {
		t.Fatal("a DPoP-bound access token must not work as a bearer token")
	}
	if r, _ := h.do(http.MethodPost, "/oauth2/userinfo", url.Values{"access_token": {access}}); r.status == http.StatusOK {
		t.Fatal("a DPoP-bound access token must not work in the form body")
	}
	if r, _ := h.do(http.MethodGet, "/oauth2/userinfo", nil, dpop(access), header("DPoP", other.proof("GET", userinfoURL, access, nil))); r.status == http.StatusOK {
		t.Fatal("a DPoP-bound access token must not work with another key")
	}
	if r, headers := h.do(http.MethodGet, "/oauth2/userinfo", nil, dpop(access), header("DPoP", key.proof("GET", userinfoURL, "", nil))); r.status != http.StatusUnauthorized || r.body["error"] != "invalid_dpop_proof" || !strings.HasPrefix(headers.Get("WWW-Authenticate"), "DPoP") {
		t.Fatalf("userinfo proof without ath: %d %v", r.status, r.body)
	}
	if r, _ := h.do(http.MethodGet, "/oauth2/userinfo", nil, dpop(access), header("DPoP", key.proof("GET", tokenURL, access, nil))); r.status != http.StatusUnauthorized {
		t.Fatalf("userinfo proof for another URL: %d %v", r.status, r.body)
	}
	if r, _ := h.do(http.MethodGet, "/oauth2/userinfo", nil, dpop(plain.body["access_token"].(string)), header("DPoP", key.proof("GET", userinfoURL, plain.body["access_token"].(string), nil))); r.status == http.StatusOK {
		t.Fatal("a bearer token must not be accepted as a DPoP token")
	}
	if r, _ := h.do(http.MethodGet, "/oauth2/userinfo", nil, dpop(access), header("DPoP", key.proof("GET", userinfoURL, access, nil))); r.status != http.StatusOK || r.body["sub"] != h.user.ID {
		t.Fatalf("userinfo with DPoP: %d %v", r.status, r.body)
	}

	if r := refresh(refreshToken); r.status != http.StatusBadRequest || r.body["error"] != "invalid_grant" {
		t.Fatalf("refresh of a bound token without a proof: %d %v", r.status, r.body)
	}
	if r := refresh(refreshToken, header("DPoP", other.proof("POST", tokenURL, "", nil))); r.status != http.StatusBadRequest || r.body["error"] != "invalid_grant" {
		t.Fatalf("refresh of a bound token with another key: %d %v", r.status, r.body)
	}
	refreshed := refresh(refreshToken, header("DPoP", key.proof("POST", tokenURL, "", nil)))
	if refreshed.status != http.StatusOK || refreshed.body["token_type"] != "DPoP" {
		t.Fatalf("refresh with the bound key: %d %v", refreshed.status, refreshed.body)
	}
	if r := refresh(refreshed.body["refresh_token"].(string)); r.status != http.StatusBadRequest {
		t.Fatalf("rotated refresh token must stay bound: %d %v", r.status, r.body)
	}
}

func TestDPoPBoundCLITokensAreRefusedByTheAPI(t *testing.T) {
	h := newInfra(t, nil)
	deviceCode, userCode := h.startDevice("openid profile email offline_access")
	if r := h.send(http.MethodPost, "/api/v1/device/"+userCode+"/approve", nil, cookie(h.token)); r.status != http.StatusOK {
		t.Fatalf("approve: %d %v", r.status, r.body)
	}
	key := newDPoPSigner(t)
	tokens := h.send(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {string(oidc.GrantTypeDeviceCode)}, "device_code": {deviceCode}, "client_id": {halooidc.CLIClientID}}, header("DPoP", key.proof("POST", h.srv.URL+"/oauth2/token", "", nil)))
	access, _ := tokens.body["access_token"].(string)
	if tokens.status != http.StatusOK || tokens.body["token_type"] != "DPoP" {
		t.Fatalf("device tokens with DPoP: %d %v", tokens.status, tokens.body)
	}
	if r := h.send(http.MethodGet, "/api/v1/me/probe", nil, bearerToken(access)); r.status != http.StatusUnauthorized {
		t.Fatalf("a DPoP-bound CLI token used as a bearer token on the API: %d %v", r.status, r.body)
	}
}
