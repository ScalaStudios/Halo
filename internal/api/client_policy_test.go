package api_test

import (
	"net/http"
	"testing"

	"halo/internal/store"
)

func TestOrganizationClientPolicy(t *testing.T) {
	st, h := registrationStack(t, "open")
	_, global := person(t, st, "global@example.com", "global_admin")
	_, security := person(t, st, "security@example.com", "security_admin")
	_, admin := person(t, st, "apps@example.com", "app_admin")

	policy := map[string]any{"clientPolicies": []map[string]any{{
		"applicationType": "web", "requirePar": true, "requirePkce": true, "requireDpop": true,
		"allowedAuthMethods": []string{"private_key_jwt"}, "maxAccessTokenTtl": 600,
	}}}
	expect(t, call(t, h, security, "PATCH", "/api/v1/settings", policy), http.StatusForbidden)
	expect(t, call(t, h, global, "PATCH", "/api/v1/settings", policy), http.StatusOK)

	body := map[string]any{"name": "Portal", "type": "web", "protocol": "oidc", "redirectUris": []string{toolCallback}}
	expect(t, call(t, h, admin, "POST", "/api/v1/applications", body), http.StatusUnprocessableEntity)
	spa := map[string]any{"name": "Dashboard", "type": "spa", "protocol": "oidc", "redirectUris": []string{toolCallback}}
	expect(t, call(t, h, admin, "POST", "/api/v1/applications", spa), http.StatusCreated)

	body["tokenEndpointAuthMethod"] = "private_key_jwt"
	body["jwks"] = testKeys(t, false)
	rec := call(t, h, admin, "POST", "/api/v1/applications", body)
	expect(t, rec, http.StatusCreated)
	created := decode[struct {
		Application  store.Application `json:"application"`
		ClientSecret string            `json:"clientSecret"`
	}](t, rec)
	app := created.Application
	if app.ID == "" || !app.RequirePAR || !app.RequirePKCE || !app.RequireDPoP || app.TokenPolicy.AccessTokenTTL != 600 || created.ClientSecret != "" {
		t.Fatalf("unexpected client configuration: %+v", app)
	}
	path := "/api/v1/applications/" + app.ID
	expect(t, call(t, h, admin, "PATCH", path, map[string]any{"requireDpop": false}), http.StatusUnprocessableEntity)
	expect(t, call(t, h, admin, "PATCH", path, map[string]any{"tokenPolicy": map[string]any{"accessTokenTtl": 601}}), http.StatusUnprocessableEntity)

	metadata := map[string]any{"redirect_uris": []string{toolCallback}}
	expect(t, oauth(t, h, "POST", "/oauth2/register", "", metadata), http.StatusBadRequest)
	metadata["token_endpoint_auth_method"] = "private_key_jwt"
	metadata["jwks"] = testKeys(t, false)
	metadata["require_pushed_authorization_requests"] = true
	metadata["dpop_bound_access_tokens"] = true
	rec = oauth(t, h, "POST", "/oauth2/register", "", metadata)
	expect(t, rec, http.StatusCreated)
	client := decode[registration](t, rec)

	expect(t, call(t, h, global, "PATCH", "/api/v1/settings", map[string]any{
		"clientPolicies": []map[string]any{{"applicationType": "web", "allowedAuthMethods": []string{"client_secret_basic"}}},
	}), http.StatusOK)
	metadata["client_id"] = client.ClientID
	metadata["client_name"] = "Renamed"
	expect(t, oauth(t, h, "PUT", "/oauth2/register/"+client.ClientID, client.RegistrationAccessToken, metadata), http.StatusBadRequest)
}
