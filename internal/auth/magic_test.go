package auth_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"

	"halo/internal/access"
	"halo/internal/auth"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/mail"
	"halo/internal/store"
	"halo/internal/testdb"
)

type inbox chan mail.Message

func (b inbox) Send(_ context.Context, m mail.Message) error {
	b <- m
	return nil
}

func (inbox) Configured() bool { return true }

type policyFunc func(access.Input) access.Decision

func (f policyFunc) Evaluate(_ context.Context, in access.Input) (access.Decision, error) {
	return f(in), nil
}

var tokenPattern = regexp.MustCompile(`/sign-in/magic\?token=([A-Za-z0-9_-]+)`)

func (b inbox) token(t *testing.T) (mail.Message, string) {
	t.Helper()
	select {
	case m := <-b:
		match := tokenPattern.FindStringSubmatch(m.Text)
		if match == nil {
			t.Fatalf("no sign-in link in %q", m.Text)
		}
		return m, match[1]
	case <-time.After(5 * time.Second):
		t.Fatal("no email arrived")
	}
	return mail.Message{}, ""
}

func (b inbox) empty(t *testing.T) {
	t.Helper()
	select {
	case m := <-b:
		t.Fatalf("unexpected email to %s", m.To)
	case <-time.After(300 * time.Millisecond):
	}
}

func newMagicEnv(t *testing.T, policy access.Engine) (*env, inbox) {
	st := testdb.New(t)
	public, _ := url.Parse("http://localhost:3200")
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	box := make(inbox, 10)
	mux := http.NewServeMux()
	if err := auth.Register(mux, httpx.Deps{Config: config.Config{PublicURL: public, Dev: true}, Store: st, Auth: authn, Mailer: box, Policy: policy}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(authn.Resolve(mux))
	t.Cleanup(srv.Close)
	return &env{t: t, st: st, srv: srv, rp: virtualwebauthn.RelyingParty{ID: "localhost", Name: "Halo", Origin: "http://localhost:3200"}}, box
}

func (e *env) raw(path string, body string) (int, string) {
	e.t.Helper()
	res, err := http.Post(e.srv.URL+path, "application/json", strings.NewReader(body))
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func TestMagicLink(t *testing.T) {
	var blocked atomic.Bool
	e, box := newMagicEnv(t, policyFunc(func(in access.Input) access.Decision {
		if blocked.Load() && in.Method == "magic-link" {
			return access.Decision{Effect: access.Block, Policy: "No email links", Reason: "Magic links are turned off."}
		}
		return access.Decision{Effect: access.Allow}
	}))
	ctx := context.Background()
	group, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Observability", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := e.st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active", GroupIDs: []string{group.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.CreateUser(ctx, store.NewUser{Email: "sam@example.com", Name: "Sam Lee", Status: "suspended"}); err != nil {
		t.Fatal(err)
	}

	knownStatus, knownBody := e.raw("/api/v1/auth/magic-link", `{"email":"ADA@example.com"}`)
	unknownStatus, unknownBody := e.raw("/api/v1/auth/magic-link", `{"email":"nobody@example.com"}`)
	suspendedStatus, suspendedBody := e.raw("/api/v1/auth/magic-link", `{"email":"sam@example.com"}`)
	if knownStatus != http.StatusAccepted || unknownStatus != knownStatus || suspendedStatus != knownStatus || unknownBody != knownBody || suspendedBody != knownBody {
		t.Fatalf("responses differ: %d %q / %d %q / %d %q", knownStatus, knownBody, unknownStatus, unknownBody, suspendedStatus, suspendedBody)
	}
	m, token := box.token(t)
	box.empty(t)
	if m.To != "ada@example.com" || m.Subject != "Your Halo sign-in link" || !strings.HasPrefix(m.Text, "Hi Ada,") || !strings.Contains(m.Text, "http://localhost:3200/sign-in/magic?token=") {
		t.Fatalf("email: %+v", m)
	}

	c := client()
	var out struct{ Redirect string }
	if status := e.do(c, "POST", "/api/v1/auth/magic-link/redeem", map[string]any{"token": token}, &out); status != http.StatusOK || out.Redirect != "/account" {
		t.Fatalf("redeem: %d %+v", status, out)
	}
	if me, status := e.me(c); status != http.StatusOK || me.ID != u.ID {
		t.Fatalf("session after redeem: %d", status)
	}
	e.expect(client(), "POST", "/api/v1/auth/magic-link/redeem", map[string]any{"token": token}, http.StatusNotFound, "ERR_LINK_EXPIRED")
	expired, err := e.st.CreateMagicLink(ctx, u.ID, "", time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	e.expect(client(), "POST", "/api/v1/auth/magic-link/redeem", map[string]any{"token": expired}, http.StatusNotFound, "ERR_LINK_EXPIRED")
	e.expect(client(), "POST", "/api/v1/auth/magic-link/redeem", map[string]any{"token": "not-a-token"}, http.StatusNotFound, "ERR_LINK_EXPIRED")

	app, err := e.st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://grafana.example.com/cb"}, GroupIDs: []string{group.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.CreateAuthRequest(ctx, store.AuthRequest{ID: "are_magic", ClientID: app.ClientID, RedirectURI: app.RedirectURIs[0], Scopes: []string{"openid"}, ResponseType: "code"}); err != nil {
		t.Fatal(err)
	}
	e.expect(client(), "POST", "/api/v1/auth/magic-link", map[string]any{"email": "ada@example.com", "authRequest": "are_missing"}, http.StatusNotFound, "ERR_AUTH_REQUEST_EXPIRED")
	if status := e.do(client(), "POST", "/api/v1/auth/magic-link", map[string]any{"email": "ada@example.com", "authRequest": "are_magic"}, nil); status != http.StatusAccepted {
		t.Fatalf("magic link for an auth request: %d", status)
	}
	m, token = box.token(t)
	if !strings.Contains(m.Text, "sign in to Grafana:") || !strings.Contains(m.Text, "in the browser where you started signing in") {
		t.Fatalf("app email: %q", m.Text)
	}
	if status := e.do(client(), "POST", "/api/v1/auth/magic-link/redeem", map[string]any{"token": token}, &out); status != http.StatusOK || out.Redirect != store.AuthorizeCallbackPath("are_magic") {
		t.Fatalf("redeem for auth request: %d %+v", status, out)
	}
	req, err := e.st.GetAuthRequest(ctx, "are_magic")
	if err != nil || !req.Done || req.UserID == nil || *req.UserID != u.ID || req.SessionID == nil || !slices.Equal(req.AMR, []string{"otp"}) {
		t.Fatalf("auth request after redeem: %+v %v", req, err)
	}

	blocked.Store(true)
	e.do(client(), "POST", "/api/v1/auth/magic-link", map[string]any{"email": "ada@example.com"}, nil)
	_, token = box.token(t)
	e.expect(client(), "POST", "/api/v1/auth/magic-link/redeem", map[string]any{"token": token}, http.StatusForbidden, "ERR_BLOCKED_BY_POLICY")

	e.do(client(), "POST", "/api/v1/auth/magic-link", map[string]any{"email": "ada@example.com"}, nil)
	box.token(t)
	e.do(client(), "POST", "/api/v1/auth/magic-link", map[string]any{"email": "ada@example.com"}, nil)
	box.empty(t)

	events, err := e.st.ListSignIns(ctx, store.SignInFilter{UserID: u.ID})
	if err != nil {
		t.Fatal(err)
	}
	reasons := []string{}
	for _, ev := range events {
		if ev.Method != "magic-link" {
			t.Fatalf("event method: %+v", ev)
		}
		reasons = append(reasons, ev.Result+": "+ev.Reason)
	}
	slices.Sort(reasons)
	want := []string{"failure: Policy “No email links”: Magic links are turned off.", "failure: The sign-in link expired.", "failure: The sign-in link was already used.", "success: ", "success: "}
	if !slices.Equal(reasons, want) {
		t.Fatalf("events:\n%q\nwant\n%q", reasons, want)
	}
}

func TestDevOutbox(t *testing.T) {
	e, _ := newMagicEnv(t, nil)
	ctx := context.Background()
	if _, err := e.st.QueueMail(ctx, "ada@example.com", "Your Halo sign-in link", "Open http://localhost:3200/sign-in/magic?token=abc", false); err != nil {
		t.Fatal(err)
	}
	session := func(roles ...string) *http.Client {
		u, err := e.st.CreateUser(ctx, store.NewUser{Email: strings.Join(roles, "") + "x@example.com", Name: "X", Status: "active", Roles: roles})
		if err != nil {
			t.Fatal(err)
		}
		_, token, err := e.st.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: "passkey"})
		if err != nil {
			t.Fatal(err)
		}
		c := client()
		base, _ := url.Parse(e.srv.URL)
		c.Jar.SetCookies(base, []*http.Cookie{{Name: httpx.SessionCookie, Value: token}})
		return c
	}
	var messages []store.OutboxMessage
	if status := e.do(session("global_admin"), "GET", "/api/v1/dev/outbox", nil, &messages); status != http.StatusOK || len(messages) != 1 || !strings.Contains(messages[0].Text, "token=abc") {
		t.Fatalf("outbox: %d %+v", status, messages)
	}
	e.expect(session("user_admin"), "GET", "/api/v1/dev/outbox", nil, http.StatusForbidden, "ERR_FORBIDDEN")
}

func TestMagicLinkDisabledSendsNothing(t *testing.T) {
	e, box := newMagicEnv(t, nil)
	ctx := context.Background()
	ada, err := e.st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.SetMethodEnabled(ctx, "magic-link", false); err != nil {
		t.Fatal(err)
	}
	knownStatus, knownBody := e.raw("/api/v1/auth/magic-link", `{"email":"ada@example.com"}`)
	unknownStatus, unknownBody := e.raw("/api/v1/auth/magic-link", `{"email":"nobody@example.com"}`)
	if knownStatus != http.StatusAccepted || unknownStatus != knownStatus || unknownBody != knownBody {
		t.Fatalf("responses differ: %d %q / %d %q", knownStatus, knownBody, unknownStatus, unknownBody)
	}
	box.empty(t)
	if n, err := e.st.CountMagicLinks(ctx, ada.ID, time.Now().Add(-time.Hour)); err != nil || n != 0 {
		t.Fatalf("links created while disabled: %d %v", n, err)
	}
}
