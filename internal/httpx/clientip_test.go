package httpx

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	TrustProxies([]string{"127.0.0.1/32", "::1/128", "10.0.0.0/8"})
	t.Cleanup(func() { TrustProxies(nil) })
	for _, c := range []struct {
		peer, forwarded, want string
	}{
		{"203.0.113.7:4100", "198.51.100.1", "203.0.113.7"},
		{"192.168.1.20:4100", "198.51.100.1", "192.168.1.20"},
		{"127.0.0.1:4100", "", "127.0.0.1"},
		{"127.0.0.1:4100", "198.51.100.1", "198.51.100.1"},
		{"127.0.0.1:4100", "6.6.6.6, 198.51.100.1, 10.0.0.5", "198.51.100.1"},
		{"[::1]:4100", "2001:db8::9", "2001:db8::9"},
		{"127.0.0.1:4100", "::1", "::1"},
		{"127.0.0.1:4100", "not-an-ip, 10.0.0.5", "10.0.0.5"},
		{"127.0.0.1:4100", "198.51.100.1, garbage", "127.0.0.1"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.peer
		if c.forwarded != "" {
			r.Header.Set("X-Forwarded-For", c.forwarded)
		}
		if got := ClientIP(r); got != c.want {
			t.Errorf("peer %s, X-Forwarded-For %q: got %s, want %s", c.peer, c.forwarded, got, c.want)
		}
	}

	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:4100"
	r.Header.Add("X-Forwarded-For", "6.6.6.6")
	r.Header.Add("X-Forwarded-For", "198.51.100.1")
	if got := ClientIP(r); got != "198.51.100.1" {
		t.Errorf("repeated headers: got %s", got)
	}

	TrustProxies(nil)
	r.Header.Set("X-Forwarded-For", "198.51.100.1")
	if got := ClientIP(r); got != "127.0.0.1" {
		t.Errorf("no trusted proxies must ignore X-Forwarded-For: got %s", got)
	}
}
