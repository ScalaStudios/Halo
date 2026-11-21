package oidc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/zitadel/oidc/v3/pkg/oidc"

	"halo/internal/httpx"
	"halo/internal/store"
)

func (h *harness) call(method, path, sessionToken string, body, out any) int {
	h.t.Helper()
	var payload bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&payload).Encode(body)
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, &payload)
	req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: sessionToken})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

func TestConsent(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	setConsent := func(on bool) {
		t.Helper()
		app, err := h.st.GetApplication(ctx, h.app.ID)
		if err != nil {
			t.Fatal(err)
		}
		app.RequiresConsent = on
		if err := h.st.UpdateApplication(ctx, app); err != nil {
			t.Fatal(err)
		}
	}
	setConsent(true)
	sess, token := h.session(h.user, "passkey")
	_, otherToken := h.session(h.user, "passkey")

	signIn := func(scope string, extra url.Values) (string, *url.URL) {
		t.Helper()
		q := url.Values{"client_id": {h.app.ClientID}, "redirect_uri": {callback}, "response_type": {"code"}, "scope": {scope}, "state": {"st-1"},
			"code_challenge": {oidc.NewSHACodeChallenge(pkceSecret)}, "code_challenge_method": {"S256"}}
		for k, v := range extra {
			q[k] = v
		}
		_, location := h.get("/oauth2/authorize?"+q.Encode(), token)
		requestID, ok := strings.CutPrefix(location.String(), "/sign-in?authRequest=")
		if !ok {
			t.Fatalf("authorize ended at %s", location)
		}
		if err := h.st.CompleteAuthRequest(ctx, requestID, h.user.ID, sess.ID, sess.AMR(), sess.CreatedAt, ""); err != nil {
			t.Fatal(err)
		}
		status, location := h.get(store.AuthorizeCallbackPath(requestID), token)
		if status != http.StatusFound {
			t.Fatalf("callback: %d", status)
		}
		return requestID, location
	}
	asksConsent := func(requestID string, location *url.URL) {
		t.Helper()
		if location.String() != "/sign-in?consent="+requestID {
			t.Fatalf("callback did not ask for consent: %s", location)
		}
	}
	decide := func(requestID string, allow bool) {
		t.Helper()
		var out struct{ Redirect string }
		if status := h.call("POST", "/api/v1/auth/consent", token, map[string]any{"authRequest": requestID, "allow": allow}, &out); status != http.StatusOK || out.Redirect != store.AuthorizeCallbackPath(requestID) {
			t.Fatalf("decide: %d %+v", status, out)
		}
	}
	code := func(location *url.URL) string {
		t.Helper()
		if !strings.HasPrefix(location.String(), callback+"?") || location.Query().Get("code") == "" {
			t.Fatalf("no code: %s", location)
		}
		return location.Query().Get("code")
	}
	granted := func() []string {
		t.Helper()
		scopes, err := h.st.ConsentedScopes(ctx, h.user.ID, h.app.ID)
		if err != nil {
			t.Fatal(err)
		}
		return scopes
	}

	const scope = "openid email offline_access"
	if got := h.silent(token, nil).Query().Get("error"); got != "consent_required" {
		t.Fatalf("prompt=none before consent: %s", got)
	}

	requestID, location := signIn(scope, nil)
	asksConsent(requestID, location)
	if status := h.call("GET", "/api/v1/auth/consent/"+requestID, otherToken, nil, nil); status != http.StatusNotFound {
		t.Fatalf("consent screen from another session: %d", status)
	}
	var shown struct {
		Application struct{ Name string }
		Email       string
		Scopes      []store.APIScope
	}
	if status := h.call("GET", "/api/v1/auth/consent/"+requestID, token, nil, &shown); status != http.StatusOK || shown.Application.Name != "Grafana" || shown.Email != h.user.Email ||
		!slices.Equal(slices.Collect(func(yield func(string) bool) {
			for _, s := range shown.Scopes {
				yield(s.Name)
			}
		}), []string{"openid", "email", "offline_access"}) {
		t.Fatalf("consent screen: %d %+v", status, shown)
	}
	if status := h.call("POST", "/api/v1/auth/consent", otherToken, map[string]any{"authRequest": requestID, "allow": true}, nil); status != http.StatusNotFound {
		t.Fatalf("consent from another session: %d", status)
	}
	decide(requestID, true)
	if status := h.call("POST", "/api/v1/auth/consent", token, map[string]any{"authRequest": requestID, "allow": false}, nil); status != http.StatusNotFound {
		t.Fatalf("second decision: %d", status)
	}
	_, location = h.get(store.AuthorizeCallbackPath(requestID), token)
	_, refreshToken, _ := h.tokens(code(location))
	if !slices.Equal(granted(), []string{"openid", "email", "offline_access"}) {
		t.Fatalf("grant: %v", granted())
	}

	if h.silent(token, nil).Query().Get("code") == "" {
		t.Fatal("prompt=none after consent did not return a code")
	}
	if _, location := signIn(scope, nil); location.Query().Get("code") == "" {
		t.Fatalf("remembered consent asked again: %s", location)
	}

	if got := h.silent(token, url.Values{"scope": {scope + " profile"}}).Query().Get("error"); got != "consent_required" {
		t.Fatalf("prompt=none with a wider scope: %s", got)
	}
	requestID, location = signIn(scope+" profile", nil)
	asksConsent(requestID, location)
	decide(requestID, true)
	if !slices.Equal(granted(), []string{"openid", "email", "offline_access", "profile"}) {
		t.Fatalf("widened grant: %v", granted())
	}

	requestID, location = signIn(scope, url.Values{"prompt": {"consent"}})
	asksConsent(requestID, location)
	decide(requestID, false)
	_, location = h.get(store.AuthorizeCallbackPath(requestID), token)
	if !strings.HasPrefix(location.String(), callback+"?") || location.Query().Get("error") != "access_denied" || location.Query().Get("state") != "st-1" {
		t.Fatalf("declined: %s", location)
	}
	events, err := h.st.ListSignIns(ctx, store.SignInFilter{AppID: h.app.ID})
	if err != nil || len(events) == 0 || events[0].Result != "interrupted" || events[0].Reason != "Did not allow the application to access their information." {
		t.Fatalf("decline in the sign-in log: %+v %v", events, err)
	}

	var consents []store.Consent
	if status := h.call("GET", "/api/v1/me/consents", token, nil, &consents); status != http.StatusOK || len(consents) != 1 || consents[0].AppID != h.app.ID || len(consents[0].Scopes) != 4 {
		t.Fatalf("account portal: %d %+v", status, consents)
	}
	if status := h.call("DELETE", "/api/v1/me/consents/"+h.app.ID, token, nil, nil); status != http.StatusNoContent {
		t.Fatalf("revoke: %d", status)
	}
	if status, _ := h.refresh(refreshToken); status == http.StatusOK {
		t.Fatal("refresh token still works after revoking consent")
	}
	if got := h.silent(token, nil).Query().Get("error"); got != "consent_required" {
		t.Fatalf("prompt=none after revoking: %s", got)
	}
	if status := h.call("DELETE", "/api/v1/me/consents/"+h.app.ID, token, nil, nil); status != http.StatusNotFound {
		t.Fatalf("second revoke: %d", status)
	}
	audit, err := h.st.ListAudit(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, e := range audit {
		if e.TargetID == h.app.ID {
			actions = append(actions, e.Action)
		}
	}
	slices.Sort(actions)
	if !slices.Equal(actions, []string{"consent.grant", "consent.grant", "consent.revoke"}) {
		t.Fatalf("audit: %v", actions)
	}

	setConsent(false)
	if _, location := signIn(scope, nil); location.Query().Get("code") == "" {
		t.Fatalf("an application without the consent screen asked for consent: %s", location)
	}
	requestID, location = signIn(scope, url.Values{"prompt": {"consent"}})
	asksConsent(requestID, location)
}
