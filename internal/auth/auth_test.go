package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"
	"github.com/pquerna/otp/totp"

	"halo/internal/auth"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/secret"
	"halo/internal/store"
	"halo/internal/testdb"
)

const firefox = "Mozilla/5.0 (X11; Linux x86_64; rv:143.0) Gecko/20100101 Firefox/143.0"

type env struct {
	t   *testing.T
	st  *store.Store
	srv *httptest.Server
	rp  virtualwebauthn.RelyingParty
}

type device struct {
	auth virtualwebauthn.Authenticator
	cred virtualwebauthn.Credential
}

type apiError struct {
	Error struct{ Code string } `json:"error"`
}

func newEnv(t *testing.T) *env {
	st := testdb.New(t)
	public, _ := url.Parse("http://localhost:3200")
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	mux := http.NewServeMux()
	if err := auth.Register(mux, httpx.Deps{Config: config.Config{PublicURL: public, Dev: true}, Store: st, Auth: authn}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(authn.Resolve(mux))
	t.Cleanup(srv.Close)
	return &env{t: t, st: st, srv: srv, rp: virtualwebauthn.RelyingParty{ID: "localhost", Name: "Halo", Origin: "http://localhost:3200"}}
}

func client() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func (e *env) do(c *http.Client, method, path string, body, out any) int {
	e.t.Helper()
	var payload bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&payload).Encode(body)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, &payload)
	req.Header.Set("User-Agent", firefox)
	res, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

func (e *env) expect(c *http.Client, method, path string, body any, status int, code string) {
	e.t.Helper()
	var out apiError
	if got := e.do(c, method, path, body, &out); got != status || out.Error.Code != code {
		e.t.Fatalf("%s %s: want %d %s, got %d %s", method, path, status, code, got, out.Error.Code)
	}
}

type started struct {
	Ceremony string          `json:"ceremony"`
	Options  json.RawMessage `json:"options"`
}

func (e *env) register(c *http.Client, prefix string, begin, finish map[string]any, d *device) int {
	e.t.Helper()
	var s started
	if status := e.do(c, "POST", prefix+"/begin", begin, &s); status != http.StatusOK {
		return status
	}
	opts, err := virtualwebauthn.ParseAttestationOptions(string(s.Options))
	if err != nil {
		e.t.Fatal(err)
	}
	finish["ceremony"] = s.Ceremony
	finish["credential"] = json.RawMessage(virtualwebauthn.CreateAttestationResponse(e.rp, d.auth, d.cred, *opts))
	status := e.do(c, "POST", prefix+"/finish", finish, nil)
	d.auth.Options.UserHandle = []byte(opts.UserID)
	return status
}

func (e *env) enrolledUser(email string, opts virtualwebauthn.AuthenticatorOptions) (store.User, *http.Client, *device) {
	e.t.Helper()
	ctx := context.Background()
	u, err := e.st.CreateUser(ctx, store.NewUser{Email: email, Name: "Ada Lovelace"})
	if err != nil {
		e.t.Fatal(err)
	}
	token, _, err := e.st.CreateEnrollmentToken(ctx, u.ID, "invite", nil)
	if err != nil {
		e.t.Fatal(err)
	}
	c := client()
	d := &device{auth: virtualwebauthn.NewAuthenticatorWithOptions(opts), cred: virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)}
	if status := e.register(c, "/api/v1/auth/enroll", map[string]any{"token": token}, map[string]any{"token": token}, d); status != http.StatusOK {
		e.t.Fatalf("enroll: %d", status)
	}
	return u, c, d
}

func (e *env) passkey(c *http.Client, d *device, cred virtualwebauthn.Credential, authRequest string) (int, string, string) {
	e.t.Helper()
	var s started
	e.do(c, "POST", "/api/v1/auth/passkey/begin", map[string]any{}, &s)
	opts, err := virtualwebauthn.ParseAssertionOptions(string(s.Options))
	if err != nil {
		e.t.Fatal(err)
	}
	var out struct {
		Redirect string `json:"redirect"`
		apiError
	}
	body := map[string]any{"ceremony": s.Ceremony, "credential": json.RawMessage(virtualwebauthn.CreateAssertionResponse(e.rp, d.auth, cred, *opts))}
	if authRequest != "" {
		body["authRequest"] = authRequest
	}
	status := e.do(c, "POST", "/api/v1/auth/passkey/finish", body, &out)
	return status, out.Redirect, out.Error.Code
}

func (e *env) me(c *http.Client) (store.User, int) {
	var u store.User
	return u, e.do(c, "GET", "/api/v1/me", nil, &u)
}

func TestEnrollment(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	u, err := e.st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace"})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := e.st.CreateEnrollmentToken(ctx, u.ID, "invite", nil)
	if err != nil {
		t.Fatal(err)
	}
	c := client()
	var info struct {
		User    struct{ Name, Email string }
		Purpose string
	}
	if status := e.do(c, "GET", "/api/v1/auth/enroll?token="+token, nil, &info); status != http.StatusOK || info.User.Email != "ada@example.com" || info.Purpose != "invite" {
		t.Fatalf("enroll info: %d %+v", status, info)
	}
	d := &device{auth: virtualwebauthn.NewAuthenticator(), cred: virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)}
	if status := e.register(c, "/api/v1/auth/enroll", map[string]any{"token": token}, map[string]any{"token": token}, d); status != http.StatusOK {
		t.Fatalf("enroll: %d", status)
	}
	me, status := e.me(c)
	if status != http.StatusOK || me.Status != "active" || len(me.Methods) != 1 || me.Methods[0].Kind != "passkey" || me.Methods[0].Label != "Passkey · Firefox on Linux" {
		t.Fatalf("me after enroll: %d %+v", status, me)
	}
	e.expect(client(), "POST", "/api/v1/auth/enroll/begin", map[string]any{"token": token}, http.StatusNotFound, "ERR_LINK_EXPIRED")
	e.expect(client(), "GET", "/api/v1/auth/enroll?token="+token, nil, http.StatusNotFound, "ERR_LINK_EXPIRED")

	_, keyClient, _ := e.enrolledUser("grace@example.com", virtualwebauthn.AuthenticatorOptions{Transports: []virtualwebauthn.Transport{virtualwebauthn.TransportUSB}})
	if me, _ := e.me(keyClient); me.Methods[0].Kind != "security-key" {
		t.Fatalf("usb credential without backup should be a security key: %+v", me.Methods)
	}
}

func TestPasskeySignIn(t *testing.T) {
	e := newEnv(t)
	u, first, d := e.enrolledUser("ada@example.com", virtualwebauthn.AuthenticatorOptions{})

	c := client()
	d.cred.Counter = 5
	if status, redirect, _ := e.passkey(c, d, d.cred, ""); status != http.StatusOK || redirect != "/account" {
		t.Fatalf("passkey sign-in: %d %s", status, redirect)
	}
	if _, status := e.me(c); status != http.StatusOK {
		t.Fatalf("session cookie: %d", status)
	}

	forged := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	forged.ID = d.cred.ID
	if status, _, code := e.passkey(client(), d, forged, ""); status != http.StatusUnauthorized || code != "ERR_PASSKEY_REJECTED" {
		t.Fatalf("bad signature: %d %s", status, code)
	}
	d.cred.Counter = 3
	if status, _, code := e.passkey(client(), d, d.cred, ""); status != http.StatusUnauthorized || code != "ERR_PASSKEY_REJECTED" {
		t.Fatalf("counter regression: %d %s", status, code)
	}

	var sessions []store.Session
	e.do(c, "GET", "/api/v1/me/sessions", nil, &sessions)
	if len(sessions) != 2 || slices.IndexFunc(sessions, func(s store.Session) bool { return s.Current }) < 0 {
		t.Fatalf("sessions: %+v", sessions)
	}
	var revoked struct{ Revoked int }
	if e.do(c, "POST", "/api/v1/me/sessions/revoke-others", nil, &revoked); revoked.Revoked != 1 {
		t.Fatalf("revoke others: %+v", revoked)
	}
	if _, status := e.me(first); status != http.StatusUnauthorized {
		t.Fatalf("revoked session still works: %d", status)
	}

	if err := e.st.SetUserStatus(context.Background(), u.ID, "suspended"); err != nil {
		t.Fatal(err)
	}
	d.cred.Counter = 9
	if status, _, code := e.passkey(client(), d, d.cred, ""); status != http.StatusForbidden || code != "ERR_ACCOUNT_SUSPENDED" {
		t.Fatalf("suspended: %d %s", status, code)
	}

	events, err := e.st.ListSignIns(context.Background(), store.SignInFilter{UserID: u.ID})
	if err != nil {
		t.Fatal(err)
	}
	var successes, failures int
	for _, ev := range events {
		if ev.Result == "success" {
			successes++
			if ev.Device != "Firefox 143 · Linux" || ev.Method != "passkey" {
				t.Fatalf("success event: %+v", ev)
			}
		} else if ev.Reason == "" {
			t.Fatalf("failure without reason: %+v", ev)
		} else {
			failures++
		}
	}
	if successes != 2 || failures != 3 {
		t.Fatalf("events: %d successes, %d failures", successes, failures)
	}
}

func TestTOTP(t *testing.T) {
	e := newEnv(t)
	_, c, _ := e.enrolledUser("ada@example.com", virtualwebauthn.AuthenticatorOptions{})

	var begun struct{ Ceremony, Secret, URI string }
	e.do(c, "POST", "/api/v1/me/totp/begin", nil, &begun)
	if !strings.HasPrefix(begun.URI, "otpauth://totp/Halo:ada@example.com?") || !strings.Contains(begun.URI, "issuer=Halo") {
		t.Fatalf("totp uri: %s", begun.URI)
	}
	e.expect(c, "POST", "/api/v1/me/totp/confirm", map[string]any{"ceremony": begun.Ceremony, "code": "000000"}, http.StatusUnauthorized, "ERR_CODE_MISMATCH")
	now, _ := totp.GenerateCode(begun.Secret, time.Now())
	var m store.Method
	if status := e.do(c, "POST", "/api/v1/me/totp/confirm", map[string]any{"ceremony": begun.Ceremony, "code": now}, &m); status != http.StatusOK || m.Kind != "totp" {
		t.Fatalf("confirm: %d %+v", status, m)
	}

	var identified struct{ Methods []string }
	e.do(client(), "POST", "/api/v1/auth/identify", map[string]any{"email": "ADA@example.com"}, &identified)
	if !slices.Equal(identified.Methods, []string{"passkey", "totp"}) {
		t.Fatalf("identify: %v", identified.Methods)
	}
	e.do(client(), "POST", "/api/v1/auth/identify", map[string]any{"email": "nobody@example.com"}, &identified)
	if !slices.Equal(identified.Methods, []string{"passkey", "totp"}) {
		t.Fatalf("identify unknown: %v", identified.Methods)
	}

	next, _ := totp.GenerateCode(begun.Secret, time.Now().Add(30*time.Second))
	fresh := client()
	if status := e.do(fresh, "POST", "/api/v1/auth/totp", map[string]any{"email": "ada@example.com", "code": next}, nil); status != http.StatusOK {
		t.Fatalf("totp sign-in: %d", status)
	}
	if me, status := e.me(fresh); status != http.StatusOK || me.Email != "ada@example.com" {
		t.Fatalf("totp session: %d", status)
	}
	signIn := map[string]any{"email": "ada@example.com", "code": next}
	e.expect(client(), "POST", "/api/v1/auth/totp", signIn, http.StatusUnauthorized, "ERR_CODE_MISMATCH")
	e.expect(client(), "POST", "/api/v1/auth/totp", map[string]any{"email": "nobody@example.com", "code": next}, http.StatusUnauthorized, "ERR_CODE_MISMATCH")
	for range 4 {
		e.expect(client(), "POST", "/api/v1/auth/totp", map[string]any{"email": "ada@example.com", "code": "000000"}, http.StatusUnauthorized, "ERR_CODE_MISMATCH")
	}
	later, _ := totp.GenerateCode(begun.Secret, time.Now().Add(60*time.Second))
	e.expect(client(), "POST", "/api/v1/auth/totp", map[string]any{"email": "ada@example.com", "code": later}, http.StatusTooManyRequests, "ERR_TOO_MANY_ATTEMPTS")
}

func TestAuthRequestCompletion(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	u, signedIn, d := e.enrolledUser("ada@example.com", virtualwebauthn.AuthenticatorOptions{})
	app, err := e.st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", Homepage: "https://grafana.example.com", RedirectURIs: []string{"https://grafana.example.com/login/generic_oauth"}})
	if err != nil {
		t.Fatal(err)
	}
	newRequest := func(requestID string) string {
		if _, err := e.st.CreateAuthRequest(ctx, store.AuthRequest{ID: requestID, ClientID: app.ClientID, RedirectURI: app.RedirectURIs[0], Scopes: []string{"openid"}, ResponseType: "code"}); err != nil {
			t.Fatal(err)
		}
		return requestID
	}
	first := newRequest("arq_first")

	var info struct {
		Application struct{ Name, Homepage string }
	}
	if status := e.do(client(), "GET", "/api/v1/auth/request/"+first, nil, &info); status != http.StatusOK || info.Application.Name != "Grafana" {
		t.Fatalf("auth request info: %d %+v", status, info)
	}
	e.expect(client(), "GET", "/api/v1/auth/request/arq_missing", nil, http.StatusNotFound, "ERR_AUTH_REQUEST_EXPIRED")

	if status, _, code := e.passkey(client(), d, d.cred, first); status != http.StatusForbidden || code != "ERR_NOT_ASSIGNED" {
		t.Fatalf("unassigned: %d %s", status, code)
	}
	events, _ := e.st.ListSignIns(ctx, store.SignInFilter{AppID: app.ID})
	if len(events) != 1 || events[0].Result != "failure" || events[0].Reason != "User is not assigned to this application." {
		t.Fatalf("unassigned event: %+v", events)
	}

	group, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Observability", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.AddGroupMember(ctx, group.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.st.SetApplicationGroups(ctx, app.ID, []string{group.ID}); err != nil {
		t.Fatal(err)
	}
	status, redirect, _ := e.passkey(client(), d, d.cred, first)
	if status != http.StatusOK || redirect != "/oauth2/authorize/callback?id="+first {
		t.Fatalf("assigned: %d %s", status, redirect)
	}
	req, err := e.st.GetAuthRequest(ctx, first)
	if err != nil || !req.Done || req.UserID == nil || *req.UserID != u.ID || req.SessionID == nil || !slices.Equal(req.AMR, []string{"hwk", "mfa"}) {
		t.Fatalf("completed request: %+v %v", req, err)
	}

	second := newRequest("arq_second")
	e.expect(client(), "POST", "/api/v1/auth/continue", map[string]any{"authRequest": second}, http.StatusUnauthorized, "ERR_UNAUTHENTICATED")
	var out struct{ Redirect string }
	if status := e.do(signedIn, "POST", "/api/v1/auth/continue", map[string]any{"authRequest": second}, &out); status != http.StatusOK || out.Redirect != "/oauth2/authorize/callback?id="+second {
		t.Fatalf("continue: %d %+v", status, out)
	}
	e.expect(signedIn, "POST", "/api/v1/auth/continue", map[string]any{"authRequest": second}, http.StatusNotFound, "ERR_AUTH_REQUEST_EXPIRED")
}

func TestMethodsAndRecoveryCodes(t *testing.T) {
	e := newEnv(t)
	_, c, _ := e.enrolledUser("ada@example.com", virtualwebauthn.AuthenticatorOptions{})
	me, _ := e.me(c)
	passkeyID := me.Methods[0].ID

	e.expect(c, "DELETE", "/api/v1/me/methods/"+passkeyID, nil, http.StatusConflict, "ERR_LAST_METHOD")

	var generated struct{ Codes []string }
	format := regexp.MustCompile(`^[a-z2-7]{4}(-[a-z2-7]{4}){3}$`)
	if e.do(c, "POST", "/api/v1/me/recovery-codes", nil, &generated); len(generated.Codes) != 10 || !format.MatchString(generated.Codes[0]) {
		t.Fatalf("recovery codes: %+v", generated)
	}
	if err := e.st.UseRecoveryCode(context.Background(), me.ID, secret.Hash(strings.ReplaceAll(generated.Codes[2], "-", ""))); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("recovery codes must not be stored as unsalted SHA-256: %v", err)
	}
	e.expect(c, "DELETE", "/api/v1/me/methods/"+passkeyID, nil, http.StatusConflict, "ERR_LAST_METHOD")

	recovery := map[string]any{"email": "ada@example.com", "code": strings.ToUpper(generated.Codes[0])}
	if status := e.do(client(), "POST", "/api/v1/auth/recovery", recovery, nil); status != http.StatusOK {
		t.Fatalf("recovery sign-in: %d", status)
	}
	e.expect(client(), "POST", "/api/v1/auth/recovery", recovery, http.StatusUnauthorized, "ERR_CODE_MISMATCH")
	spaced := map[string]any{"email": "ada@example.com", "code": " " + strings.ReplaceAll(generated.Codes[1], "-", " ") + " "}
	if status := e.do(client(), "POST", "/api/v1/auth/recovery", spaced, nil); status != http.StatusOK {
		t.Fatalf("recovery sign-in with spaces: %d", status)
	}

	key := &device{auth: virtualwebauthn.NewAuthenticatorWithOptions(virtualwebauthn.AuthenticatorOptions{Transports: []virtualwebauthn.Transport{virtualwebauthn.TransportUSB}}), cred: virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)}
	if status := e.register(c, "/api/v1/me/passkeys", map[string]any{"kind": "security-key"}, map[string]any{"label": "YubiKey"}, key); status != http.StatusOK {
		t.Fatalf("add security key: %d", status)
	}
	if status := e.do(c, "DELETE", "/api/v1/me/methods/"+passkeyID, nil, nil); status != http.StatusNoContent {
		t.Fatalf("remove passkey with a security key left: %d", status)
	}
	me, _ = e.me(c)
	if len(me.Methods) != 2 || me.Methods[0].Kind != "security-key" || me.Methods[0].Label != "YubiKey" || me.Methods[1].Detail != "8 of 10 remaining" {
		t.Fatalf("methods after swap: %+v", me.Methods)
	}
}
