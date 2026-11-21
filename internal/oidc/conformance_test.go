package oidc_test

import (
	"net/http"
	"net/url"
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
