package radius

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"log/slog"
	"math"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	layeh "layeh.com/radius"
	"layeh.com/radius/rfc2865"
	"layeh.com/radius/rfc2866"
	"layeh.com/radius/rfc2869"

	"halo/internal/auth"
	"halo/internal/features"
	"halo/internal/httpx"
	"halo/internal/metrics"
	"halo/internal/store"
)

const (
	workers       = 2
	clientRefresh = 10 * time.Second
	replayWindow  = 30 * time.Second
	rateLimit     = 600
)

type Server struct {
	st    *store.Store
	check *auth.RADIUS

	mu      sync.Mutex
	clients []store.RADIUSClient
	loaded  time.Time
	replies map[string]cached
	swept   time.Time
}

type cached struct {
	at    time.Time
	reply []byte
}

func New(d httpx.Deps) *Server {
	return &Server{st: d.Store, check: auth.NewRADIUS(d), replies: map[string]cached{}}
}

func (s *Server) Serve(ctx context.Context, conn *net.UDPConn) error {
	defer context.AfterFunc(ctx, func() { conn.Close() })()
	busy := make(chan struct{}, workers)
	for {
		b := make([]byte, layeh.MaxPacketLength)
		n, from, err := conn.ReadFromUDPAddrPort(b)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return err
		}
		busy <- struct{}{}
		go func() {
			defer func() { <-busy }()
			if reply := s.handle(ctx, from, b[:n]); reply != nil {
				if _, err := conn.WriteToUDPAddrPort(reply, from); err != nil {
					slog.WarnContext(ctx, "radius reply not sent", "client", from, "error", err)
				}
			}
		}()
	}
}

func (s *Server) handle(ctx context.Context, from netip.AddrPort, b []byte) []byte {
	if off, err := features.Off(ctx, s.st, "radius"); off || err != nil {
		if err != nil {
			slog.ErrorContext(ctx, "radius feature switch not read", "error", err)
		}
		metrics.RADIUSRequests.Inc("dropped")
		return nil
	}
	p, err := layeh.Parse(b, nil)
	if err != nil || p.Code != layeh.CodeAccessRequest && p.Code != layeh.CodeAccountingRequest {
		metrics.RADIUSRequests.Inc("dropped")
		return nil
	}
	b = b[:binary.BigEndian.Uint16(b[2:4])]
	nas := from.Addr().Unmap()
	appID, secret, err := s.client(ctx, nas, p.Code, b)
	if err != nil {
		slog.ErrorContext(ctx, "radius clients not loaded", "error", err)
	}
	if appID == "" {
		metrics.RADIUSRequests.Inc("dropped")
		return nil
	}
	p.Secret = secret.Value
	key := from.String() + string(b[1:2]) + string(b[4:20])
	if reply, fresh := s.remember(key); !fresh {
		return reply
	}
	reply, outcome := s.respond(ctx, appID, secret, nas, p)
	metrics.RADIUSRequests.Inc(outcome)
	s.keep(key, reply)
	return reply
}

func (s *Server) client(ctx context.Context, nas netip.Addr, code layeh.Code, b []byte) (string, store.RADIUSSecret, error) {
	clients, err := s.load(ctx)
	for _, c := range clients {
		if !slices.ContainsFunc(c.Addresses, func(p netip.Prefix) bool { return p.Contains(nas) }) {
			continue
		}
		for _, secret := range c.Secrets {
			if code == layeh.CodeAccessRequest && authentic(b, secret.Value) || code == layeh.CodeAccountingRequest && layeh.IsAuthenticRequest(b, secret.Value) {
				return c.AppID, secret, nil
			}
		}
	}
	return "", store.RADIUSSecret{}, err
}

func (s *Server) load(ctx context.Context) ([]store.RADIUSClient, error) {
	s.mu.Lock()
	clients, fresh := s.clients, time.Since(s.loaded) < clientRefresh
	s.mu.Unlock()
	if fresh {
		return clients, nil
	}
	clients, err := s.st.RADIUSClients(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.clients, s.loaded = clients, time.Now()
	s.mu.Unlock()
	return clients, nil
}

func (s *Server) remember(key string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if now.Sub(s.swept) > replayWindow {
		for k, c := range s.replies {
			if now.Sub(c.at) > replayWindow {
				delete(s.replies, k)
			}
		}
		s.swept = now
	}
	if c, ok := s.replies[key]; ok {
		return c.reply, false
	}
	s.replies[key] = cached{at: now}
	return nil, true
}

func (s *Server) keep(key string, reply []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if reply == nil {
		delete(s.replies, key)
		return
	}
	s.replies[key] = cached{at: time.Now(), reply: reply}
}

func (s *Server) respond(ctx context.Context, appID string, secret store.RADIUSSecret, nas netip.Addr, p *layeh.Packet) ([]byte, string) {
	if p.Code == layeh.CodeAccountingRequest {
		if err := s.account(ctx, appID, nas, p); err != nil {
			slog.ErrorContext(ctx, "radius accounting not recorded", "application", appID, "error", err)
			return nil, "dropped"
		}
		return encode(p.Response(layeh.CodeAccountingResponse)), "accounting"
	}
	n, _, err := s.st.CountRequest(ctx, appID, "radius", rateLimit, time.Minute)
	if err != nil || n > rateLimit {
		if err == nil {
			metrics.RateLimited.Inc("radius")
		}
		return nil, "dropped"
	}
	if err := s.st.TouchCredential(ctx, secret.CredentialID); err != nil {
		slog.WarnContext(ctx, "radius secret use not recorded", "application", appID, "error", err)
	}
	password, err := rfc2865.UserPassword_LookupString(p)
	if err != nil {
		slog.WarnContext(ctx, "radius request has no PAP password", "application", appID, "client", nas)
		return encode(p.Response(layeh.CodeAccessReject)), "reject"
	}
	result, err := s.check.SignIn(ctx, auth.RADIUSRequest{AppID: appID, Email: rfc2865.UserName_GetString(p), Password: password, State: string(rfc2865.State_Get(p)), IP: caller(p, nas), Device: device(nas)})
	if err != nil {
		slog.ErrorContext(ctx, "radius sign-in failed", "application", appID, "error", err)
		return nil, "dropped"
	}
	switch {
	case result.Accept:
		reply := p.Response(layeh.CodeAccessAccept)
		if seconds := result.SessionTimeout / time.Second; seconds > 0 {
			rfc2865.SessionTimeout_Set(reply, rfc2865.SessionTimeout(min(seconds, math.MaxUint32)))
		}
		return encode(reply), "accept"
	case result.Challenge != "":
		reply := p.Response(layeh.CodeAccessChallenge)
		rfc2865.State_SetString(reply, result.Challenge)
		rfc2865.ReplyMessage_SetString(reply, "Enter the code from your authenticator app.")
		return encode(reply), "challenge"
	}
	return encode(p.Response(layeh.CodeAccessReject)), "reject"
}

func (s *Server) account(ctx context.Context, appID string, nas netip.Addr, p *layeh.Packet) error {
	if rfc2866.AcctStatusType_Get(p) != rfc2866.AcctStatusType_Value_Stop {
		return nil
	}
	reason := "The RADIUS session ended after " + (time.Duration(rfc2866.AcctSessionTime_Get(p)) * time.Second).String()
	if cause, err := rfc2866.AcctTerminateCause_Lookup(p); err == nil {
		reason += " (" + cause.String() + ")"
	}
	e := store.SignInEvent{Result: "ended", Method: "password", Email: strings.TrimSpace(rfc2865.UserName_GetString(p)), AppID: &appID, IP: caller(p, nas), Device: device(nas), Reason: reason + "."}
	u, err := s.st.GetUserByEmail(ctx, e.Email)
	if err == nil {
		e.UserID, e.Email = &u.ID, u.Email
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	return s.st.RecordSignIn(ctx, e)
}

func authentic(b, secret []byte) bool {
	b = bytes.Clone(b)
	at := -1
	for i := 20; i+1 < len(b); i += int(b[i+1]) {
		if b[i] == byte(rfc2869.MessageAuthenticator_Type) {
			if at >= 0 || b[i+1] != 18 {
				return false
			}
			at = i + 2
		}
	}
	if at < 0 {
		return false
	}
	sent := bytes.Clone(b[at : at+16])
	clear(b[at : at+16])
	return hmac.Equal(sign(b, secret), sent)
}

func sign(b, secret []byte) []byte {
	mac := hmac.New(md5.New, secret)
	mac.Write(b)
	return mac.Sum(nil)
}

func encode(p *layeh.Packet) []byte {
	if p.Code != layeh.CodeAccountingResponse {
		p.Attributes = append(layeh.Attributes{{Type: rfc2869.MessageAuthenticator_Type, Attribute: make(layeh.Attribute, 16)}}, p.Attributes...)
		b, err := p.MarshalBinary()
		if err != nil {
			return nil
		}
		copy(p.Attributes[0].Attribute, sign(b, p.Secret))
	}
	b, err := p.Encode()
	if err != nil {
		return nil
	}
	return b
}

func caller(p *layeh.Packet, nas netip.Addr) string {
	if ip, err := netip.ParseAddr(strings.TrimSpace(rfc2865.CallingStationID_GetString(p))); err == nil && ip.Zone() == "" {
		return ip.Unmap().String()
	}
	return nas.String()
}

func device(nas netip.Addr) string {
	return "RADIUS · " + nas.String()
}
