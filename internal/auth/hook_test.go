package auth_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"

	"halo/internal/store"
	"halo/internal/webhooks"
)

func TestSignInHook(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	u, signedIn, _ := e.enrolledUser("ada@example.com", virtualwebauthn.AuthenticatorOptions{})
	group, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Observability", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.AddGroupMember(ctx, group.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	app, err := e.st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://grafana.example.com/cb"}, GroupIDs: []string{group.ID}})
	if err != nil {
		t.Fatal(err)
	}

	const secret = "hook-secret-0123456789abcdefghijklmnop"
	var answer atomic.Value
	var sent atomic.Value
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		sent.Store(string(body))
		reply := answer.Load().(string)
		if reply == "slow" {
			time.Sleep(4 * time.Second)
		}
		stamp := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(stamp + "." + reply))
		w.Header().Set("Halo-Signature", "t="+stamp+",v1="+hex.EncodeToString(mac.Sum(nil)))
		_, _ = w.Write([]byte(reply))
	}))
	defer hook.Close()
	prev := webhooks.HookClient
	webhooks.HookClient = hook.Client()
	t.Cleanup(func() { webhooks.HookClient = prev })
	save := func(failOpen bool) {
		t.Helper()
		if err := e.st.SaveApplicationHook(ctx, store.ApplicationHook{AppID: app.ID, Host: "checks.example.com", URLSealed: e.st.Sealer.Seal([]byte(hook.URL)), SecretSealed: e.st.Sealer.Seal([]byte(secret)),
			Claims: []string{}, FailOpen: failOpen}); err != nil {
			t.Fatal(err)
		}
	}
	request := func(id string) string {
		t.Helper()
		if _, err := e.st.CreateAuthRequest(ctx, store.AuthRequest{ID: id, ClientID: app.ClientID, RedirectURI: app.RedirectURIs[0], Scopes: []string{"openid", "email"}, ResponseType: "code"}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	type reply struct {
		Redirect string
		Error    struct{ Code, Message string }
	}
	latest := func() store.SignInEvent {
		t.Helper()
		events, err := e.st.ListSignIns(ctx, store.SignInFilter{AppID: app.ID})
		if err != nil || len(events) == 0 {
			t.Fatalf("sign-in log: %v %v", events, err)
		}
		return events[0]
	}

	save(false)
	answer.Store(`{"decision":"deny","reason":"Your laptop is missing the latest security update."}`)
	denied := request("arq_denied")
	var out reply
	if status := e.do(signedIn, "POST", "/api/v1/auth/continue", map[string]any{"authRequest": denied}, &out); status != http.StatusForbidden || out.Error.Code != "ERR_BLOCKED_BY_HOOK" ||
		out.Error.Message != "Your laptop is missing the latest security update." {
		t.Fatalf("denied: %d %+v", status, out)
	}
	if ev := latest(); ev.Result != "failure" || ev.Hook != "denied" || !strings.Contains(ev.Reason, "missing the latest security update") {
		t.Fatalf("denied event: %+v", ev)
	}
	if req, err := e.st.GetAuthRequest(ctx, denied); err != nil || req.Done {
		t.Fatalf("a denied sign-in completed the request: %+v %v", req, err)
	}
	var payload struct {
		Type   string
		Scopes []string
		User   struct{ Email string }
	}
	if json.Unmarshal([]byte(sent.Load().(string)), &payload) != nil || payload.Type != "sign_in" || payload.User.Email != u.Email || strings.Join(payload.Scopes, " ") != "openid email" {
		t.Fatalf("hook request: %s", sent.Load())
	}

	answer.Store(`{"decision":"allow"}`)
	if status := e.do(signedIn, "POST", "/api/v1/auth/continue", map[string]any{"authRequest": request("arq_allowed")}, &out); status != http.StatusOK || out.Redirect != "/oauth2/authorize/callback?id=arq_allowed" {
		t.Fatalf("allowed: %d %+v", status, out)
	}
	if ev := latest(); ev.Result != "success" || ev.Hook != "allowed" {
		t.Fatalf("allowed event: %+v", ev)
	}

	answer.Store("slow")
	out = reply{}
	if status := e.do(signedIn, "POST", "/api/v1/auth/continue", map[string]any{"authRequest": request("arq_closed")}, &out); status != http.StatusForbidden || out.Error.Code != "ERR_BLOCKED_BY_HOOK" {
		t.Fatalf("timeout, fail closed: %d %+v", status, out)
	}
	if ev := latest(); ev.Result != "failure" || ev.Hook != "failed_closed" || !strings.Contains(ev.Reason, "did not answer within 3s") {
		t.Fatalf("fail closed event: %+v", ev)
	}
	save(true)
	if status := e.do(signedIn, "POST", "/api/v1/auth/continue", map[string]any{"authRequest": request("arq_open")}, &out); status != http.StatusOK {
		t.Fatalf("timeout, fail open: %d %+v", status, out)
	}
	if ev := latest(); ev.Result != "success" || ev.Hook != "failed_open" || !strings.Contains(ev.Reason, "fails open") {
		t.Fatalf("fail open event: %+v", ev)
	}
}
