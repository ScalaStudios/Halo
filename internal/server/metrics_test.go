package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"

	"halo/internal/config"
	"halo/internal/metrics"
	"halo/internal/store"
)

func scrape(h http.Handler, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func withToken(c *config.Config) { c.MetricsToken = "scrape-secret" }

func TestMetricsAreOffByDefault(t *testing.T) {
	e := setup(t)
	if rec := scrape(e.h, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("metrics without configuration: %d", rec.Code)
	}
	if e.app.Metrics != nil {
		t.Fatal("a metrics listener exists without HALO_METRICS_LISTEN")
	}
}

func TestMetricsNeedTheToken(t *testing.T) {
	e := setup(t, withToken)
	for _, authorization := range []string{"", "Bearer wrong", "scrape-secret", "Basic scrape-secret"} {
		if rec := scrape(e.h, authorization); rec.Code != http.StatusUnauthorized {
			t.Fatalf("authorization %q: %d", authorization, rec.Code)
		}
	}
	rec := scrape(e.h, "Bearer scrape-secret")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain; version=0.0.4") {
		t.Fatalf("with the token: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	for _, name := range []string{"halo_db_pool_total_connections ", "go_goroutines ", "# TYPE halo_http_request_duration_seconds histogram"} {
		if !strings.Contains(rec.Body.String(), name) {
			t.Fatalf("scrape has no %q:\n%s", name, rec.Body)
		}
	}
}

func TestMetricsOnTheirOwnListener(t *testing.T) {
	e := setup(t, func(c *config.Config) { c.MetricsListen = ":9090" })
	if rec := scrape(e.h, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("metrics on the main listener: %d", rec.Code)
	}
	if e.app.Metrics == nil {
		t.Fatal("no metrics handler for the separate listener")
	}
	if rec := scrape(e.app.Metrics, ""); rec.Code != http.StatusOK {
		t.Fatalf("metrics listener: %d", rec.Code)
	}
}

func TestMetricsCountWithoutPersonalLabels(t *testing.T) {
	e := setup(t, withToken)
	u, session, _ := e.person("ada@example.com", "passkey", false, "global_admin")

	before := metrics.SignIns.Value("totp", "failure", "high")
	if err := e.st.RecordSignIn(context.Background(), store.SignInEvent{UserID: &u.ID, Email: u.Email, Result: "failure", Method: "totp", Risk: "high", IP: "203.0.113.7"}); err != nil {
		t.Fatal(err)
	}
	if got := metrics.SignIns.Value("totp", "failure", "high"); got != before+1 {
		t.Fatalf("sign-in counter went from %v to %v", before, got)
	}
	before = metrics.SignIns.Value("token", "success", "none")
	if err := e.st.RecordSignIn(context.Background(), store.SignInEvent{UserID: &u.ID, Email: u.Email, Result: "success", Method: "token"}); err != nil {
		t.Fatal(err)
	}
	if got := metrics.SignIns.Value("token", "success", "none"); got != before {
		t.Fatalf("token hook rows counted as sign-ins: %v to %v", before, got)
	}

	route := "GET /api/v1/users/{id}"
	before = metrics.HTTPRequests.Value(route, "200")
	e.expect(session, http.MethodGet, "/api/v1/users/"+u.ID, nil, http.StatusOK, "")
	if got := metrics.HTTPRequests.Value(route, "200"); got != before+1 {
		t.Fatalf("request counter for %s went from %v to %v", route, before, got)
	}
	before = metrics.HTTPRequests.Value("unmatched", "404")
	e.call("", http.MethodGet, "/no/such/"+u.ID, nil)
	if got := metrics.HTTPRequests.Value("unmatched", "404"); got != before+1 {
		t.Fatalf("unmatched counter went from %v to %v", before, got)
	}

	body := scrape(e.h, "Bearer scrape-secret").Body.String()
	for _, personal := range []string{u.ID, u.Email, "203.0.113.7", session} {
		if strings.Contains(body, personal) {
			t.Fatalf("scrape contains %q", personal)
		}
	}
	allowed := []string{"method", "kind", "result", "risk", "scope", "grant", "resource", "action", "outcome", "route", "status", "le"}
	for _, m := range regexp.MustCompile(`[{,]([a-zA-Z_]+)="`).FindAllStringSubmatch(body, -1) {
		if !slices.Contains(allowed, m[1]) {
			t.Fatalf("unexpected label %q in:\n%s", m[1], body)
		}
	}
}
