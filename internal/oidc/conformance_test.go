package oidc_test

import (
	"cmp"
	"context"
	"encoding/base64"
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

func formPost(t *testing.T, h *harness, q url.Values, wantError, wantState string) {
	t.Helper()
	res, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Get(h.srv.URL + "/oauth2/authorize?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	page := string(body)
	if res.StatusCode != http.StatusOK || res.Header.Get("Location") != "" || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("want a form post page: %d %v %s", res.StatusCode, res.Header, page)
	}
	for _, want := range []string{`<form method="post" action="` + cmp.Or(q.Get("redirect_uri"), callback) + `">`, `name="error" value="` + wantError + `"`, `name="state" value="` + wantState + `"`} {
		if !strings.Contains(page, want) {
			t.Errorf("form post page lacks %s: %s", want, page)
		}
	}
}

func TestFormPostErrorResponses(t *testing.T) {
	h := newHarness(t, nil)
	q := authorization(h.app.ClientID, true)
	q.Set("response_mode", "form_post")
	q.Set("prompt", "none")
	formPost(t, h, q, "login_required", "st-fapi")
	pushed, _ := h.do(http.MethodPost, "/oauth2/par", q, basicAuth(h.app.ClientID, h.secret))
	requestURI, _ := pushed.body["request_uri"].(string)
	if pushed.status != http.StatusCreated || requestURI == "" {
		t.Fatalf("pushed authorization request: %d %v", pushed.status, pushed.body)
	}
	formPost(t, h, url.Values{"client_id": {h.app.ClientID}, "request_uri": {requestURI}}, "login_required", "st-fapi")

	app := h.app
	app.RedirectURIs = append(app.RedirectURIs, callback+"?state=route")
	if err := h.st.UpdateApplication(context.Background(), app); err != nil {
		t.Fatal(err)
	}
	q.Set("redirect_uri", callback+"?state=route")
	formPost(t, h, q, "login_required", "st-fapi")
}

func TestUnsupportedRequestObjectAnswersAsTheObjectAsks(t *testing.T) {
	h := newHarness(t, nil)
	object := `{"client_id":"` + h.app.ClientID + `","redirect_uri":"` + callback + `","response_type":"code","scope":"openid","state":"st-object","response_mode":"form_post"}`
	q := url.Values{"client_id": {h.app.ClientID}, "redirect_uri": {callback}, "response_type": {"code"}, "scope": {"openid"}, "request": {"eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString([]byte(object)) + "."}}
	formPost(t, h, q, "request_not_supported", "st-object")
}

func TestForeignRequestURIIsNotSupported(t *testing.T) {
	h := newHarness(t, nil)
	q := url.Values{"client_id": {h.app.ClientID}, "redirect_uri": {callback}, "response_type": {"code"}, "scope": {"openid"}, "request_uri": {"https://rp.example.com/request.jwt"}}
	r, _ := h.do(http.MethodGet, "/oauth2/authorize?"+q.Encode(), nil)
	location, _ := url.Parse(r.location)
	if r.status != http.StatusFound || !strings.HasPrefix(r.location, callback+"?") || location.Query().Get("error") != "request_uri_not_supported" {
		t.Fatalf("foreign request_uri: %d %q", r.status, r.location)
	}
	q.Set("redirect_uri", "https://attacker.example.com/cb")
	if r, _ := h.do(http.MethodGet, "/oauth2/authorize?"+q.Encode(), nil); r.status != http.StatusBadRequest || r.location != "" {
		t.Fatalf("foreign request_uri with an unregistered redirect URI: %d %q", r.status, r.location)
	}
}
