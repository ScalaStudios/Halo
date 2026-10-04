package oidc_test

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	halooidc "halo/internal/oidc"
	"halo/internal/store"
)

func (h *infra) signInCLI() (string, string) {
	h.t.Helper()
	deviceCode, userCode := h.startDevice("openid profile email offline_access")
	if r := h.send(http.MethodPost, "/api/v1/device/"+userCode+"/approve", nil, cookie(h.token)); r.status != http.StatusOK {
		h.t.Fatalf("approve: %d %v", r.status, r.body)
	}
	tokens := h.poll(deviceCode)
	access, _ := tokens.body["access_token"].(string)
	refresh, _ := tokens.body["refresh_token"].(string)
	if tokens.status != http.StatusOK || access == "" || refresh == "" {
		h.t.Fatalf("tokens: %d %v", tokens.status, tokens.body)
	}
	return access, refresh
}

func (h *infra) cliSessions() []store.Session {
	h.t.Helper()
	sessions, err := h.st.ListSessions(context.Background(), h.user.ID)
	h.must(err)
	return slices.DeleteFunc(sessions, func(s store.Session) bool { return s.Client != halooidc.CLIClientID })
}

func (h *infra) refresh(token string) result {
	h.t.Helper()
	return h.send(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {token}, "client_id": {halooidc.CLIClientID}})
}

func TestDiscoveryAdvertisesOnlyServedFeatures(t *testing.T) {
	h := newInfra(t, nil)
	d := h.send(http.MethodGet, "/.well-known/openid-configuration", nil, func(r *http.Request) { r.Header.Set("Origin", "https://app.example.com") })
	if d.status != http.StatusOK || d.body["issuer"] != h.srv.URL {
		t.Fatalf("discovery: %d %v", d.status, d.body)
	}
	list := func(field string) []string {
		var out []string
		for _, v := range d.body[field].([]any) {
			out = append(out, v.(string))
		}
		return out
	}
	if got := list("response_types_supported"); !slices.Equal(got, []string{"code"}) {
		t.Errorf("response types: %v", got)
	}
	grants := list("grant_types_supported")
	for _, unsupported := range []string{"implicit", string(oidc.GrantTypeBearer), string(oidc.GrantTypeTokenExchange)} {
		if slices.Contains(grants, unsupported) {
			t.Errorf("grant %s is advertised but not served: %v", unsupported, grants)
		}
	}
	if !slices.Contains(grants, string(oidc.GrantTypeDeviceCode)) || !slices.Contains(grants, "client_credentials") {
		t.Errorf("grants: %v", grants)
	}
	claims := list("claims_supported")
	if slices.Contains(claims, "phone_number") || slices.Contains(claims, "locale") || !slices.Contains(claims, "groups") || !slices.Contains(claims, "email_verified") {
		t.Errorf("claims: %v", claims)
	}
	for _, field := range []string{"introspection_endpoint_auth_signing_alg_values_supported", "revocation_endpoint_auth_signing_alg_values_supported", "check_session_iframe", "registration_endpoint"} {
		if _, ok := d.body[field]; ok {
			t.Errorf("%s is advertised: %v", field, d.body[field])
		}
	}

	path := func(field string) string {
		return strings.TrimPrefix(d.body[field].(string), h.srv.URL)
	}
	if r := h.send(http.MethodPost, path("device_authorization_endpoint"), url.Values{"client_id": {halooidc.CLIClientID}, "scope": {"openid"}}); r.status != http.StatusOK || r.body["device_code"] == nil {
		t.Fatalf("advertised device authorization endpoint: %d %v", r.status, r.body)
	}
	if r := h.send(http.MethodGet, path("jwks_uri"), nil); r.status != http.StatusOK || len(r.body["keys"].([]any)) != 1 {
		t.Fatalf("advertised jwks: %d %v", r.status, r.body)
	}
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, field := range []string{"authorization_endpoint", "token_endpoint", "userinfo_endpoint", "revocation_endpoint", "introspection_endpoint", "end_session_endpoint"} {
		resp, err := noFollow.PostForm(d.body[field].(string), url.Values{})
		h.must(err)
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
			t.Errorf("advertised %s is not served: %d", field, resp.StatusCode)
		}
	}
}

func TestEmailVerifiedClaim(t *testing.T) {
	h := newInfra(t, nil)
	access, _ := h.signInCLI()
	if r := h.send(http.MethodGet, "/oauth2/userinfo", nil, bearerToken(access)); r.status != http.StatusOK || r.body["email_verified"] != false {
		t.Fatalf("unverified userinfo: %d %v", r.status, r.body)
	}
	h.must(h.st.MarkEmailVerified(context.Background(), h.user.ID))
	if r := h.send(http.MethodGet, "/oauth2/userinfo", nil, bearerToken(access)); r.status != http.StatusOK || r.body["email_verified"] != true {
		t.Fatalf("verified userinfo: %d %v", r.status, r.body)
	}
}

func TestCLISessions(t *testing.T) {
	h := newInfra(t, nil)
	ctx := context.Background()
	access, refresh := h.signInCLI()
	sessions := h.cliSessions()
	if len(sessions) != 1 || sessions[0].Browser != "Halo CLI" || sessions[0].Device != "Linux computer" || sessions[0].Method != "totp" || sessions[0].IP != "127.0.0.1" {
		t.Fatalf("CLI session: %+v", sessions)
	}
	approved := sessions[0].LastActiveAt
	time.Sleep(10 * time.Millisecond)
	refreshed := h.refresh(refresh)
	refresh, _ = refreshed.body["refresh_token"].(string)
	if refreshed.status != http.StatusOK || refresh == "" {
		t.Fatalf("refresh: %d %v", refreshed.status, refreshed.body)
	}
	if after := h.cliSessions(); len(after) != 1 || !after[0].LastActiveAt.After(approved) {
		t.Fatalf("last refresh not recorded: %+v", after)
	}

	h.must(h.st.RevokeSession(ctx, sessions[0].ID, h.user.ID))
	if r := h.refresh(refresh); r.status != http.StatusBadRequest {
		t.Fatalf("refresh after signing the CLI out: %d %v", r.status, r.body)
	}
	if r := h.send(http.MethodGet, "/api/v1/me/probe", nil, bearerToken(access)); r.status != http.StatusUnauthorized {
		t.Fatalf("access token after signing the CLI out: %d %v", r.status, r.body)
	}

	access, _ = h.signInCLI()
	if n, err := h.st.RevokeUserSessions(ctx, h.user.ID, ""); err != nil || n != 2 {
		t.Fatalf("revoke all: %d %v", n, err)
	}
	if r := h.send(http.MethodGet, "/api/v1/me/probe", nil, bearerToken(access)); r.status != http.StatusUnauthorized || len(h.cliSessions()) != 0 {
		t.Fatalf("revoking every session must include the CLI: %d", r.status)
	}

	_, h.token, _ = h.st.CreateSession(ctx, store.NewSession{UserID: h.user.ID, Method: "totp"})
	_, refresh = h.signInCLI()
	if r := h.send(http.MethodPost, "/oauth2/revoke", url.Values{"token": {refresh}, "token_type_hint": {"refresh_token"}, "client_id": {halooidc.CLIClientID}}); r.status != http.StatusOK || len(h.cliSessions()) != 0 {
		t.Fatalf("halo logout must end the session: %d %+v", r.status, h.cliSessions())
	}
}

func TestDevicePollingAndRateLimit(t *testing.T) {
	h := newInfra(t, nil)
	deviceCode, _ := h.startDevice("openid")
	if r := h.poll(deviceCode); r.body["error"] != "authorization_pending" {
		t.Fatalf("first poll: %d %v", r.status, r.body)
	}
	if r := h.poll(deviceCode); r.status != http.StatusBadRequest || r.body["error"] != "slow_down" {
		t.Fatalf("poll faster than the interval: %d %v", r.status, r.body)
	}
	for range 19 {
		h.startDevice("openid")
	}
	r := h.send(http.MethodPost, "/oauth2/device_authorization", url.Values{"client_id": {halooidc.CLIClientID}, "scope": {"openid"}})
	if r.status != http.StatusTooManyRequests || r.body["error"] != "slow_down" {
		t.Fatalf("device authorization over the limit: %d %v", r.status, r.body)
	}
}

func TestSigningKeyRotationKeepsOldTokensValid(t *testing.T) {
	h := newInfra(t, nil)
	ctx := context.Background()
	before, _ := h.signInCLI()
	der, err := halooidc.NewSigningKey()
	h.must(err)
	rotated, err := h.st.CreateSigningKey(ctx, "RS256", der)
	h.must(err)
	_, h.token, err = h.st.CreateSession(ctx, store.NewSession{UserID: h.user.ID, Method: "totp"})
	h.must(err)
	after, _ := h.signInCLI()

	keys := rp.NewRemoteKeySet(http.DefaultClient, h.srv.URL+"/oauth2/keys")
	for name, token := range map[string]string{"before": before, "after": after} {
		jws, err := jose.ParseSigned(token, []jose.SignatureAlgorithm{jose.RS256})
		h.must(err)
		if _, err := keys.VerifySignature(ctx, jws); err != nil {
			t.Fatalf("token issued %s the rotation does not verify: %v", name, err)
		}
		if kid := jws.Signatures[0].Header.KeyID; (name == "after") != (kid == rotated.ID) {
			t.Fatalf("token issued %s the rotation was signed with %s", name, kid)
		}
	}
	h.must(h.st.RetireSigningKeys(ctx, time.Now().Add(store.SigningKeyOverlap+time.Minute)))
	if r := h.send(http.MethodGet, "/oauth2/keys", nil); len(r.body["keys"].([]any)) != 1 {
		t.Fatalf("retired keys must leave the JWKS: %v", r.body)
	}
}
