package auth_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"testing"

	"github.com/descope/virtualwebauthn"

	"halo/internal/store"
)

var signUpPattern = regexp.MustCompile(`/sign-up\?token=([A-Za-z0-9_-]+)`)

func (e *env) signUp(email, name, challenge string) (int, string) {
	e.t.Helper()
	var out apiError
	status := e.do(http.DefaultClient, "POST", "/api/v1/auth/sign-up", map[string]any{"email": email, "name": name, "challenge": challenge}, &out)
	return status, out.Error.Code
}

func newDevice() *device {
	return &device{auth: virtualwebauthn.NewAuthenticator(), cred: virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)}
}

func TestSignUpOpen(t *testing.T) {
	e, box := newMagicEnv(t, nil)
	ctx := context.Background()
	members, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Members", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Staff", Kind: "assigned"}); err != nil {
		t.Fatal(err)
	}
	rule := `user.department == "Finance"`
	finance, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Finance", Kind: "dynamic", Rule: &rule})
	if err != nil {
		t.Fatal(err)
	}
	e.org(func(s *store.Settings) {
		s.SignUp, s.SignUpDomains, s.SignUpGroupIDs = "open", []string{"example.com"}, []string{members.ID, finance.ID}
	})

	if status, code := e.signUp("Ada@Example.com", "Ada Lovelace", ""); status != http.StatusAccepted {
		t.Fatalf("sign up: %d %s", status, code)
	}
	m, token := box.match(t, signUpPattern)
	if m.To != "Ada@Example.com" || m.Subject != "Finish signing up for Halo" {
		t.Fatalf("email: %+v", m)
	}
	if _, err := e.st.GetUserByEmail(ctx, "ada@example.com"); err == nil {
		t.Fatal("an account exists before the email address is confirmed")
	}

	c := client()
	var info struct {
		User    struct{ Name, Email string }
		Purpose string
	}
	if status := e.do(c, "GET", "/api/v1/auth/sign-up?token="+token, nil, &info); status != http.StatusOK || info.User.Name != "Ada Lovelace" || info.Purpose != "sign-up" {
		t.Fatalf("sign-up info: %d %+v", status, info)
	}
	d := newDevice()
	if status := e.register(c, "/api/v1/auth/sign-up", map[string]any{"token": token}, map[string]any{"token": token}, d); status != http.StatusOK {
		t.Fatalf("finish: %d", status)
	}
	me, status := e.me(c)
	if status != http.StatusOK || me.Status != "active" || !me.EmailVerified || me.Source != "Sign-up" || len(me.Methods) != 1 {
		t.Fatalf("me after sign-up: %d %+v", status, me)
	}
	if !slices.Equal(me.GroupIDs, []string{members.ID}) || len(me.RoleKeys)+len(me.EligibleRoles) != 0 {
		t.Fatalf("a signed-up person got more than the sign-up groups: groups %v roles %v eligible %v", me.GroupIDs, me.RoleKeys, me.EligibleRoles)
	}
	if status, _, code := e.passkey(client(), d, d.cred, ""); status != http.StatusOK {
		t.Fatalf("passkey sign-in after sign-up: %d %s", status, code)
	}
	e.expect(client(), "POST", "/api/v1/auth/sign-up/begin", map[string]any{"token": token}, http.StatusNotFound, "ERR_LINK_EXPIRED")

	audit, err := e.st.ListAudit(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(audit, func(a store.AuditEvent) bool {
		return a.Action == "user.sign_up" && a.TargetID == me.ID && a.ActorID != nil && *a.ActorID == me.ID
	}) {
		t.Fatalf("no sign-up in the audit log: %+v", audit)
	}
}

func TestSignUpApproval(t *testing.T) {
	e, box := newMagicEnv(t, nil)
	ctx := context.Background()
	domain, err := e.st.AddDomain(ctx, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.AddDomain(ctx, "waiting.org"); err != nil {
		t.Fatal(err)
	}
	e.org(func(s *store.Settings) { s.SignUp = "approval" })

	if status, code := e.signUp("ada@example.com", "Ada Lovelace", ""); status != http.StatusForbidden || code != "ERR_SIGN_UP_DOMAIN" {
		t.Fatalf("no verified domain yet: %d %s", status, code)
	}
	if _, err := e.st.VerifyDomain(ctx, domain.ID); err != nil {
		t.Fatal(err)
	}
	if status, code := e.signUp("sam@waiting.org", "Sam Lee", ""); status != http.StatusForbidden || code != "ERR_SIGN_UP_DOMAIN" {
		t.Fatalf("unverified domain: %d %s", status, code)
	}
	if status, code := e.signUp("ada@EXAMPLE.com", "Ada Lovelace", ""); status != http.StatusAccepted {
		t.Fatalf("verified domain: %d %s", status, code)
	}
	_, token := box.match(t, signUpPattern)

	c := client()
	d := newDevice()
	var s started
	if status := e.do(c, "POST", "/api/v1/auth/sign-up/begin", map[string]any{"token": token}, &s); status != http.StatusOK {
		t.Fatalf("begin: %d", status)
	}
	opts, err := virtualwebauthn.ParseAttestationOptions(string(s.Options))
	if err != nil {
		t.Fatal(err)
	}
	d.auth.Options.UserHandle = []byte(opts.UserID)
	var out struct {
		Pending  bool   `json:"pending"`
		Redirect string `json:"redirect"`
	}
	finish := map[string]any{"token": token, "ceremony": s.Ceremony, "credential": json.RawMessage(virtualwebauthn.CreateAttestationResponse(e.rp, d.auth, d.cred, *opts))}
	if status := e.do(c, "POST", "/api/v1/auth/sign-up/finish", finish, &out); status != http.StatusOK || !out.Pending || out.Redirect != "" {
		t.Fatalf("finish in approval mode: %d %+v", status, out)
	}
	if _, status := e.me(c); status != http.StatusUnauthorized {
		t.Fatalf("a sign-up waiting for approval got a session: %d", status)
	}
	if _, err := e.st.GetUserByEmail(ctx, "ada@example.com"); err == nil {
		t.Fatal("an account exists before approval")
	}
	if status, _, code := e.passkey(client(), d, d.cred, ""); status != http.StatusUnauthorized || code != "ERR_PASSKEY_REJECTED" {
		t.Fatalf("passkey before approval: %d %s", status, code)
	}
	if status, code := e.signUp("ada@example.com", "Mallory", ""); status != http.StatusAccepted {
		t.Fatalf("repeat sign-up: %d %s", status, code)
	}
	box.empty(t)

	queue, err := e.st.ListSignUps(ctx)
	if err != nil || len(queue) != 1 || queue[0].Name != "Ada Lovelace" || queue[0].VerifiedAt == nil {
		t.Fatalf("queue: %+v %v", queue, err)
	}
	u, err := e.st.ActivateSignUp(ctx, queue[0].ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if u.Status != "active" || !u.EmailVerified || len(u.GroupIDs)+len(u.RoleKeys) != 0 {
		t.Fatalf("approved account: %+v", u)
	}
	if status, _, code := e.passkey(client(), d, d.cred, ""); status != http.StatusOK {
		t.Fatalf("passkey after approval: %d %s", status, code)
	}
	audit, err := e.st.ListAudit(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(audit, func(a store.AuditEvent) bool {
		return a.Action == "user.sign_up" && a.TargetType == "sign_up" && a.TargetID == queue[0].ID && a.ActorID == nil
	}) {
		t.Fatalf("no sign-up waiting for approval in the audit log: %+v", audit)
	}
}

func TestSignUpRefusals(t *testing.T) {
	e, box := newMagicEnv(t, nil)
	if status, code := e.signUp("ada@example.com", "Ada Lovelace", ""); status != http.StatusNotFound || code != "ERR_SIGN_UP_OFF" {
		t.Fatalf("sign-up off: %d %s", status, code)
	}
	e.org(func(s *store.Settings) { s.SignUp, s.SignUpDomains = "open", []string{"example.com"} })
	for email, want := range map[string]string{
		"ada@example.org":     "ERR_SIGN_UP_DOMAIN",
		"ada@sub.example.com": "ERR_SIGN_UP_DOMAIN",
		"not an address":      "ERR_INVALID",
	} {
		var out apiError
		status := e.do(http.DefaultClient, "POST", "/api/v1/auth/sign-up", map[string]any{"email": email, "name": "Ada"}, &out)
		if status < 400 || out.Error.Code != want {
			t.Errorf("%s: %d %s", email, status, out.Error.Code)
		}
	}
	if status, code := e.signUp("ada@example.com", "  ", ""); status != http.StatusUnprocessableEntity {
		t.Fatalf("empty name: %d %s", status, code)
	}
	box.empty(t)

	if status, _ := e.signUp("ada@example.com", "Ada Lovelace", ""); status != http.StatusAccepted {
		t.Fatal(status)
	}
	_, token := box.match(t, signUpPattern)
	e.org(func(s *store.Settings) { s.SignUpDomains = []string{"example.org"} })
	e.expect(client(), "GET", "/api/v1/auth/sign-up?token="+token, nil, http.StatusForbidden, "ERR_SIGN_UP_DOMAIN")
	e.org(func(s *store.Settings) { s.SignUp = "off" })
	e.expect(client(), "POST", "/api/v1/auth/sign-up/begin", map[string]any{"token": token}, http.StatusNotFound, "ERR_SIGN_UP_OFF")
	e.org(func(s *store.Settings) { s.SignUp, s.SignUpDomains = "open", []string{"example.com"} })
	if _, err := e.st.CreateUser(context.Background(), store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "invited"}); err != nil {
		t.Fatal(err)
	}
	if status := e.register(client(), "/api/v1/auth/sign-up", map[string]any{"token": token}, map[string]any{"token": token}, newDevice()); status != http.StatusConflict {
		t.Fatalf("finish for an address that got an account meanwhile: %d", status)
	}
}

func TestSignUpDoesNotRevealAccounts(t *testing.T) {
	e, box := newMagicEnv(t, nil)
	ctx := context.Background()
	if _, err := e.st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.CreateUser(ctx, store.NewUser{Email: "sam@example.com", Name: "Sam Lee", Status: "suspended"}); err != nil {
		t.Fatal(err)
	}
	e.org(func(s *store.Settings) { s.SignUp, s.SignUpDomains = "approval", []string{"example.com"} })

	body := `{"email":"%s","name":"Someone"}`
	knownStatus, knownBody := e.raw("/api/v1/auth/sign-up", fmt.Sprintf(body, "ADA@example.com"))
	suspendedStatus, suspendedBody := e.raw("/api/v1/auth/sign-up", fmt.Sprintf(body, "sam@example.com"))
	box.empty(t)
	newStatus, newBody := e.raw("/api/v1/auth/sign-up", fmt.Sprintf(body, "grace@example.com"))
	if knownStatus != http.StatusAccepted || suspendedStatus != knownStatus || newStatus != knownStatus || suspendedBody != knownBody || newBody != knownBody {
		t.Fatalf("responses differ: %d %q / %d %q / %d %q", knownStatus, knownBody, suspendedStatus, suspendedBody, newStatus, newBody)
	}
	m, first := box.match(t, signUpPattern)
	if m.To != "grace@example.com" {
		t.Fatalf("email went to %s", m.To)
	}

	e.raw("/api/v1/auth/sign-up", `{"email":"grace@example.com","name":"Grace Hopper"}`)
	_, second := box.match(t, signUpPattern)
	if second == first {
		t.Fatal("a repeated sign-up reused the link")
	}
	e.expect(client(), "GET", "/api/v1/auth/sign-up?token="+first, nil, http.StatusNotFound, "ERR_LINK_EXPIRED")
	var info struct{ User struct{ Name string } }
	if e.do(client(), "GET", "/api/v1/auth/sign-up?token="+second, nil, &info); info.User.Name != "Grace Hopper" {
		t.Fatalf("the latest sign-up should name the account: %+v", info)
	}
	e.raw("/api/v1/auth/sign-up", `{"email":"grace@example.com","name":"Grace Hopper"}`)
	box.match(t, signUpPattern)
	e.raw("/api/v1/auth/sign-up", `{"email":"grace@example.com","name":"Grace Hopper"}`)
	box.empty(t)
}

func TestSignUpRateLimit(t *testing.T) {
	e, box := newMagicEnv(t, nil)
	e.org(func(s *store.Settings) { s.SignUp, s.SignUpDomains = "open", []string{"example.com"} })
	for i := range 10 {
		if status, code := e.signUp(fmt.Sprintf("person%d@example.com", i), "Someone", ""); status != http.StatusAccepted {
			t.Fatalf("sign-up %d: %d %s", i, status, code)
		}
		box.match(t, signUpPattern)
	}
	if status, code := e.signUp("one-more@example.com", "Someone", ""); status != http.StatusTooManyRequests || code != "ERR_RATE_LIMITED" {
		t.Fatalf("11th sign-up from one address: %d %s", status, code)
	}
	box.empty(t)
}

func TestSignUpChallenge(t *testing.T) {
	e, box := newMagicEnv(t, nil)
	e.org(func(s *store.Settings) { s.SignUp, s.SignUpDomains = "open", []string{"example.com"} })
	e.protect()
	for challenge, want := range map[string]string{"": "ERR_CHALLENGE_REQUIRED", "fail": "ERR_CHALLENGE_FAILED", "outage": "ERR_CHALLENGE_UNAVAILABLE"} {
		if _, code := e.signUp("ada@example.com", "Ada Lovelace", challenge); code != want {
			t.Errorf("challenge %q: got %s, want %s", challenge, code, want)
		}
	}
	box.empty(t)
	if status, code := e.signUp("ada@example.com", "Ada Lovelace", "pass"); status != http.StatusAccepted {
		t.Fatalf("passed challenge: %d %s", status, code)
	}
	box.match(t, signUpPattern)
	events, err := e.st.ListSignIns(context.Background(), store.SignInFilter{})
	if err != nil || len(events) != 0 {
		t.Fatalf("sign-up checks are not sign-ins: %+v %v", events, err)
	}
}
