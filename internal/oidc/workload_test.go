package oidc_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"halo/internal/config"
	"halo/internal/httpx"
	halooidc "halo/internal/oidc"
	"halo/internal/server"
	"halo/internal/store"
	"halo/internal/testdb"
)

const workloadSubject = "repo:acme/infra:ref:refs/heads/main"

type workloadIssuer struct {
	t        *testing.T
	srv      *httptest.Server
	requests atomic.Int64
	mu       sync.Mutex
	keys     []jose.JSONWebKey
}

func newWorkloadIssuer(t *testing.T) *workloadIssuer {
	i := &workloadIssuer{t: t}
	i.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": i.srv.URL, "jwks_uri": i.srv.URL + "/keys"})
		case "/keys":
			i.mu.Lock()
			defer i.mu.Unlock()
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: i.keys})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(i.srv.Close)
	return i
}

func (i *workloadIssuer) publish(kid string) *rsa.PrivateKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		i.t.Fatal(err)
	}
	i.mu.Lock()
	i.keys = []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: kid, Algorithm: string(jose.RS256), Use: "sig"}}
	i.mu.Unlock()
	return key
}

func (i *workloadIssuer) claims() map[string]any {
	now := time.Now()
	return map[string]any{"iss": i.srv.URL, "sub": workloadSubject, "aud": "http://localhost:3200", "iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(), "jti": "run-1", "repository_owner": "acme"}
}

func sign(t *testing.T, alg jose.SignatureAlgorithm, key any, kid string, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: alg, Key: jose.JSONWebKey{Key: key, KeyID: kid}}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

type workloadEnv struct {
	t       *testing.T
	st      *store.Store
	h       http.Handler
	admin   string
	account store.ServiceAccount
}

func newWorkloadEnv(t *testing.T) *workloadEnv {
	t.Helper()
	ctx := context.Background()
	st := testdb.New(t)
	public, _ := url.Parse("http://localhost:3200")
	secretKey := make([]byte, 32)
	_, _ = rand.Read(secretKey)
	app, err := server.New(config.Config{Organization: "Test", PublicURL: public, SecretKey: secretKey, Dev: true}, st)
	if err != nil {
		t.Fatal(err)
	}
	e := &workloadEnv{t: t, st: st, h: app.Handler}
	admin, err := st.CreateUser(ctx, store.NewUser{Email: "gia@example.com", Name: "Gia", Status: "active", Roles: []string{"global_admin"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, e.admin, err = st.CreateSession(ctx, store.NewSession{UserID: admin.ID, Method: "passkey"}); err != nil {
		t.Fatal(err)
	}
	if e.account, err = st.CreateServiceAccount(ctx, store.NewServiceAccount{Name: "Deployer", Roles: []string{"app_admin"}}); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *workloadEnv) api(credential, method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if strings.HasPrefix(credential, "hlk_") {
		req.Header.Set("Authorization", "Bearer "+credential)
	} else {
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: credential})
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e *workloadEnv) exchange(clientID, assertion string) (int, map[string]any) {
	e.t.Helper()
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {clientID}, "client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"}, "client_assertion": {assertion}}
	req := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	body := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

func (e *workloadEnv) accept(assertion string) string {
	e.t.Helper()
	status, body := e.exchange(e.account.ID, assertion)
	token, _ := body["access_token"].(string)
	if status != http.StatusOK || !strings.HasPrefix(token, "hlk_") || body["token_type"] != "Bearer" || body["expires_in"] != float64(3600) || body["scope"] != "api" {
		e.t.Fatalf("exchange: %d %v", status, body)
	}
	return token
}

func (e *workloadEnv) reject(clientID, assertion string, status int, code, description string) {
	e.t.Helper()
	got, body := e.exchange(clientID, assertion)
	if got != status || body["error"] != code || !strings.Contains(body["error_description"].(string), description) {
		e.t.Fatalf("want %d %s %q, got %d %v", status, code, description, got, body)
	}
}

func (e *workloadEnv) addCredential(issuer string) store.FederatedCredential {
	e.t.Helper()
	rec := e.api(e.admin, "POST", "/api/v1/service-accounts/"+e.account.ID+"/federated-credentials", map[string]any{"name": "Deploy workflow", "issuer": issuer, "subject": workloadSubject, "conditions": map[string]string{"repository_owner": "acme"}, "scopes": []string{"api"}})
	var c store.FederatedCredential
	if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &c) != nil || c.Audience != "http://localhost:3200" {
		e.t.Fatalf("create credential: %d %s", rec.Code, rec.Body)
	}
	return c
}

func TestWorkloadIdentityFederation(t *testing.T) {
	e := newWorkloadEnv(t)
	ctx := context.Background()
	issuer := newWorkloadIssuer(t)
	key := issuer.publish("one")
	credential := e.addCredential(issuer.srv.URL)

	token := e.accept(sign(t, jose.RS256, key, "one", issuer.claims()))
	if rec := e.api(token, "GET", "/api/v1/applications", nil); rec.Code != http.StatusOK {
		t.Fatalf("issued token on the API: %d %s", rec.Code, rec.Body)
	}
	if rec := e.api(token, "GET", "/scim/v2/Users", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("issued token outside its scope: %d %s", rec.Code, rec.Body)
	}
	if rec := e.api(token, "POST", "/api/v1/users", map[string]any{"email": "x@example.com", "name": "X"}); rec.Code != http.StatusForbidden {
		t.Fatalf("issued token beyond the account's roles: %d %s", rec.Code, rec.Body)
	}

	mutated := func(change func(map[string]any)) string {
		claims := issuer.claims()
		change(claims)
		return sign(t, jose.RS256, key, "one", claims)
	}
	hour := time.Hour
	e.reject(e.account.ID, mutated(func(c map[string]any) { c["aud"] = "https://elsewhere.example.com" }), 401, "invalid_client", `does not include http://localhost:3200`)
	e.accept(mutated(func(c map[string]any) { c["aud"] = []string{"https://elsewhere.example.com", "http://localhost:3200"} }))
	e.reject(e.account.ID, mutated(func(c map[string]any) { c["sub"] = "repo:acme/infra:pull_request" }), 401, "invalid_client", `no federated credential for the subject "repo:acme/infra:pull_request"`)
	e.reject(e.account.ID, mutated(func(c map[string]any) {
		c["iat"], c["nbf"], c["exp"] = time.Now().Add(-hour).Unix(), time.Now().Add(-hour).Unix(), time.Now().Add(-10*time.Minute).Unix()
	}), 401, "invalid_client", "The token expired at")
	e.reject(e.account.ID, mutated(func(c map[string]any) { c["nbf"] = time.Now().Add(10 * time.Minute).Unix() }), 401, "invalid_client", "not valid before")
	e.reject(e.account.ID, mutated(func(c map[string]any) { c["iat"] = time.Now().Add(10 * time.Minute).Unix() }), 401, "invalid_client", "issued in the future")
	e.reject(e.account.ID, mutated(func(c map[string]any) { delete(c, "exp") }), 401, "invalid_client", "no exp or iat claim")
	e.reject(e.account.ID, mutated(func(c map[string]any) { c["repository_owner"] = "evil" }), 401, "invalid_client", "repository_owner claim")
	e.reject(e.account.ID, mutated(func(c map[string]any) { c["repository_owner"] = 7 }), 401, "invalid_client", "repository_owner claim")
	e.reject("usr_unknown", mutated(func(map[string]any) {}), 401, "invalid_client", "not the ID of a service account")

	other, err := e.st.CreateServiceAccount(ctx, store.NewServiceAccount{Name: "Other", Roles: []string{"app_admin"}})
	if err != nil {
		t.Fatal(err)
	}
	e.reject(other.ID, mutated(func(map[string]any) {}), 401, "invalid_client", "Other does not trust tokens from the issuer")
	if _, err := e.st.CreateFederatedCredential(ctx, store.FederatedCredential{ServiceAccountID: other.ID, Name: "Pull requests", Issuer: issuer.srv.URL, Subject: "repo:acme/infra:pull_request", Audience: "http://localhost:3200", Conditions: map[string]string{}, Scopes: []string{"api"}}); err != nil {
		t.Fatal(err)
	}
	e.reject(other.ID, mutated(func(map[string]any) {}), 401, "invalid_client", "Other has no federated credential for the subject")

	before := issuer.requests.Load()
	e.reject(e.account.ID, mutated(func(c map[string]any) { c["iss"] = "https://evil.example.com" }), 401, "invalid_client", `does not trust tokens from the issuer "https://evil.example.com"`)
	if issuer.requests.Load() != before {
		t.Fatal("an untrusted issuer made Halo fetch keys")
	}

	impostor, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	e.reject(e.account.ID, sign(t, jose.RS256, impostor, "one", issuer.claims()), 401, "invalid_client", "signature of client_assertion does not match")
	public, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	e.reject(e.account.ID, sign(t, jose.HS256, public, "one", issuer.claims()), 401, "invalid_client", "must be a JWT signed with RS256")
	payload, _ := json.Marshal(issuer.claims())
	unsigned := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
	e.reject(e.account.ID, unsigned, 401, "invalid_client", "must be a JWT signed with RS256")

	events, err := e.st.ListSignIns(ctx, store.SignInFilter{UserID: e.account.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(events, func(s store.SignInEvent) bool {
		return s.Result == "success" && s.Method == "federated" && s.Device == "Workload identity · Deploy workflow"
	}) ||
		!slices.ContainsFunc(events, func(s store.SignInEvent) bool { return s.Result == "failure" && strings.Contains(s.Reason, "expired") }) ||
		slices.ContainsFunc(events, func(s store.SignInEvent) bool {
			return strings.Contains(s.Reason, "signature") || strings.Contains(s.Reason, "evil.example.com") || strings.Contains(s.Reason, "pull_request")
		}) {
		t.Fatalf("sign-in log: %+v", events)
	}
	audit, err := e.st.ListAudit(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(audit, func(a store.AuditEvent) bool {
		return a.Action == "federated_credential.exchange" && a.ActorID != nil && *a.ActorID == e.account.ID && strings.Contains(a.Summary, workloadSubject)
	}) || !slices.ContainsFunc(audit, func(a store.AuditEvent) bool { return a.Action == "federated_credential.create" }) {
		t.Fatal("exchange or credential creation is not in the audit log")
	}

	var account store.ServiceAccount
	if rec := e.api(e.admin, "GET", "/api/v1/service-accounts/"+e.account.ID, nil); json.Unmarshal(rec.Body.Bytes(), &account) != nil || len(account.Keys) != 0 || len(account.FederatedCredentials) != 1 || account.FederatedCredentials[0].LastUsedAt == nil {
		t.Fatalf("issued tokens must stay out of the key list: %s", rec.Body)
	}
	if rec := e.api(e.admin, "GET", "/api/v1/api-keys", nil); rec.Body.String() != "[]\n" {
		t.Fatalf("issued tokens must stay out of the key list: %s", rec.Body)
	}

	if rec := e.api(e.admin, "PATCH", "/api/v1/service-accounts/"+e.account.ID, map[string]any{"disabled": true}); rec.Code != http.StatusOK {
		t.Fatalf("disable: %d %s", rec.Code, rec.Body)
	}
	e.reject(e.account.ID, sign(t, jose.RS256, key, "one", issuer.claims()), 401, "invalid_client", "Deployer is disabled")
	if rec := e.api(token, "GET", "/api/v1/applications", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("token of a disabled account: %d", rec.Code)
	}
	if rec := e.api(e.admin, "PATCH", "/api/v1/service-accounts/"+e.account.ID, map[string]any{"disabled": false}); rec.Code != http.StatusOK {
		t.Fatalf("enable: %d %s", rec.Code, rec.Body)
	}

	if rec := e.api(e.admin, "DELETE", "/api/v1/service-accounts/"+e.account.ID+"/federated-credentials/"+credential.ID, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete credential: %d %s", rec.Code, rec.Body)
	}
	if rec := e.api(token, "GET", "/api/v1/applications", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("token issued through a removed credential: %d", rec.Code)
	}
	e.reject(e.account.ID, sign(t, jose.RS256, key, "one", issuer.claims()), 401, "invalid_client", "does not trust tokens from the issuer")
}

func TestWorkloadIdentityKeyRotation(t *testing.T) {
	refetch := *halooidc.IssuerKeyRefetch
	t.Cleanup(func() { *halooidc.IssuerKeyRefetch = refetch })
	e := newWorkloadEnv(t)
	issuer := newWorkloadIssuer(t)
	one := issuer.publish("one")
	e.addCredential(issuer.srv.URL)
	e.accept(sign(t, jose.RS256, one, "one", issuer.claims()))

	two := issuer.publish("two")
	e.reject(e.account.ID, sign(t, jose.RS256, two, "two", issuer.claims()), 401, "invalid_client", "signed with a key that")

	*halooidc.IssuerKeyRefetch = 0
	e.accept(sign(t, jose.RS256, two, "two", issuer.claims()))
	e.reject(e.account.ID, sign(t, jose.RS256, one, "one", issuer.claims()), 401, "invalid_client", "signed with a key that")
}

func TestFederatedCredentialValidation(t *testing.T) {
	e := newWorkloadEnv(t)
	path := "/api/v1/service-accounts/" + e.account.ID + "/federated-credentials"
	valid := func() map[string]any {
		return map[string]any{"name": "Deploy", "issuer": "https://token.actions.githubusercontent.com", "subject": workloadSubject, "scopes": []string{"api"}}
	}
	for _, c := range []struct {
		change func(map[string]any)
		want   string
	}{
		{func(b map[string]any) { b["issuer"] = "http://token.actions.githubusercontent.com" }, "starting with https://"},
		{func(b map[string]any) { b["subject"] = " " }, "sub claim"},
		{func(b map[string]any) { b["scopes"] = []string{} }, "at least one scope"},
		{func(b map[string]any) { b["conditions"] = map[string]string{"repository_owner": ""} }, "claim condition"},
	} {
		body := valid()
		c.change(body)
		if rec := e.api(e.admin, "POST", path, body); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), c.want) {
			t.Fatalf("%v: %d %s", body, rec.Code, rec.Body)
		}
	}
	if rec := e.api(e.admin, "POST", path, valid()); rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	if rec := e.api(e.admin, "POST", path, valid()); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate: %d %s", rec.Code, rec.Body)
	}

	ctx := context.Background()
	limited, err := e.st.CreateUser(ctx, store.NewUser{Email: "sec@example.com", Name: "Sec", Status: "active", Roles: []string{"security_admin"}})
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := e.st.CreateSession(ctx, store.NewSession{UserID: limited.ID, Method: "passkey"})
	if err != nil {
		t.Fatal(err)
	}
	if rec := e.api(session, "POST", path, map[string]any{"name": "Other", "issuer": "https://gitlab.com", "subject": "project_path:acme/infra:ref_type:branch:ref:main", "scopes": []string{"api"}}); rec.Code != http.StatusForbidden {
		t.Fatalf("a security administrator without app_admin trusted a workload for the account: %d %s", rec.Code, rec.Body)
	}
}

func TestWorkloadTokensShareTheCredentialRateLimit(t *testing.T) {
	e := newWorkloadEnv(t)
	issuer := newWorkloadIssuer(t)
	key := issuer.publish("one")
	e.addCredential(issuer.srv.URL)
	if rec := e.api(e.admin, "PATCH", "/api/v1/service-accounts/"+e.account.ID, map[string]any{"rateLimit": 1}); rec.Code != http.StatusOK {
		t.Fatalf("rate limit: %d %s", rec.Code, rec.Body)
	}
	first := e.accept(sign(t, jose.RS256, key, "one", issuer.claims()))
	second := e.accept(sign(t, jose.RS256, key, "one", issuer.claims()))
	if rec := e.api(first, "GET", "/api/v1/applications", nil); rec.Code != http.StatusOK {
		t.Fatalf("first key: %d %s", rec.Code, rec.Body)
	}
	if rec := e.api(second, "GET", "/api/v1/applications", nil); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("a fresh exchange must not reset the credential's rate limit: %d %s", rec.Code, rec.Body)
	}
}
