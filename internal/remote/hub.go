package remote

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"

	"halo/internal/httpx"
	"halo/internal/secret"
	"halo/internal/store"
)

const (
	heartbeat   = 25 * time.Second
	dialTimeout = 15 * time.Second
)

var (
	errOffline       = errors.New("the outpost is not connected to Halo")
	errReenrolled    = errors.New("The outpost that reaches this host was enrolled again with a new key.")
	errDeleted       = errors.New("The outpost that reaches this host was deleted.")
	errStaleKey      = errors.New("The outpost that reaches this host was deleted or enrolled again with a new key.")
	errBadKey        = httpx.Fail(http.StatusUnauthorized, "ERR_INVALID_KEY", "This outpost key is not valid. The outpost may have been deleted or enrolled again. Enroll it with a new token from the Remote access page of the Halo console.")
	errInvalidToken  = httpx.Fail(http.StatusUnauthorized, "ERR_INVALID_TOKEN", "This enrollment token is not valid. A token works once, for 24 hours. Create a new one for the outpost on the Remote access page of the Halo console.")
	errUnknownStream = httpx.Fail(http.StatusNotFound, "ERR_NOT_FOUND", "Halo is not waiting for this connection. It may have timed out.")
)

type Message struct {
	Type    string `json:"type"`
	Stream  string `json:"stream,omitempty"`
	Address string `json:"address,omitempty"`
	Message string `json:"message,omitempty"`
}

type link struct {
	conn      *websocket.Conn
	done      chan struct{}
	outpostID string
	key       string
}

type stream struct {
	conn *websocket.Conn
	done chan struct{}
	once sync.Once
}

func (s *stream) Close() error {
	err := s.conn.Close()
	s.once.Do(func() { close(s.done) })
	return err
}

type claim struct {
	outpostID string
	ready     chan *stream
	failed    chan string
	gone      chan struct{}
}

type live struct {
	outpostID string
	end       context.CancelCauseFunc
}

type Hub struct {
	st       *store.Store
	mu       sync.Mutex
	outposts map[string]*link
	claims   map[string]*claim
	sessions map[string]*live
}

func NewHub(st *store.Store) *Hub {
	return &Hub{st: st, outposts: map[string]*link{}, claims: map[string]*claim{}, sessions: map[string]*live{}}
}

func (h *Hub) Connected(outpostID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.outposts[outpostID] != nil
}

func (h *Hub) Drop(outpostID string, cause error) {
	h.mu.Lock()
	l := h.outposts[outpostID]
	delete(h.outposts, outpostID)
	var ends []context.CancelCauseFunc
	for _, s := range h.sessions {
		if s.outpostID == outpostID {
			ends = append(ends, s.end)
		}
	}
	h.mu.Unlock()
	if l != nil {
		l.conn.Close()
	}
	for _, end := range ends {
		end(cause)
	}
}

func (h *Hub) Terminate(remoteID string, cause error) bool {
	h.mu.Lock()
	s := h.sessions[remoteID]
	h.mu.Unlock()
	if s != nil {
		s.end(cause)
	}
	return s != nil
}

func (h *Hub) track(remoteID, outpostID string, end context.CancelCauseFunc) func() {
	h.mu.Lock()
	h.sessions[remoteID] = &live{outpostID: outpostID, end: end}
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		delete(h.sessions, remoteID)
		h.mu.Unlock()
	}
}

func (h *Hub) Dial(ctx context.Context, outpostID, address string) (*stream, error) {
	token := secret.Token(32)
	c := &claim{outpostID: outpostID, ready: make(chan *stream), failed: make(chan string, 1), gone: make(chan struct{})}
	h.mu.Lock()
	l := h.outposts[outpostID]
	if l != nil {
		h.claims[token] = c
	}
	h.mu.Unlock()
	if l == nil {
		return nil, errOffline
	}
	defer func() {
		h.mu.Lock()
		delete(h.claims, token)
		h.mu.Unlock()
		close(c.gone)
	}()
	if err := send(l.conn, Message{Type: "dial", Stream: token, Address: address}); err != nil {
		return nil, errOffline
	}
	timer := time.NewTimer(dialTimeout)
	defer timer.Stop()
	select {
	case s := <-c.ready:
		return s, nil
	case reason := <-c.failed:
		return nil, errors.New(reason)
	case <-l.done:
		return nil, errOffline
	case <-timer.C:
		return nil, errors.New("the outpost did not answer within 15 seconds")
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}

func send(conn *websocket.Conn, m Message) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return websocket.Message.Send(conn, string(raw))
}

func bearer(r *http.Request) string {
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return token
}

func agent(r *http.Request) string {
	return strings.TrimSpace(strings.TrimPrefix(r.UserAgent(), "halo-outpost/"))
}

func (h *Hub) outpost(r *http.Request) (store.Outpost, error) {
	key := bearer(r)
	if !strings.HasPrefix(key, "hlo_") {
		return store.Outpost{}, errBadKey
	}
	o, err := h.st.OutpostByKey(r.Context(), key)
	if errors.Is(err, store.ErrNotFound) {
		return o, errBadKey
	}
	return o, err
}

func acceptAgent(*websocket.Config, *http.Request) error {
	return nil
}

func (h *Hub) enroll(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Token string `json:"token"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	var o store.Outpost
	var key string
	err := h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		if o, key, err = tx.EnrollOutpost(r.Context(), in.Token, httpx.ClientIP(r), agent(r)); errors.Is(err, store.ErrNotFound) {
			return errInvalidToken
		} else if err != nil {
			return err
		}
		return tx.RecordAudit(r.Context(), store.AuditEvent{Action: "outpost.enroll", Summary: "Enrolled the outpost " + o.Name + "; any key it had before stopped working", TargetType: "outpost", TargetID: o.ID, TargetLabel: o.Name, IP: httpx.ClientIP(r)})
	})
	if err != nil {
		return err
	}
	h.Drop(o.ID, errReenrolled)
	return httpx.JSON(w, http.StatusCreated, map[string]any{"outpost": o, "key": key})
}

func (h *Hub) connect(w http.ResponseWriter, r *http.Request) error {
	o, err := h.outpost(r)
	if err != nil {
		return err
	}
	ip, version := httpx.ClientIP(r), agent(r)
	websocket.Server{Handshake: acceptAgent, Handler: func(conn *websocket.Conn) {
		ctx := context.WithoutCancel(r.Context())
		l := &link{conn: conn, done: make(chan struct{}), outpostID: o.ID, key: bearer(r)}
		h.mu.Lock()
		previous := h.outposts[o.ID]
		h.outposts[o.ID] = l
		h.mu.Unlock()
		if previous != nil {
			previous.conn.Close()
		}
		if err := h.st.TouchOutpost(ctx, o.ID, ip, version); err != nil {
			slog.ErrorContext(ctx, "outpost connect", "outpost", o.ID, "error", err)
		}
		slog.InfoContext(ctx, "outpost connected", "outpost", o.ID, "name", o.Name, "ip", ip)
		go h.ping(l)
		h.listen(l)
		close(l.done)
		h.mu.Lock()
		if h.outposts[o.ID] == l {
			delete(h.outposts, o.ID)
		}
		h.mu.Unlock()
		if err := h.st.TouchOutpost(ctx, o.ID, ip, version); err != nil {
			slog.ErrorContext(ctx, "outpost disconnect", "outpost", o.ID, "error", err)
		}
		slog.InfoContext(ctx, "outpost disconnected", "outpost", o.ID, "name", o.Name)
	}}.ServeHTTP(w, r)
	return nil
}

func (h *Hub) ping(l *link) {
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-l.done:
			return
		case <-ticker.C:
			if send(l.conn, Message{Type: "ping"}) != nil {
				l.conn.Close()
				return
			}
			if o, err := h.st.OutpostByKey(context.Background(), l.key); errors.Is(err, store.ErrNotFound) || err == nil && o.ID != l.outpostID {
				h.mu.Lock()
				current := h.outposts[l.outpostID] == l
				h.mu.Unlock()
				if current {
					h.Drop(l.outpostID, errStaleKey)
				}
				l.conn.Close()
				return
			}
		}
	}
}

func (h *Hub) listen(l *link) {
	for {
		l.conn.SetReadDeadline(time.Now().Add(3 * heartbeat))
		var raw string
		if err := websocket.Message.Receive(l.conn, &raw); err != nil {
			l.conn.Close()
			return
		}
		var m Message
		if json.Unmarshal([]byte(raw), &m) != nil || m.Type != "failed" {
			continue
		}
		h.mu.Lock()
		c := h.claims[m.Stream]
		h.mu.Unlock()
		if c != nil {
			select {
			case c.failed <- m.Message:
			default:
			}
		}
	}
}

func (h *Hub) attach(w http.ResponseWriter, r *http.Request) error {
	o, err := h.outpost(r)
	if err != nil {
		return err
	}
	h.mu.Lock()
	c := h.claims[r.PathValue("stream")]
	if c != nil && c.outpostID == o.ID {
		delete(h.claims, r.PathValue("stream"))
	} else {
		c = nil
	}
	h.mu.Unlock()
	if c == nil {
		return errUnknownStream
	}
	websocket.Server{Handshake: acceptAgent, Handler: func(conn *websocket.Conn) {
		conn.PayloadType = websocket.BinaryFrame
		s := &stream{conn: conn, done: make(chan struct{})}
		select {
		case c.ready <- s:
			<-s.done
		case <-c.gone:
			conn.Close()
		}
	}}.ServeHTTP(w, r)
	return nil
}
