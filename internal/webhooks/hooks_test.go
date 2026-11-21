package webhooks

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"halo/internal/store"
	"halo/internal/testdb"
)

const hookSecret = "hook-secret-0123456789abcdefghijklmnop"

type hookServer struct {
	mu      sync.Mutex
	secret  string
	answer  string
	sign    bool
	status  int
	delay   time.Duration
	bodies  []string
	headers []string
}

func (s *hookServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.bodies = append(s.bodies, string(body))
	s.headers = append(s.headers, r.Header.Get("Halo-Signature"))
	answer, signed, status, delay := s.answer, s.sign, s.status, s.delay
	s.mu.Unlock()
	time.Sleep(delay)
	if signed {
		w.Header().Set("Halo-Signature", sign([]byte(s.secret), time.Now(), []byte(answer)))
	}
	if status != 0 {
		w.WriteHeader(status)
	}
	_, _ = w.Write([]byte(answer))
}

func hookApp(t *testing.T, st *store.Store, typ string) store.Application {
	t.Helper()
	app, err := st.CreateApplication(context.Background(), store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: typ, RedirectURIs: []string{"https://grafana.example.com/callback"}})
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func TestHookAdministration(t *testing.T) {
	st, _, h := setup(t)
	ctx := context.Background()
	admin := person(t, st, "ada@example.com", "app_admin")
	auditor := person(t, st, "aud@example.com", "auditor")
	app := hookApp(t, st, "web")
	path := "/api/v1/applications/" + app.ID + "/hook"
	valid := map[string]any{"url": "https://checks.example.com/halo/s3cr3tPath", "secret": hookSecret, "claims": []string{"department", "department", "https://example.com/cost_center"}}

	if rec := call(t, h, auditor, "PUT", path, valid); rec.Code != 403 {
		t.Fatalf("auditor save: %d", rec.Code)
	}
	if rec := call(t, h, auditor, "GET", path, nil); rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "null" {
		t.Fatalf("no hook yet: %d %s", rec.Code, rec.Body.String())
	}
	for name, body := range map[string]map[string]any{
		"http":           {"url": "http://checks.example.com/halo", "secret": hookSecret},
		"private":        {"url": "https://10.0.0.8/halo", "secret": hookSecret},
		"loopback":       {"url": "https://127.0.0.1/halo", "secret": hookSecret},
		"localhost":      {"url": "https://localhost/halo", "secret": hookSecret},
		"link-local":     {"url": "https://[fe80::1]/halo", "secret": hookSecret},
		"short secret":   {"url": "https://checks.example.com/halo", "secret": "short"},
		"no secret":      {"url": "https://checks.example.com/halo"},
		"sub claim":      {"url": "https://checks.example.com/halo", "secret": hookSecret, "claims": []string{"sub"}},
		"roles claim":    {"url": "https://checks.example.com/halo", "secret": hookSecret, "claims": []string{"roles"}},
		"exp claim":      {"url": "https://checks.example.com/halo", "secret": hookSecret, "claims": []string{"exp"}},
		"bad claim name": {"url": "https://checks.example.com/halo", "secret": hookSecret, "claims": []string{"has space"}},
	} {
		if rec := call(t, h, admin, "PUT", path, body); rec.Code != 422 {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	service := hookApp(t, st, "service")
	if rec := call(t, h, admin, "PUT", "/api/v1/applications/"+service.ID+"/hook", valid); rec.Code != 422 {
		t.Fatalf("service application: %d", rec.Code)
	}
	if hookable(store.Application{Protocol: "oidc", Type: "native", ClientID: store.CLIClientID}) == nil {
		t.Fatalf("the Halo CLI can take a hook")
	}

	rec := call(t, h, admin, "PUT", path, valid)
	var saved store.ApplicationHook
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &saved) != nil || saved.Host != "checks.example.com" || saved.FailOpen ||
		!reflect.DeepEqual(saved.Claims, []string{"department", "https://example.com/cost_center"}) {
		t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
	}
	got := call(t, h, auditor, "GET", path, nil)
	for _, body := range []string{rec.Body.String(), got.Body.String()} {
		if strings.Contains(body, hookSecret) || strings.Contains(body, "s3cr3tPath") || strings.Contains(body, "Sealed") {
			t.Fatalf("the API returned the hook URL or secret: %s", body)
		}
	}
	before, err := st.GetApplicationHook(ctx, app.ID)
	if err != nil || len(before.URLSealed) == 0 || strings.Contains(string(before.SecretSealed), hookSecret) {
		t.Fatalf("stored hook: %+v %v", before, err)
	}

	if rec := call(t, h, admin, "PUT", path, map[string]any{"failOpen": true, "claims": []string{"department"}}); rec.Code != 200 {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	after, _ := st.GetApplicationHook(ctx, app.ID)
	if !after.FailOpen || string(after.URLSealed) != string(before.URLSealed) || string(after.SecretSealed) != string(before.SecretSealed) {
		t.Fatalf("an update without a URL or secret replaced them: %+v", after)
	}

	if rec := call(t, h, admin, "DELETE", path, nil); rec.Code != 204 {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec := call(t, h, admin, "DELETE", path, nil); rec.Code != 404 {
		t.Fatalf("second delete: %d", rec.Code)
	}
	audit, err := st.ListAudit(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]int{}
	for _, e := range audit {
		actions[e.Action]++
		if strings.Contains(e.Summary, "s3cr3tPath") || strings.Contains(e.Summary, hookSecret) {
			t.Fatalf("the audit log names the hook URL or secret: %s", e.Summary)
		}
	}
	if actions["application.hook.update"] != 2 || actions["application.hook.delete"] != 1 {
		t.Fatalf("audit actions: %v", actions)
	}
}

func TestRunHook(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	app := hookApp(t, st, "web")
	user, err := st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	call := HookCall{Stage: HookToken, User: user, Scopes: []string{"openid", "email"}, GrantType: "refresh_token"}
	if v, err := RunHook(ctx, st, app, call); err != nil || v.Decision != "" {
		t.Fatalf("without a hook: %+v %v", v, err)
	}

	hs := &hookServer{secret: hookSecret, sign: true}
	srv := httptest.NewServer(hs)
	defer srv.Close()
	prevClient, prevTimeout := HookClient, hookTimeout
	HookClient, hookTimeout = srv.Client(), 200*time.Millisecond
	t.Cleanup(func() { HookClient, hookTimeout = prevClient, prevTimeout })
	save := func(failOpen bool) {
		t.Helper()
		err := st.SaveApplicationHook(ctx, store.ApplicationHook{AppID: app.ID, Host: "checks.example.com", URLSealed: st.Sealer.Seal([]byte(srv.URL)), SecretSealed: st.Sealer.Seal([]byte(hookSecret)),
			Claims: []string{"department", "sub", "roles"}, FailOpen: failOpen})
		if err != nil {
			t.Fatal(err)
		}
	}
	answer := func(body string, signed bool, status int, delay time.Duration) {
		hs.mu.Lock()
		hs.answer, hs.sign, hs.status, hs.delay = body, signed, status, delay
		hs.mu.Unlock()
	}
	save(false)

	answer(`{"decision":"allow","claims":{"department":"R&D","sub":"usr_attacker","iss":"https://evil.example.com","roles":["global_admin"],"unlisted":true}}`, true, 0, 0)
	v, err := RunHook(ctx, st, app, call)
	if err != nil || v.Decision != "allowed" || v.Denied() || !reflect.DeepEqual(v.Claims, map[string]any{"department": "R&D"}) {
		t.Fatalf("allow: %+v %v", v, err)
	}
	verify(t, hookSecret, hs.headers[0], hs.bodies[0])
	var sent struct {
		Type        string
		GrantType   string
		Scopes      []string
		User        map[string]string
		Application map[string]string
	}
	if json.Unmarshal([]byte(hs.bodies[0]), &sent) != nil || sent.Type != "token" || sent.GrantType != "refresh_token" || sent.User["email"] != "ada@example.com" ||
		sent.Application["clientId"] != app.ClientID || !reflect.DeepEqual(sent.Scopes, []string{"openid", "email"}) {
		t.Fatalf("request body: %s", hs.bodies[0])
	}

	answer(`{"decision":"deny","reason":"Your laptop is missing the latest security update."}`, true, 0, 0)
	if v, _ := RunHook(ctx, st, app, call); v.Decision != "denied" || v.Reason != "Your laptop is missing the latest security update." || !strings.Contains(v.Log, "missing the latest") {
		t.Fatalf("deny: %+v", v)
	}

	for name, c := range map[string]struct {
		body   string
		signed bool
		status int
		delay  time.Duration
		cause  string
	}{
		"unsigned":         {`{"decision":"allow"}`, false, 0, 0, "Halo-Signature"},
		"timeout":          {`{"decision":"allow"}`, true, 0, time.Second, "did not answer within 200ms"},
		"server error":     {`{"decision":"allow"}`, true, 500, 0, "500"},
		"unknown decision": {`{"decision":"maybe"}`, true, 0, 0, "decision"},
		"not json":         {`<html>`, true, 0, 0, "decision"},
	} {
		answer(c.body, c.signed, c.status, c.delay)
		save(false)
		if v, err := RunHook(ctx, st, app, call); err != nil || v.Decision != "failed_closed" || !v.Denied() || v.Reason == "" || !strings.Contains(v.Log, c.cause) || len(v.Claims) != 0 {
			t.Errorf("%s, fail closed: %+v %v", name, v, err)
		}
		save(true)
		if v, err := RunHook(ctx, st, app, call); err != nil || v.Decision != "failed_open" || v.Denied() || !strings.Contains(v.Log, c.cause) || len(v.Claims) != 0 {
			t.Errorf("%s, fail open: %+v %v", name, v, err)
		}
	}

	answer(`{"decision":"allow","claims":{"department":"R&D"}}`, false, 0, 0)
	hs.mu.Lock()
	hs.secret = "a-different-secret-0123456789abcdefghij"
	hs.sign = true
	hs.mu.Unlock()
	save(false)
	if v, _ := RunHook(ctx, st, app, call); v.Decision != "failed_closed" || len(v.Claims) != 0 {
		t.Fatalf("answer signed with another secret: %+v", v)
	}
}

func TestRunHookRefusesPrivateAddresses(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	app := hookApp(t, st, "web")
	user, err := st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	hs := &hookServer{secret: hookSecret, sign: true, answer: `{"decision":"allow"}`}
	srv := httptest.NewServer(hs)
	defer srv.Close()
	prev := HookClient
	HookClient = guardedClient()
	t.Cleanup(func() { HookClient = prev })
	if err := st.SaveApplicationHook(ctx, store.ApplicationHook{AppID: app.ID, Host: "checks.example.com", URLSealed: st.Sealer.Seal([]byte(srv.URL)), SecretSealed: st.Sealer.Seal([]byte(hookSecret)), Claims: []string{}}); err != nil {
		t.Fatal(err)
	}
	v, err := RunHook(ctx, st, app, HookCall{Stage: HookSignIn, User: user})
	if err != nil || v.Decision != "failed_closed" || !strings.Contains(v.Log, "private") || strings.Contains(v.Log, srv.URL) {
		t.Fatalf("private address: %+v %v", v, err)
	}
	if len(hs.bodies) != 0 {
		t.Fatalf("the guarded client reached a loopback address %d times", len(hs.bodies))
	}
}
