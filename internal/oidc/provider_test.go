package oidc_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
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

	"halo/internal/config"
	"halo/internal/httpx"
	halooidc "halo/internal/oidc"
	"halo/internal/store"
	"halo/internal/testdb"
)

type result struct {
	status   int
	location string
	body     map[string]any
}

func TestAuthorizationCodeFlow(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	group, err := st.CreateGroup(ctx, store.NewGroup{Name: "Engineering", Kind: "assigned"})
	must(err)
	user, err := st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active", GroupIDs: []string{group.ID}})
	must(err)
	avatarAt := time.Unix(1790000000, 0)
	must(st.SetAvatarUpdated(ctx, user.ID, &avatarAt))
	must(st.MarkEmailVerified(ctx, user.ID))
	const redirect = "https://grafana.example.com/login/generic_oauth"
	web, err := st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", RedirectURIs: []string{redirect}, GroupIDs: []string{group.ID}})
	must(err)
	_, secret, err := st.AddClientSecret(ctx, web.ID, "Test", time.Hour)
	must(err)

	var handler http.Handler
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
	t.Cleanup(srv.Close)
	issuer, _ := url.Parse(srv.URL)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	authn := &httpx.Auth{Store: st, Public: issuer, Dev: true}
	provider, err := halooidc.New(httpx.Deps{Config: config.Config{PublicURL: issuer, SecretKey: key, Dev: true}, Store: st, Auth: authn})
	must(err)
	mux := http.NewServeMux()
	mux.Handle("/oauth2/", authn.Resolve(provider))
	mux.Handle("/.well-known/", provider)
	handler = mux

	httpClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	send := func(method, path string, form url.Values, auth func(*http.Request)) result {
		t.Helper()
		var body io.Reader
		if form != nil {
			body = strings.NewReader(form.Encode())
		}
		req, err := http.NewRequest(method, srv.URL+path, body)
		must(err)
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		if auth != nil {
			auth(req)
		}
		resp, err := httpClient.Do(req)
		must(err)
		defer resp.Body.Close()
		r := result{status: resp.StatusCode, location: resp.Header.Get("Location")}
		_ = json.NewDecoder(resp.Body).Decode(&r.body)
		return r
	}
	basic := func(id, secret string) func(*http.Request) {
		return func(r *http.Request) { r.SetBasicAuth(id, secret) }
	}
	bearer := func(token string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
	}

	discovery := send(http.MethodGet, "/.well-known/openid-configuration", nil, nil)
	for field, path := range map[string]string{"authorization_endpoint": "/oauth2/authorize", "token_endpoint": "/oauth2/token", "userinfo_endpoint": "/oauth2/userinfo",
		"jwks_uri": "/oauth2/keys", "end_session_endpoint": "/oauth2/logout", "revocation_endpoint": "/oauth2/revoke", "introspection_endpoint": "/oauth2/introspect"} {
		if discovery.body[field] != srv.URL+path {
			t.Errorf("discovery %s = %v, want %s", field, discovery.body[field], srv.URL+path)
		}
	}
	if discovery.body["issuer"] != srv.URL {
		t.Errorf("discovery issuer = %v", discovery.body["issuer"])
	}

	const verifier = "Zs8rHNw0qX3cE1vYp7LkT2mB9dF4gJ6hA5uW0oQiRtUe"
	authorize := func(clientID, redirectURI string, pkce bool) result {
		t.Helper()
		q := url.Values{"client_id": {clientID}, "redirect_uri": {redirectURI}, "response_type": {"code"}, "scope": {"openid email profile groups offline_access"}, "state": {"st-123"}, "nonce": {"n-456"}}
		if pkce {
			q.Set("code_challenge", oidc.NewSHACodeChallenge(verifier))
			q.Set("code_challenge_method", "S256")
		}
		return send(http.MethodGet, "/oauth2/authorize?"+q.Encode(), nil, nil)
	}
	signIn := func(auth result) string {
		t.Helper()
		requestID, ok := strings.CutPrefix(auth.location, "/sign-in?authRequest=")
		if auth.status != http.StatusFound || !ok || !strings.HasPrefix(requestID, "are_") {
			t.Fatalf("authorize: want 302 to /sign-in, got %d %q", auth.status, auth.location)
		}
		sess, token, err := st.CreateSession(ctx, store.NewSession{UserID: user.ID, Method: "passkey"})
		must(err)
		must(st.CompleteAuthRequest(ctx, requestID, user.ID, sess.ID, []string{"hwk", "mfa"}, time.Now()))
		if r := send(http.MethodGet, store.AuthorizeCallbackPath(requestID), nil, nil); r.status != http.StatusForbidden {
			t.Fatalf("callback without the signing-in session: want 403, got %d", r.status)
		}
		cb := send(http.MethodGet, store.AuthorizeCallbackPath(requestID), nil, func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: token})
		})
		loc, err := url.Parse(cb.location)
		must(err)
		if cb.status != http.StatusFound || !strings.HasPrefix(cb.location, redirect+"?") || loc.Query().Get("state") != "st-123" || loc.Query().Get("code") == "" {
			t.Fatalf("callback: got %d %q", cb.status, cb.location)
		}
		return loc.Query().Get("code")
	}
	exchange := func(code, codeVerifier string) result {
		t.Helper()
		form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirect}, "code_verifier": {codeVerifier}}
		return send(http.MethodPost, "/oauth2/token", form, basic(web.ClientID, secret))
	}
	refresh := func(token string) result {
		t.Helper()
		return send(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {token}}, basic(web.ClientID, secret))
	}

	code := signIn(authorize(web.ClientID, redirect, true))
	if r := exchange(code, strings.Repeat("x", 43)); r.status != http.StatusBadRequest || r.body["error"] != "invalid_grant" {
		t.Fatalf("wrong verifier: got %d %v", r.status, r.body)
	}
	tokens := exchange(code, verifier)
	if tokens.status != http.StatusOK {
		t.Fatalf("token exchange: %d %v", tokens.status, tokens.body)
	}
	if r := exchange(code, verifier); r.status != http.StatusBadRequest || r.body["error"] != "invalid_grant" {
		t.Fatalf("code reuse: got %d %v", r.status, r.body)
	}
	accessToken, _ := tokens.body["access_token"].(string)
	refreshToken, _ := tokens.body["refresh_token"].(string)
	idToken, _ := tokens.body["id_token"].(string)
	if refreshToken == "" {
		t.Fatalf("no refresh token: %v", tokens.body)
	}

	idVerifier := rp.NewIDTokenVerifier(srv.URL, web.ClientID, rp.NewRemoteKeySet(http.DefaultClient, srv.URL+"/oauth2/keys"), rp.WithNonce(func(context.Context) string { return "n-456" }))
	claims, err := rp.VerifyTokens[*oidc.IDTokenClaims](ctx, accessToken, idToken, idVerifier)
	must(err)
	if claims.Subject != user.ID || claims.Email != user.Email || !bool(claims.EmailVerified) || claims.Nonce != "n-456" || !slices.Contains(claims.Audience, web.ClientID) ||
		fmt.Sprint(claims.Claims["groups"]) != "[Engineering]" || !slices.Equal(claims.AuthenticationMethodsReferences, []string{"hwk", "mfa"}) {
		t.Fatalf("id token claims: %+v", claims)
	}

	info := send(http.MethodGet, "/oauth2/userinfo", nil, bearer(accessToken))
	if info.status != http.StatusOK || info.body["sub"] != user.ID || info.body["email"] != user.Email || info.body["preferred_username"] != user.Email || info.body["picture"] != srv.URL+"/api/v1/users/"+user.ID+"/avatar?v=1790000000" || fmt.Sprint(info.body["groups"]) != "[Engineering]" {
		t.Fatalf("userinfo: %d %v", info.status, info.body)
	}
	introspection := send(http.MethodPost, "/oauth2/introspect", url.Values{"token": {accessToken}}, basic(web.ClientID, secret))
	if introspection.body["active"] != true || introspection.body["sub"] != user.ID || fmt.Sprint(introspection.body["groups"]) != "[Engineering]" {
		t.Fatalf("introspection: %v", introspection.body)
	}

	refreshed := refresh(refreshToken)
	nextRefresh, _ := refreshed.body["refresh_token"].(string)
	nextAccess, _ := refreshed.body["access_token"].(string)
	if refreshed.status != http.StatusOK || nextRefresh == "" || nextRefresh == refreshToken || refreshed.body["id_token"] == nil {
		t.Fatalf("refresh: %d %v", refreshed.status, refreshed.body)
	}
	if r := refresh(refreshToken); r.status != http.StatusBadRequest {
		t.Fatalf("rotated refresh token must stop working, got %d", r.status)
	}
	if r := send(http.MethodGet, "/oauth2/userinfo", nil, bearer(nextAccess)); r.status != http.StatusOK {
		t.Fatalf("userinfo after refresh: %d", r.status)
	}
	if r := send(http.MethodPost, "/oauth2/revoke", url.Values{"token": {nextRefresh}}, basic(web.ClientID, secret)); r.status != http.StatusOK {
		t.Fatalf("revoke: %d %v", r.status, r.body)
	}
	if r := refresh(nextRefresh); r.status != http.StatusBadRequest {
		t.Fatalf("revoked refresh token must not work, got %d", r.status)
	}
	if r := send(http.MethodGet, "/oauth2/userinfo", nil, bearer(nextAccess)); r.status == http.StatusOK {
		t.Fatal("access token issued from a revoked refresh token must not work")
	}

	if r := authorize(web.ClientID, "https://evil.example.com/callback", true); r.status != http.StatusBadRequest || r.location != "" {
		t.Fatalf("unregistered redirect uri: %d %q", r.status, r.location)
	}
	disabled, err := st.CreateApplication(ctx, store.NewApplication{Name: "Wiki", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://wiki.example.com/cb"}, GroupIDs: []string{group.ID}})
	must(err)
	must(st.SetApplicationStatus(ctx, disabled.ID, "disabled"))
	if r := authorize(disabled.ClientID, "https://wiki.example.com/cb", true); r.status != http.StatusBadRequest || r.location != "" {
		t.Fatalf("disabled app: %d %q", r.status, r.location)
	}
	spa, err := st.CreateApplication(ctx, store.NewApplication{Name: "Dashboard", Protocol: "oidc", Type: "spa", RedirectURIs: []string{"https://dash.example.com/cb"}, GroupIDs: []string{group.ID}})
	must(err)
	if r := authorize(spa.ClientID, "https://dash.example.com/cb", false); r.status != http.StatusFound || !strings.Contains(r.location, "error=invalid_request") {
		t.Fatalf("public client without pkce: %d %q", r.status, r.location)
	}

	tokens = exchange(signIn(authorize(web.ClientID, redirect, true)), verifier)
	must(st.RemoveGroupMember(ctx, group.ID, user.ID))
	if r := refresh(tokens.body["refresh_token"].(string)); r.status != http.StatusBadRequest || r.body["error"] != "invalid_grant" {
		t.Fatalf("unassigned user refresh: %d %v", r.status, r.body)
	}

	service, err := st.CreateApplication(ctx, store.NewApplication{Name: "Billing sync", Protocol: "oidc", Type: "service", Scopes: []string{"billing.read"}})
	must(err)
	_, serviceSecret, err := st.AddClientSecret(ctx, service.ID, "Test", time.Hour)
	must(err)
	cc := send(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {"client_credentials"}, "scope": {"billing.read billing.write"}}, basic(service.ClientID, serviceSecret))
	jwt, _ := cc.body["access_token"].(string)
	if cc.status != http.StatusOK || strings.Count(jwt, ".") != 2 || cc.body["scope"] != "billing.read" {
		t.Fatalf("client credentials: %d %v", cc.status, cc.body)
	}
	if r := send(http.MethodPost, "/oauth2/introspect", url.Values{"token": {jwt}}, basic(service.ClientID, serviceSecret)); r.body["active"] != true || r.body["sub"] != service.ClientID {
		t.Fatalf("service token introspection: %v", r.body)
	}
}
