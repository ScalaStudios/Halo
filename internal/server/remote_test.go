package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"

	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/outpost"
	"halo/internal/store"
	"halo/internal/testdb"
)

const remoteOrigin = "http://localhost:3200"

type remoteEnv struct {
	*env
	ca      ssh.PublicKey
	srv     *httptest.Server
	admin   string
	outpost string
	key     string
	host    *sshHost
	group   store.Group
}

type remoteApp struct {
	store.Application
	Remote store.RemoteTarget `json:"remote"`
}

type sshHost struct {
	addr   string
	public ssh.PublicKey
}

func setupRemote(t *testing.T, password string) *remoteEnv {
	t.Helper()
	return remoteOn(t, setup(t), password)
}

func remoteOn(t *testing.T, e *env, password string) *remoteEnv {
	t.Helper()
	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)
	_, admin, _ := e.person("admin@example.com", "passkey", false, "global_admin")
	r := &remoteEnv{env: e, srv: srv, admin: admin}
	var created struct {
		Outpost store.Outpost `json:"outpost"`
		Token   string        `json:"token"`
	}
	decodeInto(t, r.expect(admin, "POST", "/api/v1/outposts", map[string]string{"name": "Office"}, http.StatusCreated, ""), &created)
	key, err := outpost.Enroll(context.Background(), srv.URL, created.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := outpost.Enroll(context.Background(), srv.URL, created.Token); err == nil {
		t.Fatal("an enrollment token works only once")
	}
	r.outpost, r.key = created.Outpost.ID, key
	r.runAgent(key)
	var authority struct {
		PublicKey string `json:"publicKey"`
	}
	decodeInto(t, r.expect(admin, "GET", "/api/v1/ssh/authority", nil, http.StatusOK, ""), &authority)
	ca, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authority.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	r.ca = ca
	r.host = startSSHHost(t, ca, password)
	ctx := context.Background()
	if r.group, err = e.st.CreateGroup(ctx, store.NewGroup{Name: "Operators", Kind: "assigned"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.SaveSSHPrincipalMapping(ctx, store.SSHPrincipalMapping{GroupID: r.group.ID, Principals: []string{"ops"}}); err != nil {
		t.Fatal(err)
	}
	return r
}

func decodeInto(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
}

func (r *remoteEnv) runAgent(key string) {
	ctx, cancel := context.WithCancel(context.Background())
	r.t.Cleanup(cancel)
	guacdPort, _ := strconv.Atoi(os.Getenv("HALO_TEST_GUACD_PORT"))
	agent := &outpost.Agent{Server: r.srv.URL, Key: key, GuacdPort: guacdPort, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	go agent.Run(ctx)
	r.waitConnected(true)
}

func (r *remoteEnv) waitConnected(want bool) {
	r.t.Helper()
	for range 100 {
		var outposts []struct {
			ID        string `json:"id"`
			Connected bool   `json:"connected"`
		}
		decodeInto(r.t, r.expect(r.admin, "GET", "/api/v1/outposts", nil, http.StatusOK, ""), &outposts)
		online := false
		for _, o := range outposts {
			online = online || o.ID == r.outpost && o.Connected
		}
		if online == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	r.t.Fatalf("the outpost never reached connected=%v", want)
}

func (r *remoteEnv) app(settings map[string]any) remoteApp {
	r.t.Helper()
	body := map[string]any{"outpostId": r.outpost, "host": "127.0.0.1", "port": portOf(r.host.addr), "hostKey": string(ssh.MarshalAuthorizedKey(r.host.public))}
	for k, v := range settings {
		body[k] = v
	}
	var out struct {
		Application remoteApp `json:"application"`
	}
	decodeInto(r.t, r.expect(r.admin, "POST", "/api/v1/applications", map[string]any{"name": "Database host", "protocol": "remote", "groupIds": []string{r.group.ID}, "remote": body}, http.StatusCreated, ""), &out)
	return out.Application
}

func portOf(addr string) int {
	_, port, _ := net.SplitHostPort(addr)
	n, _ := strconv.Atoi(port)
	return n
}

func (r *remoteEnv) member(email, method string) (store.User, string) {
	r.t.Helper()
	u, token, _ := r.person(email, method, false)
	if err := r.st.AddGroupMember(context.Background(), r.group.ID, u.ID); err != nil {
		r.t.Fatal(err)
	}
	return u, token
}

func (r *remoteEnv) start(token, appID string) store.RemoteSession {
	r.t.Helper()
	var rs store.RemoteSession
	decodeInto(r.t, r.expect(token, "POST", "/api/v1/me/remote-sessions", map[string]string{"applicationId": appID}, http.StatusCreated, ""), &rs)
	return rs
}

func (r *remoteEnv) connect(token, remoteID, origin string) (*websocket.Conn, error) {
	config, err := websocket.NewConfig("ws"+strings.TrimPrefix(r.srv.URL, "http")+"/api/v1/me/remote-sessions/"+remoteID+"/connect?cols=100&rows=30", origin)
	if err != nil {
		return nil, err
	}
	config.Header.Set("Cookie", httpx.SessionCookie+"="+token)
	ws, err := websocket.DialConfig(config)
	if err == nil {
		r.t.Cleanup(func() { ws.Close() })
	}
	return ws, err
}

type terminalEvent struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type screen struct {
	t      *testing.T
	ws     *websocket.Conn
	output strings.Builder
	events []terminalEvent
	raw    strings.Builder
}

var anyFrame = websocket.Codec{Unmarshal: func(data []byte, kind byte, v any) error {
	*v.(*[2]any) = [2]any{kind, data}
	return nil
}}

func (s *screen) until(done func() bool) {
	s.t.Helper()
	s.ws.SetReadDeadline(time.Now().Add(10 * time.Second))
	for !done() {
		var f [2]any
		if err := anyFrame.Receive(s.ws, &f); err != nil {
			s.t.Fatalf("connection ended early: %v; output %q; events %+v", err, s.output.String(), s.events)
		}
		data := f[1].([]byte)
		s.raw.Write(data)
		if f[0].(byte) == websocket.BinaryFrame {
			s.output.Write(data)
			continue
		}
		var e terminalEvent
		json.Unmarshal(data, &e)
		s.events = append(s.events, e)
	}
}

func (s *screen) saw(kind string) bool {
	for _, e := range s.events {
		if e.Type == kind {
			return true
		}
	}
	return false
}

func (s *screen) ended() string {
	for _, e := range s.events {
		if e.Type == "end" {
			return e.Message
		}
	}
	return ""
}

func (s *screen) waitEnd() string {
	s.t.Helper()
	s.until(func() bool { return s.saw("end") })
	return s.ended()
}

func newHostKey(t *testing.T) ssh.Signer {
	t.Helper()
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	key, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func startSSHHost(t *testing.T, userCA ssh.PublicKey, password string, keys ...ssh.Signer) *sshHost {
	t.Helper()
	if len(keys) == 0 {
		keys = []ssh.Signer{newHostKey(t)}
	}
	checker := &ssh.CertChecker{IsUserAuthority: func(k ssh.PublicKey) bool { return bytes.Equal(k.Marshal(), userCA.Marshal()) }}
	config := &ssh.ServerConfig{
		PublicKeyCallback: checker.Authenticate,
		PasswordCallback: func(_ ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) {
			if password != "" && string(p) == password {
				return nil, nil
			}
			return nil, errors.New("wrong password")
		},
	}
	for _, key := range keys {
		config.AddHostKey(key)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			go serveSSH(c, config)
		}
	}()
	return &sshHost{addr: listener.Addr().String(), public: keys[0].PublicKey()}
}

func serveSSH(c net.Conn, config *ssh.ServerConfig) {
	conn, chans, reqs, err := ssh.NewServerConn(c, config)
	if err != nil {
		c.Close()
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			nc.Reject(ssh.UnknownChannelType, "")
			continue
		}
		ch, requests, err := nc.Accept()
		if err != nil {
			return
		}
		go func() {
			for req := range requests {
				switch req.Type {
				case "pty-req", "window-change":
					req.Reply(true, nil)
				case "shell":
					req.Reply(true, nil)
					go func() {
						fmt.Fprintf(ch, "welcome %s\r\n", conn.User())
						buf := make([]byte, 1024)
						for {
							n, err := ch.Read(buf)
							if err != nil {
								return
							}
							if strings.Contains(string(buf[:n]), "exit") {
								ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
								ch.Close()
								return
							}
							ch.Write([]byte("echo:" + string(buf[:n])))
						}
					}()
				default:
					req.Reply(false, nil)
				}
			}
		}()
	}
}

func (r *remoteEnv) sessions(appID string) []store.RemoteSession {
	r.t.Helper()
	var list []store.RemoteSession
	decodeInto(r.t, r.expect(r.admin, "GET", "/api/v1/remote-sessions?app="+appID, nil, http.StatusOK, ""), &list)
	return list
}

func TestRemoteSSHSessionWithCertificateIsLoggedAndRecorded(t *testing.T) {
	r := setupRemote(t, "")
	app := r.app(map[string]any{"auth": "certificate", "record": true})
	_, token := r.member("ana@example.com", "passkey")
	rs := r.start(token, app.ID)
	if rs.Username != "ops" || rs.Address != r.host.addr || rs.Status != "pending" {
		t.Fatalf("the session signs in as the person's first principal: %+v", rs)
	}
	ws, err := r.connect(token, rs.ID, remoteOrigin)
	if err != nil {
		t.Fatal(err)
	}
	s := &screen{t: t, ws: ws}
	s.until(func() bool { return strings.Contains(s.output.String(), "welcome ops") })
	if !s.saw("connected") {
		t.Fatalf("the browser hears that the session connected: %+v", s.events)
	}
	websocket.Message.Send(ws, `{"type":"resize","cols":120,"rows":40}`)
	websocket.Message.Send(ws, []byte("ls\r"))
	s.until(func() bool { return strings.Contains(s.output.String(), "echo:ls") })
	websocket.Message.Send(ws, []byte("exit\r"))
	if msg := s.waitEnd(); !strings.Contains(msg, "exited") {
		t.Fatalf("the end message says the shell exited: %q", msg)
	}
	if again, err := r.connect(token, rs.ID, remoteOrigin); err == nil {
		s := &screen{t: t, ws: again}
		if msg := s.waitEnd(); !strings.Contains(msg, "already used") {
			t.Fatalf("a session connects only once: %q", msg)
		}
	}
	var logged store.RemoteSession
	for range 50 {
		logged = r.sessions(app.ID)[0]
		if logged.Status == "ended" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if logged.Status != "ended" || logged.StartedAt == nil || logged.EndedAt == nil || logged.BytesIn != int64(len("ls\rexit\r")) || logged.BytesOut == 0 || !logged.Recorded || logged.Email != "ana@example.com" {
		t.Fatalf("the session log has the person, target, times and bytes: %+v", logged)
	}
	rec := r.expect(r.admin, "GET", "/api/v1/remote-sessions/"+rs.ID+"/recording", nil, http.StatusOK, "")
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	var header map[string]any
	if json.Unmarshal([]byte(lines[0]), &header) != nil || header["version"] != float64(2) || header["width"] != float64(100) || !strings.Contains(rec.Body.String(), `"o","welcome ops`) || !strings.Contains(rec.Body.String(), `"r","120x40"`) {
		t.Fatalf("the recording is an asciicast with the output: %s", rec.Body)
	}
	certs, err := r.st.ListSSHCertificates(context.Background(), 10)
	if err != nil || len(certs) != 1 || certs[0].ValidBefore.Sub(certs[0].ValidAfter) > 7*time.Minute {
		t.Fatalf("Halo signs one short-lived certificate for the session: %+v %v", certs, err)
	}
	signIns, err := r.st.ListSignIns(context.Background(), store.SignInFilter{AppID: app.ID})
	if err != nil || len(signIns) != 1 || signIns[0].Result != "success" {
		t.Fatalf("starting a session is a sign-in to the application: %+v %v", signIns, err)
	}
}

func TestRemoteSessionRefusesWhatAnApplicationSignInRefuses(t *testing.T) {
	r := setupRemote(t, "")
	app := r.app(map[string]any{"auth": "certificate"})
	_, outsider, _ := r.person("eve@example.com", "passkey", false)
	r.expect(outsider, "POST", "/api/v1/me/remote-sessions", map[string]string{"applicationId": app.ID}, http.StatusForbidden, "ERR_NOT_ASSIGNED")
	_, totp := r.member("tom@example.com", "totp")
	policy := map[string]any{"name": "Passkeys for hosts", "description": "", "enabled": true, "mode": "enforce", "effect": "require-phishing-resistant", "conditions": store.PolicyConditions{AllUsers: true, AppIDs: []string{app.ID}}}
	r.expect(r.admin, "POST", "/api/v1/policies", policy, http.StatusCreated, "")
	r.expect(totp, "POST", "/api/v1/me/remote-sessions", map[string]string{"applicationId": app.ID}, http.StatusForbidden, "ERR_STRONGER_AUTH_REQUIRED")
	_, passkey := r.member("pia@example.com", "passkey")
	r.start(passkey, app.ID)
	var zone struct {
		ID string `json:"id"`
	}
	decodeInto(t, r.expect(r.admin, "POST", "/api/v1/network-zones", map[string]any{"name": "Office", "kind": "trusted", "cidrs": []string{"10.20.0.0/16"}}, http.StatusCreated, ""), &zone)
	block := map[string]any{"name": "Office only", "description": "", "enabled": true, "mode": "enforce", "effect": "block", "conditions": store.PolicyConditions{AllUsers: true, AppIDs: []string{app.ID}, Network: "not-in", ZoneIDs: []string{zone.ID}}}
	r.expect(r.admin, "POST", "/api/v1/policies", block, http.StatusCreated, "")
	r.expect(passkey, "POST", "/api/v1/me/remote-sessions", map[string]string{"applicationId": app.ID}, http.StatusForbidden, "ERR_BLOCKED_BY_POLICY")
	r.expect(r.admin, "POST", "/api/v1/applications/"+app.ID+"/disable", nil, http.StatusOK, "")
	r.expect(passkey, "POST", "/api/v1/me/remote-sessions", map[string]string{"applicationId": app.ID}, http.StatusForbidden, "ERR_APP_DISABLED")
	signIns, err := r.st.ListSignIns(context.Background(), store.SignInFilter{AppID: app.ID})
	if err != nil || len(signIns) != 5 {
		t.Fatalf("every refusal and the start are in the sign-in log: %d %v", len(signIns), err)
	}
}

func TestRemotePendingSessionRechecksAssignmentAndTarget(t *testing.T) {
	r := setupRemote(t, "")
	app := r.app(map[string]any{"auth": "certificate"})
	user, token := r.member("ana@example.com", "passkey")
	expectRefusal := func(rs store.RemoteSession, contains string) {
		t.Helper()
		ws, err := r.connect(token, rs.ID, remoteOrigin)
		if err != nil {
			t.Fatal(err)
		}
		defer ws.Close()
		screen := &screen{t: t, ws: ws}
		if msg := screen.waitEnd(); !strings.Contains(msg, contains) || screen.saw("connected") {
			t.Fatalf("expected refusal before connecting, got %q", msg)
		}
	}
	rs := r.start(token, app.ID)
	if err := r.st.RemoveGroupMember(t.Context(), r.group.ID, user.ID); err != nil {
		t.Fatal(err)
	}
	expectRefusal(rs, "Your access to this host ended")
	if err := r.st.AddGroupMember(t.Context(), r.group.ID, user.ID); err != nil {
		t.Fatal(err)
	}
	rs = r.start(token, app.ID)
	r.expect(r.admin, "PUT", "/api/v1/applications/"+app.ID+"/remote", map[string]any{"host": "other.internal"}, http.StatusOK, "")
	expectRefusal(rs, "The connection settings changed")
}

func TestRemoteSessionEndsAtTimeLimitAndOnTermination(t *testing.T) {
	r := setupRemote(t, "")
	app := r.app(map[string]any{"auth": "certificate"})
	target := app.Remote
	target.AppID, target.SessionLimit = app.ID, 1
	if err := r.st.SaveRemoteTarget(context.Background(), target, nil); err != nil {
		t.Fatal(err)
	}
	_, token := r.member("ana@example.com", "passkey")
	ws, err := r.connect(token, r.start(token, app.ID).ID, remoteOrigin)
	if err != nil {
		t.Fatal(err)
	}
	s := &screen{t: t, ws: ws}
	if msg := s.waitEnd(); !strings.Contains(msg, "reached its time limit") {
		t.Fatalf("the session ends at its time limit: %q", msg)
	}
	target.SessionLimit = 3600
	if err := r.st.SaveRemoteTarget(context.Background(), target, nil); err != nil {
		t.Fatal(err)
	}
	rs := r.start(token, app.ID)
	ws, err = r.connect(token, rs.ID, remoteOrigin)
	if err != nil {
		t.Fatal(err)
	}
	s = &screen{t: t, ws: ws}
	s.until(func() bool { return s.saw("connected") })
	_, helpdesk, _ := r.person("hal@example.com", "passkey", false, "helpdesk_admin")
	r.expect(helpdesk, "POST", "/api/v1/remote-sessions/"+rs.ID+"/terminate", nil, http.StatusForbidden, "")
	r.expect(r.admin, "POST", "/api/v1/remote-sessions/"+rs.ID+"/terminate", nil, http.StatusOK, "")
	if msg := s.waitEnd(); !strings.Contains(msg, "ended the session from the Halo console") {
		t.Fatalf("an administrator ends a live session: %q", msg)
	}
	r.expect(r.admin, "POST", "/api/v1/remote-sessions/"+rs.ID+"/terminate", nil, http.StatusConflict, "ERR_SESSION_ENDED")
}

func TestRemoteSessionTimeLimitIncludesConnectionSetup(t *testing.T) {
	r := setupRemote(t, "")
	host, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { host.Close() })
	app := r.app(map[string]any{"auth": "certificate", "port": portOf(host.Addr().String())})
	target := app.Remote
	target.AppID, target.SessionLimit = app.ID, 1
	if err := r.st.SaveRemoteTarget(context.Background(), target, nil); err != nil {
		t.Fatal(err)
	}
	_, token := r.member("ana@example.com", "passkey")
	ws, err := r.connect(token, r.start(token, app.ID).ID, remoteOrigin)
	if err != nil {
		t.Fatal(err)
	}
	s := &screen{t: t, ws: ws}
	if msg := s.waitEnd(); !strings.Contains(msg, "reached its time limit") || s.saw("connected") {
		t.Fatalf("the session limit applies while waiting for the host: %q", msg)
	}
}

func TestRemotePasswordNeverReachesTheBrowser(t *testing.T) {
	password := "correct-horse-battery-staple"
	r := setupRemote(t, password)
	app := r.app(map[string]any{"auth": "password", "username": "deploy", "credential": password, "record": true})
	if !app.Remote.HasCredential {
		t.Fatalf("the application says it has a stored credential: %+v", app.Remote)
	}
	r.expect(r.admin, "PUT", "/api/v1/applications/"+app.ID+"/remote", map[string]any{"sessionLimit": 600, "credential": ""}, http.StatusOK, "")
	r.expect(r.admin, "PUT", "/api/v1/applications/"+app.ID+"/remote", map[string]any{"auth": "key"}, http.StatusUnprocessableEntity, "ERR_INVALID")
	_, token := r.member("ana@example.com", "passkey")
	ws, err := r.connect(token, r.start(token, app.ID).ID, remoteOrigin)
	if err != nil {
		t.Fatal(err)
	}
	s := &screen{t: t, ws: ws}
	s.until(func() bool { return strings.Contains(s.output.String(), "welcome deploy") })
	websocket.Message.Send(ws, []byte("exit\r"))
	s.waitEnd()
	seen := []string{s.raw.String()}
	for _, path := range []string{"/api/v1/applications/" + app.ID, "/api/v1/applications", "/api/v1/audit", "/api/v1/export", "/api/v1/remote-sessions", "/api/v1/sign-ins"} {
		seen = append(seen, r.expect(r.admin, "GET", path, nil, http.StatusOK, "").Body.String())
	}
	seen = append(seen, r.expect(token, "GET", "/api/v1/me/applications", nil, http.StatusOK, "").Body.String())
	recording := r.expect(r.admin, "GET", "/api/v1/remote-sessions/"+r.sessions(app.ID)[0].ID+"/recording", nil, http.StatusOK, "")
	seen = append(seen, recording.Body.String())
	for _, body := range seen {
		if strings.Contains(body, password) {
			t.Fatalf("the stored password appeared in a response: %s", body)
		}
	}
	export := r.expect(r.admin, "GET", "/api/v1/export", nil, http.StatusOK, "").Body.String()
	if !strings.Contains(export, `"remote":{`) || !strings.Contains(export, `"outpostId":"`+r.outpost+`"`) {
		t.Fatalf("the export carries the remote settings for import: %s", export)
	}
	r.expect(r.admin, "PUT", "/api/v1/applications/"+app.ID+"/remote", map[string]any{"credential": "wrong"}, http.StatusOK, "")
	ws, err = r.connect(token, r.start(token, app.ID).ID, remoteOrigin)
	if err != nil {
		t.Fatal(err)
	}
	s = &screen{t: t, ws: ws}
	if msg := s.waitEnd(); !strings.Contains(msg, "refused the sign-in as deploy") {
		t.Fatalf("a wrong password says the host refused it: %q", msg)
	}
}

func TestRemoteHostKeyMustBeTrusted(t *testing.T) {
	r := setupRemote(t, "")
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	wrong, _ := ssh.NewSignerFromKey(other)
	app := r.app(map[string]any{"auth": "certificate", "hostKey": string(ssh.MarshalAuthorizedKey(wrong.PublicKey()))})
	revoked := "@revoked " + r.host.addr + " " + string(ssh.MarshalAuthorizedKey(r.host.public))
	r.expect(r.admin, "PUT", "/api/v1/applications/"+app.ID+"/remote", map[string]any{"hostKey": revoked}, http.StatusUnprocessableEntity, "ERR_INVALID")
	_, token := r.member("ana@example.com", "passkey")
	ws, err := r.connect(token, r.start(token, app.ID).ID, remoteOrigin)
	if err != nil {
		t.Fatal(err)
	}
	s := &screen{t: t, ws: ws}
	if msg := s.waitEnd(); !strings.Contains(msg, "does not trust the host key") || !strings.Contains(msg, ssh.FingerprintSHA256(r.host.public)) {
		t.Fatalf("an unexpected host key stops the session: %q", msg)
	}
	r.expect(r.admin, "PUT", "/api/v1/applications/"+app.ID+"/remote", map[string]any{"hostKey": ""}, http.StatusOK, "")
	ws, err = r.connect(token, r.start(token, app.ID).ID, remoteOrigin)
	if err != nil {
		t.Fatal(err)
	}
	s = &screen{t: t, ws: ws}
	if msg := s.waitEnd(); !strings.Contains(msg, "does not trust the host key") {
		t.Fatalf("without a pinned key or a Halo host certificate the session stops: %q", msg)
	}
}

func TestRemoteTrustsHostsWithAHaloHostCertificate(t *testing.T) {
	r := setupRemote(t, "")
	hostKey := newHostKey(t)
	var host struct {
		Host store.SSHHost `json:"host"`
	}
	decodeInto(t, r.expect(r.admin, "POST", "/api/v1/ssh/hosts", map[string]any{"hostnames": []string{"127.0.0.1"}}, http.StatusCreated, ""), &host)
	var issued struct {
		Certificate string `json:"certificate"`
	}
	decodeInto(t, r.expect(r.admin, "POST", "/api/v1/ssh/hosts/"+host.Host.ID+"/certificate", map[string]string{"publicKey": string(ssh.MarshalAuthorizedKey(hostKey.PublicKey()))}, http.StatusCreated, ""), &issued)
	parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(issued.Certificate))
	if err != nil {
		t.Fatal(err)
	}
	certSigner, err := ssh.NewCertSigner(parsed.(*ssh.Certificate), hostKey)
	if err != nil {
		t.Fatal(err)
	}
	r.host = startSSHHost(t, r.ca, "", hostKey, certSigner)
	app := r.app(map[string]any{"auth": "certificate", "hostKey": ""})
	_, token := r.member("ana@example.com", "passkey")
	ws, err := r.connect(token, r.start(token, app.ID).ID, remoteOrigin)
	if err != nil {
		t.Fatal(err)
	}
	s := &screen{t: t, ws: ws}
	s.until(func() bool { return strings.Contains(s.output.String(), "welcome ops") })
	websocket.Message.Send(ws, []byte("exit\r"))
	s.waitEnd()
	r.expect(r.admin, "POST", "/api/v1/ssh/hosts/"+host.Host.ID+"/revoke", nil, http.StatusOK, "")
	ws, err = r.connect(token, r.start(token, app.ID).ID, remoteOrigin)
	if err != nil {
		t.Fatal(err)
	}
	s = &screen{t: t, ws: ws}
	if msg := s.waitEnd(); !strings.Contains(msg, "does not trust the host key") {
		t.Fatalf("a revoked host's certificate is not trusted: %q", msg)
	}
}

func TestRemoteConnectionsAreAuthenticated(t *testing.T) {
	r := setupRemote(t, "")
	app := r.app(map[string]any{"auth": "certificate"})
	ana, token := r.member("ana@example.com", "passkey")
	rs := r.start(token, app.ID)
	if _, err := r.connect(token, rs.ID, "https://evil.example.com"); err == nil {
		t.Fatal("a WebSocket from another site is refused")
	}
	if _, err := r.connect("", rs.ID, remoteOrigin); err == nil {
		t.Fatal("a WebSocket without a session is refused")
	}
	_, second, err := r.st.CreateSession(context.Background(), store.NewSession{UserID: ana.ID, Method: "passkey"})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := r.connect(second, rs.ID, remoteOrigin)
	if err != nil {
		t.Fatal(err)
	}
	s := &screen{t: t, ws: ws}
	if msg := s.waitEnd(); !strings.Contains(msg, "expired or was already used") {
		t.Fatalf("a session connects only from the Halo session that started it: %q", msg)
	}
	for _, key := range []string{"", "hlo_wrong"} {
		req, _ := http.NewRequest("GET", r.srv.URL+"/api/v1/outpost/connect", nil)
		req.Header.Set("Authorization", "Bearer "+key)
		res, err := http.DefaultClient.Do(req)
		if err != nil || res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("an outpost without a valid key is refused: %v %v", res, err)
		}
		res.Body.Close()
	}
	var second2 struct {
		Outpost store.Outpost `json:"outpost"`
		Token   string        `json:"token"`
	}
	decodeInto(t, r.expect(r.admin, "POST", "/api/v1/outposts", map[string]string{"name": "Lab"}, http.StatusCreated, ""), &second2)
	labKey, err := outpost.Enroll(context.Background(), r.srv.URL, second2.Token)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", r.srv.URL+"/api/v1/outpost/streams/anything", nil)
	req.Header.Set("Authorization", "Bearer "+labKey)
	if res, err := http.DefaultClient.Do(req); err != nil || res.StatusCode != http.StatusNotFound {
		t.Fatalf("an outpost can't claim a connection Halo did not ask it for: %v %v", res, err)
	}
	r.expect(r.admin, "DELETE", "/api/v1/outposts/"+r.outpost, nil, http.StatusConflict, "ERR_OUTPOST_IN_USE")
	r.expect(r.admin, "POST", "/api/v1/outposts/"+r.outpost+"/enrollment-token", nil, http.StatusOK, "")
	r.expect(r.admin, "DELETE", "/api/v1/applications/"+app.ID, nil, http.StatusNoContent, "")
	r.expect(r.admin, "DELETE", "/api/v1/outposts/"+r.outpost, nil, http.StatusNoContent, "")
	r.waitConnected(false)
	if _, err := r.st.OutpostByKey(context.Background(), r.key); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("a deleted outpost's key stops working: %v", err)
	}
}

func TestRemoteTargetOnlyUsesItsOwnOutpost(t *testing.T) {
	r := setupRemote(t, "")
	var lab struct {
		Outpost store.Outpost `json:"outpost"`
	}
	decodeInto(t, r.expect(r.admin, "POST", "/api/v1/outposts", map[string]string{"name": "Lab"}, http.StatusCreated, ""), &lab)
	app := r.app(map[string]any{"auth": "certificate", "outpostId": lab.Outpost.ID})
	_, token := r.member("ana@example.com", "passkey")
	r.expect(token, "POST", "/api/v1/me/remote-sessions", map[string]string{"applicationId": app.ID}, http.StatusServiceUnavailable, "ERR_OUTPOST_OFFLINE")
}

func TestRemoteSessionEndsWhenAnotherInstanceTerminatesIt(t *testing.T) {
	public, _ := url.Parse(remoteOrigin)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	cfg := config.Config{Organization: "Test", PublicURL: public, SecretKey: key, Dev: true}
	var envs []*env
	for _, st := range testdb.Instances(t, 2) {
		app, err := New(cfg, st)
		if err != nil {
			t.Fatal(err)
		}
		envs = append(envs, &env{t: t, st: st, h: app.Handler, app: app})
	}
	r := remoteOn(t, envs[0], "")
	app := r.app(map[string]any{"auth": "certificate"})
	_, token := r.member("ana@example.com", "passkey")
	rs := r.start(token, app.ID)
	ws, err := r.connect(token, rs.ID, remoteOrigin)
	if err != nil {
		t.Fatal(err)
	}
	s := &screen{t: t, ws: ws}
	s.until(func() bool { return s.saw("connected") })
	envs[1].expect(r.admin, "POST", "/api/v1/remote-sessions/"+rs.ID+"/terminate", nil, http.StatusOK, "")
	if msg := s.waitEnd(); !strings.Contains(msg, "ended the session from the Halo console") {
		t.Fatalf("a session ends when another instance terminates it: %q", msg)
	}
	if ended := r.sessions(app.ID)[0]; ended.Status != "ended" || ended.TerminatedBy == nil {
		t.Fatalf("the session log says who ended it: %+v", ended)
	}
}

func TestRemoteSocketClosesWhenSessionRecordEnds(t *testing.T) {
	r := setupRemote(t, "")
	app := r.app(map[string]any{"auth": "certificate"})
	_, token := r.member("ana@example.com", "passkey")
	rs := r.start(token, app.ID)
	ws, err := r.connect(token, rs.ID, remoteOrigin)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	s := &screen{t: t, ws: ws}
	s.until(func() bool { return s.saw("connected") })
	if err := r.st.EndRemoteSession(t.Context(), rs.ID, "Session cleanup marked it ended.", 0, 0, nil); err != nil {
		t.Fatal(err)
	}
	if msg := s.waitEnd(); msg != "The session is no longer active in Halo." {
		t.Fatalf("the live connection should stop after its record ends: %q", msg)
	}
}

func TestRemoteCertificateSessionDefaultsToTheLinuxAccount(t *testing.T) {
	r := setupRemote(t, "")
	ctx := context.Background()
	linux, err := r.st.CreateGroup(ctx, store.NewGroup{Name: "Linux", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.st.SaveHostGroup(ctx, store.HostGroup{Name: "Servers", Rules: []store.HostRule{{GroupID: linux.ID, Sudo: []string{}}}}); err != nil {
		t.Fatal(err)
	}
	app := r.app(map[string]any{"auth": "certificate"})
	if err := r.st.SetApplicationGroups(ctx, app.ID, []string{r.group.ID, linux.ID}); err != nil {
		t.Fatal(err)
	}
	u, token, _ := r.person("lin@example.com", "passkey", false)
	if err := r.st.AddGroupMember(ctx, linux.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	u, err = r.st.GetUser(ctx, u.ID)
	if err != nil || u.POSIX == nil {
		t.Fatalf("people get a Linux account: %+v %v", u.POSIX, err)
	}
	if rs := r.start(token, app.ID); rs.Username != u.POSIX.Username {
		t.Fatalf("without a principal mapping the session signs in as the person's Linux account, not its host principal: %q", rs.Username)
	}
	if err := r.st.AddGroupMember(ctx, r.group.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	if rs := r.start(token, app.ID); rs.Username != "ops" {
		t.Fatalf("a mapped principal comes first: %q", rs.Username)
	}
}

func (r *remoteEnv) agentSocket(key, path string) (*websocket.Conn, error) {
	config, err := websocket.NewConfig("ws"+strings.TrimPrefix(r.srv.URL, "http")+path, r.srv.URL)
	if err != nil {
		return nil, err
	}
	config.Header.Set("Authorization", "Bearer "+key)
	conn, err := websocket.DialConfig(config)
	if err == nil {
		r.t.Cleanup(func() { conn.Close() })
	}
	return conn, err
}

func TestRemoteStreamIsClaimedOnlyByTheOutpostThatWasAsked(t *testing.T) {
	r := setupRemote(t, "")
	var lab struct {
		Outpost store.Outpost `json:"outpost"`
		Token   string        `json:"token"`
	}
	decodeInto(t, r.expect(r.admin, "POST", "/api/v1/outposts", map[string]string{"name": "Lab"}, http.StatusCreated, ""), &lab)
	labKey, err := outpost.Enroll(context.Background(), r.srv.URL, lab.Token)
	if err != nil {
		t.Fatal(err)
	}
	control, err := r.agentSocket(labKey, "/api/v1/outpost/connect")
	if err != nil {
		t.Fatal(err)
	}
	app := r.app(map[string]any{"auth": "certificate", "outpostId": lab.Outpost.ID})
	_, token := r.member("ana@example.com", "passkey")
	var rs store.RemoteSession
	eventually(t, "the hand-made outpost connection must be registered", func() bool {
		response := r.call(token, "POST", "/api/v1/me/remote-sessions", map[string]string{"applicationId": app.ID})
		if response.Code != http.StatusCreated {
			return false
		}
		decodeInto(t, response, &rs)
		return true
	})
	ws, err := r.connect(token, rs.ID, remoteOrigin)
	if err != nil {
		t.Fatal(err)
	}
	var dial struct {
		Type    string `json:"type"`
		Stream  string `json:"stream"`
		Address string `json:"address"`
	}
	control.SetReadDeadline(time.Now().Add(10 * time.Second))
	for dial.Type != "dial" {
		var raw string
		if err := websocket.Message.Receive(control, &raw); err != nil {
			t.Fatal(err)
		}
		json.Unmarshal([]byte(raw), &dial)
	}
	if dial.Address != r.host.addr {
		t.Fatalf("Halo asks for the application's own address: %+v", dial)
	}
	claim := func(key string) int {
		req, _ := http.NewRequest("GET", r.srv.URL+"/api/v1/outpost/streams/"+dial.Stream, nil)
		req.Header.Set("Authorization", "Bearer "+key)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if code := claim(r.key); code != http.StatusNotFound {
		t.Fatalf("another outpost's key can't claim the stream: %d", code)
	}
	stream, err := r.agentSocket(labKey, "/api/v1/outpost/streams/"+dial.Stream)
	if err != nil {
		t.Fatalf("the asked outpost claims its stream: %v", err)
	}
	stream.PayloadType = websocket.BinaryFrame
	host, err := net.Dial("tcp", dial.Address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { host.Close() })
	go io.Copy(stream, host)
	go io.Copy(host, stream)
	s := &screen{t: t, ws: ws}
	s.until(func() bool { return strings.Contains(s.output.String(), "welcome ops") })
	if code := claim(labKey); code != http.StatusNotFound {
		t.Fatalf("a stream is claimed once: %d", code)
	}
}

func TestRemoteDesktopsThroughOutpost(t *testing.T) {
	if os.Getenv("HALO_TEST_GUACD_PORT") == "" {
		t.Skip("set HALO_TEST_GUACD_PORT and the RDP and VNC fixture settings")
	}
	for _, protocol := range []string{"rdp", "vnc"} {
		t.Run(protocol, func(t *testing.T) {
			prefix := "HALO_TEST_" + strings.ToUpper(protocol) + "_"
			port, err := strconv.Atoi(os.Getenv(prefix + "PORT"))
			if err != nil {
				t.Fatal(err)
			}
			r := setupRemote(t, "")
			secret := os.Getenv(prefix + "PASSWORD")
			settings := map[string]any{
				"protocol": protocol, "port": port, "auth": "password", "credential": secret,
				"username": os.Getenv(prefix + "USERNAME"), "hostKey": os.Getenv(prefix + "FINGERPRINT"),
			}
			if protocol == "rdp" {
				settings["rdpSecurity"] = "tls"
			}
			app := r.app(settings)
			_, unassigned, _ := r.person("outside@example.com", "passkey", false)
			request := map[string]string{"applicationId": app.ID}
			r.expect(unassigned, "POST", "/api/v1/me/remote-sessions", request, http.StatusForbidden, "ERR_NOT_ASSIGNED")
			_, totp := r.member("otp@example.com", "totp")
			policy := map[string]any{"name": "Passkeys for desktops", "enabled": true, "mode": "enforce", "effect": "require-phishing-resistant", "conditions": store.PolicyConditions{AllUsers: true, AppIDs: []string{app.ID}}}
			r.expect(r.admin, "POST", "/api/v1/policies", policy, http.StatusCreated, "")
			r.expect(totp, "POST", "/api/v1/me/remote-sessions", request, http.StatusForbidden, "ERR_STRONGER_AUTH_REQUIRED")
			_, token := r.member("ana@example.com", "passkey")
			rs := r.start(token, app.ID)
			ws, err := r.connect(token, rs.ID, remoteOrigin)
			if err != nil {
				t.Fatal(err)
			}
			display := &screen{t: t, ws: ws}
			display.until(func() bool { return display.saw("end") || strings.Contains(display.output.String(), "4.size,1.0") })
			if display.saw("end") || !display.saw("connected") {
				t.Fatalf("the desktop did not connect: %+v", display.events)
			}
			r.expect(r.admin, "POST", "/api/v1/remote-sessions/"+rs.ID+"/terminate", nil, http.StatusOK, "")
			if msg := display.waitEnd(); !strings.Contains(msg, "ended the session from the Halo console") {
				t.Fatalf("desktop did not terminate: %s", msg)
			}
			saved := r.sessions(app.ID)[0]
			if saved.Protocol != protocol || saved.StartedAt == nil || saved.EndedAt == nil || saved.Status != "ended" {
				t.Fatalf("session log was incomplete: %+v", saved)
			}
			if secret == "" || strings.Contains(display.raw.String(), secret) {
				t.Fatal("desktop credential unexpectedly available in the browser")
			}
			target := app.Remote
			target.AppID, target.SessionLimit = app.ID, 4
			if err := r.st.SaveRemoteTarget(t.Context(), target, nil); err != nil {
				t.Fatal(err)
			}
			ws, err = r.connect(token, r.start(token, app.ID).ID, remoteOrigin)
			if err != nil {
				t.Fatal(err)
			}
			limited := &screen{t: t, ws: ws}
			if msg := limited.waitEnd(); !limited.saw("connected") || !strings.Contains(msg, "reached its time limit") {
				t.Fatalf("the live desktop did not end at its time limit: %s", msg)
			}
			if protocol == "rdp" {
				r.expect(r.admin, "PUT", "/api/v1/applications/"+app.ID+"/remote", map[string]any{"hostKey": strings.Repeat("00", 32), "sessionLimit": 60}, http.StatusOK, "")
				ws, err := r.connect(token, r.start(token, app.ID).ID, remoteOrigin)
				if err != nil {
					t.Fatal(err)
				}
				bad := &screen{t: t, ws: ws}
				bad.waitEnd()
				if bad.saw("connected") {
					t.Fatal("unrecognized RDP certificate was accepted")
				}
			}
			var zone struct {
				ID string `json:"id"`
			}
			decodeInto(t, r.expect(r.admin, "POST", "/api/v1/network-zones", map[string]any{"name": "Desktop network", "kind": "trusted", "cidrs": []string{"10.20.0.0/16"}}, http.StatusCreated, ""), &zone)
			block := map[string]any{"name": "Desktops only from the trusted network", "enabled": true, "mode": "enforce", "effect": "block", "conditions": store.PolicyConditions{AllUsers: true, AppIDs: []string{app.ID}, Network: "not-in", ZoneIDs: []string{zone.ID}}}
			r.expect(r.admin, "POST", "/api/v1/policies", block, http.StatusCreated, "")
			r.expect(token, "POST", "/api/v1/me/remote-sessions", request, http.StatusForbidden, "ERR_BLOCKED_BY_POLICY")
			path := "/api/v1/applications/" + app.ID + "/remote"
			r.expect(r.admin, "PUT", path, map[string]any{"rdpSecurity": "other", "sessionLimit": 60}, http.StatusUnprocessableEntity, "ERR_INVALID")
			r.expect(r.admin, "PUT", path, map[string]any{"protocol": "ssh", "sessionLimit": 60}, http.StatusOK, "")
			sshTarget, err := r.st.RemoteTarget(t.Context(), app.ID)
			if err != nil || sshTarget.Protocol != "ssh" || sshTarget.Port != 22 || sshTarget.Auth != "certificate" || sshTarget.HasCredential || sshTarget.HostKey != "" {
				t.Fatalf("switching to SSH should reset the desktop settings: %+v, %v", sshTarget, err)
			}
		})
	}
}
