package auth_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"halo/internal/auth"
	"halo/internal/httpx"
	"halo/internal/store"
)

func (e *env) protect() *atomic.Value {
	e.t.Helper()
	var remote atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.PostFormValue("response")
		if token == "outage" {
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
			return
		}
		remote.Store(r.PostFormValue("remoteip"))
		codes := []string{}
		if r.PostFormValue("secret") != "s3cret" {
			codes = append(codes, "invalid-input-secret")
		} else if token != "pass" {
			codes = append(codes, "invalid-input-response")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": len(codes) == 0, "error-codes": codes})
	}))
	previous := auth.Siteverify["turnstile"]
	auth.Siteverify["turnstile"] = srv.URL
	e.t.Cleanup(func() {
		srv.Close()
		auth.Siteverify["turnstile"] = previous
	})
	if err := e.st.SetBotProtectionSecret(context.Background(), "s3cret"); err != nil {
		e.t.Fatal(err)
	}
	e.org(func(s *store.Settings) {
		s.BotProtection, s.BotProtectionSiteKey = "turnstile", "1x00000000000000000000AA"
	})
	return &remote
}

func TestMagicLinkChallenge(t *testing.T) {
	e, box := newMagicEnv(t, nil)
	ctx := context.Background()
	u, err := e.st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	c := http.DefaultClient
	request := func(challenge string, status int, code string) {
		t.Helper()
		e.expect(c, "POST", "/api/v1/auth/magic-link", map[string]any{"email": "ada@example.com", "challenge": challenge}, status, code)
	}

	request("", http.StatusAccepted, "")
	box.token(t)

	remote := e.protect()
	request("", http.StatusForbidden, "ERR_CHALLENGE_REQUIRED")
	request("fail", http.StatusForbidden, "ERR_CHALLENGE_FAILED")
	request("outage", http.StatusServiceUnavailable, "ERR_CHALLENGE_UNAVAILABLE")
	box.empty(t)
	request("pass", http.StatusAccepted, "")
	box.token(t)
	if remote.Load() != "127.0.0.1" {
		t.Fatalf("siteverify got remoteip %v", remote.Load())
	}

	events, err := e.st.ListSignIns(ctx, store.SignInFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var challenges []string
	for _, ev := range events {
		if ev.Method != "magic-link" || ev.Result != "failure" || ev.Email != "ada@example.com" {
			t.Fatalf("unexpected sign-in event: %+v", ev)
		}
		challenges = append(challenges, ev.Challenge)
	}
	if !slices.Equal(challenges, []string{"unavailable", "failed"}) {
		t.Fatalf("challenge results in the sign-in log: %v", challenges)
	}
	for range 10 {
		request("fail", http.StatusForbidden, "ERR_CHALLENGE_FAILED")
	}
	if n, err := e.st.RecentFailures(ctx, u.ID, u.Email, []string{"password", "totp", "recovery-codes", "magic-link"}, time.Now().Add(-time.Hour)); err != nil || n != 0 {
		t.Fatalf("failed checks must not lock the account: %d %v", n, err)
	}

	if err := e.st.SetBotProtectionSecret(ctx, "rotated-elsewhere"); err != nil {
		t.Fatal(err)
	}
	request("pass", http.StatusServiceUnavailable, "ERR_CHALLENGE_UNAVAILABLE")
	box.empty(t)
}

func TestSignInChallengeAfterFailuresFromAddress(t *testing.T) {
	e := newEnv(t)
	e.withPassword("ada@example.com", "correct horse battery staple", false)
	e.allowPasswords(false)
	httpx.TrustProxies([]string{"127.0.0.1/32", "::1/128"})
	t.Cleanup(func() { httpx.TrustProxies(nil) })
	e.protect()
	bot := from("203.0.113.7")
	ada := func(challenge string) map[string]any {
		return map[string]any{"email": "ada@example.com", "password": "correct horse battery staple", "challenge": challenge}
	}

	for i := range 5 {
		e.expect(bot, "POST", "/api/v1/auth/password", map[string]any{"email": fmt.Sprintf("victim%d@example.com", i), "password": "Summer2026!Summer2026!"}, http.StatusUnauthorized, "ERR_PASSWORD_MISMATCH")
	}
	e.expect(bot, "POST", "/api/v1/auth/password", ada(""), http.StatusForbidden, "ERR_CHALLENGE_REQUIRED")
	e.expect(bot, "POST", "/api/v1/auth/totp", map[string]any{"email": "nobody@example.com", "code": "123456"}, http.StatusForbidden, "ERR_CHALLENGE_REQUIRED")
	e.expect(bot, "POST", "/api/v1/auth/password", ada("fail"), http.StatusForbidden, "ERR_CHALLENGE_FAILED")
	if status := e.do(bot, "POST", "/api/v1/auth/password", ada("pass"), nil); status != http.StatusOK {
		t.Fatalf("a passed check must let the sign-in through: %d", status)
	}
	if status := e.do(from("198.51.100.4"), "POST", "/api/v1/auth/password", ada(""), nil); status != http.StatusOK {
		t.Fatalf("an address without failures must not be challenged: %d", status)
	}

	events, err := e.st.ListSignIns(context.Background(), store.SignInFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if events[0].IP != "198.51.100.4" || events[0].Challenge != "" || events[1].IP != "203.0.113.7" || events[1].Result != "success" || events[1].Challenge != "passed" || events[2].Challenge != "failed" {
		t.Fatalf("challenge results in the sign-in log: %+v", events[:3])
	}

	e.org(func(s *store.Settings) { s.BotProtection = "" })
	if status := e.do(bot, "POST", "/api/v1/auth/password", ada(""), nil); status != http.StatusOK {
		t.Fatalf("with bot protection off, sign-in must work as before: %d", status)
	}
}

func TestProviderOutageDoesNotThrottleAnAddress(t *testing.T) {
	e := newEnv(t)
	e.withPassword("ada@example.com", "correct horse battery staple", false)
	e.allowPasswords(false)
	httpx.TrustProxies([]string{"127.0.0.1/32", "::1/128"})
	t.Cleanup(func() { httpx.TrustProxies(nil) })
	e.protect()
	office := from("203.0.113.9")
	for range 25 {
		e.expect(office, "POST", "/api/v1/auth/magic-link", map[string]any{"email": "ada@example.com", "challenge": "outage"}, http.StatusServiceUnavailable, "ERR_CHALLENGE_UNAVAILABLE")
	}
	if status := e.do(office, "POST", "/api/v1/auth/password", map[string]any{"email": "ada@example.com", "password": "correct horse battery staple"}, nil); status != http.StatusOK {
		t.Fatalf("an outage must not challenge or throttle the address afterwards: %d", status)
	}
}
