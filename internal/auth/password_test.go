package auth_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"
	"github.com/pquerna/otp/totp"

	"halo/internal/httpx"
	"halo/internal/secret"
	"halo/internal/store"
)

const authenticatorSeed = "JBSWY3DPEHPK3PXP"

func (e *env) org(change func(*store.Settings)) {
	e.t.Helper()
	ctx := context.Background()
	s, err := e.st.Settings(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	change(&s)
	if err := e.st.SaveSettings(ctx, s); err != nil {
		e.t.Fatal(err)
	}
	e.st.ForgetSettings()
}

func (e *env) allowPasswords(secondFactor bool) {
	e.t.Helper()
	if err := e.st.SetMethodEnabled(context.Background(), "password", true); err != nil {
		e.t.Fatal(err)
	}
	e.org(func(s *store.Settings) { s.PasswordSecondFactor, s.PasswordBreachCheck = secondFactor, false })
}

func (e *env) withPassword(email, password string, temporary bool) store.User {
	e.t.Helper()
	ctx := context.Background()
	u, err := e.st.CreateUser(ctx, store.NewUser{Email: email, Name: strings.Split(email, "@")[0], Status: "active"})
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.st.SetPassword(ctx, u.ID, secret.HashPassword(password), temporary); err != nil {
		e.t.Fatal(err)
	}
	return u
}

func (e *env) withAuthenticator(u store.User) {
	e.t.Helper()
	ctx := context.Background()
	secretID, err := e.st.CreateTOTPSecret(ctx, u.ID, "Authenticator app", e.st.Sealer.Seal([]byte(authenticatorSeed)))
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.st.ConfirmTOTPSecret(ctx, secretID, "Authenticator app", 0); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) reasons(u store.User) []string {
	e.t.Helper()
	events, err := e.st.ListSignIns(context.Background(), store.SignInFilter{UserID: u.ID})
	if err != nil {
		e.t.Fatal(err)
	}
	reasons := []string{}
	for _, ev := range slices.Backward(events) {
		if ev.Method != "password" {
			e.t.Fatalf("password attempt logged as %s", ev.Method)
		}
		reasons = append(reasons, ev.Result+": "+ev.Reason)
	}
	return reasons
}

func (e *env) identify(email string) []string {
	var out struct{ Methods []string }
	e.do(client(), "POST", "/api/v1/auth/identify", map[string]any{"email": email}, &out)
	return out.Methods
}

func TestPasswordSignInGating(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	ada := e.withPassword("ada@example.com", "correct horse battery staple", false)
	signIn := map[string]any{"email": "ada@example.com", "password": "correct horse battery staple"}

	e.expect(client(), "POST", "/api/v1/auth/password", signIn, http.StatusUnauthorized, "ERR_PASSWORD_MISMATCH")

	if err := e.st.SetMethodEnabled(ctx, "password", true); err != nil {
		t.Fatal(err)
	}
	e.expect(client(), "POST", "/api/v1/auth/password", signIn, http.StatusForbidden, "ERR_SECOND_FACTOR_MISSING")

	e.allowPasswords(false)
	contractors, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Contractors", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	e.org(func(s *store.Settings) { s.PasswordGroupIDs = []string{contractors.ID} })
	e.expect(client(), "POST", "/api/v1/auth/password", signIn, http.StatusUnauthorized, "ERR_PASSWORD_MISMATCH")
	if methods := e.identify("ada@example.com"); slices.Contains(methods, "password") {
		t.Fatalf("identify offers a password outside the allowed groups: %v", methods)
	}

	if err := e.st.AddGroupMember(ctx, contractors.ID, ada.ID); err != nil {
		t.Fatal(err)
	}
	if methods := e.identify("ada@example.com"); !slices.Equal(methods, []string{"password"}) {
		t.Fatalf("identify: %v", methods)
	}
	e.expect(client(), "POST", "/api/v1/auth/password", map[string]any{"email": "ada@example.com", "password": "correct horse battery stapler"}, http.StatusUnauthorized, "ERR_PASSWORD_MISMATCH")
	e.expect(client(), "POST", "/api/v1/auth/password", map[string]any{"email": "nobody@example.com", "password": "correct horse battery staple"}, http.StatusUnauthorized, "ERR_PASSWORD_MISMATCH")

	c := client()
	var out struct{ Redirect string }
	if status := e.do(c, "POST", "/api/v1/auth/password", signIn, &out); status != http.StatusOK || out.Redirect != "/account" {
		t.Fatalf("password sign-in: %d %+v", status, out)
	}
	var sessions []store.Session
	if e.do(c, "GET", "/api/v1/me/sessions", nil, &sessions); len(sessions) != 1 || sessions[0].Method != "password" {
		t.Fatalf("sessions: %+v", sessions)
	}
	me, _ := e.me(c)
	if i := slices.IndexFunc(me.Methods, func(m store.Method) bool { return m.Kind == "password" }); i < 0 || me.Methods[i].LastUsedAt == nil || me.Strength != "single-factor" {
		t.Fatalf("password method: %+v", me)
	}

	want := []string{
		"failure: Sign-in with passwords is turned off for this organization.",
		"failure: The password was correct, but the account has no authenticator app for the second factor.",
		"failure: Sign-in with passwords is limited to groups this account is not in.",
		"failure: The password did not match.",
		"success: ",
	}
	if got := e.reasons(ada); !slices.Equal(got, want) {
		t.Fatalf("sign-in log:\n%s", strings.Join(got, "\n"))
	}
}

func TestPasswordSecondFactor(t *testing.T) {
	e := newEnv(t)
	ada := e.withPassword("ada@example.com", "correct horse battery staple", false)
	e.withAuthenticator(ada)
	e.allowPasswords(true)

	c := client()
	signIn := map[string]any{"email": "ada@example.com", "password": "correct horse battery staple"}
	e.expect(c, "POST", "/api/v1/auth/password", signIn, http.StatusUnauthorized, "ERR_SECOND_FACTOR_REQUIRED")
	if _, status := e.me(c); status != http.StatusUnauthorized {
		t.Fatalf("a password alone must not start a session: %d", status)
	}
	signIn["code"] = "000000"
	e.expect(c, "POST", "/api/v1/auth/password", signIn, http.StatusUnauthorized, "ERR_CODE_MISMATCH")

	code, _ := totp.GenerateCode(authenticatorSeed, time.Now())
	e.expect(c, "POST", "/api/v1/auth/password", map[string]any{"email": "ada@example.com", "password": "not the password at all", "code": code}, http.StatusUnauthorized, "ERR_PASSWORD_MISMATCH")
	signIn["code"] = code
	if status := e.do(c, "POST", "/api/v1/auth/password", signIn, nil); status != http.StatusOK {
		t.Fatalf("password and code: %d", status)
	}
	if _, status := e.me(c); status != http.StatusOK {
		t.Fatalf("session after second factor: %d", status)
	}
	e.expect(client(), "POST", "/api/v1/auth/password", signIn, http.StatusUnauthorized, "ERR_CODE_MISMATCH")
}

func TestPasswordFailuresCountTowardLockout(t *testing.T) {
	e := newEnv(t)
	ada := e.withPassword("ada@example.com", "correct horse battery staple", false)
	e.withAuthenticator(ada)
	e.allowPasswords(false)

	for range 5 {
		e.expect(client(), "POST", "/api/v1/auth/password", map[string]any{"email": "ada@example.com", "password": "guess number whatever"}, http.StatusUnauthorized, "ERR_PASSWORD_MISMATCH")
	}
	e.expect(client(), "POST", "/api/v1/auth/password", map[string]any{"email": "ada@example.com", "password": "correct horse battery staple"}, http.StatusTooManyRequests, "ERR_TOO_MANY_ATTEMPTS")
	code, _ := totp.GenerateCode(authenticatorSeed, time.Now())
	e.expect(client(), "POST", "/api/v1/auth/totp", map[string]any{"email": "ada@example.com", "code": code}, http.StatusTooManyRequests, "ERR_TOO_MANY_ATTEMPTS")

	for range 5 {
		e.expect(client(), "POST", "/api/v1/auth/password", map[string]any{"email": "ghost@example.com", "password": "guess number whatever"}, http.StatusUnauthorized, "ERR_PASSWORD_MISMATCH")
	}
	e.expect(client(), "POST", "/api/v1/auth/password", map[string]any{"email": "ghost@example.com", "password": "guess number whatever"}, http.StatusTooManyRequests, "ERR_TOO_MANY_ATTEMPTS")
}

type forwardedFor string

func (f forwardedFor) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("X-Forwarded-For", string(f))
	return http.DefaultTransport.RoundTrip(r)
}

func from(ip string) *http.Client {
	return &http.Client{Transport: forwardedFor(ip)}
}

func TestPasswordSprayFromOneAddress(t *testing.T) {
	e := newEnv(t)
	e.withPassword("ada@example.com", "correct horse battery staple", false)
	e.allowPasswords(false)
	httpx.TrustProxies([]string{"127.0.0.1/32", "::1/128"})
	t.Cleanup(func() { httpx.TrustProxies(nil) })

	spray := func(c *http.Client, n int, status int, code string) {
		t.Helper()
		for i := range n {
			e.expect(c, "POST", "/api/v1/auth/password", map[string]any{"email": fmt.Sprintf("victim%d@example.com", i), "password": "Summer2026!Summer2026!"}, status, code)
		}
	}
	spray(from("203.0.113.7"), 20, http.StatusUnauthorized, "ERR_PASSWORD_MISMATCH")
	spray(from("203.0.113.7"), 1, http.StatusTooManyRequests, "ERR_TOO_MANY_ATTEMPTS")
	e.expect(from("203.0.113.7"), "POST", "/api/v1/auth/password", map[string]any{"email": "ada@example.com", "password": "correct horse battery staple"}, http.StatusTooManyRequests, "ERR_TOO_MANY_ATTEMPTS")
	e.expect(from("6.6.6.6, 203.0.113.7"), "POST", "/api/v1/auth/password", map[string]any{"email": "ada@example.com", "password": "correct horse battery staple"}, http.StatusTooManyRequests, "ERR_TOO_MANY_ATTEMPTS")

	if status := e.do(from("198.51.100.4"), "POST", "/api/v1/auth/password", map[string]any{"email": "ada@example.com", "password": "correct horse battery staple"}, nil); status != http.StatusOK {
		t.Fatalf("a different address must not be throttled: %d", status)
	}
	events, err := e.st.ListSignIns(context.Background(), store.SignInFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(events, func(ev store.SignInEvent) bool { return ev.Reason == "Too many failed passwords from this IP address." }); i < 0 || events[i].IP != "203.0.113.7" || events[i].Result != "failure" {
		t.Fatalf("throttled attempts are not in the sign-in log: %+v", events)
	}

	httpx.TrustProxies(nil)
	spray(from("192.0.2.1"), 20, http.StatusUnauthorized, "ERR_PASSWORD_MISMATCH")
	spray(from("192.0.2.2"), 1, http.StatusTooManyRequests, "ERR_TOO_MANY_ATTEMPTS")
}

func TestPasswordAMR(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	ada := e.withPassword("ada@example.com", "correct horse battery staple", false)
	e.withAuthenticator(ada)
	group, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Engineering", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.AddGroupMember(ctx, group.ID, ada.ID); err != nil {
		t.Fatal(err)
	}
	app, err := e.st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://grafana.example.com/login/generic_oauth"}, GroupIDs: []string{group.ID}})
	if err != nil {
		t.Fatal(err)
	}
	amr := func(c *http.Client, path string, body map[string]any) []string {
		t.Helper()
		requestID := "arq_" + strings.ToLower(secret.Token(8))
		if _, err := e.st.CreateAuthRequest(ctx, store.AuthRequest{ID: requestID, ClientID: app.ClientID, RedirectURI: app.RedirectURIs[0], Scopes: []string{"openid"}, ResponseType: "code"}); err != nil {
			t.Fatal(err)
		}
		body["authRequest"] = requestID
		if status := e.do(c, "POST", path, body, nil); status != http.StatusOK {
			t.Fatalf("%s: %d", path, status)
		}
		req, err := e.st.GetAuthRequest(ctx, requestID)
		if err != nil {
			t.Fatal(err)
		}
		return req.AMR
	}

	e.allowPasswords(false)
	if got := amr(client(), "/api/v1/auth/password", map[string]any{"email": "ada@example.com", "password": "correct horse battery staple"}); !slices.Equal(got, []string{"pwd"}) {
		t.Fatalf("password alone: %v", got)
	}

	e.allowPasswords(true)
	c := client()
	code, _ := totp.GenerateCode(authenticatorSeed, time.Now())
	if got := amr(c, "/api/v1/auth/password", map[string]any{"email": "ada@example.com", "password": "correct horse battery staple", "code": code}); !slices.Equal(got, []string{"pwd", "otp", "mfa"}) {
		t.Fatalf("password and code: %v", got)
	}
	if got := amr(c, "/api/v1/auth/continue", map[string]any{}); !slices.Equal(got, []string{"pwd", "otp", "mfa"}) {
		t.Fatalf("continuing the password and code session: %v", got)
	}
}

func TestTemporaryPasswordMustBeChanged(t *testing.T) {
	e := newEnv(t)
	ada := e.withPassword("ada@example.com", "temporary passphrase one", true)
	e.allowPasswords(false)

	c := client()
	signIn := map[string]any{"email": "ada@example.com", "password": "temporary passphrase one"}
	e.expect(c, "POST", "/api/v1/auth/password", signIn, http.StatusUnauthorized, "ERR_PASSWORD_CHANGE_REQUIRED")
	if _, status := e.me(c); status != http.StatusUnauthorized {
		t.Fatalf("a temporary password must not start a session: %d", status)
	}
	for _, replacement := range []string{"temporary passphrase one", "too short"} {
		signIn["newPassword"] = replacement
		e.expect(c, "POST", "/api/v1/auth/password", signIn, http.StatusUnprocessableEntity, "ERR_INVALID")
	}
	signIn["newPassword"] = "a brand new passphrase"
	if status := e.do(c, "POST", "/api/v1/auth/password", signIn, nil); status != http.StatusOK {
		t.Fatalf("change at first sign-in: %d", status)
	}
	me, _ := e.me(c)
	if i := slices.IndexFunc(me.Methods, func(m store.Method) bool { return m.Kind == "password" }); i < 0 || me.Methods[i].Detail != "" || me.Methods[i].LastUsedAt == nil {
		t.Fatalf("password after the change: %+v", me.Methods)
	}
	e.expect(client(), "POST", "/api/v1/auth/password", map[string]any{"email": "ada@example.com", "password": "temporary passphrase one"}, http.StatusUnauthorized, "ERR_PASSWORD_MISMATCH")
	if status := e.do(client(), "POST", "/api/v1/auth/password", map[string]any{"email": "ada@example.com", "password": "a brand new passphrase"}, nil); status != http.StatusOK {
		t.Fatalf("sign-in with the new password: %d", status)
	}
	audit, _ := e.st.ListAudit(context.Background(), 1)
	if len(audit) != 1 || audit[0].Summary != "ada replaced their temporary password" || audit[0].TargetID != ada.ID {
		t.Fatalf("audit: %+v", audit)
	}

	grace := e.withPassword("grace@example.com", "temporary passphrase two", true)
	e.withAuthenticator(grace)
	e.org(func(s *store.Settings) { s.PasswordSecondFactor = true })
	signIn = map[string]any{"email": "grace@example.com", "password": "temporary passphrase two"}
	e.expect(client(), "POST", "/api/v1/auth/password", signIn, http.StatusUnauthorized, "ERR_PASSWORD_CHANGE_REQUIRED")
	signIn["newPassword"] = "graces own passphrase"
	e.expect(client(), "POST", "/api/v1/auth/password", signIn, http.StatusUnauthorized, "ERR_SECOND_FACTOR_REQUIRED")
	signIn["code"], _ = totp.GenerateCode(authenticatorSeed, time.Now())
	if status := e.do(client(), "POST", "/api/v1/auth/password", signIn, nil); status != http.StatusOK {
		t.Fatalf("change with a second factor: %d", status)
	}
}

func TestOwnPassword(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	ada, c, _ := e.enrolledUser("ada@example.com", virtualwebauthn.AuthenticatorOptions{})
	var state struct{ Allowed, SecondFactor bool }
	if e.do(c, "GET", "/api/v1/me/password", nil, &state); state.Allowed || !state.SecondFactor {
		t.Fatalf("passwords are off by default and need a second factor: %+v", state)
	}
	e.expect(c, "PUT", "/api/v1/me/password", map[string]any{"password": "my own long passphrase"}, http.StatusForbidden, "ERR_FORBIDDEN")

	e.allowPasswords(false)
	if e.do(c, "GET", "/api/v1/me/password", nil, &state); !state.Allowed || state.SecondFactor {
		t.Fatalf("passwords turned on for everyone without a second factor: %+v", state)
	}
	e.expect(c, "PUT", "/api/v1/me/password", map[string]any{"password": "short"}, http.StatusUnprocessableEntity, "ERR_INVALID")
	if status := e.do(c, "PUT", "/api/v1/me/password", map[string]any{"password": "my own long passphrase"}, nil); status != http.StatusNoContent {
		t.Fatalf("set password: %d", status)
	}
	var raw json.RawMessage
	if e.do(c, "GET", "/api/v1/me", nil, &raw); strings.Contains(string(raw), "argon2") || strings.Contains(string(raw), "passphrase") {
		t.Fatalf("password material in /me: %s", raw)
	}
	if status := e.do(client(), "POST", "/api/v1/auth/password", map[string]any{"email": "ada@example.com", "password": "my own long passphrase"}, nil); status != http.StatusOK {
		t.Fatalf("sign in with own password: %d", status)
	}
	if status := e.do(c, "PUT", "/api/v1/me/password", map[string]any{"password": "a changed long passphrase"}, nil); status != http.StatusNoContent {
		t.Fatalf("change password: %d", status)
	}
	e.expect(client(), "POST", "/api/v1/auth/password", map[string]any{"email": "ada@example.com", "password": "my own long passphrase"}, http.StatusUnauthorized, "ERR_PASSWORD_MISMATCH")

	group, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Contractors", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	e.org(func(s *store.Settings) { s.PasswordGroupIDs = []string{group.ID} })
	if e.do(c, "GET", "/api/v1/me/password", nil, &state); state.Allowed {
		t.Fatal("passwords are limited to a group ada is not in")
	}
	if status := e.do(c, "DELETE", "/api/v1/me/methods/password", nil, nil); status != http.StatusNoContent {
		t.Fatalf("remove password: %d", status)
	}
	if me, _ := e.me(c); slices.ContainsFunc(me.Methods, func(m store.Method) bool { return m.Kind == "password" }) {
		t.Fatalf("password still listed: %+v", me.Methods)
	}
	audit, _ := e.st.ListAudit(ctx, 3)
	if len(audit) != 3 || audit[0].Summary != "Ada Lovelace removed Password" || audit[1].Summary != "Ada Lovelace changed their password" || audit[2].Summary != "Ada Lovelace set a password" || audit[2].TargetID != ada.ID {
		t.Fatalf("audit: %+v", audit)
	}
}
