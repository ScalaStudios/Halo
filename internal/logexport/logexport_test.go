package logexport

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"halo/internal/httpx"
	"halo/internal/metrics"
	"halo/internal/store"
	"halo/internal/testdb"
)

var rfc5424 = regexp.MustCompile(`^<(\d{1,3})>1 \d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z auth\.example\.com halo - ([!-~]{1,32}) - (\{.*\})$`)

type collector struct {
	mu       sync.Mutex
	statuses []int
	bodies   []string
	auth     []string
}

func (c *collector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bodies = append(c.bodies, string(body))
	c.auth = append(c.auth, r.Header.Get("Authorization"))
	if len(c.statuses) > 0 {
		w.WriteHeader(c.statuses[0])
		c.statuses = c.statuses[1:]
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
	return st, &handler{st: st, host: "auth.example.com"}, authn.Resolve(authn.SameOrigin(mux))
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

func events(t *testing.T, st *store.Store) {
	t.Helper()
	ctx := context.Background()
	at := time.Now().Add(time.Second)
	if err := st.RecordSignIn(ctx, store.SignInEvent{Time: at, Email: "ada@example.com", Result: "failure", Method: "password", IP: "203.0.113.7", Reason: "Wrong password"}); err != nil {
		t.Fatal(err)
	}
	changes := store.Diff(map[string]string{"clientSecret": "old-secret-value", "name": "Wiki"}, map[string]string{"clientSecret": "new-secret-value", "name": "Team wiki"})
	if err := st.RecordAudit(ctx, store.AuditEvent{Time: at, Action: "application.update", Summary: "Changed the application", TargetType: "application", TargetLabel: "Wiki", Changes: changes}); err != nil {
		t.Fatal(err)
	}
}

func later() time.Time {
	return time.Now().Add(time.Minute)
}

func TestSyslogOverTLSAndUDP(t *testing.T) {
	st, h, _ := setup(t)
	ctx := context.Background()
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	cert, roots := srv.TLS.Certificates[0], x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	srv.Close()
	h.tls = &tls.Config{RootCAs: roots}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	received := make(chan []string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		var messages []string
		for len(messages) < 2 {
			size, err := r.ReadString(' ')
			n, _ := strconv.Atoi(strings.TrimSpace(size))
			if err != nil || n == 0 {
				break
			}
			msg := make([]byte, n)
			if _, err := io.ReadFull(r, msg); err != nil {
				break
			}
			messages = append(messages, string(msg))
		}
		received <- messages
	}()
	if err := st.SaveLogExport(ctx, store.LogExport{Protocol: "syslog-tls", Endpoint: ln.Addr().String(), AllowPrivate: true}, ""); err != nil {
		t.Fatal(err)
	}
	events(t, st)
	if err := h.run(ctx, later()); err != nil {
		t.Fatal(err)
	}
	messages := <-received
	if len(messages) != 2 {
		t.Fatalf("messages: %q", messages)
	}
	ids := map[string]string{}
	for _, msg := range messages {
		m := rfc5424.FindStringSubmatch(msg)
		if m == nil {
			t.Fatalf("not RFC 5424: %q", msg)
		}
		ids[m[2]] = m[1]
		if strings.Contains(msg, "secret-value") {
			t.Fatalf("a secret reached syslog: %s", msg)
		}
		var e store.WebhookEvent
		if err := json.Unmarshal([]byte(m[3]), &e); err != nil || e.Type != m[2] {
			t.Fatalf("message body: %s", m[3])
		}
	}
	if ids["sign_in.failure"] != "84" || ids["application.update"] != "85" {
		t.Fatalf("priorities: %v", ids)
	}
	x, err := st.GetLogExport(ctx)
	if queued, _ := st.LogQueue(ctx); err != nil || x.LastSentAt == nil || x.LastError != "" || queued != 0 {
		t.Fatalf("after sending: %+v, %d queued", x, queued)
	}

	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	if err := h.send(ctx, store.LogExport{Protocol: "syslog-udp", Endpoint: udp.LocalAddr().String(), AllowPrivate: true}, []store.WebhookEvent{{ID: "evt_1", Type: "log_export.test", Time: time.Now(), Data: json.RawMessage(`{}`)}}); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 2048)
	udp.SetDeadline(time.Now().Add(5 * time.Second))
	n, _, err := udp.ReadFrom(buf)
	if err != nil || !rfc5424.Match(buf[:n]) {
		t.Fatalf("datagram %q: %v", buf[:n], err)
	}
}

func TestOTLPPayloadOutageAndRetry(t *testing.T) {
	st, h, _ := setup(t)
	ctx := context.Background()
	c := &collector{statuses: []int{http.StatusServiceUnavailable}}
	var open atomic.Int32
	srv := httptest.NewUnstartedServer(c)
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		switch state {
		case http.StateNew:
			open.Add(1)
		case http.StateClosed, http.StateHijacked:
			open.Add(-1)
		}
	}
	srv.Start()
	defer srv.Close()
	if err := st.SaveLogExport(ctx, store.LogExport{Protocol: "otlp-http", Endpoint: srv.URL + "/v1/logs", AllowPrivate: true}, "Authorization=Bearer collector-token"); err != nil {
		t.Fatal(err)
	}
	events(t, st)
	failed, sent := metrics.LogExport.Value("failed"), metrics.LogExport.Value("sent")

	now := later()
	if err := h.run(ctx, now); err != nil {
		t.Fatal(err)
	}
	x, _ := st.GetLogExport(ctx)
	if queued, _ := st.LogQueue(ctx); queued != 2 || x.Failures != 1 || !x.NextAttemptAt.After(now) || !strings.Contains(x.LastError, "503") || x.LastSentAt != nil {
		t.Fatalf("an outage must keep the events queued: %+v, %d queued", x, queued)
	}
	if err := h.run(ctx, now.Add(time.Second)); err != nil || len(c.bodies) != 1 {
		t.Fatalf("retried before the backoff: %d requests, %v", len(c.bodies), err)
	}
	if err := h.run(ctx, x.NextAttemptAt); err != nil {
		t.Fatal(err)
	}
	x, _ = st.GetLogExport(ctx)
	if queued, _ := st.LogQueue(ctx); queued != 0 || x.Failures != 0 || x.LastSentAt == nil || x.LastError != "" || len(c.bodies) != 2 {
		t.Fatalf("after the retry: %+v, %d queued, %d requests", x, queued, len(c.bodies))
	}
	for i := 0; open.Load() != 0 && i < 100; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if open.Load() != 0 {
		t.Fatalf("%d connections to the collector were left open", open.Load())
	}
	if metrics.LogExport.Value("failed")-failed != 2 || metrics.LogExport.Value("sent")-sent != 2 {
		t.Fatal("the counters did not move")
	}
	if c.auth[1] != "Bearer collector-token" || strings.Contains(c.bodies[1], "secret-value") || strings.Contains(c.bodies[1], "collector-token") {
		t.Fatalf("headers or secrets: %q %s", c.auth[1], c.bodies[1])
	}

	var payload struct {
		ResourceLogs []struct {
			Resource struct {
				Attributes []struct {
					Key   string
					Value struct{ StringValue string }
				}
			}
			ScopeLogs []struct {
				Scope      struct{ Name string }
				LogRecords []struct {
					TimeUnixNano   string
					SeverityNumber int
					SeverityText   string
					Body           struct{ StringValue string }
					Attributes     []struct {
						Key   string
						Value struct{ StringValue string }
					}
				}
			}
		}
	}
	if err := json.Unmarshal([]byte(c.bodies[1]), &payload); err != nil || len(payload.ResourceLogs) != 1 || len(payload.ResourceLogs[0].ScopeLogs) != 1 {
		t.Fatalf("payload: %s", c.bodies[1])
	}
	if a := payload.ResourceLogs[0].Resource.Attributes; len(a) == 0 || a[0].Key != "service.name" || a[0].Value.StringValue != "halo" {
		t.Fatalf("resource: %+v", a)
	}
	records := payload.ResourceLogs[0].ScopeLogs[0].LogRecords
	if len(records) != 2 {
		t.Fatalf("records: %+v", records)
	}
	for _, r := range records {
		var e store.WebhookEvent
		if _, err := strconv.ParseInt(r.TimeUnixNano, 10, 64); err != nil || json.Unmarshal([]byte(r.Body.StringValue), &e) != nil || r.Attributes[0].Value.StringValue != e.Type {
			t.Fatalf("record: %+v", r)
		}
		if e.Type == "sign_in.failure" && (r.SeverityNumber != 13 || r.SeverityText != "WARN") {
			t.Fatalf("severity: %+v", r)
		}
	}

	c.statuses = []int{http.StatusBadRequest}
	events(t, st)
	if err := h.run(ctx, x.NextAttemptAt); err != nil {
		t.Fatal(err)
	}
	x, _ = st.GetLogExport(ctx)
	if queued, _ := st.LogQueue(ctx); queued != 0 || x.Failures != 0 || !strings.Contains(x.LastError, "rejected") {
		t.Fatalf("a rejected batch must not block the queue: %+v, %d queued", x, queued)
	}
}

func TestBoundedQueue(t *testing.T) {
	st, _, _ := setup(t)
	ctx := context.Background()
	if err := st.SaveLogExport(ctx, store.LogExport{Protocol: "syslog-udp", Endpoint: "203.0.113.9:514"}, ""); err != nil {
		t.Fatal(err)
	}
	events(t, st)
	events(t, st)
	dropped, err := st.TrimLogQueue(ctx, 1)
	if queued, _ := st.LogQueue(ctx); err != nil || dropped != 2 || queued != 2 {
		t.Fatalf("dropped %d, %d queued: %v", dropped, queued, err)
	}
}

func TestConsole(t *testing.T) {
	st, h, mux := setup(t)
	ctx := context.Background()
	security := person(t, st, "sam@example.com", "security_admin")
	auditor := person(t, st, "ann@example.com", "auditor")

	rec := call(t, mux, auditor, "GET", "/api/v1/log-export", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"exporter":null`) {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, mux, auditor, "PUT", "/api/v1/log-export", input{Protocol: "syslog-tls", Endpoint: "siem.example.com:6514"}); rec.Code != 403 {
		t.Fatalf("auditors must not configure the exporter: %d", rec.Code)
	}
	for _, in := range []input{
		{Protocol: "syslog-tcp", Endpoint: "siem.example.com:514"},
		{Protocol: "syslog-tls", Endpoint: "siem.example.com"},
		{Protocol: "syslog-tls", Endpoint: "10.0.0.5:6514"},
		{Protocol: "otlp-http", Endpoint: "http://otel.example.com:4318"},
		{Protocol: "otlp-http", Endpoint: "https://127.0.0.1:4318"},
		{Protocol: "otlp-http", Endpoint: "https://otel.example.com", Headers: ptr("no-equals-sign")},
	} {
		if rec := call(t, mux, security, "PUT", "/api/v1/log-export", in); rec.Code != 422 {
			t.Fatalf("%+v: %d %s", in, rec.Code, rec.Body.String())
		}
	}
	if rec := call(t, mux, security, "PUT", "/api/v1/log-export", input{Protocol: "syslog-tls", Endpoint: "10.0.0.5:6514", AllowPrivate: true}); rec.Code != 200 {
		t.Fatalf("private with the setting: %d %s", rec.Code, rec.Body.String())
	}
	rec = call(t, mux, security, "PUT", "/api/v1/log-export", input{Protocol: "otlp-http", Endpoint: "https://otel.example.com", Headers: ptr("Authorization=Bearer hidden-token")})
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "hidden-token") || !strings.Contains(rec.Body.String(), `"endpoint":"https://otel.example.com/v1/logs"`) || !strings.Contains(rec.Body.String(), `"headersSet":true`) {
		t.Fatalf("otlp: %d %s", rec.Code, rec.Body.String())
	}
	audit, err := st.ListAudit(ctx, 10)
	if err != nil || len(audit) != 2 || audit[0].Action != "log_export.update" {
		t.Fatalf("audit: %+v %v", audit, err)
	}
	raw, _ := json.Marshal(audit)
	if strings.Contains(string(raw), "hidden-token") || !strings.Contains(string(raw), `{"field":"headersSecret","redacted":true}`) {
		t.Fatalf("audit changes: %s", raw)
	}

	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	if rec := call(t, mux, security, "PUT", "/api/v1/log-export", input{Protocol: "syslog-udp", Endpoint: udp.LocalAddr().String()}); rec.Code != 422 {
		t.Fatalf("loopback without the setting: %d", rec.Code)
	}
	if err := h.send(ctx, store.LogExport{Protocol: "syslog-udp", Endpoint: udp.LocalAddr().String()}, nil); err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("the dialer must refuse private addresses: %v", err)
	}
	if rec := call(t, mux, security, "PUT", "/api/v1/log-export", input{Protocol: "syslog-udp", Endpoint: udp.LocalAddr().String(), AllowPrivate: true}); rec.Code != 200 {
		t.Fatalf("udp: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, mux, security, "POST", "/api/v1/log-export/test", nil); rec.Code != 204 {
		t.Fatalf("test: %d %s", rec.Code, rec.Body.String())
	}
	buf := make([]byte, 2048)
	udp.SetDeadline(time.Now().Add(5 * time.Second))
	if n, _, err := udp.ReadFrom(buf); err != nil || !strings.Contains(string(buf[:n]), " halo - log_export.test - ") {
		t.Fatalf("test event %q: %v", buf[:n], err)
	}

	if rec := call(t, mux, security, "DELETE", "/api/v1/log-export", nil); rec.Code != 204 {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec := call(t, mux, security, "POST", "/api/v1/log-export/test", nil); rec.Code != 404 {
		t.Fatalf("test without an exporter: %d", rec.Code)
	}
}

func TestEnvironment(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	d := httpx.Deps{Store: st}
	d.Config.LogExport.Protocol, d.Config.LogExport.Endpoint = "otlp-http", "ftp://otel.example.com"
	if err := Register(http.NewServeMux(), d); err == nil || !strings.Contains(err.Error(), "HALO_LOG_EXPORT") {
		t.Fatalf("invalid environment: %v", err)
	}
	d.Config.LogExport.Endpoint, d.Config.LogExport.Headers = "https://otel.example.com/v1/logs", "Authorization=Bearer env-token"
	public, _ := url.Parse("http://localhost:3200")
	d.Auth = &httpx.Auth{Store: st, Public: public, Dev: true}
	mux := http.NewServeMux()
	if err := Register(mux, d); err != nil {
		t.Fatal(err)
	}
	x, err := st.GetLogExport(ctx)
	if err != nil || x.Endpoint != "https://otel.example.com/v1/logs" || !x.HeadersSet {
		t.Fatalf("saved from the environment: %+v %v", x, err)
	}
	security := person(t, st, "sam@example.com", "security_admin")
	h := d.Auth.Resolve(d.Auth.SameOrigin(mux))
	if rec := call(t, h, security, "GET", "/api/v1/log-export", nil); !strings.Contains(rec.Body.String(), `"environment":true`) {
		t.Fatalf("get: %s", rec.Body.String())
	}
	if rec := call(t, h, security, "DELETE", "/api/v1/log-export", nil); rec.Code != 409 {
		t.Fatalf("environment-managed exporters can't be removed in the console: %d", rec.Code)
	}
}

func ptr(s string) *string {
	return &s
}
