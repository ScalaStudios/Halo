package auth_test

import (
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"
	"github.com/pquerna/otp/totp"

	"halo/internal/access"
	"halo/internal/mail"
	"halo/internal/store"
)

func noSecrets(t *testing.T, m mail.Message, codes ...string) {
	t.Helper()
	for _, s := range append(codes, "token=", "/sign-in", "/enroll") {
		if strings.Contains(m.Text, s) {
			t.Fatalf("%q email contains %q: %q", m.Subject, s, m.Text)
		}
	}
}

type securityEmail struct {
	Kind       string
	On         bool
	CanTurnOff bool
}

func TestNewSignInEmail(t *testing.T) {
	var fresh atomic.Bool
	e, box := newMagicEnv(t, policyFunc(func(access.Input) access.Decision {
		if fresh.Load() {
			return access.Decision{Effect: access.Allow, Signals: []string{"new-network", "new-device"}}
		}
		return access.Decision{Effect: access.Allow}
	}))
	ada, _, adaKey := e.enrolledUser("ada@example.com", virtualwebauthn.AuthenticatorOptions{})
	_, bobSession, bobKey := e.enrolledUser("bob@example.com", virtualwebauthn.AuthenticatorOptions{})
	box.empty(t)
	fresh.Store(true)

	if status, _, _ := e.passkey(client(), adaKey, adaKey.cred, ""); status != http.StatusOK {
		t.Fatalf("sign in: %d", status)
	}
	m, _ := box.match(t, regexp.MustCompile(`(signed in from a device or network)`))
	if m.To != ada.Email || m.Subject != "New sign-in to your Halo account" || !strings.Contains(m.Text, "IP address: 127.0.0.1") || !strings.Contains(m.Text, "http://localhost:3200/account/sessions") {
		t.Fatalf("new sign-in email: %+v", m)
	}
	noSecrets(t, m)
	if status, _, _ := e.passkey(client(), adaKey, adaKey.cred, ""); status != http.StatusOK {
		t.Fatalf("second sign-in: %d", status)
	}
	box.empty(t)

	var prefs []securityEmail
	if status := e.do(bobSession, "GET", "/api/v1/me/security-emails", nil, &prefs); status != http.StatusOK || len(prefs) != 8 || prefs[0].Kind != "access_pass" || !prefs[0].On || prefs[0].CanTurnOff {
		t.Fatalf("preferences: %d %+v", status, prefs)
	}
	e.expect(bobSession, "PUT", "/api/v1/me/security-emails", map[string]any{"off": []string{"method_added"}}, http.StatusUnprocessableEntity, "ERR_INVALID")
	if status := e.do(bobSession, "PUT", "/api/v1/me/security-emails", map[string]any{"off": []string{"new_sign_in"}}, &prefs); status != http.StatusOK ||
		!slices.ContainsFunc(prefs, func(p securityEmail) bool { return p.Kind == "new_sign_in" && !p.On && p.CanTurnOff }) {
		t.Fatalf("turn off new sign-in emails: %d %+v", status, prefs)
	}
	if status, _, _ := e.passkey(client(), bobKey, bobKey.cred, ""); status != http.StatusOK {
		t.Fatalf("bob signs in: %d", status)
	}
	box.empty(t)
}

func TestAccountChangeEmails(t *testing.T) {
	e, box := newMagicEnv(t, nil)
	ada, c, _ := e.enrolledUser("ada@example.com", virtualwebauthn.AuthenticatorOptions{})
	box.empty(t)
	about := func(subject string) mail.Message {
		t.Helper()
		m, _ := box.match(t, regexp.MustCompile(`(.+)`))
		if m.To != ada.Email || m.Subject != subject || !strings.Contains(m.Text, "http://localhost:3200/account/security") {
			t.Fatalf("want %q to %s, got %+v", subject, ada.Email, m)
		}
		noSecrets(t, m)
		return m
	}

	key := &device{auth: virtualwebauthn.NewAuthenticatorWithOptions(virtualwebauthn.AuthenticatorOptions{Transports: []virtualwebauthn.Transport{virtualwebauthn.TransportUSB}}), cred: virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)}
	if status := e.register(c, "/api/v1/me/passkeys", map[string]any{"kind": "security-key"}, map[string]any{"label": "YubiKey"}, key); status != http.StatusOK {
		t.Fatalf("add security key: %d", status)
	}
	if m := about("A sign-in method was added to your Halo account"); !strings.Contains(m.Text, "YubiKey was added") {
		t.Fatalf("added: %q", m.Text)
	}

	var begun struct{ Ceremony, Secret string }
	if status := e.do(c, "POST", "/api/v1/me/totp/begin", nil, &begun); status != http.StatusOK {
		t.Fatalf("begin authenticator app: %d", status)
	}
	code, _ := totp.GenerateCode(begun.Secret, time.Now())
	if status := e.do(c, "POST", "/api/v1/me/totp/confirm", map[string]any{"ceremony": begun.Ceremony, "code": code, "label": "Phone"}, nil); status != http.StatusOK {
		t.Fatalf("add authenticator app: %d", status)
	}
	app := about("A sign-in method was added to your Halo account")
	if !strings.Contains(app.Text, "Phone was added") {
		t.Fatalf("authenticator app added: %q", app.Text)
	}
	noSecrets(t, app, begun.Secret, code)

	var generated struct{ Codes []string }
	if status := e.do(c, "POST", "/api/v1/me/recovery-codes", nil, &generated); status != http.StatusOK || len(generated.Codes) != 10 {
		t.Fatalf("recovery codes: %d", status)
	}
	noSecrets(t, about("New recovery codes for your Halo account"), generated.Codes...)
	if status := e.do(c, "POST", "/api/v1/me/recovery-codes", nil, &generated); status != http.StatusOK {
		t.Fatalf("recovery codes again: %d", status)
	}
	box.empty(t)

	me, _ := e.me(c)
	passkey := me.Methods[slices.IndexFunc(me.Methods, func(m store.Method) bool { return m.Kind == "passkey" })]
	if status := e.do(c, "DELETE", "/api/v1/me/methods/"+passkey.ID, nil, nil); status != http.StatusNoContent {
		t.Fatalf("remove passkey: %d", status)
	}
	if m := about("A sign-in method was removed from your Halo account"); !strings.Contains(m.Text, passkey.Label+" was removed from your Halo account on") {
		t.Fatalf("removed: %q", m.Text)
	}
}
