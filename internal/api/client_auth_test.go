package api_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"testing"

	jose "github.com/go-jose/go-jose/v4"

	"halo/internal/store"
)

func testKeys(t *testing.T, private bool) json.RawMessage {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	jwk := jose.JSONWebKey{Key: key, KeyID: "k1", Use: "sig", Algorithm: "ES256"}
	if !private {
		jwk = jwk.Public()
	}
	raw, err := json.Marshal(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{jwk}})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestApplicationClientAuthentication(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "abe@example.com", "app_admin")
	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Bank portal", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://bank.example.com/cb"}})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/applications/" + app.ID
	keys := testKeys(t, false)

	for name, body := range map[string]map[string]any{
		"private_key_jwt without keys":  {"tokenEndpointAuthMethod": "private_key_jwt"},
		"unknown method":                {"tokenEndpointAuthMethod": "tls_client_auth", "jwks": json.RawMessage(keys)},
		"private key in the JWKS":       {"tokenEndpointAuthMethod": "private_key_jwt", "jwks": json.RawMessage(testKeys(t, true))},
		"symmetric key in the JWKS":     {"tokenEndpointAuthMethod": "private_key_jwt", "jwks": map[string]any{"keys": []any{map[string]any{"kty": "oct", "k": "c2VjcmV0c2VjcmV0c2VjcmV0c2VjcmV0"}}}},
		"empty JWKS":                    {"tokenEndpointAuthMethod": "private_key_jwt", "jwks": map[string]any{"keys": []any{}}},
		"JWKS and JWKS URL":             {"tokenEndpointAuthMethod": "private_key_jwt", "jwks": json.RawMessage(keys), "jwksUri": "https://bank.example.com/jwks.json"},
		"plain http JWKS URL":           {"tokenEndpointAuthMethod": "private_key_jwt", "jwksUri": "http://bank.example.com/jwks.json"},
		"private JWKS URL":              {"tokenEndpointAuthMethod": "private_key_jwt", "jwksUri": "https://10.0.0.8/jwks.json"},
		"profile with a client secret":  {"fapi2Profile": true},
		"JWKS that is not a key set":    {"jwks": 5},
		"private key while on a secret": {"jwks": json.RawMessage(testKeys(t, true))},
		"profile without keys":          {"fapi2Profile": true, "tokenEndpointAuthMethod": "private_key_jwt"},
	} {
		expect(t, call(t, h, admin, "PATCH", path, body), 422)
		if got, _ := st.GetApplication(ctx, app.ID); got.TokenEndpointAuthMethod != "client_secret_basic" || got.FAPI2Profile {
			t.Fatalf("%s changed the application: %+v", name, got)
		}
	}

	rec := call(t, h, admin, "PATCH", path, map[string]any{"tokenEndpointAuthMethod": "private_key_jwt", "jwks": json.RawMessage(keys)})
	expect(t, rec, 200)
	got := decode[store.Application](t, rec)
	if got.TokenEndpointAuthMethod != "private_key_jwt" || len(got.JWKS) == 0 || got.FAPI2Profile {
		t.Fatalf("private_key_jwt not saved: %+v", got)
	}
	if e := lastAudit(t, st); e.Summary != "Updated client authentication, public keys" || diff(e)["tokenEndpointAuthMethod"] != `"client_secret_basic" → "private_key_jwt"` {
		t.Fatalf("audit: %+v %v", e, diff(e))
	}

	expect(t, call(t, h, admin, "PATCH", path, map[string]any{"redirectUris": []string{"https://bank.example.com/cb", "http://localhost:3000/cb"}}), 200)
	expect(t, call(t, h, admin, "PATCH", path, map[string]any{"fapi2Profile": true}), 422)
	expect(t, call(t, h, admin, "PATCH", path, map[string]any{"redirectUris": []string{"https://bank.example.com/cb"}}), 200)

	rec = call(t, h, admin, "PATCH", path, map[string]any{"fapi2Profile": true})
	expect(t, rec, 200)
	got = decode[store.Application](t, rec)
	if !got.FAPI2Profile || !got.RequirePAR || !got.RequirePKCE || !got.RequireDPoP || got.TokenPolicy.IDTokenAlg != "ES256" {
		t.Fatalf("FAPI 2.0 profile did not apply its requirements: %+v", got)
	}
	if e := lastAudit(t, st); e.Summary != "Updated FAPI 2.0 profile, pushed authorization requirement, PKCE requirement, DPoP requirement, ID token signing algorithm" || diff(e)["fapi2Profile"] != "false → true" {
		t.Fatalf("audit: %+v %v", e, diff(e))
	}
	for _, body := range []map[string]any{
		{"requirePar": false},
		{"requirePkce": false},
		{"requireDpop": false},
		{"tokenEndpointAuthMethod": "client_secret_basic"},
		{"tokenPolicy": map[string]any{"idTokenAlg": "RS256"}},
		{"redirectUris": []string{"http://localhost:3000/cb"}},
		{"jwks": nil},
	} {
		expect(t, call(t, h, admin, "PATCH", path, body), 422)
	}
	expect(t, call(t, h, admin, "PATCH", path, map[string]any{"tokenPolicy": map[string]any{"idTokenAlg": "EdDSA"}}), 200)
	rec = call(t, h, admin, "PATCH", path, map[string]any{"fapi2Profile": false, "requirePar": false, "tokenEndpointAuthMethod": "client_secret_basic"})
	expect(t, rec, 200)
	if got := decode[store.Application](t, rec); got.FAPI2Profile || got.RequirePAR || got.TokenEndpointAuthMethod != "client_secret_basic" {
		t.Fatalf("profile not turned off: %+v", got)
	}

	service, err := st.CreateApplication(ctx, store.NewApplication{Name: "Ledger sync", Protocol: "oauth", Type: "service"})
	if err != nil {
		t.Fatal(err)
	}
	rec = call(t, h, admin, "PATCH", "/api/v1/applications/"+service.ID, map[string]any{"fapi2Profile": true, "tokenEndpointAuthMethod": "private_key_jwt", "jwksUri": "https://ledger.example.com/jwks.json"})
	expect(t, rec, 200)
	if got := decode[store.Application](t, rec); !got.FAPI2Profile || got.RequirePAR || got.RequirePKCE || !got.RequireDPoP || got.JWKSURI != "https://ledger.example.com/jwks.json" {
		t.Fatalf("service application under the profile: %+v", got)
	}

	spa, err := st.CreateApplication(ctx, store.NewApplication{Name: "Dashboard", Protocol: "oidc", Type: "spa", RedirectURIs: []string{"https://dash.example.com/cb"}})
	if err != nil {
		t.Fatal(err)
	}
	saml, err := st.CreateApplication(ctx, store.NewApplication{Name: "Slack", Protocol: "saml", Type: "web", ClientID: "https://slack.com", RedirectURIs: []string{"https://example.slack.com/sso/saml"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{spa.ID, saml.ID} {
		expect(t, call(t, h, admin, "PATCH", "/api/v1/applications/"+id, map[string]any{"tokenEndpointAuthMethod": "private_key_jwt", "jwks": json.RawMessage(keys)}), 422)
		expect(t, call(t, h, admin, "PATCH", "/api/v1/applications/"+id, map[string]any{"fapi2Profile": true}), 422)
	}
}
