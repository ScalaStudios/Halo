package avatars

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/store"
	"halo/internal/testdb"
)

func TestAvatars(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	public, _ := url.Parse("http://localhost:3200")
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	mux := http.NewServeMux()
	if err := Register(mux, httpx.Deps{Config: config.Config{PublicURL: public, Dev: true}, Store: st, Auth: authn}); err != nil {
		t.Fatal(err)
	}
	h := authn.Resolve(authn.SameOrigin(mux))
	person := func(email string, roles ...string) (store.User, string) {
		u, err := st.CreateUser(ctx, store.NewUser{Email: email, Name: email, Status: "active", Roles: roles})
		if err != nil {
			t.Fatal(err)
		}
		_, token, err := st.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: "passkey"})
		if err != nil {
			t.Fatal(err)
		}
		return u, token
	}
	call := func(token, method, path string, body []byte, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		if token != "" {
			req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: token})
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("%s %s: status %d, want %d: %s", method, path, rec.Code, want, rec.Body.String())
		}
		return rec
	}
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	sam, samToken := person("sam@example.com")
	admin, _ := person("gia@example.com", "user_admin")
	_, helpdesk := person("hal@example.com", "helpdesk_admin")

	call(samToken, "PUT", "/api/v1/me/avatar", []byte("<svg/>"), 422)
	call(samToken, "PUT", "/api/v1/me/avatar", png, 200)
	if u, _ := st.GetUser(ctx, sam.ID); u.AvatarURL == nil {
		t.Fatal("no avatar URL after upload")
	}
	rec := call("", "GET", "/api/v1/users/"+sam.ID+"/avatar", nil, 200)
	if rec.Header().Get("Content-Type") != "image/png" || !bytes.Equal(rec.Body.Bytes(), png) {
		t.Fatalf("served %q", rec.Header().Get("Content-Type"))
	}

	call(samToken, "PUT", "/api/v1/users/"+admin.ID+"/avatar", png, 403)
	call(helpdesk, "PUT", "/api/v1/users/"+admin.ID+"/avatar", png, 403)
	call(helpdesk, "DELETE", "/api/v1/users/"+sam.ID+"/avatar", nil, 200)
	call("", "GET", "/api/v1/users/"+sam.ID+"/avatar", nil, 404)
	call(samToken, "DELETE", "/api/v1/me/avatar", nil, 404)

	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Aether", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://aether.example.com/callback"}})
	if err != nil {
		t.Fatal(err)
	}
	_, appAdmin := person("ada@example.com", "app_admin")
	logo := "/api/v1/applications/" + app.ID + "/logo"
	svg := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><script>alert(1)</script></svg>`)
	call(samToken, "PUT", logo, svg, 403)
	call(helpdesk, "PUT", logo, svg, 403)
	call(appAdmin, "PUT", logo, []byte(`<svg viewBox="0 0 8 8"/>`), 422)
	call(appAdmin, "PUT", logo, []byte(`<html><body><svg xmlns="http://www.w3.org/2000/svg"/></body></html>`), 422)
	call(appAdmin, "PUT", logo, append(png, make([]byte, 512<<10)...), 413)
	call(appAdmin, "PUT", logo, svg, 200)
	rec = call("", "GET", logo, nil, 200)
	if rec.Header().Get("Content-Type") != "image/svg+xml" || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "sandbox") || !bytes.Equal(rec.Body.Bytes(), svg) {
		t.Fatalf("served %q with CSP %q", rec.Header().Get("Content-Type"), rec.Header().Get("Content-Security-Policy"))
	}
	if a, _ := st.GetApplication(ctx, app.ID); a.LogoURL == nil {
		t.Fatal("no logo URL after upload")
	}
	if events, _ := st.ListAudit(ctx, 1); len(events) == 0 || events[0].Action != "application.logo.update" || events[0].TargetID != app.ID {
		t.Fatalf("audit %+v", events)
	}
	call(appAdmin, "DELETE", logo, nil, 200)
	call("", "GET", logo, nil, 404)
	call(appAdmin, "DELETE", logo, nil, 404)
}
