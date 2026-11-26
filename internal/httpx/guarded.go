package httpx

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"syscall"
	"time"
)

var (
	sharedSpace = netip.MustParsePrefix("100.64.0.0/10")
	errPrivate  = errors.New("the address is private, loopback or link-local, and Halo only calls public addresses here")
	guarded     = GuardedClient()
	development = DialerClient(&net.Dialer{Timeout: 10 * time.Second})
	allowed     []netip.Prefix
)

func AllowOutbound(prefixes []netip.Prefix) {
	allowed = prefixes
}

func PublicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if slices.ContainsFunc(allowed, func(p netip.Prefix) bool { return p.Contains(ip) }) {
		return true
	}
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !sharedSpace.Contains(ip)
}

func GuardedDialer() *net.Dialer {
	return &net.Dialer{Timeout: 10 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if ip, parseErr := netip.ParseAddr(host); err != nil || parseErr != nil || !PublicAddress(ip) {
			return errPrivate
		}
		return nil
	}}
}

func GuardedClient() *http.Client {
	return DialerClient(GuardedDialer())
}

func OutboundClient(dev bool) *http.Client {
	if dev {
		return development
	}
	return guarded
}

func DialerClient(dialer *net.Dialer) *http.Client {
	noRedirect := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &http.Client{Timeout: 10 * time.Second, CheckRedirect: noRedirect, Transport: &http.Transport{DialContext: dialer.DialContext, TLSHandshakeTimeout: 10 * time.Second, ForceAttemptHTTP2: true}}
}
