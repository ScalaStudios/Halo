package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"halo/internal/config"
	"halo/internal/forwardauth"
	"halo/internal/httpx"
	"halo/internal/store"
)

type forwardEnv struct {
	*env
	app store.Application
}

func (f *forwardEnv) verify(host, uri string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.verifyFrom("192.0.2.1:4000", f.app.ID, host, uri, cookies...)
}

func (f *forwardEnv) verifyFrom(peer, appID, host, uri string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest("GET", "/forward-auth/"+appID+"/verify", nil)
	req.RemoteAddr = peer
	req.Header.Set("X-Forwarded-Host", host)
	req.Header.Set("X-Forwarded-Uri", uri)
	for _, c := range cookies {
		if c != nil {
			req.AddCookie(c)
		}
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	if cookie(rec, httpx.DeviceCookie) != nil || cookie(rec, httpx.SessionCookie) != nil {
		f.t.Fatalf("verify must never set Halo's own cookies: %v", rec.Header().Values("Set-Cookie"))
	}
	return rec
}

func (f *forwardEnv) open(location, session string) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest("GET", location, nil)
	if session != "" {
		req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: session})
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	return rec
}

func (f *forwardEnv) startURL(rd, state string) string {
	return "/forward-auth/" + f.app.ID + "/start?" + url.Values{"rd": {rd}, "state": {state}}.Encode()
}

func (f *forwardEnv) signIn(session, uri string) (*http.Cookie, *http.Cookie) {
	f.t.Helper()
	first := f.verify("files.example.com", uri)
	location, _ := url.Parse(first.Header().Get("Location"))
	state := cookie(first, forwardauth.StateCookie)
	if first.Code != http.StatusFound || location.Path != "/forward-auth/"+f.app.ID+"/start" || location.Query().Get("rd") != "https://files.example.com"+uri || state == nil || !state.Secure || !state.HttpOnly {
		f.t.Fatalf("unauthenticated check must redirect to start with the return URL and set a state cookie: %d %q %v", first.Code, location, state)
	}
	started := f.open(location.RequestURI(), session)
	callback, _ := url.Parse(started.Header().Get("Location"))
	if started.Code != http.StatusSeeOther || callback.Scheme+"://"+callback.Host != "https://files.example.com" || callback.Path != forwardauth.CallbackPath {
		f.t.Fatalf("start must send the browser to the callback on the protected host: %d %q %s", started.Code, callback, started.Body)
	}
	back := f.verify("files.example.com", callback.RequestURI(), state)
	appCookie := cookie(back, forwardauth.CookieName(f.app.ID))
	if back.Code != http.StatusFound || back.Header().Get("Location") != "https://files.example.com"+uri || appCookie == nil || !appCookie.Secure || !appCookie.HttpOnly || appCookie.Value == callback.Query().Get("code") {
		f.t.Fatalf("callback must set a fresh app cookie and return to the original URL: %d %q %v", back.Code, back.Header().Get("Location"), appCookie)
	}
	return appCookie, state
}

func cookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func setupForward(t *testing.T, addresses ...string) (*forwardEnv, string, store.Group) {
	t.Helper()
	e := setup(t, func(c *config.Config) { c.TrustedProxies = []string{"192.0.2.0/24"} })
	_, admin, _ := e.person("admin@example.com", "passkey", false, "global_admin")
	staff, err := e.st.CreateGroup(context.Background(), store.NewGroup{Name: "Staff", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	if len(addresses) == 0 {
		addresses = []string{"https://files.example.com"}
	}
	rec := e.expect(admin, "POST", "/api/v1/applications", map[string]any{"name": "Files", "protocol": "forward-auth", "redirectUris": addresses, "groupIds": []string{staff.ID}}, http.StatusCreated, "")
	var out struct {
		Application  store.Application `json:"application"`
		ClientSecret *string           `json:"clientSecret"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.ClientSecret != nil || out.Application.Homepage != addresses[0] || len(out.Application.Credentials) != 0 {
		t.Fatalf("a forward-auth application has no secret and opens its first address: %+v", out)
	}
	return &forwardEnv{env: e, app: out.Application}, admin, staff
}

func TestForwardAuthSignsInAssignedPeopleWithHeaders(t *testing.T) {
	f, _, staff := setupForward(t)
	ctx := context.Background()
	ada, session, _ := f.person("ada@example.com", "passkey", false)
	if err := f.st.AddGroupMember(ctx, staff.ID, ada.ID); err != nil {
		t.Fatal(err)
	}
	appCookie, _ := f.signIn(session, "/reports?year=2026")
	ok := f.verify("files.example.com", "/reports?year=2026", appCookie)
	if ok.Code != http.StatusOK || ok.Header().Get("X-Halo-User") != ada.ID || ok.Header().Get("X-Halo-Email") != "ada@example.com" || ok.Header().Get("X-Halo-Name") != "ada" || ok.Header().Get("X-Halo-Groups") != "Staff" {
		t.Fatalf("assigned person must get identity headers: %d %v", ok.Code, ok.Header())
	}
	events, err := f.st.ListSignIns(ctx, store.SignInFilter{AppID: f.app.ID})
	if err != nil || len(events) != 1 || events[0].Result != "success" || *events[0].UserID != ada.ID {
		t.Fatalf("one successful sign-in must be logged: %v %+v", err, events)
	}
	if forged := f.verifyFrom("203.0.113.9:4000", f.app.ID, "files.example.com", "/", appCookie); forged.Code != http.StatusForbidden || forged.Header().Get("X-Halo-User") != "" {
		t.Fatalf("forwarded headers from an untrusted address must be ignored: %d %v", forged.Code, forged.Header())
	}
	if other := f.verifyFrom("192.0.2.1:4000", f.app.ID, "files.example.com", "/", &http.Cookie{Name: forwardauth.CookieName("app_other"), Value: appCookie.Value}); other.Code != http.StatusFound {
		t.Fatalf("an app cookie only counts under its own application's name: %d", other.Code)
	}

	req := httptest.NewRequest("GET", "/forward-auth/"+f.app.ID+"/verify?nginx", nil)
	req.Header.Set("X-Forwarded-Host", "files.example.com")
	req.Header.Set("X-Forwarded-Uri", "/")
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Header().Get("Location"), "/forward-auth/"+f.app.ID+"/start?") {
		t.Fatalf("nginx mode must answer 401 with the sign-in address: %d %v", rec.Code, rec.Header())
	}

	signOut := f.verify("files.example.com", forwardauth.SignOutPath, appCookie)
	cleared := cookie(signOut, forwardauth.CookieName(f.app.ID))
	if signOut.Code != http.StatusFound || signOut.Header().Get("Location") != "http://localhost:3200/signed-out" || cleared == nil || cleared.MaxAge >= 0 {
		t.Fatalf("sign-out must clear the app cookie and end at Halo's sign-out page: %d %v", signOut.Code, signOut.Header())
	}
	if again := f.verify("files.example.com", "/", appCookie); again.Code != http.StatusFound {
		t.Fatalf("a signed-out app cookie must not work: %d", again.Code)
	}
}

func TestForwardAuthRefusesPeopleWithoutAccess(t *testing.T) {
	f, _, staff := setupForward(t)
	ctx := context.Background()
	ada, adaSession, _ := f.person("ada@example.com", "passkey", false)
	_, bobSession, _ := f.person("bob@example.com", "passkey", false)
	carol, carolSession, _ := f.person("carol@example.com", "passkey", false)
	for _, u := range []string{ada.ID, carol.ID} {
		if err := f.st.AddGroupMember(ctx, staff.ID, u); err != nil {
			t.Fatal(err)
		}
	}
	state := strings.Repeat("s", 43)
	unassigned := f.open(f.startURL("https://files.example.com/", state), bobSession)
	if unassigned.Code != http.StatusForbidden || !strings.Contains(unassigned.Body.String(), "You don&#39;t have access to Files") {
		t.Fatalf("unassigned person must get 403: %d %s", unassigned.Code, unassigned.Body)
	}
	if _, err := f.st.CreateAccessPolicy(ctx, store.AccessPolicy{Name: "No files for Carol", Enabled: true, Mode: "enforce", Effect: "block", Conditions: store.PolicyConditions{UserIDs: []string{carol.ID}, AppIDs: []string{f.app.ID}}}); err != nil {
		t.Fatal(err)
	}
	blocked := f.open(f.startURL("https://files.example.com/", state), carolSession)
	if blocked.Code != http.StatusForbidden || !strings.Contains(blocked.Body.String(), "No files for Carol") {
		t.Fatalf("policy must block: %d %s", blocked.Code, blocked.Body)
	}
	events, err := f.st.ListSignIns(ctx, store.SignInFilter{AppID: f.app.ID})
	if err != nil || len(events) != 2 || slices.ContainsFunc(events, func(e store.SignInEvent) bool { return e.Result != "failure" || e.Reason == "" }) {
		t.Fatalf("refusals must be logged with a reason: %v %+v", err, events)
	}

	appCookie, _ := f.signIn(adaSession, "/")
	if err := f.st.RemoveGroupMember(ctx, staff.ID, ada.ID); err != nil {
		t.Fatal(err)
	}
	if removed := f.verify("files.example.com", "/", appCookie); removed.Code != http.StatusForbidden || removed.Header().Get("X-Halo-User") != "" {
		t.Fatalf("a person removed from the application must get 403 on the next request: %d %v", removed.Code, removed.Header())
	}
	if err := f.st.AddGroupMember(ctx, staff.ID, ada.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.st.SetUserStatus(ctx, ada.ID, "suspended"); err != nil {
		t.Fatal(err)
	}
	if suspended := f.verify("files.example.com", "/", appCookie); suspended.Code != http.StatusFound {
		t.Fatalf("a suspended person's app cookie must stop working: %d", suspended.Code)
	}
	if err := f.st.SetUserStatus(ctx, ada.ID, "active"); err != nil {
		t.Fatal(err)
	}
	sess, err := f.st.SessionByToken(ctx, adaSession)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.RevokeSession(ctx, sess.ID, ada.ID); err != nil {
		t.Fatal(err)
	}
	if revoked := f.verify("files.example.com", "/", appCookie); revoked.Code != http.StatusFound {
		t.Fatalf("ending the Halo session must end the app session: %d", revoked.Code)
	}
}

func TestForwardAuthSignInFrequency(t *testing.T) {
	f, _, staff := setupForward(t)
	ctx := context.Background()
	ada, session, _ := f.person("ada@example.com", "passkey", false)
	if err := f.st.AddGroupMember(ctx, staff.ID, ada.ID); err != nil {
		t.Fatal(err)
	}
	appCookie, state := f.signIn(session, "/")
	app, err := f.st.GetApplication(ctx, f.app.ID)
	if err != nil {
		t.Fatal(err)
	}
	app.MaxAuthAge = 1
	if err := f.st.UpdateApplication(ctx, app); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	stale := f.verify("files.example.com", "/", appCookie)
	if stale.Code != http.StatusFound || !strings.Contains(stale.Header().Get("Location"), "/start?") {
		t.Fatalf("a session past the sign-in frequency must go back to Halo: %d %v", stale.Code, stale.Header())
	}
	again := f.open(f.startURL("https://files.example.com/", state.Value), session)
	if location := again.Header().Get("Location"); again.Code != http.StatusSeeOther || !strings.HasPrefix(location, "/sign-in?next="+url.QueryEscape("/forward-auth/"+f.app.ID+"/start?")) {
		t.Fatalf("start must ask for a fresh sign-in: %d %q", again.Code, location)
	}
}

func TestForwardAuthRefusesOpenRedirectsAndForgedHeaders(t *testing.T) {
	f, _, staff := setupForward(t, "https://files.example.com", "https://docs.example.com")
	ctx := context.Background()
	ada, session, _ := f.person("ada@example.com", "passkey", false)
	if err := f.st.AddGroupMember(ctx, staff.ID, ada.ID); err != nil {
		t.Fatal(err)
	}
	state := strings.Repeat("s", 43)
	for _, rd := range []string{
		"https://evil.example.com/",
		"https://files.example.com.evil.com/",
		"https://files.example.com@evil.com/",
		"https://user:pass@files.example.com/",
		"https://files.example.com\\@evil.com/",
		"//evil.com/",
		"//files.example.com/",
		"https:evil.com",
		"/relative",
		"javascript:alert(1)",
		"http://files.example.com/",
		"https://files.example.com:8443/",
		"https://files.example.com/\r\nLocation: https://evil.com",
		"",
	} {
		if rec := f.open(f.startURL(rd, state), session); rec.Code != http.StatusBadRequest {
			t.Fatalf("start must refuse %q: %d %q", rd, rec.Code, rec.Header().Get("Location"))
		}
	}
	if rec := f.open(f.startURL("https://files.example.com/", ""), session); rec.Code != http.StatusBadRequest {
		t.Fatalf("start must refuse a missing state: %d", rec.Code)
	}
	accepted := f.open(f.startURL("https://DOCS.example.com//x?y=1#frag", state), session)
	if location := accepted.Header().Get("Location"); accepted.Code != http.StatusSeeOther || !strings.HasPrefix(location, "https://docs.example.com/_halo/callback?code=") {
		t.Fatalf("start must accept a configured host in any case: %d %q", accepted.Code, location)
	}
	code := strings.TrimPrefix(accepted.Header().Get("Location"), "https://docs.example.com")
	stateCookie := &http.Cookie{Name: forwardauth.StateCookie, Value: state}
	if rec := f.verify("files.example.com", code, stateCookie); rec.Code != http.StatusForbidden {
		t.Fatalf("a code must only work on the host it was issued for: %d", rec.Code)
	}
	if rec := f.verify("docs.example.com", code, &http.Cookie{Name: forwardauth.StateCookie, Value: strings.Repeat("t", 43)}); rec.Code != http.StatusForbidden {
		t.Fatalf("a code must only work in the browser that started sign-in: %d", rec.Code)
	}
	if rec := f.verify("docs.example.com", code, stateCookie); rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://docs.example.com//x?y=1" {
		t.Fatalf("the code must work once in the right browser: %d %v", rec.Code, rec.Header())
	}
	if rec := f.verify("docs.example.com", code, stateCookie); rec.Code != http.StatusForbidden {
		t.Fatalf("a code must not work twice: %d", rec.Code)
	}

	if rec := f.verify("evil.example.com", "/"); rec.Code != http.StatusForbidden || rec.Header().Get("Location") != "" {
		t.Fatalf("an unknown host must not be redirected anywhere: %d %v", rec.Code, rec.Header())
	}
	if rec := f.verify("files.example.com", "//evil.com/"); rec.Code != http.StatusFound || !strings.Contains(rec.Header().Get("Location"), url.QueryEscape("https://files.example.com//evil.com/")) {
		t.Fatalf("the return URL must stay on the protected host: %d %v", rec.Code, rec.Header())
	}
	appCookie, _ := f.signIn(session, "/")
	forged := f.verifyFrom("203.0.113.9:4000", f.app.ID, "files.example.com", "/", appCookie)
	if forged.Code != http.StatusForbidden || forged.Header().Get("X-Halo-User") != "" || forged.Header().Get("Location") != "" {
		t.Fatalf("forwarded headers from an untrusted address must be ignored: %d %v", forged.Code, forged.Header())
	}
}

func TestForwardAuthHeaders(t *testing.T) {
	f, admin, staff := setupForward(t)
	ctx := context.Background()
	ops, err := f.st.CreateGroup(ctx, store.NewGroup{Name: "Ops,Admins", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	ada, err := f.st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Lovelace, Ada\r\nX-Injected: 1\n", Status: "active", GroupIDs: []string{staff.ID, ops.ID}})
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := f.st.CreateSession(ctx, store.NewSession{UserID: ada.ID, Method: "passkey"})
	if err != nil {
		t.Fatal(err)
	}
	f.expect(admin, "PATCH", "/api/v1/applications/"+f.app.ID, map[string]any{"claimMappings": []store.Claim{{Name: "Set-Cookie", Source: "user.name"}}}, http.StatusUnprocessableEntity, "ERR_INVALID")
	f.expect(admin, "PATCH", "/api/v1/applications/"+f.app.ID, map[string]any{"claimMappings": []store.Claim{{Name: "X-Forwarded-User", Source: "user.email"}}}, http.StatusUnprocessableEntity, "ERR_INVALID")
	f.expect(admin, "PATCH", "/api/v1/applications/"+f.app.ID, map[string]any{"claimMappings": []store.Claim{{Name: "remote-user", Source: "user.email"}, {Name: "x-halo-user", Source: "none"}}}, http.StatusOK, "")
	f.expect(admin, "PATCH", "/api/v1/applications/"+f.app.ID, map[string]any{"scopes": []string{"openid"}}, http.StatusUnprocessableEntity, "ERR_INVALID")
	f.expect(admin, "PATCH", "/api/v1/applications/"+f.app.ID, map[string]any{"backchannelLogoutUri": "https://files.example.com/logout"}, http.StatusUnprocessableEntity, "ERR_INVALID")
	f.expect(admin, "PATCH", "/api/v1/applications/"+f.app.ID, map[string]any{"redirectUris": []string{"https://files.example.com/app"}}, http.StatusUnprocessableEntity, "ERR_INVALID")
	f.expect(admin, "POST", "/api/v1/applications/"+f.app.ID+"/secrets", nil, http.StatusUnprocessableEntity, "ERR_INVALID")
	f.expect(admin, "POST", "/api/v1/applications", map[string]any{"name": "Wiki", "protocol": "forward-auth", "redirectUris": []string{"https://wiki.example.com/login"}}, http.StatusUnprocessableEntity, "ERR_INVALID")

	if _, err := f.st.CreateAppRole(ctx, f.app.ID, store.AppRole{Key: "editor", Name: "Editor", GroupIDs: []string{staff.ID}}); err != nil {
		t.Fatal(err)
	}
	f.app, err = f.st.ForwardAuthApplication(ctx, f.app.ID)
	if err != nil {
		t.Fatal(err)
	}
	appCookie, _ := f.signIn(session, "/")
	ok := f.verify("files.example.com", "/", appCookie)
	if ok.Code != http.StatusOK || ok.Header().Get("Remote-User") != "ada@example.com" || ok.Header().Values("X-Halo-User") != nil || ok.Header().Get("X-Halo-Name") != "Lovelace, AdaX-Injected: 1" || ok.Header().Get("X-Injected") != "" ||
		!slices.Equal(slices.Sorted(slices.Values(strings.Split(ok.Header().Get("X-Halo-Groups"), ","))), []string{"Ops Admins", "Staff"}) || ok.Header().Get("X-Halo-Roles") != "editor" {
		t.Fatalf("headers must follow the mappings and carry no line breaks: %d %v", ok.Code, ok.Header())
	}
}
