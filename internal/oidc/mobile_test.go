package oidc_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/zitadel/oidc/v3/pkg/oidc"

	"halo/internal/store"
)

func mobileAuthorization() url.Values {
	return url.Values{"client_id": {store.MobileClientID}, "redirect_uri": {store.MobileRedirectURI}, "response_type": {"code"}, "scope": {"openid profile email groups offline_access"}, "state": {"st-mobile"},
		"code_challenge": {oidc.NewSHACodeChallenge(pkceSecret)}, "code_challenge_method": {"S256"}}
}

func (h *harness) mobileCode(session string) string {
	h.t.Helper()
	status, location := h.get("/oauth2/authorize?"+mobileAuthorization().Encode(), session)
	if status != http.StatusFound || location.Path != "/sign-in" {
		h.t.Fatalf("authorize: %d %v", status, location)
	}
	status, redirect, _ := h.signIn(session, "/api/v1/auth/continue", map[string]string{"authRequest": location.Query().Get("authRequest")})
	if status != http.StatusOK {
		h.t.Fatalf("continue: %d %s", status, redirect)
	}
	status, location = h.get(redirect, session)
	if status != http.StatusFound || !strings.HasPrefix(location.String(), store.MobileRedirectURI+"?") || location.Query().Get("state") != "st-mobile" || location.Query().Get("code") == "" {
		h.t.Fatalf("callback: %d %v", status, location)
	}
	return location.Query().Get("code")
}

func (h *harness) mobileTokens(session string) result {
	h.t.Helper()
	r, _ := h.do(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {"authorization_code"}, "code": {h.mobileCode(session)}, "redirect_uri": {store.MobileRedirectURI}, "code_verifier": {pkceSecret}, "client_id": {store.MobileClientID}})
	return r
}

func (h *harness) bearer(path, token string, out any) int {
	h.t.Helper()
	req, _ := http.NewRequest(http.MethodGet, h.srv.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil && res.StatusCode == http.StatusOK {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			h.t.Fatal(err)
		}
	}
	return res.StatusCode
}

func TestMobileClient(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	app, err := h.st.GetApplicationByClientID(ctx, store.MobileClientID)
	if err != nil || app.Name != "Halo Mobile" || app.Type != "native" || app.TokenPolicy.RefreshTokenTTL != 30*24*3600 || len(app.GroupIDs) != 0 || slices.Contains(h.user.AppIDs, app.ID) {
		t.Fatalf("built-in mobile app: %v %+v", err, app)
	}
	_, session := h.session(h.user, "passkey")

	for name, change := range map[string]func(url.Values){
		"without PKCE":     func(q url.Values) { q.Del("code_challenge"); q.Del("code_challenge_method") },
		"with plain PKCE":  func(q url.Values) { q.Set("code_challenge", pkceSecret); q.Set("code_challenge_method", "plain") },
		"without a method": func(q url.Values) { q.Del("code_challenge_method") },
	} {
		q := mobileAuthorization()
		change(q)
		status, location := h.get("/oauth2/authorize?"+q.Encode(), session)
		if status != http.StatusFound || !strings.HasPrefix(location.String(), store.MobileRedirectURI+"?") || location.Query().Get("error") != "invalid_request" || location.Query().Get("code") != "" {
			t.Errorf("%s: %d %v", name, status, location)
		}
	}
	for _, uri := range []string{"gg.scala.halo:/oauth/callback/", "gg.scala.halo://oauth/callback", "gg.scala.halo:/oauth/callback?x=1", "com.example.halo:/oauth/callback", "http://127.0.0.1:8080/oauth/callback", "http://localhost/oauth/callback", callback} {
		q := mobileAuthorization()
		q.Set("redirect_uri", uri)
		if status, location := h.get("/oauth2/authorize?"+q.Encode(), session); status == http.StatusFound {
			t.Errorf("redirect_uri %s must be refused: %d %v", uri, status, location)
		}
	}
	q := mobileAuthorization()
	q.Set("prompt", "none")
	status, location := h.get("/oauth2/authorize?"+q.Encode(), session)
	if location.Path == "/oauth2/authorize/callback" {
		status, location = h.get(location.String(), session)
	}
	if status != http.StatusFound || !strings.HasPrefix(location.String(), store.MobileRedirectURI+"?") || location.Query().Get("code") == "" {
		t.Errorf("silent sign-in for an unassigned user: %d %v", status, location)
	}
	q = mobileAuthorization()
	q.Set("response_type", "token")
	if _, location := h.get("/oauth2/authorize?"+q.Encode(), session); location.Query().Get("error") == "" || strings.Contains(location.String(), "access_token") {
		t.Errorf("implicit flow must be refused: %v", location)
	}
	if r, _ := h.do(http.MethodPost, "/oauth2/device_authorization", url.Values{"client_id": {store.MobileClientID}, "scope": {"openid"}}); r.status == http.StatusOK || r.body["error"] != "unauthorized_client" {
		t.Errorf("device flow must be refused: %d %v", r.status, r.body)
	}

	tokens := h.mobileTokens(session)
	access, _ := tokens.body["access_token"].(string)
	refresh, _ := tokens.body["refresh_token"].(string)
	if tokens.status != http.StatusOK || access == "" || refresh == "" || tokens.body["id_token"] == nil {
		t.Fatalf("token: %d %v", tokens.status, tokens.body)
	}
	var sessions []store.Session
	if status := h.bearer("/api/v1/me/sessions", access, &sessions); status != http.StatusOK {
		t.Fatalf("mobile token on /api/v1/me/sessions: %d", status)
	}
	var me store.User
	if status := h.bearer("/api/v1/me", access, &me); status != http.StatusOK || me.ID != h.user.ID {
		t.Fatalf("mobile token on /api/v1/me: %d %+v", status, me)
	}
	if i := slices.IndexFunc(sessions, func(s store.Session) bool { return s.Client == store.MobileClientID }); i < 0 || sessions[i].Method != "passkey" {
		t.Fatalf("the mobile sign-in has its own session: %+v", sessions)
	}

	form := url.Values{"grant_type": {"authorization_code"}, "code": {h.silent(session, nil).Query().Get("code")}, "redirect_uri": {callback}, "code_verifier": {pkceSecret}}
	other := h.post("/oauth2/token", form, h.app.ClientID, h.secret)
	if otherAccess, _ := other.body["access_token"].(string); other.status != http.StatusOK || h.bearer("/api/v1/me/sessions", otherAccess, nil) != http.StatusUnauthorized {
		t.Fatalf("a token for an ordinary application must not reach /api/v1/me: %d %v", other.status, other.body)
	}

	refreshed, _ := h.do(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {store.MobileClientID}})
	next, _ := refreshed.body["refresh_token"].(string)
	nextAccess, _ := refreshed.body["access_token"].(string)
	if refreshed.status != http.StatusOK || next == "" || next == refresh || h.bearer("/api/v1/me/sessions", nextAccess, nil) != http.StatusOK {
		t.Fatalf("refresh rotates: %d %v", refreshed.status, refreshed.body)
	}
	if r, _ := h.do(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {store.MobileClientID}}); r.status != http.StatusBadRequest || r.body["error"] != "invalid_grant" {
		t.Fatalf("a rotated refresh token is spent: %d %v", r.status, r.body)
	}
	if r, _ := h.do(http.MethodPost, "/oauth2/revoke", url.Values{"token": {next}, "token_type_hint": {"refresh_token"}, "client_id": {store.MobileClientID}}); r.status != http.StatusOK {
		t.Fatalf("revoke: %d %v", r.status, r.body)
	}
	if status := h.bearer("/api/v1/me/sessions", nextAccess, nil); status != http.StatusUnauthorized {
		t.Fatalf("a revoked mobile token on /api/v1/me: %d", status)
	}
	sessions, err = h.st.ListSessions(ctx, h.user.ID)
	if err != nil || slices.ContainsFunc(sessions, func(s store.Session) bool { return s.Client == store.MobileClientID }) {
		t.Fatalf("revoking ends the mobile session: %v %+v", err, sessions)
	}
}

func TestMobileSessionLimit(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	org := func(change func(*store.Settings)) {
		s, err := h.st.Settings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		change(&s)
		if err := h.st.SaveSettings(ctx, s); err != nil {
			t.Fatal(err)
		}
		h.st.ForgetSettings()
	}
	_, session := h.session(h.user, "passkey")
	org(func(s *store.Settings) { s.SessionLimit, s.SessionLimitCLI = 1, true })
	if r := h.mobileTokens(session); r.status != http.StatusOK {
		t.Fatalf("first mobile sign-in: %d %v", r.status, r.body)
	}
	if r := h.mobileTokens(session); r.status != http.StatusBadRequest || r.body["error"] != "access_denied" {
		t.Fatalf("mobile sign-in over the limit: %d %v", r.status, r.body)
	}
	org(func(s *store.Settings) { s.SessionLimitAction = "end-oldest" })
	if r := h.mobileTokens(session); r.status != http.StatusOK {
		t.Fatalf("mobile sign-in after ending the oldest: %d %v", r.status, r.body)
	}
}

func TestMobileCodeRedeemedOnce(t *testing.T) {
	h := newHarness(t, nil)
	_, session := h.session(h.user, "passkey")
	form := url.Values{"grant_type": {"authorization_code"}, "code": {h.mobileCode(session)}, "redirect_uri": {store.MobileRedirectURI}, "code_verifier": {pkceSecret}, "client_id": {store.MobileClientID}}
	statuses := make(chan int, 10)
	var wg sync.WaitGroup
	for range cap(statuses) {
		wg.Go(func() {
			r, _ := h.do(http.MethodPost, "/oauth2/token", form)
			statuses <- r.status
		})
	}
	wg.Wait()
	close(statuses)
	redeemed := 0
	for status := range statuses {
		if status == http.StatusOK {
			redeemed++
		}
	}
	sessions, err := h.st.ListSessions(context.Background(), h.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mobile := slices.DeleteFunc(sessions, func(s store.Session) bool { return s.Client != store.MobileClientID }); redeemed != 1 || len(mobile) != 1 {
		t.Fatalf("one code makes one mobile session: %d redemptions, %d sessions", redeemed, len(mobile))
	}
}

func TestMobileRefusesAppOnlySessions(t *testing.T) {
	h := newHarness(t, nil)
	_, session, err := h.st.CreateSession(context.Background(), store.NewSession{UserID: h.user.ID, Method: "passkey", AppOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if r := h.mobileTokens(session); r.status != http.StatusBadRequest || r.body["error"] != "access_denied" {
		t.Fatalf("a session Halo's own policy keeps out of Halo must not reach the self-service API through the app: %d %v", r.status, r.body)
	}
}

func TestMobileRefusesEndedSessions(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	sess, session := h.session(h.user, "passkey")
	code := h.mobileCode(session)
	if err := h.st.RevokeSession(ctx, sess.ID, h.user.ID); err != nil {
		t.Fatal(err)
	}
	r, _ := h.do(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {store.MobileRedirectURI}, "code_verifier": {pkceSecret}, "client_id": {store.MobileClientID}})
	if r.status != http.StatusBadRequest || r.body["error"] != "access_denied" {
		t.Fatalf("a code from an ended session must not start a mobile session: %d %v", r.status, r.body)
	}
	sessions, err := h.st.ListSessions(ctx, h.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(sessions, func(s store.Session) bool { return s.Client == store.MobileClientID }) {
		t.Fatalf("no mobile session may be left behind: %+v", sessions)
	}
}
