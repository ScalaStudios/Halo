package auth_test

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"

	"halo/internal/access"
	"halo/internal/store"
)

func TestAccessPass(t *testing.T) {
	var policy atomic.Value
	policy.Store(access.Allow)
	e, _ := newMagicEnv(t, policyFunc(func(access.Input) access.Decision {
		return access.Decision{Effect: policy.Load().(string), Reason: "Test policy."}
	}))
	ctx := context.Background()
	e.org(func(s *store.Settings) { s.LockoutThreshold = 3 })
	person := func(email string) store.User {
		t.Helper()
		u, err := e.st.CreateUser(ctx, store.NewUser{Email: email, Name: "Ada Lovelace", Status: "invited"})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	issue := func(u store.User, uses int, expires time.Time) string {
		t.Helper()
		pass, err := e.st.CreateAccessPass(ctx, u.ID, "", uses, expires)
		if err != nil {
			t.Fatal(err)
		}
		return pass
	}
	use := func(c *http.Client, email, pass string) (int, string, string) {
		t.Helper()
		var out struct {
			Redirect string `json:"redirect"`
			apiError
		}
		status := e.do(c, "POST", "/api/v1/auth/access-pass", map[string]any{"email": email, "code": pass, "authRequest": "are_ignored"}, &out)
		return status, out.Redirect, out.Error.Code
	}
	hour := time.Now().Add(time.Hour)

	ada := person("ada@example.com")
	pass := issue(ada, 1, hour)
	var identified struct{ Methods []string }
	if e.do(client(), "POST", "/api/v1/auth/identify", map[string]any{"email": "ada@example.com"}, &identified); len(identified.Methods) != 1 || identified.Methods[0] != "access-pass" {
		t.Fatalf("identify with an active pass: %+v", identified)
	}
	if status, _, code := use(client(), "ada@example.com", "aaaa-bbbb-cccc-dddd"); status != http.StatusUnauthorized || code != "ERR_CODE_MISMATCH" {
		t.Fatalf("wrong pass: %d %s", status, code)
	}
	c := client()
	status, redirect, _ := use(c, "ADA@example.com", strings.ToUpper(strings.ReplaceAll(pass, "-", " ")))
	if status != http.StatusOK || !strings.HasPrefix(redirect, "/enroll?token=") {
		t.Fatalf("right pass: %d %q", status, redirect)
	}
	if _, status := e.me(c); status != http.StatusUnauthorized {
		t.Fatalf("a pass must not sign anyone in by itself: %d", status)
	}
	token := strings.TrimPrefix(redirect, "/enroll?token=")
	var info struct{ Purpose string }
	if e.do(c, "GET", "/api/v1/auth/enroll?token="+token, nil, &info); info.Purpose != "access-pass" {
		t.Fatalf("enrollment purpose: %+v", info)
	}
	d := &device{auth: virtualwebauthn.NewAuthenticator(), cred: virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)}
	if status := e.register(c, "/api/v1/auth/enroll", map[string]any{"token": token}, map[string]any{"token": token}, d); status != http.StatusOK {
		t.Fatalf("enroll with the pass: %d", status)
	}
	if me, status := e.me(c); status != http.StatusOK || me.Status != "active" || len(me.Methods) != 1 || me.Methods[0].Kind != "passkey" {
		t.Fatalf("after enrolling with the pass: %d %+v", status, me)
	}
	if status, _, code := use(client(), "ada@example.com", pass); status != http.StatusUnauthorized || code != "ERR_CODE_MISMATCH" {
		t.Fatalf("single-use pass used twice: %d %s", status, code)
	}
	events, err := e.st.ListSignIns(ctx, store.SignInFilter{UserID: ada.ID})
	if err != nil {
		t.Fatal(err)
	}
	results := []string{}
	for _, ev := range events {
		if ev.Method == "access-pass" {
			results = append(results, ev.Result)
		}
	}
	if strings.Join(results, ",") != "failure,success,failure" {
		t.Fatalf("sign-in log: %v", results)
	}
	audit, err := e.st.ListAudit(ctx, 5)
	if err != nil || !strings.Contains(auditActions(audit), "user.access-pass.use") {
		t.Fatalf("audit: %+v %v", audit, err)
	}
	issue(ada, 1, hour)
	me, _ := e.me(c)
	if len(me.Methods) != 2 || me.Methods[1].Kind != "access-pass" {
		t.Fatalf("methods with a pass: %+v", me.Methods)
	}
	e.expect(c, "DELETE", "/api/v1/me/methods/"+me.Methods[0].ID, nil, http.StatusConflict, "ERR_LAST_METHOD")

	bob := person("bob@example.com")
	pass = issue(bob, 2, hour)
	for range 2 {
		if status, _, _ := use(client(), "bob@example.com", pass); status != http.StatusOK {
			t.Fatalf("multi-use pass: %d", status)
		}
	}
	if status, _, _ := use(client(), "bob@example.com", pass); status != http.StatusUnauthorized {
		t.Fatalf("pass used past its uses: %d", status)
	}

	cy := person("cy@example.com")
	pass = issue(cy, 1, time.Now().Add(-time.Minute))
	if status, _, _ := use(client(), "cy@example.com", pass); status != http.StatusUnauthorized {
		t.Fatalf("expired pass: %d", status)
	}

	dee := person("dee@example.com")
	pass = issue(dee, 3, hour)
	if err := e.st.DeleteMethod(ctx, dee.ID, "access-pass"); err != nil {
		t.Fatal(err)
	}
	if status, _, _ := use(client(), "dee@example.com", pass); status != http.StatusUnauthorized {
		t.Fatalf("revoked pass: %d", status)
	}

	eve := person("eve@example.com")
	pass = issue(eve, 1, hour)
	policy.Store(access.Block)
	if status, _, code := use(client(), "eve@example.com", pass); status != http.StatusForbidden || code != "ERR_BLOCKED_BY_POLICY" {
		t.Fatalf("blocked by policy: %d %s", status, code)
	}
	policy.Store(access.RequirePhishingResistant)
	pass = issue(eve, 1, hour)
	if status, _, _ := use(client(), "eve@example.com", pass); status != http.StatusOK {
		t.Fatalf("a pass leads to a passkey, so a passkey requirement allows it: %d", status)
	}
	policy.Store(access.Allow)

	fay := person("fay@example.com")
	pass = issue(fay, 1, hour)
	for range 3 {
		use(client(), "fay@example.com", "aaaa-bbbb-cccc-dddd")
	}
	if status, _, code := use(client(), "fay@example.com", pass); status != http.StatusTooManyRequests || code != "ERR_TOO_MANY_ATTEMPTS" {
		t.Fatalf("lockout: %d %s", status, code)
	}
}

func auditActions(events []store.AuditEvent) string {
	actions := make([]string, len(events))
	for i, ev := range events {
		actions[i] = ev.Action
	}
	return strings.Join(actions, ",")
}
