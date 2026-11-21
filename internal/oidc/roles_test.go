package oidc_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"halo/internal/httpx"
	"halo/internal/store"
)

func TestApplicationRoles(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	editors, err := h.st.CreateGroup(ctx, store.NewGroup{Name: "Editors", Kind: "assigned"})
	must(err)
	must(h.st.AddGroupMember(ctx, editors.ID, h.user.ID))
	for _, role := range []store.AppRole{
		{Key: "editor", Name: "Editor", GroupIDs: []string{editors.ID}},
		{Key: "billing.viewer", Name: "Viewer", UserIDs: []string{h.user.ID}},
		{Key: "owner", Name: "Owner"},
	} {
		_, err := h.st.CreateAppRole(ctx, h.app.ID, role)
		must(err)
	}
	billing, err := h.st.SaveAPIResource(ctx, store.APIResource{Name: "Billing", Identifier: "https://billing.example.com", AccessTokenTTL: 300, Scopes: []store.APIScope{{Name: "billing:read"}}})
	must(err)
	must(h.st.SetAPIResourceGrant(ctx, billing.ID, h.app.ID, []string{"billing:read"}))

	_, session := h.session(h.user, "passkey")
	var refresh string
	issue := func(form url.Values) map[string]map[string]any {
		t.Helper()
		r := h.post("/oauth2/token", form, h.app.ClientID, h.secret)
		access, _ := r.body["access_token"].(string)
		idToken, _ := r.body["id_token"].(string)
		refresh, _ = r.body["refresh_token"].(string)
		if r.status != http.StatusOK || access == "" || idToken == "" || refresh == "" {
			t.Fatalf("token: %d %v", r.status, r.body)
		}
		req, _ := http.NewRequest(http.MethodGet, h.srv.URL+"/oauth2/userinfo", nil)
		req.Header.Set("Authorization", "Bearer "+access)
		var userinfo map[string]any
		fetch(t, req, &userinfo)
		return map[string]map[string]any{"id token": claimsOf(t, idToken).Claims, "access token": claimsOf(t, access).Claims, "userinfo": userinfo}
	}
	expect := func(claim, want string) {
		t.Helper()
		code := h.silent(session, nil).Query().Get("code")
		for _, form := range []url.Values{
			{"grant_type": {"refresh_token"}, "refresh_token": {refresh}},
			{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callback}, "code_verifier": {pkceSecret}},
		} {
			if form.Get("grant_type") == "refresh_token" && refresh == "" {
				continue
			}
			for name, claims := range issue(form) {
				if got := fmt.Sprint(claims[claim]); got != want {
					t.Errorf("%s from %s: %s = %s, want %s (%v)", name, form.Get("grant_type"), claim, got, want, claims)
				}
			}
		}
	}

	expect("roles", "[editor billing.viewer]")
	req, _ := http.NewRequest(http.MethodGet, h.srv.URL+"/api/v1/me/applications", nil)
	req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: session})
	var mine []struct {
		Roles []string `json:"roles"`
	}
	fetch(t, req, &mine)
	if len(mine) != 1 || fmt.Sprint(mine[0].Roles) != "[Editor Viewer]" {
		t.Errorf("account portal: %+v", mine)
	}

	must(h.st.RemoveGroupMember(ctx, editors.ID, h.user.ID))
	expect("roles", "[billing.viewer]")

	h.app.ClaimMappings = []store.Claim{{Name: "app_roles", Source: "user.roles"}, {Name: "roles", Source: store.OmitClaim}}
	must(h.st.UpdateApplication(ctx, h.app))
	expect("app_roles", "[billing.viewer]")
	expect("roles", "<nil>")
}
