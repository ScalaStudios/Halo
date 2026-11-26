package httpx

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOutboundClient(t *testing.T) {
	var redirected atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer srv.Close()
	if _, err := OutboundClient(false).Get(srv.URL); err == nil || !strings.Contains(err.Error(), "private, loopback or link-local") {
		t.Fatalf("loopback outside development: %v", err)
	}
	res, err := OutboundClient(true).Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusFound || redirected.Load() != 0 {
		t.Fatalf("redirect in development: %d, target saw %d requests", res.StatusCode, redirected.Load())
	}
}

func TestAllowedOutboundRanges(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	AllowOutbound([]netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
	defer AllowOutbound(nil)
	res, err := OutboundClient(false).Get(srv.URL)
	if err != nil {
		t.Fatalf("an allowed range must be reachable: %v", err)
	}
	res.Body.Close()
	if PublicAddress(netip.MustParseAddr("10.0.0.1")) {
		t.Fatal("ranges outside the list must stay refused")
	}
}
