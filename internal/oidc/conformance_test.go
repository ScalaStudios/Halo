package oidc_test

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestTokenResponsesForbidCaching(t *testing.T) {
	h := newHarness(t, nil)
	_, session := h.session(h.user, "passkey")
	authorize, _ := h.do(http.MethodGet, "/oauth2/authorize?"+authorization(h.app.ClientID, true).Encode(), nil)
	exchanged, headers := h.do(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {"authorization_code"}, "code": {h.code(authorize, session)}, "redirect_uri": {callback}, "code_verifier": {pkceSecret}}, basicAuth(h.app.ClientID, h.secret))
	if exchanged.status != http.StatusOK || headers.Get("Cache-Control") != "no-store" || headers.Get("Pragma") != "no-cache" {
		t.Fatalf("code exchange: %d %v %v", exchanged.status, headers, exchanged.body)
	}
	refreshed, headers := h.do(http.MethodPost, "/oauth2/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {exchanged.body["refresh_token"].(string)}}, basicAuth(h.app.ClientID, h.secret))
	if refreshed.status != http.StatusOK || headers.Get("Cache-Control") != "no-store" || headers.Get("Pragma") != "no-cache" {
		t.Fatalf("refresh: %d %v %v", refreshed.status, headers, refreshed.body)
	}
}

func TestFormPostErrorResponses(t *testing.T) {
	h := newHarness(t, nil)
	q := authorization(h.app.ClientID, true)
	q.Set("response_mode", "form_post")
	q.Set("prompt", "none")
	res, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Get(h.srv.URL + "/oauth2/authorize?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	page := string(body)
	if res.StatusCode != http.StatusOK || res.Header.Get("Location") != "" || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("prompt=none without a session: %d %v %s", res.StatusCode, res.Header, page)
	}
	for _, want := range []string{`<form method="post" action="` + callback + `">`, `name="error" value="login_required"`, `name="state" value="st-fapi"`} {
		if !strings.Contains(page, want) {
			t.Errorf("form post page lacks %s: %s", want, page)
		}
	}
}
