package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	"halo/internal/config"
	"halo/internal/httpx"
	haloOIDC "halo/internal/oidc"
	"halo/internal/store"
	"halo/internal/testdb"
)

func TestBalanceRequiresResourceBoundTokenAndProof(t *testing.T) {
	signingPublic, signingKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/keys" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: signingPublic, KeyID: "issuer", Algorithm: "EdDSA", Use: "sig"}}})
	}))
	defer issuer.Close()

	proofPublic, proofPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	proofJWK := jose.JSONWebKey{Key: proofPublic}
	thumbprint, err := oidc.JWKThumbprint(&proofJWK)
	if err != nil {
		t.Fatal(err)
	}

	endpoint, _ := url.Parse("https://billing.example.com/balance")
	api := balanceAPI{
		audience: "https://billing.example.com",
		endpoint: endpoint,
		issuer:   issuer.URL,
		keys:     rp.NewRemoteKeySet(issuer.Client(), issuer.URL+"/oauth2/keys"),
		proofs:   make(map[proofKey]time.Time),
	}
	now := time.Now()
	newAccessToken := func(audience, scopes, jkt string, expiry time.Time) string {
		return signJWT(t, jose.JSONWebKey{Key: signingKey, KeyID: "issuer"}, nil, map[string]any{
			"iss": issuer.URL, "aud": []string{audience}, "sub": "billing-service",
			"iat": now.Unix(), "exp": expiry.Unix(), "scope": scopes, "cnf": map[string]string{"jkt": jkt},
		})
	}
	send := func(token, proof, scheme string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, endpoint.String()+"?format=json", nil)
		req.Header.Set("Authorization", scheme+" "+token)
		if proof != "" {
			req.Header.Set("DPoP", proof)
		}
		response := httptest.NewRecorder()
		api.balance(response, req)
		return response
	}
	valid := newAccessToken(api.audience, scope, thumbprint, now.Add(time.Minute))
	for _, tc := range []struct {
		name   string
		token  string
		hash   string
		target string
		scheme string
		want   int
	}{
		{"valid", valid, valid, endpoint.String(), "DPoP", http.StatusOK},
		{"wrong audience", newAccessToken("https://other.example.com", scope, thumbprint, now.Add(time.Minute)), "", endpoint.String(), "DPoP", http.StatusUnauthorized},
		{"expired", newAccessToken(api.audience, scope, thumbprint, now.Add(-time.Minute)), "", endpoint.String(), "DPoP", http.StatusUnauthorized},
		{"wrong key binding", newAccessToken(api.audience, scope, "different-thumbprint", now.Add(time.Minute)), "", endpoint.String(), "DPoP", http.StatusUnauthorized},
		{"wrong token hash", valid, "different-token", endpoint.String(), "DPoP", http.StatusUnauthorized},
		{"wrong request path", valid, valid, "https://billing.example.com/other", "DPoP", http.StatusUnauthorized},
		{"insufficient scope", newAccessToken(api.audience, "billing.write", thumbprint, now.Add(time.Minute)), "", endpoint.String(), "DPoP", http.StatusForbidden},
		{"bearer scheme", valid, valid, endpoint.String(), "Bearer", http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.hash == "" {
				tc.hash = tc.token
			}
			hash := sha256.Sum256([]byte(tc.hash))
			options := (&jose.SignerOptions{}).WithType("dpop+jwt").WithHeader("jwk", proofJWK)
			proof := signJWT(t, proofPrivate, options, map[string]any{
				"jti": tc.name, "htm": http.MethodGet, "htu": tc.target, "iat": now.Unix(), "ath": base64.RawURLEncoding.EncodeToString(hash[:]),
			})
			if result := send(tc.token, proof, tc.scheme); result.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", result.Code, tc.want, result.Body.String())
			}
			if tc.want == http.StatusOK && send(tc.token, proof, tc.scheme).Code != http.StatusUnauthorized {
				t.Fatal("reused proof accepted")
			}
		})
	}
	if send(valid, "", "DPoP").Code != http.StatusUnauthorized {
		t.Fatal("request without a proof accepted")
	}
}

func TestBalanceWithHaloAccessToken(t *testing.T) {
	ctx := t.Context()
	st := testdb.New(t)
	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Billing service", Protocol: "oauth", Type: "service"})
	if err != nil {
		t.Fatal(err)
	}
	app.RequireDPoP = true
	if err := st.UpdateApplication(ctx, app); err != nil {
		t.Fatal(err)
	}
	_, secret, err := st.AddClientSecret(ctx, app.ID, "Test", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	audience := "http://localhost:9002"
	resource, err := st.SaveAPIResource(ctx, store.APIResource{
		Name: "Billing", Identifier: audience, AccessTokenTTL: 300, Scopes: []store.APIScope{{Name: scope}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetAPIResourceGrant(ctx, resource.ID, app.ID, []string{scope}); err != nil {
		t.Fatal(err)
	}
	var provider http.Handler
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { provider.ServeHTTP(w, r) }))
	defer issuer.Close()
	publicURL, _ := url.Parse(issuer.URL)
	provider, err = haloOIDC.New(httpx.Deps{
		Config: config.Config{PublicURL: publicURL, Dev: true}, Store: st,
	})
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	options := (&jose.SignerOptions{}).WithType("dpop+jwt").WithHeader("jwk", jose.JSONWebKey{Key: publicKey})
	endpoint, _ := url.Parse(audience + "/balance")
	tokenURL := issuer.URL + "/oauth2/token"
	form := url.Values{"grant_type": {"client_credentials"}, "scope": {scope}, "resource": {audience}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth(app.ClientID, secret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("DPoP", signJWT(t, privateKey, options, map[string]any{
		"jti": "token-proof", "htm": req.Method, "htu": tokenURL, "iat": time.Now().Unix(),
	}))
	response, err := issuer.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var tokens struct {
		AccessToken string `json:"access_token"`
		Type        string `json:"token_type"`
		Error       string `json:"error_description"`
	}
	if err := json.NewDecoder(response.Body).Decode(&tokens); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || tokens.Type != "DPoP" || tokens.AccessToken == "" {
		t.Fatalf("Halo token response was %d (%s): %s", response.StatusCode, tokens.Type, tokens.Error)
	}

	req = httptest.NewRequest(http.MethodGet, endpoint.String(), nil)
	req.Header.Set("Authorization", "DPoP "+tokens.AccessToken)
	hash := sha256.Sum256([]byte(tokens.AccessToken))
	req.Header.Set("DPoP", signJWT(t, privateKey, options, map[string]any{
		"jti": "balance-proof", "htm": req.Method, "htu": endpoint.String(), "iat": time.Now().Unix(),
		"ath": base64.RawURLEncoding.EncodeToString(hash[:]),
	}))
	api := balanceAPI{audience: audience, endpoint: endpoint, issuer: issuer.URL,
		keys: rp.NewRemoteKeySet(issuer.Client(), issuer.URL+"/oauth2/keys"), proofs: make(map[proofKey]time.Time)}
	recorder := httptest.NewRecorder()
	api.balance(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("API rejected Halo JWT: %d: %s", recorder.Code, recorder.Body.String())
	}
}

func signJWT(t *testing.T, key any, options *jose.SignerOptions, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: key}, options)
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return token
}
