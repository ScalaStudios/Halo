package oidc_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"halo/internal/store"
)

func TestClaimMappings(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	h.user.Attributes = map[string]any{"employeeNumber": 1042, "teams": []string{"Platform", "Security"}}
	must(h.st.SetUserAttributes(ctx, h.user.ID, h.user.Attributes))
	h.app.ClaimMappings = []store.Claim{
		{Name: "employee_id", Source: "attribute.employeeNumber"},
		{Name: "teams", Source: "attribute.teams"},
		{Name: "mail", Source: "user.email"},
		{Name: "email", Source: store.OmitClaim},
		{Name: "roles", Source: "user.groups"},
		{Name: "title", Source: "user.title"},
		{Name: "missing", Source: "attribute.deleted"},
		{Name: "sid", Source: "user.email"},
	}
	must(h.st.UpdateApplication(ctx, h.app))

	_, session := h.session(h.user, "passkey")
	form := url.Values{"grant_type": {"authorization_code"}, "code": {h.silent(session, nil).Query().Get("code")}, "redirect_uri": {callback}, "code_verifier": {pkceSecret}}
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/oauth2/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(h.app.ClientID, h.secret)
	var tokens struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
	}
	fetch(t, req, &tokens)

	parts := strings.Split(tokens.IDToken, ".")
	if len(parts) != 3 {
		t.Fatalf("id token: %q", tokens.IDToken)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	must(err)
	var idToken map[string]any
	must(json.Unmarshal(payload, &idToken))

	req, _ = http.NewRequest(http.MethodGet, h.srv.URL+"/oauth2/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	var userinfo map[string]any
	fetch(t, req, &userinfo)

	for name, claims := range map[string]map[string]any{"id token": idToken, "userinfo": userinfo} {
		if claims["employee_id"] != float64(1042) || fmt.Sprint(claims["teams"]) != "[Platform Security]" || claims["mail"] != h.user.Email || fmt.Sprint(claims["roles"]) != "[Engineering]" {
			t.Errorf("%s: mapped claims missing: %v", name, claims)
		}
		if _, ok := claims["email"]; ok {
			t.Errorf("%s: email is mapped to none but was sent: %v", name, claims)
		}
		if _, ok := claims["title"]; ok {
			t.Errorf("%s: an empty title must not be sent: %v", name, claims)
		}
		if _, ok := claims["missing"]; ok {
			t.Errorf("%s: a mapping to a missing attribute must be skipped: %v", name, claims)
		}
		if claims["sub"] != h.user.ID || claims["email_verified"] != false || claims["sid"] == h.user.Email {
			t.Errorf("%s: built-in claims changed: %v", name, claims)
		}
	}
	if idToken["sid"] == nil {
		t.Errorf("id token lost its sid: %v", idToken)
	}
}

func fetch(t *testing.T, req *http.Request, out any) {
	t.Helper()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(out); err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("%s: %d %v", req.URL.Path, res.StatusCode, err)
	}
}
