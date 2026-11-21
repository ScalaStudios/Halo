package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"
	"github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	"halo/internal/auth"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/policy"
	"halo/internal/store"
	"halo/internal/testdb"
)

type fakeProvider struct {
	srv       *httptest.Server
	key       *rsa.PrivateKey
	claims    map[string]any
	emails    []map[string]any
	nonce     string
	challenge string
	badNonce  bool
}

func newFakeProvider(t *testing.T) *fakeProvider {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeProvider{key: key}
	mux := http.NewServeMux()
	authorize := func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		f.nonce, f.challenge = q.Get("nonce"), q.Get("code_challenge")
		http.Redirect(w, r, q.Get("redirect_uri")+"?code=fake-code&state="+url.QueryEscape(q.Get("state")), http.StatusFound)
	}
	mux.HandleFunc("GET /authorize", authorize)
	mux.HandleFunc("GET /login/oauth/authorize", authorize)
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer": f.srv.URL, "authorization_endpoint": f.srv.URL + "/authorize", "token_endpoint": f.srv.URL + "/token", "jwks_uri": f.srv.URL + "/keys",
			"response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &f.key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	token := func(w http.ResponseWriter, r *http.Request, idToken bool) {
		if r.ParseForm() != nil || r.PostForm.Get("code") != "fake-code" || oidc.NewSHACodeChallenge(r.PostForm.Get("code_verifier")) != f.challenge {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		out := map[string]any{"access_token": "fake-access", "token_type": "Bearer", "expires_in": 3600}
		if idToken {
			claims := map[string]any{"iss": f.srv.URL, "aud": "halo", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(), "nonce": f.nonce}
			if f.badNonce {
				claims["nonce"] = "attacker-nonce"
			}
			for k, v := range f.claims {
				claims[k] = v
			}
			payload, _ := json.Marshal(claims)
			signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: f.key, KeyID: "k1"}}, (&jose.SignerOptions{}).WithType("JWT"))
			signed, _ := signer.Sign(payload)
			out["id_token"], _ = signed.CompactSerialize()
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	}
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) { token(w, r, true) })
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) { token(w, r, false) })
	github := func(body func() any) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer fake-access" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			json.NewEncoder(w).Encode(body())
		}
	}
	mux.HandleFunc("GET /api/v3/user", github(func() any { return f.claims }))
	mux.HandleFunc("GET /api/v3/user/emails", github(func() any { return f.emails }))
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func newFederationEnv(t *testing.T) *env {
	st := testdb.New(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	public, _ := url.Parse("http://localhost:" + strings.TrimPrefix(listener.Addr().String(), "127.0.0.1:"))
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	mux := http.NewServeMux()
	if err := auth.Register(mux, httpx.Deps{Config: config.Config{PublicURL: public, Dev: true}, Store: st, Auth: authn, Policy: policy.NewEngine(st)}); err != nil {
		t.Fatal(err)
	}
	srv := &httptest.Server{Listener: listener, Config: &http.Server{Handler: authn.Resolve(mux)}}
	srv.Start()
	srv.URL = public.String()
	t.Cleanup(srv.Close)
	return &env{t: t, st: st, srv: srv, rp: virtualwebauthn.RelyingParty{ID: "localhost", Name: "Halo", Origin: public.String()}}
}

func (e *env) provider(p store.IdentityProvider) store.IdentityProvider {
	e.t.Helper()
	p.ClientID, p.Enabled, p.ShowOnSignIn = "halo", true, true
	if p.Scopes == nil {
		p.Scopes = []string{"openid", "email"}
	}
	if p.AllowedDomains == nil {
		p.AllowedDomains = []string{"example.com"}
	}
	if p.JITGroupIDs == nil {
		p.JITGroupIDs = []string{}
	}
	saved, err := e.st.SaveIdentityProvider(context.Background(), p, "client-secret")
	if err != nil {
		e.t.Fatal(err)
	}
	return saved
}

func (e *env) federate(c *http.Client, providerID, query string) string {
	e.t.Helper()
	c.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		if strings.HasPrefix(req.URL.String(), e.srv.URL) && req.URL.Path != auth.FederationCallbackPath {
			return http.ErrUseLastResponse
		}
		return nil
	}
	res, err := c.Get(e.srv.URL + "/api/v1/auth/federated/" + providerID + "/start?" + query)
	if err != nil {
		e.t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusFound {
		e.t.Fatalf("federate: status %d", res.StatusCode)
	}
	return res.Header.Get("Location")
}

func TestFederatedOIDC(t *testing.T) {
	e := newFederationEnv(t)
	ctx := context.Background()
	fake := newFakeProvider(t)
	p := e.provider(store.IdentityProvider{Kind: "oidc", Name: "Acme SSO", Issuer: fake.srv.URL})
	ada, err := e.st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.SeedFederation(ctx); err != nil {
		t.Fatal(err)
	}

	var listed []map[string]string
	if e.do(client(), "GET", "/api/v1/auth/providers", nil, &listed); len(listed) != 1 || listed[0]["id"] != p.ID || listed[0]["kind"] != "oidc" {
		t.Fatalf("providers: %+v", listed)
	}

	fake.claims = map[string]any{"sub": "acme-1", "email": "ADA@example.com", "email_verified": true, "name": "Ada"}
	c := client()
	if location := e.federate(c, p.ID, "next=%2Faccount%2Fsecurity"); location != "/account/security" {
		t.Fatalf("link by email: %s", location)
	}
	if me, status := e.me(c); status != http.StatusOK || me.ID != ada.ID {
		t.Fatalf("session after link: %d %+v", status, me)
	}
	var identities []store.FederatedIdentity
	if e.do(c, "GET", "/api/v1/me/identities", nil, &identities); len(identities) != 1 || identities[0].ProviderName != "Acme SSO" {
		t.Fatalf("identities: %+v", identities)
	}

	fake.claims = map[string]any{"sub": "acme-1", "email": "ada@personal.test", "email_verified": false}
	if location := e.federate(client(), p.ID, "next=https%3A%2F%2Fevil.test"); location != "/account" {
		t.Fatalf("existing link: %s", location)
	}

	fake.claims = map[string]any{"sub": "acme-2", "email": "eve@evil.test", "email_verified": true}
	if location := e.federate(client(), p.ID, ""); location != "/sign-in?error=ERR_FEDERATION_DOMAIN" {
		t.Fatalf("domain refused: %s", location)
	}
	fake.claims = map[string]any{"sub": "acme-3", "email": "grace@example.com", "email_verified": true, "given_name": "Grace", "family_name": "Hopper"}
	if location := e.federate(client(), p.ID, ""); location != "/sign-in?error=ERR_FEDERATION_NO_ACCOUNT" {
		t.Fatalf("no account: %s", location)
	}
	fake.claims["email_verified"] = false
	if location := e.federate(client(), p.ID, ""); location != "/sign-in?error=ERR_FEDERATION_EMAIL" {
		t.Fatalf("unverified email: %s", location)
	}
	failures, _ := e.st.ListSignIns(ctx, store.SignInFilter{})
	if len(failures) == 0 || failures[0].Method != "federated" || failures[0].Result != "failure" || failures[0].Email != "grace@example.com" || failures[0].Reason == "" {
		t.Fatalf("failure event: %+v", failures)
	}

	group, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Engineering", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	p.JIT, p.JITGroupIDs = true, []string{group.ID}
	if _, err := e.st.SaveIdentityProvider(ctx, p, ""); err != nil {
		t.Fatal(err)
	}
	fake.claims["email_verified"] = true
	jit := client()
	if location := e.federate(jit, p.ID, ""); location != "/account" {
		t.Fatalf("jit: %s", location)
	}
	grace, status := e.me(jit)
	if status != http.StatusOK || grace.Name != "Grace Hopper" || grace.Status != "active" || grace.Source != "Acme SSO" || !slices.Contains(grace.GroupIDs, group.ID) {
		t.Fatalf("jit user: %d %+v", status, grace)
	}

	fake.claims = map[string]any{"sub": "acme-1", "email": "ada@example.com", "email_verified": true}
	replay := client()
	replay.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		if req.URL.Path == auth.FederationCallbackPath {
			return http.ErrUseLastResponse
		}
		return nil
	}
	res, err := replay.Get(e.srv.URL + "/api/v1/auth/federated/" + p.ID + "/start")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	callback := res.Header.Get("Location")
	state, _ := url.Parse(callback)
	if location := e.federateCallback(callback, state.Query().Get("state")); location != "/account" {
		t.Fatalf("callback: %s", location)
	}
	if location := e.federateCallback(callback, state.Query().Get("state")); location != "/sign-in?error=ERR_FEDERATION_STATE" {
		t.Fatalf("replayed state: %s", location)
	}
	if location := e.federateCallback(callback, "forged"); location != "/sign-in?error=ERR_FEDERATION_STATE" {
		t.Fatalf("cookie mismatch: %s", location)
	}

	fake.badNonce = true
	if location := e.federate(client(), p.ID, ""); location != "/sign-in?error=ERR_FEDERATION_FAILED" {
		t.Fatalf("wrong nonce: %s", location)
	}
}

func (e *env) federateCallback(callback, cookie string) string {
	e.t.Helper()
	req, _ := http.NewRequest("GET", callback, nil)
	req.AddCookie(&http.Cookie{Name: "halo_federation", Value: cookie})
	res, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	res.Body.Close()
	return res.Header.Get("Location")
}

func TestFederatedAuthRequestAndPolicy(t *testing.T) {
	e := newFederationEnv(t)
	ctx := context.Background()
	fake := newFakeProvider(t)
	p := e.provider(store.IdentityProvider{Kind: "oidc", Name: "Acme SSO", Issuer: fake.srv.URL})
	ada, err := e.st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	group, _ := e.st.CreateGroup(ctx, store.NewGroup{Name: "Observability", Kind: "assigned"})
	app, err := e.st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://grafana.example.com/cb"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.AddGroupMember(ctx, group.ID, ada.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.st.SetApplicationGroups(ctx, app.ID, []string{group.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.CreateAuthRequest(ctx, store.AuthRequest{ID: "arq_fed", ClientID: app.ClientID, RedirectURI: app.RedirectURIs[0], Scopes: []string{"openid"}, ResponseType: "code"}); err != nil {
		t.Fatal(err)
	}
	fake.claims = map[string]any{"sub": "acme-1", "email": "ada@example.com", "email_verified": true}
	if location := e.federate(client(), p.ID, "authRequest=arq_fed&next=%2Fadmin"); location != "/oauth2/authorize/callback?id=arq_fed" {
		t.Fatalf("auth request: %s", location)
	}
	req, err := e.st.GetAuthRequest(ctx, "arq_fed")
	if err != nil || !req.Done || req.UserID == nil || *req.UserID != ada.ID || !slices.Equal(req.AMR, []string{"fed"}) {
		t.Fatalf("completed request: %+v %v", req, err)
	}
	if location := e.federate(client(), p.ID, "authRequest=arq_fed"); location != "/sign-in?authRequest=arq_fed&error=ERR_AUTH_REQUEST_EXPIRED" {
		t.Fatalf("used request: %s", location)
	}

	if _, err := e.st.CreateAccessPolicy(ctx, store.AccessPolicy{Name: "Phishing-resistant everywhere", Enabled: true, Mode: "enforce", Effect: "require-phishing-resistant", Conditions: store.PolicyConditions{AllUsers: true, AllApps: true}}); err != nil {
		t.Fatal(err)
	}
	if location := e.federate(client(), p.ID, ""); location != "/sign-in?error=ERR_STRONGER_AUTH_REQUIRED" {
		t.Fatalf("policy: %s", location)
	}
	events, _ := e.st.ListSignIns(ctx, store.SignInFilter{UserID: ada.ID})
	if len(events) == 0 || events[0].Result != "failure" || !strings.Contains(events[0].Reason, "Phishing-resistant everywhere") {
		t.Fatalf("policy event: %+v", events)
	}
}

func TestFederatedGitHubAndUnlink(t *testing.T) {
	e := newFederationEnv(t)
	ctx := context.Background()
	fake := newFakeProvider(t)
	oidcProvider := e.provider(store.IdentityProvider{Kind: "oidc", Name: "Acme SSO", Issuer: fake.srv.URL})
	github := e.provider(store.IdentityProvider{Kind: "github", Name: "GitHub", Issuer: fake.srv.URL, Scopes: []string{"read:user", "user:email"}})
	if _, err := e.st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active"}); err != nil {
		t.Fatal(err)
	}

	fake.claims = map[string]any{"id": 42, "login": "ada", "name": "Ada Lovelace"}
	fake.emails = []map[string]any{{"email": "ada@example.com", "primary": false, "verified": true}, {"email": "ada@unverified.test", "primary": true, "verified": false}}
	if location := e.federate(client(), github.ID, ""); location != "/sign-in?error=ERR_FEDERATION_EMAIL" {
		t.Fatalf("github without verified primary email: %s", location)
	}
	fake.emails = []map[string]any{{"email": "ada@example.com", "primary": true, "verified": true}}
	c := client()
	if location := e.federate(c, github.ID, ""); location != "/account" {
		t.Fatalf("github: %s", location)
	}
	fake.claims = map[string]any{"sub": "acme-1", "email": "ada@example.com", "email_verified": true}
	if location := e.federate(c, oidcProvider.ID, ""); location != "/account" {
		t.Fatalf("second provider: %s", location)
	}

	var identities []store.FederatedIdentity
	if e.do(c, "GET", "/api/v1/me/identities", nil, &identities); len(identities) != 2 {
		t.Fatalf("identities: %+v", identities)
	}
	if status := e.do(c, "DELETE", "/api/v1/me/identities/"+identities[0].ID, nil, nil); status != http.StatusNoContent {
		t.Fatalf("unlink with another way in: %d", status)
	}
	e.expect(c, "DELETE", "/api/v1/me/identities/"+identities[1].ID, nil, http.StatusConflict, "ERR_LAST_METHOD")
	e.expect(c, "DELETE", "/api/v1/me/identities/"+identities[0].ID, nil, http.StatusNotFound, "ERR_NOT_FOUND")
}

func TestCreateIdentityProviderAnswersCreated(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	admin, err := e.st.CreateUser(ctx, store.NewUser{Email: "sec@example.com", Name: "Sec", Status: "active", Roles: []string{"security_admin"}})
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := e.st.CreateSession(ctx, store.NewSession{UserID: admin.ID, Method: "passkey"})
	if err != nil {
		t.Fatal(err)
	}
	c := client()
	base, _ := url.Parse(e.srv.URL)
	c.Jar.SetCookies(base, []*http.Cookie{{Name: httpx.SessionCookie, Value: token}})

	body := map[string]any{"kind": "github", "name": "GitHub", "clientId": "halo", "clientSecret": "secret", "allowedDomains": []string{"example.com"}}
	var created store.IdentityProvider
	if status := e.do(c, "POST", "/api/v1/identity-providers", body, &created); status != http.StatusCreated || created.ID == "" {
		t.Fatalf("create: %d %+v", status, created)
	}
	if status := e.do(c, "PUT", "/api/v1/identity-providers/"+created.ID, body, nil); status != http.StatusOK {
		t.Fatalf("update: %d", status)
	}
}

func TestHomeRealmDiscovery(t *testing.T) {
	e := newFederationEnv(t)
	ctx := context.Background()
	fake := newFakeProvider(t)
	partner := e.provider(store.IdentityProvider{Kind: "oidc", Name: "Partner SSO", Issuer: fake.srv.URL, AllowedDomains: []string{"partner.example"}, RouteByDomain: true})
	partner.ShowOnSignIn = false
	if _, err := e.st.SaveIdentityProvider(ctx, partner, ""); err != nil {
		t.Fatal(err)
	}
	retired := e.provider(store.IdentityProvider{Kind: "oidc", Name: "Retired SSO", Issuer: fake.srv.URL, AllowedDomains: []string{"retired.example"}, RouteByDomain: true})
	retired.Enabled = false
	if _, err := e.st.SaveIdentityProvider(ctx, retired, ""); err != nil {
		t.Fatal(err)
	}
	e.provider(store.IdentityProvider{Kind: "oidc", Name: "Acme SSO", Issuer: fake.srv.URL})
	ada, err := e.st.CreateUser(ctx, store.NewUser{Email: "ada@partner.example", Name: "Ada Lovelace", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	e.withAuthenticator(ada)
	if _, err := e.st.CreateUser(ctx, store.NewUser{Email: "bob@partner.example", Name: "Bob", Status: "suspended"}); err != nil {
		t.Fatal(err)
	}

	identify := func(email string) string {
		t.Helper()
		var raw json.RawMessage
		if status := e.do(client(), "POST", "/api/v1/auth/identify", map[string]string{"email": email}, &raw); status != http.StatusOK {
			t.Fatalf("identify %s: %d", email, status)
		}
		return string(raw)
	}
	routed := identify("ada@partner.example")
	for _, email := range []string{"Bob@Partner.Example", "nobody@partner.example"} {
		if got := identify(email); got != routed {
			t.Fatalf("routing must not depend on the account: %s answered %s, ada answered %s", email, got, routed)
		}
	}
	var out struct {
		Methods  []string          `json:"methods"`
		Provider map[string]string `json:"provider"`
	}
	if err := json.Unmarshal([]byte(routed), &out); err != nil || out.Provider["id"] != partner.ID || out.Provider["name"] != "Partner SSO" || len(out.Methods) != 0 {
		t.Fatalf("routed identify: %s %v", routed, err)
	}
	for _, email := range []string{"dev@example.com", "dev@retired.example"} {
		out.Provider = nil
		if err := json.Unmarshal([]byte(identify(email)), &out); err != nil || out.Provider != nil || !slices.Equal(out.Methods, []string{"passkey", "totp"}) {
			t.Fatalf("%s falls back to the usual methods: %+v %v", email, out, err)
		}
	}

	fake.claims = map[string]any{"sub": "partner-1", "email": "ada@partner.example", "email_verified": true}
	if location := e.federate(client(), partner.ID, ""); location != "/account" {
		t.Fatalf("a routed provider without a button signs people in: %s", location)
	}

	admin, err := e.st.CreateUser(ctx, store.NewUser{Email: "sec@example.com", Name: "Sec", Status: "active", Roles: []string{"security_admin"}})
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := e.st.CreateSession(ctx, store.NewSession{UserID: admin.ID, Method: "passkey"})
	if err != nil {
		t.Fatal(err)
	}
	c := client()
	base, _ := url.Parse(e.srv.URL)
	c.Jar.SetCookies(base, []*http.Cookie{{Name: httpx.SessionCookie, Value: token}})
	body := map[string]any{"kind": "oidc", "name": "Second SSO", "issuer": fake.srv.URL, "clientId": "halo", "routeByDomain": true}
	e.expect(c, "POST", "/api/v1/identity-providers", body, http.StatusUnprocessableEntity, "ERR_INVALID")
	body["allowedDomains"] = []string{"Partner.Example"}
	e.expect(c, "POST", "/api/v1/identity-providers", body, http.StatusUnprocessableEntity, "ERR_INVALID")
	body["allowedDomains"] = []string{"retired.example"}
	e.expect(c, "POST", "/api/v1/identity-providers", body, http.StatusUnprocessableEntity, "ERR_INVALID")
	update := map[string]any{"kind": "oidc", "name": "Partner SSO", "issuer": fake.srv.URL, "clientId": "halo", "enabled": true, "allowedDomains": []string{"partner.example"}, "routeByDomain": true}
	var saved store.IdentityProvider
	if status := e.do(c, "PUT", "/api/v1/identity-providers/"+partner.ID, update, &saved); status != http.StatusOK || !saved.RouteByDomain {
		t.Fatalf("a provider does not conflict with itself: %d %+v", status, saved)
	}
	update["routeByDomain"] = false
	if status := e.do(c, "PUT", "/api/v1/identity-providers/"+partner.ID, update, &saved); status != http.StatusOK || saved.RouteByDomain {
		t.Fatalf("turn off routing: %d %+v", status, saved)
	}
	if audit, err := e.st.ListAudit(ctx, 1); err != nil || audit[0].Action != "identity_provider.update" || audit[0].Summary != "Changed the identity provider's email domain routing" {
		t.Fatalf("audit: %+v %v", audit, err)
	}
}

func TestFederatedMapping(t *testing.T) {
	e := newFederationEnv(t)
	ctx := context.Background()
	fake := newFakeProvider(t)
	p := e.provider(store.IdentityProvider{Kind: "oidc", Name: "Acme SSO", Issuer: fake.srv.URL})
	ada, err := e.st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Title: "Intern", Department: "Sales", Location: "Lisbon", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.SetUserAttributes(ctx, ada.ID, map[string]any{"employeeNumber": "E-7"}); err != nil {
		t.Fatal(err)
	}
	eng, ops, manual := e.group("Engineering"), e.group("Operations", ada), e.group("Book club", ada)
	rule := `user.department == "Engineering"`
	dynamic, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Engineers", Kind: "dynamic", Rule: &rule})
	if err != nil {
		t.Fatal(err)
	}

	admin, err := e.st.CreateUser(ctx, store.NewUser{Email: "sec@example.com", Name: "Sec", Status: "active", Roles: []string{"security_admin"}})
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := e.st.CreateSession(ctx, store.NewSession{UserID: admin.ID, Method: "passkey"})
	if err != nil {
		t.Fatal(err)
	}
	c := client()
	base, _ := url.Parse(e.srv.URL)
	c.Jar.SetCookies(base, []*http.Cookie{{Name: httpx.SessionCookie, Value: token}})
	mapping := map[string]any{
		"profile":    []map[string]string{{"claim": "name", "field": "name"}, {"claim": "jobTitle", "field": "title"}, {"claim": "department", "field": "department"}},
		"groupClaim": "groups",
		"groups":     []map[string]string{{"value": "eng-id", "groupId": eng.ID}, {"value": "ops-id", "groupId": ops.ID}, {"value": "ops-legacy", "groupId": ops.ID}},
	}
	body := map[string]any{"kind": "oidc", "name": "Acme SSO", "issuer": fake.srv.URL, "clientId": "halo", "scopes": []string{"openid", "email"}, "enabled": true, "showOnSignIn": true, "allowedDomains": []string{"example.com"}, "jit": true, "jitGroupIds": []string{ops.ID}, "mapping": mapping}
	for _, bad := range []map[string]any{
		{"profile": []map[string]string{{"claim": "roles", "field": "roles"}}},
		{"profile": []map[string]string{{"claim": "mail", "field": "email"}}},
		{"profile": []map[string]string{{"claim": "dept", "field": "department"}, {"claim": "department", "field": "department"}}},
		{"groupClaim": "groups", "groups": []map[string]string{{"value": "eng-id", "groupId": dynamic.ID}}},
		{"groups": []map[string]string{{"value": "eng-id", "groupId": eng.ID}}},
	} {
		body["mapping"] = bad
		e.expect(c, "PUT", "/api/v1/identity-providers/"+p.ID, body, http.StatusUnprocessableEntity, "ERR_INVALID")
	}
	e.expect(c, "POST", "/api/v1/identity-providers", map[string]any{"kind": "github", "name": "GitHub", "clientId": "halo", "clientSecret": "s", "mapping": mapping}, http.StatusUnprocessableEntity, "ERR_INVALID")
	body["mapping"] = mapping
	var saved store.IdentityProvider
	if status := e.do(c, "PUT", "/api/v1/identity-providers/"+p.ID, body, &saved); status != http.StatusOK || len(saved.Mapping.Profile) != 3 || len(saved.Mapping.Groups) != 3 {
		t.Fatalf("save mapping: %d %+v", status, saved.Mapping)
	}
	if audit, _ := e.st.ListAudit(ctx, 1); audit[0].Summary != "Changed the identity provider's account creation on first sign-in, groups for new accounts, profile mapping, group mapping" {
		t.Fatalf("provider audit: %+v", audit[0])
	}
	if uses, _ := e.st.GroupUses(ctx, eng.ID); len(uses) != 1 || uses[0].Kind != "identity provider" {
		t.Fatalf("mapped group in use: %+v", uses)
	}

	audit := func() []store.AuditEvent {
		t.Helper()
		events, err := e.st.ListAudit(ctx, 0)
		if err != nil {
			t.Fatal(err)
		}
		mapped := []store.AuditEvent{}
		for _, ev := range events {
			if strings.HasSuffix(ev.Summary, " from Acme SSO at sign-in") {
				mapped = append(mapped, ev)
			}
		}
		return mapped
	}
	user := func(id string) store.User {
		t.Helper()
		u, err := e.st.GetUser(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	signIn := func(claims map[string]any, query, want string) {
		t.Helper()
		fake.claims = map[string]any{"sub": "acme-1", "email": "ada@example.com", "email_verified": true}
		for k, v := range claims {
			fake.claims[k] = v
		}
		if location := e.federate(client(), p.ID, query); location != want {
			t.Fatalf("sign-in with %v: %s", claims, location)
		}
	}

	signIn(map[string]any{"name": "Ada King", "jobTitle": "Engineer", "department": "", "groups": []string{"eng-id", "unmapped"}, "roles": []string{"global_admin"}, "employeeNumber": "E-9"}, "", "/account")
	got := user(ada.ID)
	if got.Name != "Ada King" || got.Title != "Engineer" || got.Department != "" || got.Location != "Lisbon" {
		t.Fatalf("profile updated: %+v", got)
	}
	if !reflect.DeepEqual(got.Attributes, map[string]any{"employeeNumber": "E-7"}) {
		t.Fatalf("mapping never changes custom attributes: %#v", got.Attributes)
	}
	if !slices.Contains(got.GroupIDs, eng.ID) || slices.Contains(got.GroupIDs, ops.ID) || !slices.Contains(got.GroupIDs, manual.ID) {
		t.Fatalf("groups follow the provider for mapped groups only: %v", got.GroupIDs)
	}
	if len(got.RoleKeys) != 0 || len(got.EligibleRoles) != 0 {
		t.Fatalf("mapping never grants roles: %v %v", got.RoleKeys, got.EligibleRoles)
	}
	events := audit()
	if len(events) != 3 {
		t.Fatalf("one audit event per change: %+v", events)
	}
	for _, ev := range events {
		if ev.ActorID != nil || len(ev.Changes) == 0 {
			t.Fatalf("audit event: %+v", ev)
		}
		switch ev.Action {
		case "user.update":
			fields := []string{}
			for _, change := range ev.Changes {
				fields = append(fields, change.Field)
			}
			if ev.TargetID != ada.ID || !slices.Equal(fields, []string{"department", "name", "title"}) || string(ev.Changes[0].Before) != `"Sales"` || string(ev.Changes[0].After) != `""` {
				t.Fatalf("profile audit: %+v", ev)
			}
		case "group.member.add":
			if ev.TargetID != eng.ID || string(ev.Changes[0].After) != "true" {
				t.Fatalf("add audit: %+v", ev)
			}
		case "group.member.remove":
			if ev.TargetID != ops.ID || string(ev.Changes[0].After) != "false" {
				t.Fatalf("remove audit: %+v", ev)
			}
		default:
			t.Fatalf("unexpected audit: %+v", ev)
		}
	}

	signIn(map[string]any{"name": "Ada King", "jobTitle": "Engineer", "department": "", "groups": []string{"eng-id"}}, "", "/account")
	if len(audit()) != 3 {
		t.Fatalf("an unchanged sign-in writes nothing: %+v", audit())
	}

	signIn(map[string]any{"name": "", "groups": "ops-legacy"}, "", "/account")
	got = user(ada.ID)
	if got.Name != "Ada King" || got.Title != "Engineer" || slices.Contains(got.GroupIDs, eng.ID) || !slices.Contains(got.GroupIDs, ops.ID) {
		t.Fatalf("missing claims and a blank name leave the profile alone: %+v", got)
	}

	signIn(map[string]any{"groups": []any{"eng-id", map[string]any{"id": "ops-id"}}}, "", "/account")
	signIn(map[string]any{}, "", "/account")
	if got = user(ada.ID); !slices.Contains(got.GroupIDs, ops.ID) || slices.Contains(got.GroupIDs, eng.ID) || !slices.Contains(got.GroupIDs, manual.ID) {
		t.Fatalf("a missing or malformed groups claim changes no groups: %v", got.GroupIDs)
	}

	app, err := e.st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://grafana.example.com/cb"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.SetApplicationGroups(ctx, app.ID, []string{ops.ID}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"arq_drop", "arq_join"} {
		if _, err := e.st.CreateAuthRequest(ctx, store.AuthRequest{ID: id, ClientID: app.ClientID, RedirectURI: app.RedirectURIs[0], Scopes: []string{"openid"}, ResponseType: "code"}); err != nil {
			t.Fatal(err)
		}
	}
	signIn(map[string]any{"groups": []string{}}, "authRequest=arq_drop", "/sign-in?authRequest=arq_drop&error=ERR_NOT_ASSIGNED")
	if got = user(ada.ID); slices.Contains(got.GroupIDs, ops.ID) || !slices.Contains(got.GroupIDs, manual.ID) {
		t.Fatalf("an empty groups claim removes mapped groups only: %v", got.GroupIDs)
	}
	signIn(map[string]any{"groups": []string{"ops-id"}}, "authRequest=arq_join", "/oauth2/authorize/callback?id=arq_join")

	fake.claims = map[string]any{"sub": "acme-2", "email": "grace@example.com", "email_verified": true, "name": "Grace Hopper", "jobTitle": "Admiral", "groups": []string{"eng-id"}}
	if location := e.federate(client(), p.ID, ""); location != "/account" {
		t.Fatalf("jit: %s", location)
	}
	grace, err := e.st.GetUserByEmail(ctx, "grace@example.com")
	if err != nil || grace.Title != "Admiral" || !slices.Equal(grace.GroupIDs, []string{eng.ID}) {
		t.Fatalf("new account mapped on first sign-in: %+v %v", grace, err)
	}
}
