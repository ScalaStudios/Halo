package oidc_test

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	"halo/internal/config"
	"halo/internal/httpx"
	halooidc "halo/internal/oidc"
	"halo/internal/store"
	"halo/internal/testdb"
	"halo/internal/webhooks"
)

func TestTokenHook(t *testing.T) {
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
	const redirect = "https://grafana.example.com/login/generic_oauth"
	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", RedirectURIs: []string{redirect}, GroupIDs: []string{group.ID}})
	must(err)
	_, secret, err := st.AddClientSecret(ctx, app.ID, "Test", time.Hour)
	must(err)
	billing, err := st.SaveAPIResource(ctx, store.APIResource{Name: "Billing", Identifier: "https://billing.example.com", AccessTokenTTL: 300, Scopes: []store.APIScope{{Name: "billing:read"}}})
	must(err)
	must(st.SetAPIResourceGrant(ctx, billing.ID, app.ID, []string{"billing:read"}))

	const hookSecret = "hook-secret-0123456789abcdefghijklmnop"
	var answer atomic.Value
	var calls atomic.Int32
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !strings.HasPrefix(r.Header.Get("Halo-Signature"), "t=") {
			http.Error(w, "unsigned", http.StatusUnauthorized)
			return
		}
		body := answer.Load().(string)
		w.Header().Set("Halo-Signature", hookSign(hookSecret, body))
		_, _ = w.Write([]byte(body))
	}))
	defer hook.Close()
	prev := webhooks.HookClient
	webhooks.HookClient = hook.Client()
	t.Cleanup(func() { webhooks.HookClient = prev })
	must(st.SaveApplicationHook(ctx, store.ApplicationHook{AppID: app.ID, Host: "checks.example.com", URLSealed: st.Sealer.Seal([]byte(hook.URL)), SecretSealed: st.Sealer.Seal([]byte(hookSecret)),
		Claims: []string{"department", "sub", "iss"}}))

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
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	token := func(form url.Values) (int, map[string]any) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/oauth2/token", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetBasicAuth(app.ClientID, secret)
		res, err := client.Do(req)
		must(err)
		defer res.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(res.Body).Decode(&body)
		return res.StatusCode, body
	}
	const verifier = "Zs8rHNw0qX3cE1vYp7LkT2mB9dF4gJ6hA5uW0oQiRtUe"
	exchange := func() (int, map[string]any) {
		t.Helper()
		q := url.Values{"client_id": {app.ClientID}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {"openid email offline_access billing:read"}, "nonce": {"n-1"},
			"code_challenge": {oidc.NewSHACodeChallenge(verifier)}, "code_challenge_method": {"S256"}}
		res, err := client.Get(srv.URL + "/oauth2/authorize?" + q.Encode())
		must(err)
		res.Body.Close()
		requestID, _ := strings.CutPrefix(res.Header.Get("Location"), "/sign-in?authRequest=")
		sess, sessionToken, err := st.CreateSession(ctx, store.NewSession{UserID: user.ID, Method: "passkey"})
		must(err)
		must(st.CompleteAuthRequest(ctx, requestID, user.ID, sess.ID, []string{"hwk", "mfa"}, time.Now()))
		req, _ := http.NewRequest(http.MethodGet, srv.URL+store.AuthorizeCallbackPath(requestID), nil)
		req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: sessionToken})
		res, err = client.Do(req)
		must(err)
		res.Body.Close()
		loc, _ := url.Parse(res.Header.Get("Location"))
		return token(url.Values{"grant_type": {"authorization_code"}, "code": {loc.Query().Get("code")}, "redirect_uri": {redirect}, "code_verifier": {verifier}})
	}

	answer.Store(`{"decision":"allow","claims":{"department":"R&D","sub":"usr_attacker","iss":"https://evil.example.com","roles":["global_admin"],"unlisted":"x"}}`)
	status, body := exchange()
	if status != http.StatusOK {
		t.Fatalf("allowed exchange: %d %v", status, body)
	}
	idVerifier := rp.NewIDTokenVerifier(srv.URL, app.ClientID, rp.NewRemoteKeySet(http.DefaultClient, srv.URL+"/oauth2/keys"), rp.WithNonce(func(context.Context) string { return "n-1" }))
	claims, err := rp.VerifyTokens[*oidc.IDTokenClaims](ctx, body["access_token"].(string), body["id_token"].(string), idVerifier)
	must(err)
	if claims.Subject != user.ID || claims.Issuer != srv.URL || claims.Claims["department"] != "R&D" || claims.Claims["roles"] != nil || claims.Claims["unlisted"] != nil {
		t.Fatalf("id token claims: sub %s iss %s extra %v", claims.Subject, claims.Issuer, claims.Claims)
	}
	parts := strings.Split(body["access_token"].(string), ".")
	if len(parts) != 3 {
		t.Fatalf("the access token is not a JWT: %v", body["access_token"])
	}
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var access map[string]any
	if json.Unmarshal(payload, &access) != nil || access["department"] != "R&D" || access["sub"] != user.ID || access["iss"] != srv.URL || access["roles"] != nil {
		t.Fatalf("access token claims: %s", payload)
	}

	answer.Store(`{"decision":"deny","reason":"Your laptop is missing the latest security update."}`)
	status, denied := token(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {body["refresh_token"].(string)}})
	if status < 400 || denied["error"] != "access_denied" || denied["error_description"] != "Your laptop is missing the latest security update." || denied["id_token"] != nil {
		t.Fatalf("denied refresh: %d %v", status, denied)
	}

	events, err := st.ListSignIns(ctx, store.SignInFilter{AppID: app.ID})
	must(err)
	if len(events) != 2 || events[0].Hook != "denied" || events[0].Result != "failure" || events[0].Method != "token" || events[0].Device != "refresh_token" ||
		!strings.Contains(events[0].Reason, "missing the latest security update") || events[1].Hook != "allowed" || events[1].Result != "success" || events[1].Device != "authorization_code" {
		t.Fatalf("sign-in log: %+v", events)
	}
	if calls.Load() != 2 {
		t.Fatalf("hook calls: %d", calls.Load())
	}
}

func hookSign(secret, body string) string {
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stamp + "." + body))
	return "t=" + stamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}
