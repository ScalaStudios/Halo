package remote

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"

	"halo/internal/httpx"
	"halo/internal/sshca"
	"halo/internal/store"
)

const (
	recordingLimit   = 8 << 20
	handshakeLimit   = 20 * time.Second
	certificateTTL   = 60
	terminationCheck = 2 * time.Second
)

var (
	errExited  = errors.New("The shell on the host exited.")
	errClosed  = errors.New("The browser closed the connection.")
	errVanish  = errors.New("The browser stopped answering.")
	errFailure = errors.New("Halo could not start the session.")
	errRevoked = errors.New("Your access to this host ended: the Halo session that opened it was signed out, or the person or application was suspended or unassigned.")
)

type frame struct {
	text bool
	data []byte
}

var frames = websocket.Codec{
	Marshal: func(any) ([]byte, byte, error) { return nil, 0, websocket.ErrNotSupported },
	Unmarshal: func(data []byte, payloadType byte, v any) error {
		f := v.(*frame)
		f.text, f.data = payloadType == websocket.TextFrame, data
		return nil
	},
}

type event struct {
	Type      string     `json:"type"`
	Message   string     `json:"message,omitempty"`
	Username  string     `json:"username,omitempty"`
	Address   string     `json:"address,omitempty"`
	EndsAt    *time.Time `json:"endsAt,omitempty"`
	Recording bool       `json:"recording,omitempty"`
}

func emit(ws *websocket.Conn, e event) error {
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return websocket.Message.Send(ws, string(raw))
}

func (h *handlers) origin(_ *websocket.Config, r *http.Request) error {
	if r.Header.Get("Origin") != h.cfg.PublicURL.Scheme+"://"+h.cfg.PublicURL.Host {
		slog.WarnContext(r.Context(), "remote session refused a cross-site connection", "origin", r.Header.Get("Origin"))
		return httpx.ErrCrossSite
	}
	return nil
}

func (h *handlers) terminal(w http.ResponseWriter, r *http.Request) error {
	u, signedIn := httpx.CurrentUser(r)
	sess, cookie := httpx.CurrentSession(r)
	if !signedIn || !cookie {
		return httpx.ErrUnauthenticated
	}
	if sess.ImpersonationID != "" {
		return httpx.ErrImpersonating
	}
	cols, rows := dimension(r.URL.Query().Get("cols"), 80), dimension(r.URL.Query().Get("rows"), 24)
	ip, remoteID := httpx.ClientIP(r), r.PathValue("id")
	websocket.Server{Handshake: h.origin, Handler: func(ws *websocket.Conn) {
		ws.MaxPayloadBytes = 1 << 20
		h.run(ws, u, sess, remoteID, ip, cols, rows)
	}}.ServeHTTP(w, r)
	return nil
}

func dimension(raw string, fallback int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 1000 {
		return fallback
	}
	return n
}

type bridge struct {
	ws       *websocket.Conn
	in, out  atomic.Int64
	recorder *recorder
}

func (b *bridge) Write(p []byte) (int, error) {
	b.out.Add(int64(len(p)))
	if b.recorder != nil {
		b.recorder.output(p)
	}
	if err := websocket.Message.Send(b.ws, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (h *handlers) run(ws *websocket.Conn, u store.User, sess store.Session, remoteID, ip string, cols, rows int) {
	ctx, end := context.WithCancelCause(context.Background())
	defer end(nil)
	defer ws.Close()
	rs, err := h.st.ActivateRemoteSession(ctx, remoteID, u.ID, sess.ID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			slog.ErrorContext(ctx, "remote session activation failed", "session", remoteID, "error", err)
		}
		emit(ws, event{Type: "end", Message: "This connection expired or was already used. Open the application again from your account portal."})
		return
	}
	defer h.hub.track(rs.ID, deref(rs.OutpostID), end)()
	b := &bridge{ws: ws}
	if rs.Record {
		b.recorder = newRecorder(cols, rows, rs.AppName+" · "+rs.Email)
	}
	cause := h.serve(ctx, end, rs, u, ip, b, cols, rows)
	message := cause.Error()
	var recording []byte
	if b.recorder != nil {
		var full bool
		if recording, full = b.recorder.close(); full {
			message += " Recording stopped after 8 MiB of output."
		}
	}
	if err := h.st.EndRemoteSession(context.WithoutCancel(ctx), rs.ID, message, b.in.Load(), b.out.Load(), recording); err != nil {
		slog.ErrorContext(ctx, "remote session end failed", "session", rs.ID, "error", err)
	}
	emit(ws, event{Type: "end", Message: message})
}

func (h *handlers) serve(ctx context.Context, end context.CancelCauseFunc, rs store.RemoteSession, u store.User, ip string, b *bridge, cols, rows int) error {
	target, err := h.st.RemoteTarget(ctx, deref(rs.AppID))
	if err != nil {
		return failed(rs, err)
	}
	emit(b.ws, event{Type: "status", Message: "Reaching " + rs.Address + " through the outpost"})
	link, err := h.hub.Dial(ctx, deref(rs.OutpostID), rs.Address)
	if err == nil {
		defer link.Close()
	}
	switch {
	case ctx.Err() != nil:
		return context.Cause(ctx)
	case errors.Is(err, errOffline):
		return errors.New("The outpost that reaches " + rs.Address + " is not connected to Halo.")
	case err != nil:
		return fmt.Errorf("The outpost could not reach %s: %v.", rs.Address, err)
	}
	emit(b.ws, event{Type: "status", Message: "Signing in to " + rs.Address + " as " + rs.Username})
	client, err := h.handshake(ctx, link, rs, target, u, ip)
	if err != nil {
		return err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return failed(rs, err)
	}
	defer session.Close()
	session.Stdout, session.Stderr = b, b
	stdin, err := session.StdinPipe()
	if err == nil {
		err = session.RequestPty("xterm-256color", rows, cols, ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 38400, ssh.TTY_OP_OSPEED: 38400})
	}
	if err != nil {
		return failed(rs, err)
	}
	ends := time.Now().Add(time.Duration(rs.LimitSeconds) * time.Second)
	limit := time.AfterFunc(time.Until(ends), func() {
		end(errors.New("The session reached its time limit of " + limitText(rs.LimitSeconds) + "."))
	})
	defer limit.Stop()
	if err := emit(b.ws, event{Type: "connected", Username: rs.Username, Address: rs.Address, EndsAt: &ends, Recording: rs.Record}); err != nil {
		return errClosed
	}
	if err := session.Shell(); err != nil {
		return failed(rs, err)
	}
	go func() { end(b.input(stdin, session)) }()
	go func() {
		session.Wait()
		end(errExited)
	}()
	go h.watch(ctx, end, b.ws, rs)
	<-ctx.Done()
	return context.Cause(ctx)
}

func failed(rs store.RemoteSession, err error) error {
	slog.Error("remote session failed", "session", rs.ID, "address", rs.Address, "error", err)
	return errFailure
}

func (h *handlers) watch(ctx context.Context, end context.CancelCauseFunc, ws *websocket.Conn, rs store.RemoteSession) {
	ticker := time.NewTicker(terminationCheck)
	defer ticker.Stop()
	checked := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if name, err := h.st.RemoteSessionTerminator(ctx, rs.ID); err == nil {
			end(terminated(name))
			return
		}
		if time.Since(checked) < heartbeat {
			continue
		}
		checked = time.Now()
		if emit(ws, event{Type: "ping"}) != nil {
			return
		}
		if err := h.st.TouchRemoteSession(ctx, rs.ID); err != nil {
			slog.WarnContext(ctx, "remote session heartbeat failed", "session", rs.ID, "error", err)
		}
		if !h.allowed(ctx, rs) {
			end(errRevoked)
			return
		}
	}
}

func (h *handlers) allowed(ctx context.Context, rs store.RemoteSession) bool {
	active, err := h.st.SessionActive(ctx, rs.SessionID)
	if err != nil {
		return true
	}
	u, err := h.st.GetUser(ctx, deref(rs.UserID))
	if err != nil {
		return !errors.Is(err, store.ErrNotFound)
	}
	app, err := h.st.GetApplication(ctx, deref(rs.AppID))
	if err != nil {
		return !errors.Is(err, store.ErrNotFound)
	}
	return active && u.Status == "active" && app.Status == "active" && slices.Contains(u.AppIDs, app.ID)
}

func (b *bridge) input(stdin io.Writer, session *ssh.Session) error {
	for {
		b.ws.SetReadDeadline(time.Now().Add(3 * heartbeat))
		var f frame
		if err := frames.Receive(b.ws, &f); err != nil {
			var timeout net.Error
			if errors.As(err, &timeout) && timeout.Timeout() {
				return errVanish
			}
			return errClosed
		}
		if !f.text {
			b.in.Add(int64(len(f.data)))
			if _, err := stdin.Write(f.data); err != nil {
				return errExited
			}
			continue
		}
		var m struct {
			Type string `json:"type"`
			Cols int    `json:"cols"`
			Rows int    `json:"rows"`
		}
		if json.Unmarshal(f.data, &m) != nil || m.Type != "resize" || m.Cols < 1 || m.Rows < 1 || m.Cols > 1000 || m.Rows > 1000 {
			continue
		}
		if session.WindowChange(m.Rows, m.Cols) == nil && b.recorder != nil {
			b.recorder.resize(m.Cols, m.Rows)
		}
	}
}

func (h *handlers) handshake(ctx context.Context, link *stream, rs store.RemoteSession, t store.RemoteTarget, u store.User, ip string) (*ssh.Client, error) {
	auth, err := h.authMethods(ctx, t, u, ip)
	if err != nil {
		return nil, err
	}
	hostKeys, err := h.hostKeys(ctx, t)
	if err != nil {
		return nil, failed(rs, err)
	}
	var rejected *hostKeyError
	callback := func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := hostKeys(hostname, remote, key)
		if err != nil {
			rejected = &hostKeyError{key: key}
		}
		return err
	}
	stop := context.AfterFunc(ctx, func() { link.conn.Close() })
	defer stop()
	link.conn.SetDeadline(time.Now().Add(handshakeLimit))
	conn, chans, reqs, err := ssh.NewClientConn(link.conn, rs.Address, &ssh.ClientConfig{User: rs.Username, Auth: auth, HostKeyCallback: callback, ClientVersion: "SSH-2.0-Halo"})
	link.conn.SetDeadline(time.Time{})
	switch {
	case err == nil:
		return ssh.NewClient(conn, chans, reqs), nil
	case ctx.Err() != nil:
		return nil, context.Cause(ctx)
	case rejected != nil:
		return nil, errors.New("Halo does not trust the host key of " + t.Host + ", so it did not sign in. The host offered " + rejected.key.Type() + " " + ssh.FingerprintSHA256(rejected.key) +
			". If that is the right key, an administrator adds it to the application's remote access settings, or gives the host a Halo host certificate.")
	case strings.Contains(err.Error(), "unable to authenticate"):
		hint := "Check the stored " + map[string]string{"password": "password", "key": "private key"}[t.Auth] + "."
		if t.Auth == "certificate" {
			hint = "Check that the host trusts Halo's SSH certificate authority and that " + rs.Username + " is one of your SSH principals."
		}
		return nil, errors.New(t.Host + " refused the sign-in as " + rs.Username + ". " + hint)
	}
	return nil, errors.New("Halo could not open an SSH connection to " + rs.Address + ": " + strings.TrimPrefix(err.Error(), "ssh: ") + ".")
}

type hostKeyError struct {
	key ssh.PublicKey
}

func (e *hostKeyError) Error() string {
	return "untrusted host key " + ssh.FingerprintSHA256(e.key)
}

func (h *handlers) hostKeys(ctx context.Context, t store.RemoteTarget) (ssh.HostKeyCallback, error) {
	var pinned ssh.PublicKey
	if t.HostKey != "" {
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(t.HostKey))
		if err != nil {
			return nil, err
		}
		pinned = key
	}
	authority, err := sshca.HostAuthorityKey(ctx, h.st)
	if err != nil {
		return nil, err
	}
	checker := &ssh.CertChecker{
		IsHostAuthority: func(key ssh.PublicKey, _ string) bool {
			return authority != nil && bytes.Equal(key.Marshal(), authority.Marshal())
		},
		IsRevoked: func(cert *ssh.Certificate) bool {
			_, err := h.st.SSHHostByKey(ctx, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(cert.Key))))
			return err != nil
		},
		HostKeyFallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			if pinned != nil && bytes.Equal(key.Marshal(), pinned.Marshal()) {
				return nil
			}
			return &hostKeyError{key: key}
		},
	}
	return checker.CheckHostKey, nil
}

func (h *handlers) authMethods(ctx context.Context, t store.RemoteTarget, u store.User, ip string) ([]ssh.AuthMethod, error) {
	if t.Auth == "certificate" {
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		key, err := ssh.NewPublicKey(public)
		if err != nil {
			return nil, err
		}
		cert, err := sshca.UserCertificate(ctx, h.st, u, key, ip, certificateTTL)
		var problem *httpx.Error
		if errors.As(err, &problem) {
			return nil, errors.New(problem.Message)
		}
		if err != nil {
			return nil, err
		}
		signer, err := ssh.NewSignerFromKey(private)
		if err != nil {
			return nil, err
		}
		certSigner, err := ssh.NewCertSigner(cert, signer)
		if err != nil {
			return nil, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(certSigner)}, nil
	}
	credential, err := h.st.RemoteCredential(ctx, t.AppID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errors.New("The application has no stored credential for the host. Ask your administrator to add one.")
	}
	if err != nil {
		return nil, err
	}
	if t.Auth == "key" {
		signer, err := ssh.ParsePrivateKey([]byte(credential))
		if err != nil {
			return nil, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	}
	return []ssh.AuthMethod{ssh.Password(credential), ssh.KeyboardInteractive(func(_, _ string, questions []string, echos []bool) ([]string, error) {
		answers := make([]string, len(questions))
		for i := range questions {
			if !echos[i] {
				answers[i] = credential
			}
		}
		return answers, nil
	})}, nil
}

type recorder struct {
	mu    sync.Mutex
	start time.Time
	buf   bytes.Buffer
	gz    *gzip.Writer
	size  int
	tail  []byte
	full  bool
}

func newRecorder(cols, rows int, title string) *recorder {
	r := &recorder{start: time.Now()}
	r.gz = gzip.NewWriter(&r.buf)
	header, _ := json.Marshal(map[string]any{"version": 2, "width": cols, "height": rows, "timestamp": r.start.Unix(), "title": title})
	r.gz.Write(append(header, '\n'))
	return r
}

func (r *recorder) event(kind, data string) {
	line, _ := json.Marshal([]any{math.Round(time.Since(r.start).Seconds()*1e6) / 1e6, kind, data})
	r.gz.Write(append(line, '\n'))
}

func (r *recorder) output(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.full {
		return
	}
	data := append(r.tail, p...)
	cut := len(data)
	for i := len(data) - 1; i >= 0 && i >= len(data)-utf8.UTFMax; i-- {
		if utf8.RuneStart(data[i]) {
			if !utf8.FullRune(data[i:]) {
				cut = i
			}
			break
		}
	}
	r.tail = append([]byte(nil), data[cut:]...)
	if cut > 0 {
		r.event("o", string(data[:cut]))
	}
	r.size += cut
	r.full = r.size >= recordingLimit
}

func (r *recorder) resize(cols, rows int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		r.event("r", strconv.Itoa(cols)+"x"+strconv.Itoa(rows))
	}
}

func (r *recorder) close() ([]byte, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gz.Close()
	return r.buf.Bytes(), r.full
}
