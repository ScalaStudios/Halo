package auth_test

import (
	"context"
	"net/http"
	"slices"
	"sync/atomic"
	"testing"

	"halo/internal/access"
	"halo/internal/store"
)

func TestTermsBeforeAccess(t *testing.T) {
	var st *store.Store
	var version atomic.Int64
	version.Store(1)
	e, box := newMagicEnv(t, policyFunc(func(in access.Input) access.Decision {
		current := store.Terms{ID: "tou_aup", Name: "Acceptable use", Version: int(version.Load()), Text: "Use company systems for work."}
		accepted, err := st.TermsAcceptances(context.Background(), in.User.ID)
		if err != nil {
			t.Fatal(err)
		}
		if slices.ContainsFunc(accepted, func(a store.TermsAcceptance) bool { return a.TermsID == current.ID && a.Version == current.Version }) {
			return access.Decision{Effect: access.Allow}
		}
		return access.Decision{Effect: access.Allow, Terms: []store.Terms{current}}
	}))
	st = e.st
	ctx := context.Background()
	group, err := st.CreateGroup(ctx, store.NewGroup{Name: "Staff", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	ada, err := st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active", GroupIDs: []string{group.ID}})
	if err != nil {
		t.Fatal(err)
	}

	type pending struct {
		Redirect string
		Terms    string
	}
	c := client()
	e.do(c, "POST", "/api/v1/auth/magic-link", map[string]any{"email": "ada@example.com"}, nil)
	_, token := box.token(t)
	var out pending
	if status := e.do(c, "POST", "/api/v1/auth/magic-link/redeem", map[string]any{"token": token}, &out); status != http.StatusOK || out.Terms == "" || out.Redirect != "/sign-in?terms="+out.Terms {
		t.Fatalf("redeem with terms pending: %d %+v", status, out)
	}
	if _, status := e.me(c); status != http.StatusUnauthorized {
		t.Fatalf("a session was created before the terms were accepted: %d", status)
	}
	e.expect(client(), "POST", "/api/v1/auth/magic-link/redeem", map[string]any{"token": token}, http.StatusNotFound, "ERR_LINK_EXPIRED")

	var shown struct{ Terms []store.Terms }
	if status := e.do(client(), "GET", "/api/v1/auth/terms/"+out.Terms, nil, &shown); status != http.StatusOK || len(shown.Terms) != 1 || shown.Terms[0].Name != "Acceptable use" || shown.Terms[0].Version != 1 {
		t.Fatalf("terms to accept: %d %+v", status, shown)
	}
	var done struct{ Redirect string }
	if status := e.do(c, "POST", "/api/v1/auth/terms", map[string]any{"ceremony": out.Terms}, &done); status != http.StatusOK || done.Redirect != "/account" {
		t.Fatalf("accept: %d %+v", status, done)
	}
	if me, status := e.me(c); status != http.StatusOK || me.ID != ada.ID {
		t.Fatalf("session after accepting: %d", status)
	}
	accepted, err := st.TermsAcceptances(ctx, ada.ID)
	if err != nil || len(accepted) != 1 || accepted[0].Version != 1 || accepted[0].Name != "Acceptable use" {
		t.Fatalf("acceptance: %+v %v", accepted, err)
	}
	e.expect(c, "POST", "/api/v1/auth/terms", map[string]any{"ceremony": out.Terms}, http.StatusNotFound, "ERR_TERMS_EXPIRED")

	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://grafana.example.com/cb"}, GroupIDs: []string{group.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAuthRequest(ctx, store.AuthRequest{ID: "are_terms", ClientID: app.ClientID, RedirectURI: app.RedirectURIs[0], Scopes: []string{"openid"}, ResponseType: "code"}); err != nil {
		t.Fatal(err)
	}
	version.Store(2)
	if status := e.do(c, "POST", "/api/v1/auth/continue", map[string]any{"authRequest": "are_terms"}, &out); status != http.StatusOK || out.Terms == "" {
		t.Fatalf("new version on continue: %d %+v", status, out)
	}
	e.expect(client(), "POST", "/api/v1/auth/terms", map[string]any{"ceremony": out.Terms}, http.StatusNotFound, "ERR_TERMS_EXPIRED")
	e.do(c, "POST", "/api/v1/auth/continue", map[string]any{"authRequest": "are_terms"}, &out)
	if status := e.do(c, "POST", "/api/v1/auth/terms", map[string]any{"ceremony": out.Terms}, &done); status != http.StatusOK || done.Redirect != store.AuthorizeCallbackPath("are_terms") {
		t.Fatalf("accept version 2: %d %+v", status, done)
	}
	req, err := st.GetAuthRequest(ctx, "are_terms")
	if err != nil || !req.Done || req.UserID == nil || *req.UserID != ada.ID {
		t.Fatalf("auth request after accepting: %+v %v", req, err)
	}
}
