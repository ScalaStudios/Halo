package oidc_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	halooidc "halo/internal/oidc"
	"halo/internal/secret"
	"halo/internal/store"
)

type assertionSigner struct {
	t   *testing.T
	key any
	alg jose.SignatureAlgorithm
	kid string
}

func newAssertionSigner(t *testing.T, kid string) assertionSigner {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return assertionSigner{t, key, jose.ES256, kid}
}

func (a assertionSigner) jwks() json.RawMessage {
	return keySet(a.t, a)
}

func keySet(t *testing.T, signers ...assertionSigner) json.RawMessage {
	set := jose.JSONWebKeySet{}
	for _, a := range signers {
		key := jose.JSONWebKey{Key: a.key, KeyID: a.kid, Use: "sig", Algorithm: string(a.alg)}
		set.Keys = append(set.Keys, key.Public())
	}
	raw, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func (a assertionSigner) sign(clientID string, aud any, edit func(map[string]any)) string {
	now := time.Now()
	claims := map[string]any{"iss": clientID, "sub": clientID, "aud": aud, "jti": secret.Token(12), "iat": now.Unix(), "exp": now.Add(time.Minute).Unix()}
	if edit != nil {
		edit(claims)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: a.alg, Key: a.key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader(jose.HeaderKey("kid"), a.kid))
	if err != nil {
		a.t.Fatal(err)
	}
	token, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		a.t.Fatal(err)
	}
	return token
}

func asserted(form url.Values, assertion string) url.Values {
	out := url.Values{"client_assertion_type": {oidc.ClientAssertionTypeJWTAssertion}, "client_assertion": {assertion}}
	for k, v := range form {
		out[k] = v
	}
	return out
}

func (h *harness) useKeys(app store.Application, signer assertionSigner, fapi bool) store.Application {
	h.t.Helper()
	app.TokenEndpointAuthMethod, app.JWKS, app.FAPI2Profile = halooidc.PrivateKeyJWT, signer.jwks(), fapi
	if fapi {
		app.TokenPolicy.IDTokenAlg = "ES256"
	}
	if err := h.st.UpdateApplication(context.Background(), app); err != nil {
		h.t.Fatal(err)
	}
	return app
}

func codeGrant(code string) url.Values {
	return url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callback}, "code_verifier": {pkceSecret}}
}

func TestPrivateKeyJWT(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	_, session := h.session(h.user, "passkey")
	signer := newAssertionSigner(t, "key-1")
	issuer, id := h.srv.URL, h.app.ClientID

	if r, _ := h.do(http.MethodPost, "/oauth2/introspect", asserted(url.Values{"token": {"x"}}, signer.sign(id, issuer, nil))); r.status != http.StatusUnauthorized || r.body["error"] != "invalid_client" {
		t.Fatalf("assertion from an application that uses a client secret: %d %v", r.status, r.body)
	}
	h.useKeys(h.app, signer, false)

	code := h.code(mustDo(h, http.MethodGet, "/oauth2/authorize?"+authorization(id, true).Encode()), session)
	if r := h.post("/oauth2/token", codeGrant(code), id, h.secret); r.status != http.StatusUnauthorized || r.body["error"] != "invalid_client" {
		t.Fatalf("client secret of a private_key_jwt application: %d %v", r.status, r.body)
	}
	if r, _ := h.do(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callback}, "code_verifier": {pkceSecret}, "client_id": {id}, "client_secret": {h.secret}}); r.status != http.StatusUnauthorized {
		t.Fatalf("client_secret_post of a private_key_jwt application: %d %v", r.status, r.body)
	}

	hmac, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS256, Key: []byte(h.secret)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	hs256, _ := jwt.Signed(hmac).Claims(map[string]any{"iss": id, "sub": id, "aud": issuer, "jti": "hs", "exp": time.Now().Add(time.Minute).Unix()}).Serialize()
	unsigned := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"`+id+`","sub":"`+id+`","aud":"`+issuer+`","jti":"none","exp":`+jsonNumber(time.Now().Add(time.Minute).Unix())+`}`)) + "."
	other := newAssertionSigner(t, "key-1")
	for name, assertion := range map[string]string{
		"wrong aud":            signer.sign(id, "https://other.example.com", nil),
		"expired":              signer.sign(id, issuer, func(c map[string]any) { c["exp"] = time.Now().Add(-2 * time.Minute).Unix() }),
		"no exp":               signer.sign(id, issuer, func(c map[string]any) { delete(c, "exp") }),
		"exp too far ahead":    signer.sign(id, issuer, func(c map[string]any) { c["exp"] = time.Now().Add(2 * time.Hour).Unix() }),
		"issued in the future": signer.sign(id, issuer, func(c map[string]any) { c["iat"] = time.Now().Add(10 * time.Minute).Unix() }),
		"not yet valid":        signer.sign(id, issuer, func(c map[string]any) { c["nbf"] = time.Now().Add(10 * time.Minute).Unix() }),
		"no jti":               signer.sign(id, issuer, func(c map[string]any) { delete(c, "jti") }),
		"sub differs":          signer.sign(id, issuer, func(c map[string]any) { c["sub"] = "hl_other" }),
		"unknown iss":          signer.sign("hl_other", issuer, nil),
		"HS256":                hs256,
		"none":                 unsigned,
		"unregistered key":     other.sign(id, issuer, nil),
		"not a JWT":            "not-a-jwt",
	} {
		if r, _ := h.do(http.MethodPost, "/oauth2/token", asserted(codeGrant(code), assertion)); r.status != http.StatusUnauthorized || r.body["error"] != "invalid_client" {
			t.Errorf("%s: %d %v", name, r.status, r.body)
		}
	}
	if r, _ := h.do(http.MethodPost, "/oauth2/token", asserted(codeGrant(code), signer.sign(id, issuer, nil)), basicAuth(id, h.secret)); r.status != http.StatusBadRequest || r.body["error"] != "invalid_request" {
		t.Errorf("assertion together with Basic authentication: %d %v", r.status, r.body)
	}
	mismatch := asserted(codeGrant(code), signer.sign(id, issuer, nil))
	mismatch.Set("client_id", "hl_other")
	if r, _ := h.do(http.MethodPost, "/oauth2/token", mismatch); r.status != http.StatusUnauthorized {
		t.Errorf("client_id other than iss: %d %v", r.status, r.body)
	}
	wrongType := asserted(codeGrant(code), signer.sign(id, issuer, nil))
	wrongType.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:saml2-bearer")
	if r, _ := h.do(http.MethodPost, "/oauth2/token", wrongType); r.status != http.StatusUnauthorized {
		t.Errorf("another client_assertion_type: %d %v", r.status, r.body)
	}

	valid := signer.sign(id, issuer, nil)
	tokens, _ := h.do(http.MethodPost, "/oauth2/token", asserted(codeGrant(code), valid))
	if tokens.status != http.StatusOK || tokens.body["access_token"] == nil {
		t.Fatalf("code exchange with a valid assertion: %d %v", tokens.status, tokens.body)
	}
	if r, _ := h.do(http.MethodPost, "/oauth2/token", asserted(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tokens.body["refresh_token"].(string)}}, valid)); r.status != http.StatusUnauthorized || !strings.Contains(r.body["error_description"].(string), "already used") {
		t.Fatalf("replayed jti: %d %v", r.status, r.body)
	}
	refreshed, _ := h.do(http.MethodPost, "/oauth2/token", asserted(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tokens.body["refresh_token"].(string)}}, signer.sign(id, issuer+"/oauth2/token", nil)))
	if refreshed.status != http.StatusOK {
		t.Fatalf("refresh with the token endpoint as aud: %d %v", refreshed.status, refreshed.body)
	}
	access := refreshed.body["access_token"].(string)
	if r, _ := h.do(http.MethodPost, "/oauth2/introspect", url.Values{"token": {access}}, basicAuth(id, h.secret)); r.status != http.StatusUnauthorized {
		t.Fatalf("introspection with the client secret: %d %v", r.status, r.body)
	}
	if r, _ := h.do(http.MethodPost, "/oauth2/introspect", asserted(url.Values{"token": {access}}, signer.sign(id, []string{"https://other.example.com", issuer + "/oauth2/introspect"}, nil))); r.status != http.StatusOK || r.body["active"] != true {
		t.Fatalf("introspection with an assertion: %d %v", r.status, r.body)
	}
	if r, _ := h.do(http.MethodPost, "/oauth2/revoke", asserted(url.Values{"token": {access}}, signer.sign(id, issuer, nil))); r.status != http.StatusOK {
		t.Fatalf("revocation with an assertion: %d %v", r.status, r.body)
	}
	if r, _ := h.do(http.MethodPost, "/oauth2/par", asserted(authorization(id, true), signer.sign(id, issuer+"/oauth2/par", nil))); r.status != http.StatusCreated {
		t.Fatalf("pushed request with an assertion: %d %v", r.status, r.body)
	}
	if r, _ := h.do(http.MethodPost, "/oauth2/par", authorization(id, true), basicAuth(id, h.secret)); r.status != http.StatusBadRequest || r.body["error"] != "unauthorized_client" {
		t.Fatalf("pushed request with the client secret: %d %v", r.status, r.body)
	}

	rotated := newAssertionSigner(t, "key-2")
	var served atomic.Value
	served.Store(signer.jwks())
	var fetches atomic.Int32
	keys := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(served.Load().(json.RawMessage))
	}))
	t.Cleanup(keys.Close)
	prev, refetch := halooidc.JWKSClient, *halooidc.IssuerKeyRefetch
	halooidc.JWKSClient = keys.Client()
	t.Cleanup(func() { halooidc.JWKSClient, *halooidc.IssuerKeyRefetch = prev, refetch })
	remote := h.app
	remote.TokenEndpointAuthMethod, remote.JWKS, remote.JWKSURI = halooidc.PrivateKeyJWT, nil, keys.URL+"/jwks.json"
	if err := h.st.UpdateApplication(ctx, remote); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if r, _ := h.do(http.MethodPost, "/oauth2/par", asserted(authorization(id, true), signer.sign(id, issuer, nil))); r.status != http.StatusCreated {
			t.Fatalf("assertion checked against jwks_uri: %d %v", r.status, r.body)
		}
	}
	if fetches.Load() != 1 {
		t.Fatalf("jwks_uri fetched %d times, want once", fetches.Load())
	}
	served.Store(rotated.jwks())
	*halooidc.IssuerKeyRefetch = 0
	if r, _ := h.do(http.MethodPost, "/oauth2/par", asserted(authorization(id, true), rotated.sign(id, issuer, nil))); r.status != http.StatusCreated {
		t.Fatalf("assertion signed with a rotated key: %d %v", r.status, r.body)
	}
	if r, _ := h.do(http.MethodPost, "/oauth2/par", asserted(authorization(id, true), signer.sign(id, issuer, nil))); r.status != http.StatusUnauthorized {
		t.Fatalf("assertion signed with a key no longer published: %d %v", r.status, r.body)
	}

	svc, err := h.st.CreateApplication(ctx, store.NewApplication{Name: "Billing worker", Protocol: "oauth", Type: "service", Scopes: []string{"openid"}})
	if err != nil {
		t.Fatal(err)
	}
	h.useKeys(svc, signer, false)
	for _, form := range []url.Values{{"grant_type": {"client_credentials"}, "client_id": {svc.ClientID}}, {"grant_type": {"client_credentials"}}} {
		if r, _ := h.do(http.MethodPost, "/oauth2/token", asserted(form, signer.sign(svc.ClientID, issuer, nil))); r.status != http.StatusOK || r.body["access_token"] == nil {
			t.Fatalf("client credentials with an assertion %v: %d %v", form, r.status, r.body)
		}
	}
}

func jsonNumber(n int64) string {
	raw, _ := json.Marshal(n)
	return string(raw)
}

func mustDo(h *harness, method, path string) result {
	h.t.Helper()
	r, _ := h.do(method, path, nil)
	return r
}

func TestFAPI2Profile(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	_, session := h.session(h.user, "passkey")
	signer := newAssertionSigner(t, "key-1")
	issuer, id := h.srv.URL, h.app.ClientID
	app := h.useKeys(h.app, signer, true)
	if app.RequirePAR || app.RequirePKCE || app.RequireDPoP {
		t.Fatal("the profile must hold on its own, without the separate requirements")
	}

	if r, _ := h.do(http.MethodGet, "/oauth2/authorize?"+authorization(id, true).Encode(), nil); r.status != http.StatusFound || !strings.HasPrefix(r.location, callback+"?") || !strings.Contains(r.location, "error=invalid_request") {
		t.Fatalf("authorization request without PAR: %d %q", r.status, r.location)
	}
	if r, _ := h.do(http.MethodPost, "/oauth2/par", asserted(authorization(id, false), signer.sign(id, issuer, nil))); r.status != http.StatusBadRequest || r.body["error"] != "invalid_request" {
		t.Fatalf("pushed request without PKCE: %d %v", r.status, r.body)
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rs256 := assertionSigner{t, rsaKey, jose.RS256, "rsa-1"}
	withRSA := h.app
	withRSA.TokenEndpointAuthMethod, withRSA.FAPI2Profile, withRSA.TokenPolicy.IDTokenAlg = halooidc.PrivateKeyJWT, true, "ES256"
	withRSA.JWKS = keySet(t, signer, rs256)
	if err := h.st.UpdateApplication(ctx, withRSA); err != nil {
		t.Fatal(err)
	}
	for name, assertion := range map[string]string{
		"token endpoint as aud": signer.sign(id, issuer+"/oauth2/token", nil),
		"PAR endpoint as aud":   signer.sign(id, issuer+"/oauth2/par", nil),
		"aud as an array":       signer.sign(id, []string{issuer}, nil),
		"RS256":                 rs256.sign(id, issuer, nil),
	} {
		if r, _ := h.do(http.MethodPost, "/oauth2/par", asserted(authorization(id, true), assertion)); r.status != http.StatusUnauthorized || r.body["error"] != "invalid_client" {
			t.Errorf("%s under the FAPI 2.0 profile: %d %v", name, r.status, r.body)
		}
	}
	if r, _ := h.do(http.MethodPost, "/oauth2/par", authorization(id, true), basicAuth(id, h.secret)); r.status == http.StatusCreated {
		t.Fatalf("client secret under the FAPI 2.0 profile: %d %v", r.status, r.body)
	}

	dpop := newDPoPSigner(t)
	pushed, _ := h.do(http.MethodPost, "/oauth2/par", asserted(authorization(id, true), signer.sign(id, issuer, nil)), header("DPoP", dpop.proof(http.MethodPost, issuer+"/oauth2/par", "", nil)))
	if pushed.status != http.StatusCreated {
		t.Fatalf("pushed request under the FAPI 2.0 profile: %d %v", pushed.status, pushed.body)
	}
	authorize, _ := h.do(http.MethodGet, "/oauth2/authorize?"+url.Values{"client_id": {id}, "request_uri": {pushed.body["request_uri"].(string)}}.Encode(), nil)
	requestID := strings.TrimPrefix(authorize.location, "/sign-in?authRequest=")
	code := h.code(authorize, session)
	a, err := h.st.GetAuthRequestByCode(ctx, code)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != requestID || time.Until(a.ExpiresAt) > time.Minute {
		t.Fatalf("authorization code lives until %s, want at most a minute", a.ExpiresAt)
	}
	if r, _ := h.do(http.MethodPost, "/oauth2/token", asserted(codeGrant(code), signer.sign(id, issuer, nil))); r.status != http.StatusBadRequest || r.body["error"] != "invalid_dpop_proof" && r.body["error"] != "invalid_grant" {
		t.Fatalf("code exchange without DPoP under the FAPI 2.0 profile: %d %v", r.status, r.body)
	}
	tokens, _ := h.do(http.MethodPost, "/oauth2/token", asserted(codeGrant(code), signer.sign(id, issuer, nil)), header("DPoP", dpop.proof(http.MethodPost, issuer+"/oauth2/token", "", nil)))
	if tokens.status != http.StatusOK || tokens.body["token_type"] != "DPoP" {
		t.Fatalf("code exchange under the FAPI 2.0 profile: %d %v", tokens.status, tokens.body)
	}
}

func TestDPoPCodeBinding(t *testing.T) {
	h := newHarness(t, nil)
	_, session := h.session(h.user, "passkey")
	issuer, id := h.srv.URL, h.app.ClientID
	bound, other := newDPoPSigner(t), newDPoPSigner(t)
	exchange := func(code string, key *dpopSigner) result {
		opts := []func(*http.Request){basicAuth(id, h.secret)}
		if key != nil {
			opts = append(opts, header("DPoP", key.proof(http.MethodPost, issuer+"/oauth2/token", "", nil)))
		}
		r, _ := h.do(http.MethodPost, "/oauth2/token", codeGrant(code), opts...)
		return r
	}

	pushed, _ := h.do(http.MethodPost, "/oauth2/par", authorization(id, true), basicAuth(id, h.secret), header("DPoP", bound.proof(http.MethodPost, issuer+"/oauth2/par", "", nil)))
	if pushed.status != http.StatusCreated {
		t.Fatalf("pushed request with a DPoP proof: %d %v", pushed.status, pushed.body)
	}
	code := h.code(mustDo(h, http.MethodGet, "/oauth2/authorize?"+url.Values{"client_id": {id}, "request_uri": {pushed.body["request_uri"].(string)}}.Encode()), session)
	for name, key := range map[string]*dpopSigner{"no proof": nil, "another key": &other} {
		if r := exchange(code, key); r.status != http.StatusBadRequest || r.body["error"] != "invalid_grant" {
			t.Errorf("code bound at PAR, %s: %d %v", name, r.status, r.body)
		}
	}
	if r := exchange(code, &bound); r.status != http.StatusOK || r.body["token_type"] != "DPoP" {
		t.Fatalf("code bound at PAR, same key: %d %v", r.status, r.body)
	}

	params := authorization(id, true)
	params.Set("dpop_jkt", bound.thumbprint())
	code = h.code(mustDo(h, http.MethodGet, "/oauth2/authorize?"+params.Encode()), session)
	if r := exchange(code, &other); r.status != http.StatusBadRequest || r.body["error"] != "invalid_grant" {
		t.Fatalf("code bound with dpop_jkt, another key: %d %v", r.status, r.body)
	}
	if r := exchange(code, &bound); r.status != http.StatusOK {
		t.Fatalf("code bound with dpop_jkt, same key: %d %v", r.status, r.body)
	}

	mismatched := authorization(id, true)
	mismatched.Set("dpop_jkt", other.thumbprint())
	if r, _ := h.do(http.MethodPost, "/oauth2/par", mismatched, basicAuth(id, h.secret), header("DPoP", bound.proof(http.MethodPost, issuer+"/oauth2/par", "", nil))); r.status != http.StatusBadRequest || r.body["error"] != "invalid_dpop_proof" {
		t.Fatalf("dpop_jkt of another key than the PAR proof: %d %v", r.status, r.body)
	}
	malformed := authorization(id, true)
	malformed.Set("dpop_jkt", "not-a-thumbprint")
	if r, _ := h.do(http.MethodPost, "/oauth2/par", malformed, basicAuth(id, h.secret)); r.status != http.StatusBadRequest || r.body["error"] != "invalid_request" {
		t.Fatalf("malformed dpop_jkt at PAR: %d %v", r.status, r.body)
	}
	if r, _ := h.do(http.MethodGet, "/oauth2/authorize?"+malformed.Encode(), nil); r.status != http.StatusFound || !strings.Contains(r.location, "error=invalid_request") {
		t.Fatalf("malformed dpop_jkt at authorize: %d %q", r.status, r.location)
	}
	if r, _ := h.do(http.MethodPost, "/oauth2/par", authorization(id, true), basicAuth(id, h.secret), header("DPoP", bound.proof(http.MethodPost, issuer+"/oauth2/token", "", nil))); r.status != http.StatusBadRequest || r.body["error"] != "invalid_dpop_proof" {
		t.Fatalf("PAR proof for another htu: %d %v", r.status, r.body)
	}
}

func TestAuthorizationResponseIssuer(t *testing.T) {
	h := newHarness(t, nil)
	_, session := h.session(h.user, "passkey")
	issuer, id := h.srv.URL, h.app.ClientID
	callbackFor := func(params url.Values) result {
		authorize, _ := h.do(http.MethodGet, "/oauth2/authorize?"+params.Encode(), nil)
		requestID, ok := strings.CutPrefix(authorize.location, "/sign-in?authRequest=")
		if !ok || strings.Contains(authorize.location, "iss=") {
			t.Fatalf("authorize: %d %q", authorize.status, authorize.location)
		}
		sess, err := h.st.SessionByToken(context.Background(), session)
		if err != nil {
			t.Fatal(err)
		}
		if err := h.st.CompleteAuthRequest(context.Background(), requestID, h.user.ID, sess.ID, []string{"hwk"}, time.Now(), ""); err != nil {
			t.Fatal(err)
		}
		req, _ := http.NewRequest(http.MethodGet, h.srv.URL+store.AuthorizeCallbackPath(requestID), nil)
		cookie(session)(req)
		res, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		page, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return result{status: res.StatusCode, location: res.Header.Get("Location"), body: map[string]any{"page": string(page), "cache": res.Header.Get("Cache-Control")}}
	}

	query := callbackFor(authorization(id, true))
	location, _ := url.Parse(query.location)
	if query.status != http.StatusFound || location.Query().Get("iss") != issuer || location.Query().Get("code") == "" || location.Query().Get("state") != "st-fapi" {
		t.Fatalf("query response: %d %q", query.status, query.location)
	}

	fragment := authorization(id, true)
	fragment.Set("response_mode", "fragment")
	frag := callbackFor(fragment)
	location, _ = url.Parse(frag.location)
	values, _ := url.ParseQuery(location.Fragment)
	if frag.status != http.StatusFound || location.RawQuery != "" || values.Get("iss") != issuer || values.Get("code") == "" || values.Get("state") != "st-fapi" {
		t.Fatalf("fragment response: %d %q", frag.status, frag.location)
	}

	posted := authorization(id, true)
	posted.Set("response_mode", "form_post")
	form := callbackFor(posted)
	page := form.body["page"].(string)
	if form.status != http.StatusOK || form.body["cache"] != "no-store" || !strings.Contains(page, `action="`+callback+`"`) || !strings.Contains(page, `name="iss" value="`+issuer+`"`) || !strings.Contains(page, `name="state" value="st-fapi"`) {
		t.Fatalf("form_post response: %d %v", form.status, form.body)
	}
	start := strings.Index(page, `name="code" value="`) + len(`name="code" value="`)
	code := page[start : start+strings.Index(page[start:], `"`)]
	if r := h.post("/oauth2/token", codeGrant(code), id, h.secret); r.status != http.StatusOK {
		t.Fatalf("code from a form_post response: %d %v", r.status, r.body)
	}

	silent := authorization(id, true)
	silent.Set("prompt", "none")
	for _, mode := range []string{"", "form_post"} {
		silent.Set("response_mode", mode)
		r, _ := h.do(http.MethodGet, "/oauth2/authorize?"+silent.Encode(), nil)
		location, _ := url.Parse(r.location)
		if r.status != http.StatusFound || location.Query().Get("error") != "login_required" || location.Query().Get("iss") != issuer {
			t.Errorf("error response in mode %q: %d %q", mode, r.status, r.location)
		}
	}

	unregistered := authorization(id, true)
	unregistered.Set("redirect_uri", "https://evil.example.com/cb")
	if r, _ := h.do(http.MethodGet, "/oauth2/authorize?"+unregistered.Encode(), nil); r.location != "" {
		t.Fatalf("unregistered redirect URI: %d %q", r.status, r.location)
	}
}
