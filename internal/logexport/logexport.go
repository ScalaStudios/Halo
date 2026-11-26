package logexport

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"halo/internal/httpx"
	"halo/internal/id"
	"halo/internal/metrics"
	"halo/internal/store"
)

const (
	settle      = 5 * time.Second
	batch       = 500
	keep        = 100_000
	timeout     = 10 * time.Second
	maxDatagram = 65507
)

var (
	readers     = []string{"security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"}
	writers     = []string{"security_admin"}
	protocols   = map[string]string{"syslog-tls": "syslog over TLS", "syslog-udp": "syslog over UDP", "otlp-http": "OTLP/HTTP"}
	headerName  = regexp.MustCompile(`^[A-Za-z0-9!#$%&'*+.^_|~-]+$`)
	errRejected = errors.New("rejected")
	severities  = map[int]struct {
		number int
		text   string
	}{4: {13, "WARN"}, 5: {10, "NOTICE"}, 6: {9, "INFO"}}
)

type handler struct {
	st   *store.Store
	host string
	env  bool
	tls  *tls.Config
}

type input struct {
	Protocol     string  `json:"protocol"`
	Endpoint     string  `json:"endpoint"`
	AllowPrivate bool    `json:"allowPrivate"`
	Headers      *string `json:"headers"`
}

type view struct {
	Protocol      string `json:"protocol"`
	Endpoint      string `json:"endpoint"`
	AllowPrivate  bool   `json:"allowPrivate"`
	HeadersSecret string `json:"headersSecret"`
}

func Register(mux *http.ServeMux, d httpx.Deps) error {
	h := &handler{st: d.Store, host: "-"}
	if d.Config.PublicURL != nil {
		h.host = d.Config.Hostname()
	}
	if env := d.Config.LogExport; env.Protocol != "" || env.Endpoint != "" {
		x, err := input{Protocol: env.Protocol, Endpoint: env.Endpoint, AllowPrivate: env.AllowPrivate, Headers: &env.Headers}.validate()
		if err != nil {
			return fmt.Errorf("HALO_LOG_EXPORT_PROTOCOL, HALO_LOG_EXPORT_ENDPOINT or HALO_LOG_EXPORT_HEADERS: %s", err.(*httpx.Error).Message)
		}
		ctx := context.Background()
		if err := h.st.Tx(ctx, func(tx *store.Store) error { return save(ctx, tx, nil, "", x, &env.Headers) }); err != nil {
			return err
		}
		h.env = true
	}
	if d.Jobs != nil {
		d.Jobs.Every("export logs", 10*time.Second, func(ctx context.Context) error { return h.run(ctx, time.Now()) })
	}
	route := func(pattern string, roles []string, fn func(http.ResponseWriter, *http.Request, store.User) error) {
		mux.Handle(pattern, httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
			actor, err := httpx.RequireRole(r, roles...)
			if err != nil {
				return err
			}
			return fn(w, r, actor)
		}))
	}
	route("GET /api/v1/log-export", readers, h.get)
	route("PUT /api/v1/log-export", writers, h.put)
	route("DELETE /api/v1/log-export", writers, h.delete)
	route("POST /api/v1/log-export/test", writers, h.test)
	return nil
}

func (in input) validate() (store.LogExport, error) {
	x := store.LogExport{Protocol: in.Protocol, Endpoint: strings.TrimSpace(in.Endpoint), AllowPrivate: in.AllowPrivate}
	var host string
	switch x.Protocol {
	case "otlp-http":
		u, err := url.Parse(x.Endpoint)
		if err != nil || u.Host == "" || u.User != nil || len(x.Endpoint) > 2048 || !(u.Scheme == "https" || u.Scheme == "http" && x.AllowPrivate) {
			return x, httpx.Invalid("Enter the collector's https:// address, such as https://otel.example.com:4318/v1/logs. Plain http:// is allowed only with private addresses allowed.")
		}
		if u.Path == "" || u.Path == "/" {
			u.Path = "/v1/logs"
			x.Endpoint = u.String()
		}
		host = u.Hostname()
	case "syslog-tls", "syslog-udp":
		h, port, err := net.SplitHostPort(x.Endpoint)
		if n, perr := strconv.Atoi(port); err != nil || perr != nil || h == "" || n < 1 || n > 65535 || len(x.Endpoint) > 300 {
			return x, httpx.Invalid("Enter the syslog server as host:port, such as siem.example.com:6514.")
		}
		host = h
		if in.Headers != nil && strings.TrimSpace(*in.Headers) != "" {
			return x, httpx.Invalid("Headers are sent only to OpenTelemetry collectors. Remove them for syslog.")
		}
	default:
		return x, httpx.Invalid("Choose syslog-tls, syslog-udp or otlp-http as the protocol.")
	}
	ip, err := netip.ParseAddr(host)
	if !x.AllowPrivate && (err == nil && !httpx.PublicAddress(ip) || host == "localhost" || strings.HasSuffix(host, ".localhost")) {
		return x, httpx.Invalid("This is a private, loopback or link-local address. Turn on private addresses to send to a collector on your own network.")
	}
	if in.Headers != nil {
		if _, err := parseHeaders(*in.Headers); err != nil {
			return x, err
		}
	}
	return x, nil
}

func parseHeaders(s string) (http.Header, error) {
	h := http.Header{}
	for _, pair := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' }) {
		if strings.TrimSpace(pair) == "" {
			continue
		}
		name, value, ok := strings.Cut(pair, "=")
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if !ok || !headerName.MatchString(name) || value == "" || strings.ContainsAny(value, "\r\x00") {
			return nil, httpx.Invalid("Write headers as name=value pairs separated by commas or new lines, such as Authorization=Bearer <token>.")
		}
		h.Add(name, value)
	}
	return h, nil
}

func save(ctx context.Context, tx *store.Store, actorID *string, ip string, x store.LogExport, headers *string) error {
	before, err := tx.GetLogExport(ctx)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	old, err := opened(tx, before)
	if err != nil {
		return err
	}
	next := old
	if headers != nil {
		next = strings.TrimSpace(*headers)
	} else if before.Protocol != x.Protocol || before.Endpoint != x.Endpoint {
		next = ""
	}
	if x.Protocol != "otlp-http" {
		next = ""
	}
	if err := tx.SaveLogExport(ctx, x, next); err != nil {
		return err
	}
	changes := store.Diff(view{before.Protocol, before.Endpoint, before.AllowPrivate, old}, view{x.Protocol, x.Endpoint, x.AllowPrivate, next})
	if len(changes) == 0 {
		return nil
	}
	return tx.RecordAudit(ctx, store.AuditEvent{ActorID: actorID, IP: ip, Action: "log_export.update", Summary: "Streamed sign-in and audit events with " + protocols[x.Protocol],
		TargetType: "settings", TargetLabel: "Log export", Changes: changes})
}

func opened(st *store.Store, x store.LogExport) (string, error) {
	if x.HeadersSealed == nil {
		return "", nil
	}
	plain, err := st.Sealer.Open(x.HeadersSealed)
	return string(plain), err
}

func (h *handler) get(w http.ResponseWriter, r *http.Request, _ store.User) error {
	ctx := r.Context()
	x, err := h.st.GetLogExport(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return httpx.JSON(w, http.StatusOK, map[string]any{"exporter": nil, "queued": 0, "environment": h.env})
	}
	if err != nil {
		return err
	}
	queued, err := h.st.LogQueue(ctx)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"exporter": x, "queued": queued, "environment": h.env})
}

func (h *handler) managed() error {
	if h.env {
		return httpx.Fail(http.StatusConflict, "ERR_LOG_EXPORT_ENVIRONMENT", "The log exporter is set by HALO_LOG_EXPORT_* environment variables. Change them and restart Halo.")
	}
	return nil
}

func (h *handler) put(w http.ResponseWriter, r *http.Request, actor store.User) error {
	if err := h.managed(); err != nil {
		return err
	}
	var in input
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	x, err := in.validate()
	if err != nil {
		return err
	}
	ctx := r.Context()
	if err := h.st.Tx(ctx, func(tx *store.Store) error { return save(ctx, tx, &actor.ID, httpx.ClientIP(r), x, in.Headers) }); err != nil {
		return err
	}
	return h.get(w, r, actor)
}

func (h *handler) delete(w http.ResponseWriter, r *http.Request, actor store.User) error {
	if err := h.managed(); err != nil {
		return err
	}
	ctx := r.Context()
	err := h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.DeleteLogExport(ctx); err != nil {
			return err
		}
		return tx.RecordAudit(ctx, store.AuditEvent{ActorID: &actor.ID, IP: httpx.ClientIP(r), Action: "log_export.delete", Summary: "Stopped streaming sign-in and audit events", TargetType: "settings", TargetLabel: "Log export"})
	})
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("The log exporter")
	}
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *handler) test(w http.ResponseWriter, r *http.Request, actor store.User) error {
	ctx := r.Context()
	x, err := h.st.GetLogExport(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("The log exporter")
	}
	if err != nil {
		return err
	}
	data, _ := json.Marshal(map[string]string{"message": "This is a test event sent from the Halo console.", "sentBy": actor.Email})
	if err := h.send(ctx, x, []store.WebhookEvent{{ID: id.New("evt"), Type: "log_export.test", Time: time.Now().UTC(), Data: data}}); err != nil {
		return httpx.Fail(http.StatusBadGateway, "ERR_LOG_EXPORT_FAILED", "The test event was not sent: "+err.Error()+". Check the address, and that the Halo server can reach the collector.")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func backoff(failures int) time.Duration {
	return min(10*time.Second<<min(failures, 6), 5*time.Minute)
}

func (h *handler) run(ctx context.Context, now time.Time) error {
	x, err := h.st.GetLogExport(ctx)
	if errors.Is(err, store.ErrNotFound) || err == nil && now.Before(x.NextAttemptAt) {
		return nil
	}
	if err != nil {
		return err
	}
	dropped, err := h.st.TrimLogQueue(ctx, keep)
	if err != nil {
		return err
	}
	metrics.LogExport.Add(float64(dropped), "dropped")
	var failure error
	sent := 0
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		events, err := tx.PollLogEvents(ctx, now.Add(-settle), batch)
		if err != nil || len(events) == 0 {
			return err
		}
		sent, failure = len(events), h.send(ctx, x, events)
		if errors.Is(failure, errRejected) {
			return nil
		}
		return failure
	})
	switch {
	case failure == nil && err != nil:
		return err
	case sent == 0:
		return nil
	case failure == nil:
		metrics.LogExport.Add(float64(sent), "sent")
		return h.st.RecordLogExport(ctx, &now, "", 0, now)
	case errors.Is(failure, errRejected):
		metrics.LogExport.Add(float64(sent), "rejected")
		return h.st.RecordLogExport(ctx, nil, failure.Error(), 0, now)
	default:
		metrics.LogExport.Add(float64(sent), "failed")
		return h.st.RecordLogExport(ctx, nil, failure.Error(), x.Failures+1, now.Add(backoff(x.Failures+1)))
	}
}

func (h *handler) send(ctx context.Context, x store.LogExport, events []store.WebhookEvent) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	dialer := httpx.GuardedDialer()
	if x.AllowPrivate {
		dialer = &net.Dialer{Timeout: timeout}
	}
	if x.Protocol == "otlp-http" {
		return h.post(ctx, dialer, x, events)
	}
	var conn net.Conn
	var err error
	if x.Protocol == "syslog-tls" {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: h.tls}).DialContext(ctx, "tcp", x.Endpoint)
	} else {
		conn, err = dialer.DialContext(ctx, "udp", x.Endpoint)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	conn.SetDeadline(deadline)
	for _, e := range events {
		msg := syslog(h.host, e)
		if x.Protocol == "syslog-tls" {
			msg = append([]byte(strconv.Itoa(len(msg))+" "), msg...)
		} else {
			msg = msg[:min(len(msg), maxDatagram)]
		}
		if _, err := conn.Write(msg); err != nil {
			return err
		}
	}
	return nil
}

func (h *handler) post(ctx context.Context, dialer *net.Dialer, x store.LogExport, events []store.WebhookEvent) error {
	plain, err := opened(h.st, x)
	if err != nil {
		return err
	}
	headers, err := parseHeaders(plain)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, x.Endpoint, bytes.NewReader(otlp(h.host, events)))
	if err != nil {
		return err
	}
	req.Header = headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Halo-Log-Export")
	client := httpx.DialerClient(dialer)
	defer client.CloseIdleConnections()
	res, err := client.Do(req)
	if err != nil {
		var u *url.Error
		if errors.As(err, &u) {
			return u.Err
		}
		return err
	}
	io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	res.Body.Close()
	switch {
	case res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusRequestEntityTooLarge:
		return fmt.Errorf("the collector %w the events with %s", errRejected, res.Status)
	case res.StatusCode < 200 || res.StatusCode > 299:
		return fmt.Errorf("the collector answered %s", res.Status)
	}
	return nil
}

func severity(eventType string) int {
	switch {
	case eventType == "sign_in.failure":
		return 4
	case strings.HasPrefix(eventType, "sign_in."):
		return 6
	}
	return 5
}

func syslog(host string, e store.WebhookEvent) []byte {
	body, _ := json.Marshal(e)
	msgID := e.Type[:min(len(e.Type), 32)]
	return fmt.Appendf(nil, "<%d>1 %s %s halo - %s - %s", 10*8+severity(e.Type), e.Time.UTC().Format("2006-01-02T15:04:05.000000Z"), host, msgID, body)
}

func otlp(host string, events []store.WebhookEvent) []byte {
	attr := func(key, value string) map[string]any {
		return map[string]any{"key": key, "value": map[string]string{"stringValue": value}}
	}
	records := []map[string]any{}
	for _, e := range events {
		body, _ := json.Marshal(e)
		s := severities[severity(e.Type)]
		records = append(records, map[string]any{
			"timeUnixNano":   strconv.FormatInt(e.Time.UnixNano(), 10),
			"severityNumber": s.number,
			"severityText":   s.text,
			"body":           map[string]string{"stringValue": string(body)},
			"attributes":     []any{attr("event.name", e.Type)},
		})
	}
	out, _ := json.Marshal(map[string]any{"resourceLogs": []any{map[string]any{
		"resource":  map[string]any{"attributes": []any{attr("service.name", "halo"), attr("host.name", host)}},
		"scopeLogs": []any{map[string]any{"scope": map[string]string{"name": "halo"}, "logRecords": records}},
	}}})
	return out
}
