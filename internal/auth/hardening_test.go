package auth_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"

	"halo/internal/httpx"
	"halo/internal/secret"
	"halo/internal/store"
)

func (e *env) verified(userID string) bool {
	e.t.Helper()
	u, err := e.st.GetUser(context.Background(), userID)
	if err != nil {
		e.t.Fatal(err)
	}
	return u.EmailVerified
}

func TestEmailVerification(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	enroll := func(email string, emailed bool) store.User {
		t.Helper()
		u, err := e.st.CreateUser(ctx, store.NewUser{Email: email, Name: "Person"})
		if err != nil {
			t.Fatal(err)
		}
		token, _, err := e.st.CreateEnrollmentToken(ctx, u.ID, "invite", nil)
		if err != nil {
			t.Fatal(err)
		}
		if emailed {
			if err := e.st.MarkEnrollmentEmailed(ctx, token); err != nil {
				t.Fatal(err)
			}
		}
		d := &device{auth: virtualwebauthn.NewAuthenticator(), cred: virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)}
		if status := e.register(client(), "/api/v1/auth/enroll", map[string]any{"token": token}, map[string]any{"token": token}, d); status != http.StatusOK {
			t.Fatalf("enroll %s: %d", email, status)
		}
		return u
	}
	if u := enroll("ada@example.com", true); !e.verified(u.ID) {
		t.Fatal("enrolling from an emailed link must verify the address")
	}
	copied := enroll("bob@example.com", false)
	if e.verified(copied.ID) {
		t.Fatal("enrolling from a copied link must not verify the address")
	}

	token, err := e.st.CreateMagicLink(ctx, copied.ID, "", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if status := e.do(client(), "POST", "/api/v1/auth/magic-link/redeem", map[string]any{"token": token}, nil); status != http.StatusOK || !e.verified(copied.ID) {
		t.Fatalf("redeeming a magic link must verify the address: %d", status)
	}
}

func TestFederatedSignInVerifiesMatchingEmail(t *testing.T) {
	e := newFederationEnv(t)
	ctx := context.Background()
	fake := newFakeProvider(t)
	p := e.provider(store.IdentityProvider{Kind: "oidc", Name: "Acme SSO", Issuer: fake.srv.URL})
	ada, err := e.st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := e.st.CreateUser(ctx, store.NewUser{Email: "bob@example.com", Name: "Bob", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.UseFederatedIdentity(ctx, p.ID, bob.ID, "acme-9", "bob@personal.test"); err != nil {
		t.Fatal(err)
	}
	fake.claims = map[string]any{"sub": "acme-1", "email": "ADA@example.com", "email_verified": true}
	if location := e.federate(client(), p.ID, ""); location != "/account" || !e.verified(ada.ID) {
		t.Fatalf("a provider-verified matching address must verify the account: %s", location)
	}
	fake.claims = map[string]any{"sub": "acme-9", "email": "bob@personal.test", "email_verified": true}
	if location := e.federate(client(), p.ID, ""); location != "/account" || e.verified(bob.ID) {
		t.Fatalf("a different verified address must not verify the account: %s", location)
	}
}

func TestRemoveMethodCountsLinkedAccounts(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	link := func(u store.User, enabled bool, name string) {
		t.Helper()
		p, err := e.st.SaveIdentityProvider(ctx, store.IdentityProvider{Kind: "oidc", Name: name, Issuer: "https://" + name + ".example.com", ClientID: "halo", Enabled: enabled, Scopes: []string{}, AllowedDomains: []string{}, JITGroupIDs: []string{}}, "")
		if err != nil {
			t.Fatal(err)
		}
		if err := e.st.UseFederatedIdentity(ctx, p.ID, u.ID, name+"-1", u.Email); err != nil {
			t.Fatal(err)
		}
	}
	ada, c, _ := e.enrolledUser("ada@example.com", virtualwebauthn.AuthenticatorOptions{})
	me, _ := e.me(c)
	link(ada, false, "disabled")
	e.expect(c, "DELETE", "/api/v1/me/methods/"+me.Methods[0].ID, nil, http.StatusConflict, "ERR_LAST_METHOD")
	link(ada, true, "google")
	if status := e.do(c, "DELETE", "/api/v1/me/methods/"+me.Methods[0].ID, nil, nil); status != http.StatusNoContent {
		t.Fatalf("remove the passkey with a linked account left: %d", status)
	}
}

func TestSessionsRecordTheirDevice(t *testing.T) {
	e := newEnv(t)
	u, c, _ := e.enrolledUser("ada@example.com", virtualwebauthn.AuthenticatorOptions{})
	base, _ := url.Parse(e.srv.URL)
	var device string
	for _, cookie := range c.Jar.Cookies(base) {
		if cookie.Name == httpx.DeviceCookie {
			device = cookie.Value
		}
	}
	if n, err := e.st.RevokeDeviceSessions(context.Background(), u.ID, secret.Hash("another-device")); err != nil || n != 0 {
		t.Fatalf("another device: %d %v", n, err)
	}
	if n, err := e.st.RevokeDeviceSessions(context.Background(), u.ID, secret.Hash(device)); err != nil || n != 1 {
		t.Fatalf("this device: %d %v", n, err)
	}
	if _, status := e.me(c); status != http.StatusUnauthorized {
		t.Fatalf("session after revoking the device: %d", status)
	}
}
