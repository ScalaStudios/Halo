package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"halo/internal/httpx"
	"halo/internal/store"
	"halo/internal/testdb"
)

type receiver struct {
	mu       sync.Mutex
	failures int
	bodies   []string
	headers  []string
}

func (rc *receiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.bodies = append(rc.bodies, string(body))
	rc.headers = append(rc.headers, r.Header.Get("Halo-Signature"))
	if rc.failures > 0 {
		rc.failures--
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

func setup(t *testing.T) (*store.Store, *handler, http.Handler) {
	t.Helper()
	st := testdb.New(t)
	public, _ := url.Parse("http://localhost:3200")
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	mux := http.NewServeMux()
	if err := Register(mux, httpx.Deps{Store: st, Auth: authn}); err != nil {
		t.Fatal(err)
	}
	return st, &handler{st: st, client: http.DefaultClient}, authn.Resolve(authn.SameOrigin(mux))
}

func person(t *testing.T, st *store.Store, email string, roles ...string) string {
	t.Helper()
	u, err := st.CreateUser(context.Background(), store.NewUser{Email: email, Name: strings.Split(email, "@")[0], Status: "active", Roles: roles})
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := st.CreateSession(context.Background(), store.NewSession{UserID: u.ID, Method: "passkey"})
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func call(t *testing.T, h http.Handler, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: token})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func verify(t *testing.T, secret, header, body string) {
	t.Helper()
	unix, err := strconv.ParseInt(strings.TrimPrefix(strings.Split(header, ",")[0], "t="), 10, 64)
	at := time.Unix(unix, 0)
	if err != nil || time.Since(at) > time.Minute || sign([]byte(secret), at, []byte(body)) != header {
		t.Fatalf("signature %q does not match the body", header)
	}
}

func TestWebhookRolesAndValidation(t *testing.T) {
	st, _, h := setup(t)
	security := person(t, st, "sam@example.com", "security_admin")
	auditor := person(t, st, "aud@example.com", "auditor")
	valid := map[string]any{"url": "https://hooks.example.com/halo", "events": []string{"user.*", "sign_in.failure"}}

	if rec := call(t, h, auditor, "POST", "/api/v1/webhooks", valid); rec.Code != 403 {
		t.Fatalf("auditor create: %d", rec.Code)
	}
	for _, body := range []map[string]any{
		{"url": "http://hooks.example.com/halo", "events": []string{"*"}},
		{"url": "ftp://localhost/halo", "events": []string{"*"}},
		{"url": "https://hooks.example.com", "events": []string{}},
		{"url": "https://hooks.example.com", "events": []string{"User Invite"}},
	} {
		if rec := call(t, h, security, "POST", "/api/v1/webhooks", body); rec.Code != 422 {
			t.Fatalf("invalid %v: %d %s", body, rec.Code, rec.Body.String())
		}
	}
	rec := call(t, h, security, "POST", "/api/v1/webhooks", valid)
	var created struct {
		Webhook store.Webhook `json:"webhook"`
		Secret  string        `json:"secret"`
	}
	if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &created) != nil || !strings.HasPrefix(created.Secret, "whsec_") || !created.Webhook.Enabled {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, h, auditor, "GET", "/api/v1/webhooks/"+created.Webhook.ID, nil); rec.Code != 200 || strings.Contains(rec.Body.String(), created.Secret) {
		t.Fatalf("auditor read: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, h, auditor, "DELETE", "/api/v1/webhooks/"+created.Webhook.ID, nil); rec.Code != 403 {
		t.Fatalf("auditor delete: %d", rec.Code)
	}
	if rec := call(t, h, security, "PUT", "/api/v1/webhooks/"+created.Webhook.ID, map[string]any{"url": "http://127.0.0.1:9999/halo", "events": []string{"*"}, "enabled": false}); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, h, security, "DELETE", "/api/v1/webhooks/"+created.Webhook.ID, nil); rec.Code != 204 {
		t.Fatalf("delete: %d", rec.Code)
	}
}

func TestDeliverySignatureAndBackoff(t *testing.T) {
	st, h, _ := setup(t)
	ctx := context.Background()
	rc := &receiver{failures: 2}
	srv := httptest.NewServer(rc)
	defer srv.Close()

	endpointID, secret, err := st.CreateWebhook(ctx, store.NewWebhook{URL: srv.URL, Events: []string{"user.*"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateWebhook(ctx, store.NewWebhook{URL: srv.URL + "/paused", Events: []string{"*"}, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	for _, action := range []string{"user.invite", "group.create"} {
		if err := st.RecordAudit(ctx, store.AuditEvent{Action: action, Summary: "Test " + action, TargetType: "user", TargetLabel: "Ada"}); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Now().Add(settle + time.Second)
	if err := h.run(ctx, now); err != nil {
		t.Fatal(err)
	}
	deliveries, _ := st.ListWebhookDeliveries(ctx, endpointID, 10)
	if len(deliveries) != 1 || deliveries[0].EventType != "user.invite" || deliveries[0].Status != "pending" || deliveries[0].Attempts != 1 {
		t.Fatalf("after first attempt: %+v", deliveries)
	}
	if want := now.Add(30 * time.Second); !deliveries[0].NextAttemptAt.Equal(want.Truncate(time.Microsecond)) {
		t.Fatalf("next attempt %s, want %s", deliveries[0].NextAttemptAt, want)
	}

	if err := h.run(ctx, now.Add(29*time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(rc.bodies) != 1 {
		t.Fatalf("retried before the backoff elapsed: %d calls", len(rc.bodies))
	}
	now = now.Add(30 * time.Second)
	if err := h.run(ctx, now); err != nil {
		t.Fatal(err)
	}
	deliveries, _ = st.ListWebhookDeliveries(ctx, endpointID, 10)
	if deliveries[0].Attempts != 2 || !deliveries[0].NextAttemptAt.Equal(now.Add(time.Minute).Truncate(time.Microsecond)) {
		t.Fatalf("second backoff: %+v", deliveries[0])
	}
	if err := h.run(ctx, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	deliveries, _ = st.ListWebhookDeliveries(ctx, endpointID, 10)
	if deliveries[0].Status != "delivered" || deliveries[0].Attempts != 3 || *deliveries[0].ResponseStatus != 200 {
		t.Fatalf("after success: %+v", deliveries[0])
	}
	if len(rc.bodies) != 3 || rc.bodies[0] != rc.bodies[2] {
		t.Fatalf("receiver saw %d bodies", len(rc.bodies))
	}
	var event struct {
		ID   string         `json:"id"`
		Type string         `json:"type"`
		Time time.Time      `json:"time"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(rc.bodies[2]), &event); err != nil || event.Type != "user.invite" || !strings.HasPrefix(event.ID, "aud_") || event.Data["summary"] != "Test user.invite" || event.Time.IsZero() {
		t.Fatalf("payload: %s", rc.bodies[2])
	}
	for i := range rc.bodies {
		verify(t, secret, rc.headers[i], rc.bodies[i])
	}

	rc.failures = 100
	if err := st.RecordSignIn(ctx, store.SignInEvent{Email: "ada@example.com", Result: "failure", Method: "totp"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateWebhook(ctx, endpointID, store.NewWebhook{URL: srv.URL, Events: []string{"sign_in.failure"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	now = time.Now().Add(settle + time.Second)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := h.run(ctx, now); err != nil {
			t.Fatal(err)
		}
		now = now.Add(backoff(attempt))
	}
	deliveries, _ = st.ListWebhookDeliveries(ctx, endpointID, 10)
	if deliveries[0].EventType != "sign_in.failure" || deliveries[0].Status != "failed" || deliveries[0].Attempts != maxAttempts || *deliveries[0].ResponseStatus != 503 {
		t.Fatalf("after %d failures: %+v", maxAttempts, deliveries[0])
	}
	if backoff(7) != 32*time.Minute {
		t.Fatalf("seventh backoff is %s", backoff(7))
	}
}

func TestSendTestEvent(t *testing.T) {
	st, _, h := setup(t)
	security := person(t, st, "sam@example.com", "security_admin")
	rc := &receiver{}
	srv := httptest.NewServer(rc)
	defer srv.Close()
	endpointID, secret, err := st.CreateWebhook(context.Background(), store.NewWebhook{URL: srv.URL, Events: []string{"*"}, Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	rec := call(t, h, security, "POST", "/api/v1/webhooks/"+endpointID+"/test", nil)
	var delivery store.WebhookDelivery
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &delivery) != nil || delivery.Status != "delivered" || delivery.EventType != "webhook.test" {
		t.Fatalf("test event: %d %s", rec.Code, rec.Body.String())
	}
	verify(t, secret, rc.headers[0], rc.bodies[0])
}
