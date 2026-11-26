package oidc_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"halo/internal/store"
)

func TestRevokedAccessTokensReachAPIs(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	ledger, err := h.st.SaveAPIResource(ctx, store.APIResource{Name: "Ledger", Identifier: "https://ledger.example.com", AccessTokenTTL: 600, Scopes: []store.APIScope{{Name: "ledger:read"}}})
	must(err)
	must(h.st.SetAPIResourceGrant(ctx, ledger.ID, h.app.ID, []string{"ledger:read"}))
	signIn := func() (string, string, store.Session) {
		t.Helper()
		sess, token := h.session(h.user, "passkey")
		code := h.silent(token, url.Values{"scope": {"openid offline_access ledger:read"}}).Query().Get("code")
		r := h.post("/oauth2/token", url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callback}, "code_verifier": {pkceSecret}}, h.app.ClientID, h.secret)
		access, _ := r.body["access_token"].(string)
		refresh, _ := r.body["refresh_token"].(string)
		if r.status != http.StatusOK || access == "" || refresh == "" {
			t.Fatalf("sign in: %d %v", r.status, r.body)
		}
		return access, refresh, sess
	}
	active := func(token string) bool {
		t.Helper()
		return h.post("/oauth2/introspect", url.Values{"token": {token}}, h.app.ClientID, h.secret).body["active"] == true
	}
	listed := func(token string) bool {
		t.Helper()
		res, err := http.Get(h.srv.URL + "/oauth2/revoked")
		must(err)
		defer res.Body.Close()
		var body struct{ Revoked []store.RevokedToken }
		must(json.NewDecoder(res.Body).Decode(&body))
		if res.StatusCode != http.StatusOK || body.Revoked == nil {
			t.Fatalf("revocation list: %d %+v", res.StatusCode, body)
		}
		claims := claimsOf(t, token)
		return slices.ContainsFunc(body.Revoked, func(r store.RevokedToken) bool {
			return r.ID == claims.JWTID && r.ExpiresAt == int64(claims.Expiration)
		})
	}
	announced := func(token string) bool {
		t.Helper()
		events, err := h.st.PollWebhookEvents(ctx, time.Now().Add(time.Second), 100)
		must(err)
		jti := claimsOf(t, token).JWTID
		return slices.ContainsFunc(events, func(e store.WebhookEvent) bool {
			var data struct {
				UserID   string
				TokenIDs []string
				Summary  string
			}
			_ = json.Unmarshal(e.Data, &data)
			return e.Type == "token.revoke" && data.UserID == h.user.ID && slices.Contains(data.TokenIDs, jti) && data.Summary != ""
		})
	}
	revokedEverywhere := func(trigger, token string) {
		t.Helper()
		if a, l, n := active(token), listed(token), announced(token); a || !l || !n {
			t.Errorf("%s: the access token must be inactive at introspection, on the revocation list and in a token.revoke event: %v %v %v", trigger, a, l, n)
		}
	}

	access, refresh, sess := signIn()
	if !active(access) || listed(access) || announced(access) {
		t.Fatal("a new access token must be active and not revoked")
	}
	status, _ := h.refresh(refresh)
	if status != http.StatusOK || active(access) || listed(access) || announced(access) {
		t.Fatal("a token replaced by a refresh is inactive, but is not published as revoked")
	}
	must(h.st.RevokeSession(ctx, sess.ID, ""))
	revokedEverywhere("session end after a refresh", access)

	access, _, sess = signIn()
	must(h.st.RevokeSession(ctx, sess.ID, ""))
	revokedEverywhere("session end", access)

	access, _, _ = signIn()
	must(h.st.GrantConsent(ctx, h.user.ID, h.app.ID, []string{"openid", "ledger:read"}))
	must(h.st.RevokeConsent(ctx, h.user.ID, h.app))
	revokedEverywhere("consent removal", access)

	access, _, _ = signIn()
	r := h.post("/oauth2/revoke", url.Values{"token": {access}}, h.app.ClientID, h.secret)
	if r.status != http.StatusOK {
		t.Fatalf("revoke: %d %v", r.status, r.body)
	}
	revokedEverywhere("token revocation", access)

	access, _, _ = signIn()
	unbound := store.OIDCToken{ID: "atk_unbound", Kind: "access", ClientID: h.app.ClientID, UserID: &h.user.ID, ExpiresAt: time.Now().Add(time.Hour)}
	must(h.st.CreateOIDCTokens(ctx, unbound))
	_, err = h.st.RevokeUserSessions(ctx, h.user.ID, "")
	must(err)
	revokedEverywhere("ending every session", access)
	if _, err := h.st.GetActiveOIDCToken(ctx, unbound.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("ending every session must also revoke tokens no session holds: %v", err)
	}

	access, _, _ = signIn()
	must(h.st.DeleteApplication(ctx, h.app.ID))
	if !listed(access) || !announced(access) {
		t.Errorf("deleting the application must publish its access tokens as revoked: %v %v", listed(access), announced(access))
	}
}
