package oidc_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	"halo/internal/store"
)

func TestSigningAlgorithms(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	billing, err := h.st.SaveAPIResource(ctx, store.APIResource{Name: "Billing", Identifier: "https://billing.example.com", AccessTokenTTL: 300, Scopes: []store.APIScope{{Name: "billing:read"}}})
	must(err)
	ledger, err := h.st.SaveAPIResource(ctx, store.APIResource{Name: "Ledger", Identifier: "https://ledger.example.com", AccessTokenTTL: 300, Scopes: []store.APIScope{{Name: "ledger:read"}}})
	must(err)
	must(h.st.SetAPIResourceGrant(ctx, billing.ID, h.app.ID, []string{"billing:read"}))
	must(h.st.SetAPIResourceGrant(ctx, ledger.ID, h.app.ID, []string{"ledger:read"}))
	if h.app.TokenPolicy.IDTokenAlg != "RS256" || billing.AccessTokenAlg != "RS256" {
		t.Fatalf("defaults: application %s, API %s", h.app.TokenPolicy.IDTokenAlg, billing.AccessTokenAlg)
	}
	_, sessionToken := h.session(h.user, "passkey")
	signIn := func(scope string) result {
		t.Helper()
		code := h.silent(sessionToken, url.Values{"scope": {scope}}).Query().Get("code")
		return h.post("/oauth2/token", url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callback}, "code_verifier": {pkceSecret}}, h.app.ClientID, h.secret)
	}
	published := func() jose.JSONWebKeySet {
		t.Helper()
		res, err := http.Get(h.srv.URL + "/oauth2/keys")
		must(err)
		defer res.Body.Close()
		var set jose.JSONWebKeySet
		must(json.NewDecoder(res.Body).Decode(&set))
		return set
	}
	verify := func(name, token string, want jose.SignatureAlgorithm) {
		t.Helper()
		set := published()
		jws, err := jose.ParseSigned(token, []jose.SignatureAlgorithm{jose.RS256, jose.ES256, jose.EdDSA})
		must(err)
		header := jws.Signatures[0].Header
		keys := set.Key(header.KeyID)
		if header.Algorithm != string(want) || len(keys) != 1 || keys[0].Algorithm != string(want) {
			t.Fatalf("%s: header alg %s, published %+v, want %s", name, header.Algorithm, keys, want)
		}
		if _, err := jws.Verify(keys[0].Key); err != nil {
			t.Fatalf("%s does not verify with the published key: %v", name, err)
		}
	}
	verifier := rp.NewIDTokenVerifier(h.srv.URL, h.app.ClientID, rp.NewRemoteKeySet(http.DefaultClient, h.srv.URL+"/oauth2/keys"), rp.WithSupportedSigningAlgorithms("RS256", "ES256", "EdDSA"))

	r := signIn("openid billing:read")
	first, _ := r.body["id_token"].(string)
	verify("default ID token", first, jose.RS256)
	verify("default access token", r.body["access_token"].(string), jose.RS256)
	if keys := published().Keys; len(keys) != 1 {
		t.Fatalf("only RS256 is published until another algorithm is used: %+v", keys)
	}

	var last string
	for _, c := range []struct{ id, access jose.SignatureAlgorithm }{{jose.ES256, jose.EdDSA}, {jose.EdDSA, jose.ES256}, {jose.EdDSA, jose.EdDSA}} {
		h.app.TokenPolicy.IDTokenAlg = string(c.id)
		must(h.st.UpdateApplication(ctx, h.app))
		billing.AccessTokenAlg = string(c.access)
		_, err := h.st.SaveAPIResource(ctx, billing)
		must(err)
		r := signIn("openid billing:read")
		idToken, _ := r.body["id_token"].(string)
		access, _ := r.body["access_token"].(string)
		if r.status != http.StatusOK {
			t.Fatalf("%s/%s sign in: %d %v", c.id, c.access, r.status, r.body)
		}
		verify(string(c.id)+" ID token", idToken, c.id)
		verify(string(c.access)+" access token", access, c.access)
		if _, err := rp.VerifyTokens[*oidc.IDTokenClaims](ctx, access, idToken, verifier); err != nil {
			t.Fatalf("%s ID token with at_hash of a %s access token: %v", c.id, c.access, err)
		}
		req, _ := http.NewRequest(http.MethodGet, h.srv.URL+"/oauth2/userinfo", nil)
		req.Header.Set("Authorization", "Bearer "+access)
		res, err := http.DefaultClient.Do(req)
		must(err)
		var info map[string]any
		_ = json.NewDecoder(res.Body).Decode(&info)
		res.Body.Close()
		if res.StatusCode != http.StatusOK || info["sub"] != h.user.ID {
			t.Fatalf("userinfo with a %s access token: %d %v", c.access, res.StatusCode, info)
		}
		last = idToken
	}
	verify("RS256 ID token issued before the switch", first, jose.RS256)

	if r := signIn("openid billing:read ledger:read"); r.status != http.StatusBadRequest || r.body["error"] != "invalid_target" {
		t.Fatalf("one token for APIs with different algorithms: %d %v", r.status, r.body)
	}

	q := url.Values{"id_token_hint": {last}, "post_logout_redirect_uri": {loggedOut}}
	if status, location := h.get("/oauth2/logout?"+q.Encode(), sessionToken); status != http.StatusFound || location.Query().Get("next") != loggedOut {
		t.Fatalf("EdDSA id_token_hint: %d %s", status, location)
	}
}
