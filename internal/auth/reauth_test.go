package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/descope/virtualwebauthn"
)

func (e *env) reauthBegin(c *http.Client) (string, *virtualwebauthn.AssertionOptions) {
	e.t.Helper()
	var s started
	if status := e.do(c, "POST", "/api/v1/me/reauthenticate/begin", nil, &s); status != http.StatusOK {
		e.t.Fatalf("reauthenticate begin: %d", status)
	}
	opts, err := virtualwebauthn.ParseAssertionOptions(string(s.Options))
	if err != nil {
		e.t.Fatal(err)
	}
	return s.Ceremony, opts
}

func TestReauthenticateWithPasskey(t *testing.T) {
	e := newEnv(t)
	u, c, d := e.enrolledUser("ada@example.com", virtualwebauthn.AuthenticatorOptions{})
	_, other, _ := e.enrolledUser("grace@example.com", virtualwebauthn.AuthenticatorOptions{})
	assert := func(ceremony string, opts *virtualwebauthn.AssertionOptions) map[string]any {
		return map[string]any{"ceremony": ceremony, "credential": json.RawMessage(virtualwebauthn.CreateAssertionResponse(e.rp, d.auth, d.cred, *opts))}
	}

	foreign, opts := e.reauthBegin(other)
	e.expect(c, "POST", "/api/v1/me/reauthenticate", assert(foreign, opts), http.StatusUnauthorized, "ERR_PASSKEY_REJECTED")

	var signIn started
	e.do(client(), "POST", "/api/v1/auth/passkey/begin", nil, &signIn)
	signInOpts, err := virtualwebauthn.ParseAssertionOptions(string(signIn.Options))
	if err != nil {
		t.Fatal(err)
	}
	e.expect(c, "POST", "/api/v1/me/reauthenticate", assert(signIn.Ceremony, signInOpts), http.StatusUnauthorized, "ERR_PASSKEY_REJECTED")

	ceremony, opts := e.reauthBegin(c)
	e.expect(client(), "POST", "/api/v1/auth/passkey/finish", assert(ceremony, opts), http.StatusUnauthorized, "ERR_PASSKEY_REJECTED")

	ceremony, opts = e.reauthBegin(c)
	if status := e.do(c, "POST", "/api/v1/me/reauthenticate", assert(ceremony, opts), nil); status != http.StatusNoContent {
		t.Fatalf("reauthenticate: %d", status)
	}
	sessions, err := e.st.ListSessions(context.Background(), u.ID)
	if err != nil || len(sessions) != 1 || sessions[0].ReauthenticatedAt == nil {
		t.Fatalf("session not marked: %+v %v", sessions, err)
	}
}
