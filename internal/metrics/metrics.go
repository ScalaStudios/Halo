package metrics

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"net/http"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"halo/internal/secret"
)

var (
	SignIns           = NewCounter("halo_sign_in_events_total", "Sign-in log events by method, result and risk.", "method", "result", "risk")
	Lockouts          = NewCounter("halo_lockouts_total", "Sign-ins refused after too many failed attempts, for one account or from one IP address.", "scope")
	TokensIssued      = NewCounter("halo_tokens_issued_total", "Tokens issued at /oauth2/token by grant type.", "grant")
	RefreshRefused    = NewCounter("halo_refresh_tokens_refused_total", "Refresh tokens refused because they were unknown, expired, revoked or already used.")
	SCIMRuns          = NewCounter("halo_scim_outbound_runs_total", "Outbound SCIM runs by result.", "result")
	SCIMRecords       = NewCounter("halo_scim_outbound_records_total", "Users and groups that outbound SCIM changed, by resource and action.", "resource", "action")
	LDAPRuns          = NewCounter("halo_ldap_sync_runs_total", "LDAP directory sync runs by result. Dry runs are not counted.", "result")
	LDAPChanges       = NewCounter("halo_ldap_sync_changes_total", "People, groups and memberships that LDAP directory sync changed, by action.", "action")
	WebhookDeliveries = NewCounter("halo_webhook_deliveries_total", "Webhook delivery attempts by outcome: delivered, pending for a retry, or failed for good.", "outcome")
	LogExport         = NewCounter("halo_log_export_events_total", "Sign-in and audit events for the log exporter by outcome: sent, failed and retried later, rejected by the collector, or dropped from a full queue.", "outcome")
	RateLimited       = NewCounter("halo_rate_limited_requests_total", "Requests refused by the API rate limit, by scope.", "scope")
	HTTPRequests      = NewCounter("halo_http_requests_total", "HTTP requests by route pattern and status code.", "route", "status")
	RADIUSRequests    = NewCounter("halo_radius_requests_total", "RADIUS requests by outcome: accept, reject, challenge, accounting, or dropped without a reply for an unknown client, a missing or wrong Message-Authenticator, a malformed packet, the rate limit or a server error.", "outcome")
	HTTPDuration      = NewHistogram("halo_http_request_duration_seconds", "HTTP request duration by route pattern.", []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}, "route")
)

var (
	mu       sync.Mutex
	families []*family
)

func init() {
	RefreshRefused.Add(0)
	for _, v := range []string{"success", "failure"} {
		SCIMRuns.Add(0, v)
		LDAPRuns.Add(0, v)
	}
	for _, v := range []string{"delivered", "pending", "failed"} {
		WebhookDeliveries.Add(0, v)
	}
	for _, v := range []string{"sent", "failed", "rejected", "dropped"} {
		LogExport.Add(0, v)
	}
	for _, v := range []string{"account", "address"} {
		Lockouts.Add(0, v)
	}
	for _, v := range []string{"api", "scim", "bulk", "radius"} {
		RateLimited.Add(0, v)
	}
	for _, v := range []string{"accept", "reject", "challenge", "accounting", "dropped"} {
		RADIUSRequests.Add(0, v)
	}
}

type family struct {
	name, help, kind string
	labels           []string
	buckets          []float64
	series           map[string]*series
}

type series struct {
	values []string
	sum    float64
	count  uint64
	counts []uint64
}

type Counter struct{ f *family }

type Histogram struct{ f *family }

type Gauge struct {
	Name, Help string
	Value      float64
}

func register(f *family) *family {
	mu.Lock()
	defer mu.Unlock()
	f.series = map[string]*series{}
	families = append(families, f)
	return f
}

func NewCounter(name, help string, labels ...string) Counter {
	return Counter{register(&family{name: name, help: help, kind: "counter", labels: labels})}
}

func NewHistogram(name, help string, buckets []float64, labels ...string) Histogram {
	return Histogram{register(&family{name: name, help: help, kind: "histogram", labels: labels, buckets: buckets})}
}

func (c Counter) Inc(values ...string) { c.f.observe(1, values) }

func (c Counter) Add(v float64, values ...string) { c.f.observe(v, values) }

func (c Counter) Value(values ...string) float64 {
	mu.Lock()
	defer mu.Unlock()
	if s := c.f.series[strings.Join(values, "\xff")]; s != nil {
		return s.sum
	}
	return 0
}

func (h Histogram) Observe(v float64, values ...string) { h.f.observe(v, values) }

func (f *family) observe(v float64, values []string) {
	key := strings.Join(values, "\xff")
	mu.Lock()
	defer mu.Unlock()
	s := f.series[key]
	if s == nil {
		s = &series{values: slices.Clone(values), counts: make([]uint64, len(f.buckets))}
		f.series[key] = s
	}
	s.sum += v
	s.count++
	for i, b := range f.buckets {
		if v <= b {
			s.counts[i]++
		}
	}
}

var escape = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)

func labels(names, values []string) string {
	if len(names) == 0 {
		return ""
	}
	pairs := make([]string, len(names))
	for i, name := range names {
		pairs[i] = name + `="` + escape.Replace(values[i]) + `"`
	}
	return "{" + strings.Join(pairs, ",") + "}"
}

func number(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func Write(w io.Writer, gauges []Gauge) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	gauges = append(gauges,
		Gauge{"go_goroutines", "Goroutines that currently exist.", float64(runtime.NumGoroutine())},
		Gauge{"go_memstats_heap_alloc_bytes", "Bytes of allocated heap objects.", float64(mem.HeapAlloc)},
		Gauge{"go_memstats_sys_bytes", "Bytes of memory obtained from the operating system.", float64(mem.Sys)},
		Gauge{"go_gc_cycles_completed", "Garbage collection cycles completed since the process started.", float64(mem.NumGC)},
	)
	for _, g := range gauges {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %s\n", g.Name, g.Help, g.Name, g.Name, number(g.Value))
	}
	var buf bytes.Buffer
	defer buf.WriteTo(w)
	w = &buf
	mu.Lock()
	defer mu.Unlock()
	for _, f := range families {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", f.name, f.help, f.name, f.kind)
		for _, key := range slices.Sorted(maps.Keys(f.series)) {
			s := f.series[key]
			if f.kind == "counter" {
				fmt.Fprintf(w, "%s%s %s\n", f.name, labels(f.labels, s.values), number(s.sum))
				continue
			}
			bucketLabels := append(slices.Clone(f.labels), "le")
			for i, b := range f.buckets {
				fmt.Fprintf(w, "%s_bucket%s %d\n", f.name, labels(bucketLabels, append(slices.Clone(s.values), number(b))), s.counts[i])
			}
			fmt.Fprintf(w, "%s_bucket%s %d\n", f.name, labels(bucketLabels, append(slices.Clone(s.values), "+Inf")), s.count)
			fmt.Fprintf(w, "%s_sum%s %s\n", f.name, labels(f.labels, s.values), number(s.sum))
			fmt.Fprintf(w, "%s_count%s %d\n", f.name, labels(f.labels, s.values), s.count)
		}
	}
}

func Handler(token string, gauges func() []Gauge) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" {
			given, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || !secret.Equal([]byte(given), []byte(token)) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="halo metrics"`)
				http.Error(w, "Send the metrics token from HALO_METRICS_TOKEN as a bearer token.", http.StatusUnauthorized)
				return
			}
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		Write(w, gauges())
	})
}
