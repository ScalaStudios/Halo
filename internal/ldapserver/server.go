package ldapserver

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	ber "github.com/go-asn1-ber/asn1-ber"
	"github.com/go-ldap/ldap/v3"

	"halo/internal/auth"
	"halo/internal/config"
	"halo/internal/features"
	"halo/internal/httpx"
	"halo/internal/metrics"
	"halo/internal/store"
)

const (
	maxMessage     = 64 << 10
	maxConnections = 256
	idleTimeout    = 5 * time.Minute
	bindTimeout    = 30 * time.Second
	writeTimeout   = 30 * time.Second
	requestTimeout = 30 * time.Second
	rateLimit      = 1200
	startTLS       = "1.3.6.1.4.1.1466.20037"
	passwordModify = "1.3.6.1.4.1.4203.1.11.1"
)

type Server struct {
	deps     httpx.Deps
	tls      *tls.Config
	slots    chan struct{}
	mu       sync.Mutex
	listener net.Listener
}

func New(d httpx.Deps) (*Server, error) {
	l := d.Config.LDAP
	if l.Listen == "" {
		return nil, nil
	}
	s := &Server{deps: d, slots: make(chan struct{}, maxConnections)}
	if l.CertFile != "" || l.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(l.CertFile, l.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("HALO_LDAP_CERT_FILE and HALO_LDAP_KEY_FILE: %w", err)
		}
		s.tls = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	}
	return s, nil
}

type Connection struct {
	URL      string `json:"url"`
	BaseDN   string `json:"baseDn"`
	BindDN   string `json:"bindDn"`
	PeopleDN string `json:"peopleDn"`
	GroupsDN string `json:"groupsDn"`
}

func Describe(cfg config.Config, appID string) Connection {
	base := baseDN(cfg, appID)
	c := Connection{BaseDN: base.String(), BindDN: child(base, "cn", "bind").String(), PeopleDN: child(base, "ou", "people").String(), GroupsDN: child(base, "ou", "groups").String()}
	if cfg.LDAP.Listen != "" {
		scheme := "ldap://"
		if cfg.LDAP.TLS == "ldaps" {
			scheme = "ldaps://"
		}
		_, port, _ := net.SplitHostPort(cfg.LDAP.Listen)
		c.URL = scheme + net.JoinHostPort(cfg.Hostname(), port)
	}
	return c
}

func suffix(cfg config.Config) []*ldap.RelativeDN {
	var rdns []*ldap.RelativeDN
	for _, label := range strings.Split(cfg.Hostname(), ".") {
		rdns = append(rdns, rdn("dc", label))
	}
	return rdns
}

func baseDN(cfg config.Config, appID string) *ldap.DN {
	return &ldap.DN{RDNs: append([]*ldap.RelativeDN{rdn("ou", appID)}, suffix(cfg)...)}
}

func rdn(kind, value string) *ldap.RelativeDN {
	return &ldap.RelativeDN{Attributes: []*ldap.AttributeTypeAndValue{{Type: kind, Value: value}}}
}

func child(parent *ldap.DN, kind, value string) *ldap.DN {
	return &ldap.DN{RDNs: append([]*ldap.RelativeDN{rdn(kind, value)}, parent.RDNs...)}
}

func (s *Server) ListenAndServe() error {
	l, err := net.Listen("tcp", s.deps.Config.LDAP.Listen)
	if err != nil {
		return err
	}
	slog.Info("ldap listening", "addr", s.deps.Config.LDAP.Listen, "tls", s.deps.Config.LDAP.TLS)
	return s.Serve(l)
}

func (s *Server) Serve(l net.Listener) error {
	if s.deps.Config.LDAP.TLS == "ldaps" {
		l = tls.NewListener(l, s.tls)
	}
	s.mu.Lock()
	s.listener = l
	s.mu.Unlock()
	for {
		c, err := l.Accept()
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case s.slots <- struct{}{}:
			go s.serve(c)
		default:
			c.Close()
		}
	}
}

func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}

type session struct {
	*Server
	conn   net.Conn
	secure bool
	peer   string
	appID  string
	userID string
}

func (s *Server) serve(c net.Conn) {
	sess := &session{Server: s, conn: c, secure: s.deps.Config.LDAP.TLS == "ldaps"}
	sess.peer, _, _ = net.SplitHostPort(c.RemoteAddr().String())
	defer func() {
		if v := recover(); v != nil {
			slog.Error("ldap connection failed", "peer", sess.peer, "panic", v)
		}
		sess.conn.Close()
		<-s.slots
	}()
	if sess.off() {
		return
	}
	for {
		timeout := idleTimeout
		if sess.appID == "" {
			timeout = bindTimeout
		}
		sess.conn.SetReadDeadline(time.Now().Add(timeout))
		packet, err := ber.ReadPacket(&io.LimitedReader{R: sess.conn, N: maxMessage})
		if err != nil || !sess.handle(packet) {
			return
		}
	}
}

func (c *session) handle(p *ber.Packet) bool {
	if len(p.Children) < 2 || p.Children[0].Tag != ber.TagInteger || p.Children[1].ClassType != ber.ClassApplication {
		return false
	}
	id, _ := p.Children[0].Value.(int64)
	if id < 1 {
		return false
	}
	op := p.Children[1]
	var controls []*ber.Packet
	if len(p.Children) > 2 && p.Children[2].ClassType == ber.ClassContext && p.Children[2].Tag == 0 {
		controls = p.Children[2].Children
	}
	if c.off() {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	switch op.Tag {
	case ldap.ApplicationUnbindRequest:
		return false
	case ldap.ApplicationAbandonRequest:
		return true
	case ldap.ApplicationModifyRequest, ldap.ApplicationAddRequest, ldap.ApplicationDelRequest, ldap.ApplicationModifyDNRequest, ldap.ApplicationCompareRequest:
		return c.reply(id, result(op.Tag+1, ldap.LDAPResultUnwillingToPerform, "Halo's LDAP directory is read-only. Change people and groups in the Halo console."))
	case ldap.ApplicationBindRequest, ldap.ApplicationSearchRequest, ldap.ApplicationExtendedRequest:
	default:
		return false
	}
	response := map[ber.Tag]ber.Tag{ldap.ApplicationBindRequest: ldap.ApplicationBindResponse, ldap.ApplicationSearchRequest: ldap.ApplicationSearchResultDone, ldap.ApplicationExtendedRequest: ldap.ApplicationExtendedResponse}[op.Tag]
	if critical(controls, op.Tag) {
		return c.reply(id, result(response, ldap.LDAPResultUnavailableCriticalExtension, "Halo doesn't support a control marked critical in this request."))
	}
	if c.throttled(ctx) {
		return c.reply(id, result(response, ldap.LDAPResultBusy, fmt.Sprintf("This address sent more than %d LDAP requests this minute. Wait for the next minute, then try again.", rateLimit)))
	}
	switch op.Tag {
	case ldap.ApplicationBindRequest:
		code, message := c.bind(ctx, op)
		return c.reply(id, result(ldap.ApplicationBindResponse, code, message))
	case ldap.ApplicationSearchRequest:
		return c.search(ctx, id, op, controls)
	}
	return c.extended(ctx, id, op)
}

func (c *session) off() bool {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	off, err := features.Off(ctx, c.deps.Store, "ldap")
	return off || err != nil
}

func critical(controls []*ber.Packet, op ber.Tag) bool {
	for _, c := range controls {
		if len(c.Children) < 2 || c.Children[1].Tag != ber.TagBoolean {
			continue
		}
		if on, _ := c.Children[1].Value.(bool); on && !(op == ldap.ApplicationSearchRequest && c.Children[0].Data.String() == ldap.ControlTypePaging) {
			return true
		}
	}
	return false
}

func (c *session) throttled(ctx context.Context) bool {
	n, _, err := c.deps.Store.CountRequest(ctx, c.peer, "ldap", rateLimit, time.Minute)
	if err != nil || n <= rateLimit {
		return false
	}
	metrics.RateLimited.Inc("ldap")
	return true
}

func (c *session) bind(ctx context.Context, op *ber.Packet) (uint16, string) {
	c.appID, c.userID = "", ""
	if len(op.Children) != 3 || op.Children[0].Value != int64(3) {
		return ldap.LDAPResultProtocolError, "Halo only speaks LDAPv3."
	}
	if !c.secure && c.deps.Config.LDAP.TLS != "none" {
		return ldap.LDAPResultConfidentialityRequired, "Run StartTLS before you bind."
	}
	credentials := op.Children[2]
	if credentials.ClassType != ber.ClassContext || credentials.Tag != 0 || credentials.TagType != ber.TypePrimitive {
		return ldap.LDAPResultAuthMethodNotSupported, "Halo only accepts simple binds with a password."
	}
	name, password := op.Children[1].Data.String(), credentials.Data.String()
	switch {
	case name == "" && password == "":
		metrics.LDAPBinds.Inc("other", "failure")
		return ldap.LDAPResultInappropriateAuthentication, "Halo doesn't accept anonymous binds. Bind with the application's bind DN and password."
	case password == "":
		metrics.LDAPBinds.Inc("other", "failure")
		return ldap.LDAPResultUnwillingToPerform, "Halo doesn't accept binds without a password."
	}
	appID, path := c.locate(name)
	switch {
	case len(path) == 1 && is(path[0], "cn", "bind"):
		return c.applicationBind(ctx, appID, password)
	case len(path) == 2 && is(path[1], "ou", "people") && is(path[0], "uid", "") && clean(path[0].Attributes[0].Value):
		return c.personBind(ctx, appID, path[0].Attributes[0].Value, password)
	}
	metrics.LDAPBinds.Inc("other", "failure")
	return invalidCredentials()
}

func invalidCredentials() (uint16, string) {
	return ldap.LDAPResultInvalidCredentials, "The DN or password is wrong, or this account can't use this application."
}

func (c *session) applicationBind(ctx context.Context, appID, password string) (uint16, string) {
	app, err := c.deps.Store.LDAPApplication(ctx, appID)
	matched := false
	if err == nil && app.Status == "active" {
		matched, err = c.deps.Store.LDAPSecretMatches(ctx, appID, password)
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return failed(err)
	}
	if !matched {
		slog.Warn("ldap bind refused", "app", appID, "peer", c.peer)
		metrics.LDAPBinds.Inc("application", "failure")
		return invalidCredentials()
	}
	c.appID = appID
	metrics.LDAPBinds.Inc("application", "success")
	return ldap.LDAPResultSuccess, ""
}

func (c *session) personBind(ctx context.Context, appID, email, password string) (uint16, string) {
	app, err := c.deps.Store.LDAPApplication(ctx, appID)
	userID := ""
	if err == nil {
		userID, err = auth.PasswordBind(ctx, c.deps, app, email, password, c.peer)
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return failed(err)
	}
	if userID == "" {
		metrics.LDAPBinds.Inc("person", "failure")
		return invalidCredentials()
	}
	c.appID, c.userID = appID, userID
	metrics.LDAPBinds.Inc("person", "success")
	return ldap.LDAPResultSuccess, ""
}

func failed(err error) (uint16, string) {
	slog.Error("ldap request failed", "error", err)
	return ldap.LDAPResultOperationsError, "Halo couldn't answer this request. Try again; if it keeps failing, check the Halo server log."
}

func (c *session) locate(name string) (string, []*ldap.RelativeDN) {
	dn, err := ldap.ParseDN(name)
	tail := suffix(c.deps.Config)
	if err != nil || len(dn.RDNs) <= len(tail) {
		return "", nil
	}
	at := len(dn.RDNs) - len(tail) - 1
	for i, r := range tail {
		if !dn.RDNs[at+1+i].EqualFold(r) {
			return "", nil
		}
	}
	for _, r := range dn.RDNs[:at] {
		if len(r.Attributes) != 1 {
			return "", nil
		}
	}
	if !is(dn.RDNs[at], "ou", "") || !clean(dn.RDNs[at].Attributes[0].Value) {
		return "", nil
	}
	return dn.RDNs[at].Attributes[0].Value, dn.RDNs[:at]
}

func is(r *ldap.RelativeDN, kind, value string) bool {
	return len(r.Attributes) == 1 && strings.EqualFold(r.Attributes[0].Type, kind) && (value == "" || strings.EqualFold(r.Attributes[0].Value, value))
}

func clean(value string) bool {
	return len(value) <= 320 && utf8.ValidString(value) && !strings.ContainsFunc(value, unicode.IsControl)
}

func (c *session) extended(ctx context.Context, id int64, op *ber.Packet) bool {
	name := ""
	if len(op.Children) > 0 && op.Children[0].ClassType == ber.ClassContext && op.Children[0].Tag == 0 {
		name = op.Children[0].Data.String()
	}
	switch {
	case name == passwordModify:
		return c.reply(id, result(ldap.ApplicationExtendedResponse, ldap.LDAPResultUnwillingToPerform, "Halo's LDAP directory is read-only. Change your password in the Halo account portal."))
	case name != startTLS:
		return c.reply(id, result(ldap.ApplicationExtendedResponse, ldap.LDAPResultProtocolError, "Halo supports only the StartTLS extended operation."))
	case c.secure:
		return c.reply(id, result(ldap.ApplicationExtendedResponse, ldap.LDAPResultOperationsError, "TLS is already on for this connection."))
	case c.tls == nil:
		return c.reply(id, result(ldap.ApplicationExtendedResponse, ldap.LDAPResultProtocolError, "This server has no certificate, so it can't start TLS. Set HALO_LDAP_CERT_FILE and HALO_LDAP_KEY_FILE."))
	}
	response := result(ldap.ApplicationExtendedResponse, ldap.LDAPResultSuccess, "")
	response.AppendChild(ber.NewString(ber.ClassContext, ber.TypePrimitive, 10, startTLS, ""))
	if !c.reply(id, response) {
		return false
	}
	secured := tls.Server(c.conn, c.tls)
	c.conn.SetDeadline(time.Now().Add(writeTimeout))
	if err := secured.HandshakeContext(ctx); err != nil {
		return false
	}
	c.conn.SetDeadline(time.Time{})
	c.conn, c.secure, c.appID, c.userID = secured, true, "", ""
	return true
}

func result(tag ber.Tag, code uint16, message string) *ber.Packet {
	p := ber.Encode(ber.ClassApplication, ber.TypeConstructed, tag, nil, "")
	p.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagEnumerated, int64(code), ""))
	p.AppendChild(octets(""))
	p.AppendChild(octets(message))
	return p
}

func octets(value string) *ber.Packet {
	return ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, value, "")
}

func (c *session) reply(id int64, op *ber.Packet, controls ...*ber.Packet) bool {
	p := ber.NewSequence("")
	p.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagInteger, id, ""))
	p.AppendChild(op)
	if len(controls) > 0 {
		list := ber.Encode(ber.ClassContext, ber.TypeConstructed, 0, nil, "")
		for _, control := range controls {
			list.AppendChild(control)
		}
		p.AppendChild(list)
	}
	c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	_, err := c.conn.Write(p.Bytes())
	return err == nil
}
