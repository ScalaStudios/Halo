package oidc_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	"halo/internal/access"
	"halo/internal/config"
	"halo/internal/httpx"
	halooidc "halo/internal/oidc"
	"halo/internal/store"
	"halo/internal/testdb"
)

const (
	callback   = "https://grafana.example.com/login/generic_oauth"
	loggedOut  = "https://grafana.example.com/logged-out"
	pkceSecret = "Zs8rHNw0qX3cE1vYp7LkT2mB9dF4gJ6hA5uW0oQiRtUe"
)

type harness struct {
	t      *testing.T
	st     *store.Store
	srv    *httptest.Server
	app    store.Application
	secret string
	user   store.User
	group  store.Group
}

type policyFunc func(access.Input) access.Decision

func (f policyFunc) Evaluate(_ context.Context, in access.Input) (access.Decision, error) {
	return f(in), nil
}

func newHarness(t *testing.T, policy access.Engine) *harness {
	t.Helper()
	st := testdb.New(t)
	ctx := context.Background()
	h := &harness{t: t, st: st}
	var err error
	if h.group, err = st.CreateGroup(ctx, store.NewGroup{Name: "Engineering", Kind: "assigned"}); err != nil {
		t.Fatal(err)
	}
	if h.user, err = st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active", GroupIDs: []string{h.group.ID}}); err != nil {
		t.Fatal(err)
	}
	if h.app, err = st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", RedirectURIs: []string{callback}, PostLogoutURIs: []string{loggedOut}, GroupIDs: []string{h.group.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, h.secret, err = st.AddClientSecret(ctx, h.app.ID, "Test", time.Hour); err != nil {
		t.Fatal(err)
	}
	var handler http.Handler
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
	t.Cleanup(h.srv.Close)
	issuer, _ := url.Parse(h.srv.URL)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	authn := &httpx.Auth{Store: st, Public: issuer, Dev: true}
	provider, err := halooidc.New(httpx.Deps{Config: config.Config{PublicURL: issuer, SecretKey: key, Dev: true}, Store: st, Auth: authn, Policy: policy})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/oauth2/", authn.Resolve(provider))
	mux.Handle("/.well-known/", provider)
	handler = mux
	return h
}

func (h *harness) session(u store.User, method string) (store.Session, string) {
	h.t.Helper()
	sess, token, err := h.st.CreateSession(context.Background(), store.NewSession{UserID: u.ID, Method: method})
	if err != nil {
		h.t.Fatal(err)
	}
	return sess, token
}

func (h *harness) get(path, sessionToken string) (int, *url.URL) {
	h.t.Helper()
	req, _ := http.NewRequest(http.MethodGet, h.srv.URL+path, nil)
	if sessionToken != "" {
		req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: sessionToken})
	}
	res, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	res.Body.Close()
	location, _ := url.Parse(res.Header.Get("Location"))
	return res.StatusCode, location
}

func (h *harness) silent(sessionToken string, extra url.Values) *url.URL {
	h.t.Helper()
	q := url.Values{"client_id": {h.app.ClientID}, "redirect_uri": {callback}, "response_type": {"code"}, "scope": {"openid email offline_access"}, "state": {"st-1"},
		"prompt": {"none"}, "code_challenge": {oidc.NewSHACodeChallenge(pkceSecret)}, "code_challenge_method": {"S256"}}
	for k, v := range extra {
		q[k] = v
	}
	status, location := h.get("/oauth2/authorize?"+q.Encode(), sessionToken)
	if status != http.StatusFound {
		h.t.Fatalf("authorize: %d", status)
	}
	if location.Path == "/oauth2/authorize/callback" {
		if status, location = h.get(location.String(), sessionToken); status != http.StatusFound {
			h.t.Fatalf("callback: %d", status)
		}
	}
	if !strings.HasPrefix(location.String(), callback+"?") || location.Query().Get("state") != "st-1" {
		h.t.Fatalf("silent authorize ended at %s", location)
	}
	return location
}

func (h *harness) tokens(code string) (string, string, *oidc.IDTokenClaims) {
	h.t.Helper()
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callback}, "code_verifier": {pkceSecret}}
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/oauth2/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(h.app.ClientID, h.secret)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil || res.StatusCode != http.StatusOK {
		h.t.Fatalf("token exchange: %d %v", res.StatusCode, err)
	}
	verifier := rp.NewIDTokenVerifier(h.srv.URL, h.app.ClientID, rp.NewRemoteKeySet(http.DefaultClient, h.srv.URL+"/oauth2/keys"))
	claims, err := rp.VerifyTokens[*oidc.IDTokenClaims](context.Background(), body.AccessToken, body.IDToken, verifier)
	if err != nil {
		h.t.Fatal(err)
	}
	return body.IDToken, body.RefreshToken, claims
}

func (h *harness) refresh(token string) (int, string) {
	h.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/oauth2/token", strings.NewReader(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {token}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(h.app.ClientID, h.secret)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.NewDecoder(res.Body).Decode(&body)
	return res.StatusCode, body.RefreshToken
}

func TestPromptNone(t *testing.T) {
	var effect atomic.Value
	effect.Store(access.Allow)
	h := newHarness(t, policyFunc(func(access.Input) access.Decision {
		return access.Decision{Effect: effect.Load().(string), Policy: "Office only", Reason: "Outside the office network."}
	}))
	ctx := context.Background()
	errorOf := func(u *url.URL) string { return u.Query().Get("error") }

	if got := errorOf(h.silent("", nil)); got != "login_required" {
		t.Fatalf("no session: %s", got)
	}

	sess, token := h.session(h.user, "passkey")
	location := h.silent(token, nil)
	if location.Query().Get("code") == "" {
		t.Fatalf("silent sign-in did not return a code: %s", location)
	}
	idToken, _, claims := h.tokens(location.Query().Get("code"))
	if claims.Subject != h.user.ID || claims.Claims["sid"] != sess.ID || !slices.Equal(claims.AuthenticationMethodsReferences, []string{"hwk", "mfa"}) {
		t.Fatalf("claims: %+v", claims)
	}

	grace, err := h.st.CreateUser(ctx, store.NewUser{Email: "grace@example.com", Name: "Grace Hopper", Status: "active", GroupIDs: []string{h.group.ID}})
	if err != nil {
		t.Fatal(err)
	}
	_, graceToken := h.session(grace, "totp")
	if got := errorOf(h.silent(graceToken, url.Values{"id_token_hint": {idToken}})); got != "login_required" {
		t.Fatalf("id_token_hint for another user: %s", got)
	}
	if got := errorOf(h.silent(token, url.Values{"max_age": {"0"}})); got != "login_required" {
		t.Fatalf("max_age exceeded: %s", got)
	}

	outsider, err := h.st.CreateUser(ctx, store.NewUser{Email: "nora@example.com", Name: "Nora", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	_, outsiderToken := h.session(outsider, "passkey")
	if got := errorOf(h.silent(outsiderToken, nil)); got != "interaction_required" {
		t.Fatalf("unassigned: %s", got)
	}

	effect.Store(access.Block)
	if got := errorOf(h.silent(token, nil)); got != "interaction_required" {
		t.Fatalf("blocked by policy: %s", got)
	}
	effect.Store(access.RequirePhishingResistant)
	if got := errorOf(h.silent(graceToken, nil)); got != "login_required" {
		t.Fatalf("totp session under a phishing-resistant policy: %s", got)
	}
	if location := h.silent(token, nil); location.Query().Get("code") == "" {
		t.Fatalf("passkey session under a phishing-resistant policy: %s", location)
	}

	events, err := h.st.ListSignIns(ctx, store.SignInFilter{AppID: h.app.ID})
	if err != nil {
		t.Fatal(err)
	}
	var successes, failures []string
	for _, e := range events {
		if e.Result == "success" {
			successes = append(successes, e.Method)
		} else {
			failures = append(failures, e.Reason)
		}
	}
	slices.Sort(failures)
	if !slices.Equal(successes, []string{"passkey", "passkey"}) || !slices.Equal(failures, []string{"Policy “Office only”: Outside the office network.", "Policy “Office only”: Outside the office network.", "User is not assigned to this application."}) {
		t.Fatalf("events: %v / %q", successes, failures)
	}
}

func TestEndSession(t *testing.T) {
	h := newHarness(t, nil)
	sess, token := h.session(h.user, "passkey")
	idToken, refreshToken, _ := h.tokens(h.silent(token, nil).Query().Get("code"))
	other, otherToken := h.session(h.user, "passkey")
	status, refreshToken := h.refresh(refreshToken)
	if status != http.StatusOK || refreshToken == "" {
		t.Fatalf("refresh before logout: %d", status)
	}

	q := url.Values{"id_token_hint": {idToken}, "post_logout_redirect_uri": {loggedOut}, "state": {"bye"}}
	status, location := h.get("/oauth2/logout?"+q.Encode(), token)
	next := location.Query().Get("next")
	if status != http.StatusFound || location.Path != "/signed-out" || next != loggedOut+"?state=bye" {
		t.Fatalf("end session: %d %s", status, location)
	}
	if got, _ := h.refresh(refreshToken); got != http.StatusBadRequest {
		t.Fatalf("refresh token after logout: %d", got)
	}
	ctx := context.Background()
	if _, err := h.st.SessionByToken(ctx, token); err != store.ErrNotFound {
		t.Fatalf("Halo session %s survived logout: %v", sess.ID, err)
	}
	if _, err := h.st.SessionByToken(ctx, otherToken); err != nil {
		t.Fatalf("another session %s was revoked: %v", other.ID, err)
	}

	if status, _ := h.get("/oauth2/logout?"+url.Values{"id_token_hint": {idToken}, "post_logout_redirect_uri": {"https://evil.example.com/"}}.Encode(), ""); status != http.StatusBadRequest {
		t.Fatalf("unregistered post_logout_redirect_uri: %d", status)
	}
	if status, location := h.get("/oauth2/logout?client_id="+h.app.ClientID, ""); status != http.StatusFound || location.String() != "/signed-out" {
		t.Fatalf("logout without a redirect: %d %s", status, location)
	}

	for next, want := range map[string]string{
		loggedOut + "?state=bye":                 loggedOut + "?state=bye",
		loggedOut:                                loggedOut,
		"https://evil.example.com/":              "/signed-out",
		loggedOut + "/../../evil":                "/signed-out",
		"https://grafana.example.com.evil.test/": "/signed-out",
		"/admin":                                 "/signed-out",
	} {
		if status, location := h.get("/oauth2/logout/return?next="+url.QueryEscape(next), ""); status != http.StatusFound || location.String() != want {
			t.Errorf("return to %q: %d %s, want %s", next, status, location, want)
		}
	}
	if err := h.st.SetApplicationStatus(ctx, h.app.ID, "disabled"); err != nil {
		t.Fatal(err)
	}
	if _, location := h.get("/oauth2/logout/return?next="+url.QueryEscape(loggedOut), ""); location.String() != "/signed-out" {
		t.Fatalf("disabled application: %s", location)
	}
}
