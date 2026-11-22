package remote

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"halo/internal/access"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/sshca"
	"halo/internal/store"
)

var (
	readers         = []string{"security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"}
	outpostWriters  = []string{"security_admin"}
	terminators     = []string{"security_admin", "app_admin"}
	recordingViewer = []string{"security_admin", "auditor"}
	hostPattern     = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)
	userPattern     = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.@-]{0,63}$`)
	errNoOutpost    = httpx.NotFound("This outpost")
	errNoSession    = httpx.NotFound("This remote session")
	errSignInAgain  = "ERR_SIGN_IN_AGAIN"
)

type handlers struct {
	st     *store.Store
	cfg    config.Config
	policy access.Engine
	hub    *Hub
}

func Register(mux *http.ServeMux, d httpx.Deps) error {
	h := &handlers{st: d.Store, cfg: d.Config, policy: d.Policy, hub: NewHub(d.Store)}
	if h.policy == nil {
		h.policy = access.AllowAll{}
	}
	if d.Jobs != nil {
		d.Jobs.Every("end remote sessions that lost their server", time.Minute, h.st.EndStaleRemoteSessions)
	}
	route := func(pattern string, roles []string, fn func(http.ResponseWriter, *http.Request, store.User) error) {
		mux.Handle(pattern, httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
			actor, err := httpx.RequireRole(r, roles...)
			if err != nil {
				return err
			}
			return fn(w, r, actor)
		}))
	}
	route("GET /api/v1/outposts", readers, h.listOutposts)
	route("POST /api/v1/outposts", outpostWriters, h.createOutpost)
	route("POST /api/v1/outposts/{id}/enrollment-token", outpostWriters, h.outpostToken)
	route("DELETE /api/v1/outposts/{id}", outpostWriters, h.deleteOutpost)
	route("GET /api/v1/remote-sessions", readers, h.listSessions)
	route("POST /api/v1/remote-sessions/{id}/terminate", terminators, h.terminate)
	route("GET /api/v1/remote-sessions/{id}/recording", recordingViewer, h.recording)
	mux.Handle("POST /api/v1/me/remote-sessions", httpx.Handle(h.start))
	mux.Handle("GET /api/v1/me/remote-sessions/{id}/connect", httpx.Handle(h.terminal))
	mux.Handle("POST /api/v1/outpost/enroll", httpx.Handle(h.hub.enroll))
	mux.Handle("GET /api/v1/outpost/connect", httpx.Handle(h.hub.connect))
	mux.Handle("GET /api/v1/outpost/streams/{stream}", httpx.Handle(h.hub.attach))
	return nil
}

func record(r *http.Request, tx *store.Store, actor store.User, e store.AuditEvent) error {
	e.ActorID, e.IP = &actor.ID, httpx.ClientIP(r)
	return tx.RecordAudit(r.Context(), e)
}

type outpostView struct {
	store.Outpost
	Connected bool `json:"connected"`
}

func (h *handlers) listOutposts(w http.ResponseWriter, r *http.Request, _ store.User) error {
	outposts, err := h.st.ListOutposts(r.Context())
	if err != nil {
		return err
	}
	out := make([]outpostView, len(outposts))
	for i, o := range outposts {
		out[i] = outpostView{o, h.hub.Connected(o.ID)}
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h *handlers) createOutpost(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in struct {
		Name string `json:"name"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 100 {
		return httpx.Invalid("Name the outpost after the network it reaches, like Office LAN or Production VPC, in up to 100 characters.")
	}
	return h.enrollment(w, r, actor, http.StatusCreated, "outpost.create", "Registered the outpost ", func(tx *store.Store) (store.Outpost, string, error) {
		return tx.CreateOutpost(r.Context(), name)
	})
}

func (h *handlers) outpostToken(w http.ResponseWriter, r *http.Request, actor store.User) error {
	return h.enrollment(w, r, actor, http.StatusOK, "outpost.token", "Created an enrollment token for the outpost ", func(tx *store.Store) (store.Outpost, string, error) {
		return tx.IssueOutpostToken(r.Context(), r.PathValue("id"))
	})
}

func (h *handlers) enrollment(w http.ResponseWriter, r *http.Request, actor store.User, status int, action, summary string, issue func(*store.Store) (store.Outpost, string, error)) error {
	var o store.Outpost
	var token string
	err := h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		if o, token, err = issue(tx); errors.Is(err, store.ErrNotFound) {
			return errNoOutpost
		} else if errors.Is(err, store.ErrConflict) {
			return httpx.Fail(http.StatusConflict, "ERR_CONFLICT", "Another outpost already has this name. Choose a different one.")
		} else if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: action, Summary: summary + o.Name, TargetType: "outpost", TargetID: o.ID, TargetLabel: o.Name})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, status, map[string]any{"outpost": outpostView{o, h.hub.Connected(o.ID)}, "token": token})
}

func (h *handlers) deleteOutpost(w http.ResponseWriter, r *http.Request, actor store.User) error {
	err := h.st.Tx(r.Context(), func(tx *store.Store) error {
		o, err := tx.DeleteOutpost(r.Context(), r.PathValue("id"))
		switch {
		case errors.Is(err, store.ErrNotFound):
			return errNoOutpost
		case errors.Is(err, store.ErrConflict):
			return httpx.Fail(http.StatusConflict, "ERR_OUTPOST_IN_USE", o.Name+" reaches "+strconv.Itoa(o.Targets)+" remote access applications. Move them to another outpost or delete them first.")
		case err != nil:
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "outpost.delete", Summary: "Deleted the outpost " + o.Name + "; its key stopped working", TargetType: "outpost", TargetID: o.ID, TargetLabel: o.Name})
	})
	if err != nil {
		return err
	}
	h.hub.Drop(r.PathValue("id"), errDeleted)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *handlers) listSessions(w http.ResponseWriter, r *http.Request, _ store.User) error {
	sessions, err := h.st.ListRemoteSessions(r.Context(), r.URL.Query().Get("app"), 200)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, sessions)
}

func (h *handlers) terminate(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var rs store.RemoteSession
	err := h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		if rs, err = tx.MarkRemoteSessionTerminated(r.Context(), r.PathValue("id"), actor.ID); errors.Is(err, store.ErrNotFound) {
			return httpx.Fail(http.StatusConflict, "ERR_SESSION_ENDED", "This remote session already ended.")
		} else if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "remote.session.terminate", Summary: "Ended the remote session of " + rs.Email + " to " + rs.Address, TargetType: "application", TargetID: cmp.Or(deref(rs.AppID), rs.ID), TargetLabel: rs.AppName})
	})
	if err != nil {
		return err
	}
	if !h.hub.Terminate(rs.ID, terminated(actor.Name)) {
		if _, err := h.st.CancelPendingRemoteSession(r.Context(), rs.ID, actor.Name+" ended the session before it connected."); err != nil {
			return err
		}
	}
	rs, err = h.st.GetRemoteSession(r.Context(), rs.ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, rs)
}

func terminated(name string) error {
	return errors.New(name + " ended the session from the Halo console.")
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (h *handlers) recording(w http.ResponseWriter, r *http.Request, actor store.User) error {
	rs, err := h.st.GetRemoteSession(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		return errNoSession
	}
	if err != nil {
		return err
	}
	compressed, err := h.st.RemoteRecording(r.Context(), rs.ID)
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("The recording of this session")
	}
	if err != nil {
		return err
	}
	cast, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return err
	}
	if err := record(r, h.st, actor, store.AuditEvent{Action: "remote.session.recording.download", Summary: "Downloaded the recording of " + rs.Email + "'s session to " + rs.Address, TargetType: "application", TargetID: cmp.Or(deref(rs.AppID), rs.ID), TargetLabel: rs.AppName}); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/x-asciicast")
	w.Header().Set("Content-Disposition", `attachment; filename="`+rs.ID+`.cast"`)
	w.Header().Set("Cache-Control", "no-store")
	_, err = io.Copy(w, cast)
	return err
}

type Settings struct {
	OutpostID    *string `json:"outpostId"`
	Protocol     *string `json:"protocol"`
	Host         *string `json:"host"`
	Port         *int    `json:"port"`
	Username     *string `json:"username"`
	Auth         *string `json:"auth"`
	Credential   *string `json:"credential"`
	HostKey      *string `json:"hostKey"`
	SessionLimit *int    `json:"sessionLimit"`
	Record       *bool   `json:"record"`
}

func NewTarget(appID string) store.RemoteTarget {
	return store.RemoteTarget{AppID: appID, Protocol: "ssh", Port: 22, Auth: "certificate", SessionLimit: 3600}
}

func (in Settings) Apply(ctx context.Context, st *store.Store, t store.RemoteTarget) (store.RemoteTarget, []string, error) {
	before, changed := t, []string{}
	for _, f := range []struct {
		value *string
		field *string
	}{{in.OutpostID, &t.OutpostID}, {in.Protocol, &t.Protocol}, {in.Host, &t.Host}, {in.Username, &t.Username}, {in.Auth, &t.Auth}, {in.HostKey, &t.HostKey}} {
		if f.value != nil {
			*f.field = strings.TrimSpace(*f.value)
		}
	}
	for _, f := range []struct {
		value *int
		field *int
	}{{in.Port, &t.Port}, {in.SessionLimit, &t.SessionLimit}} {
		if f.value != nil {
			*f.field = *f.value
		}
	}
	if in.Record != nil {
		t.Record = *in.Record
	}
	if _, err := st.GetOutpost(ctx, t.OutpostID); errors.Is(err, store.ErrNotFound) {
		return t, nil, httpx.Invalid("Choose the outpost that reaches this host. Register one on the Remote access page first.")
	} else if err != nil {
		return t, nil, err
	}
	switch {
	case t.Protocol != "ssh":
		return t, nil, httpx.Invalid(`Choose the protocol "ssh". Browser sessions over RDP and VNC are not available yet.`)
	case !hostPattern.MatchString(t.Host) && net.ParseIP(t.Host) == nil:
		return t, nil, httpx.Invalid("Enter the host name or IP address the outpost connects to, like db1.internal or 10.0.0.5, without a port or user name.")
	case t.Port < 1 || t.Port > 65535:
		return t, nil, httpx.Invalid("Enter a port between 1 and 65535. SSH usually listens on 22.")
	case t.Username != "" && !userPattern.MatchString(t.Username):
		return t, nil, httpx.Invalid("Enter the account to sign in as on the host, like ubuntu or deploy, without spaces.")
	case !slices.Contains([]string{"certificate", "password", "key"}, t.Auth):
		return t, nil, httpx.Invalid(`Choose how Halo signs in to the host: "certificate" for a short-lived certificate from Halo's SSH CA, "password" or "key" for a stored password or private key.`)
	case t.Auth != "certificate" && t.Username == "":
		return t, nil, httpx.Invalid("Enter the account Halo signs in as with the stored " + map[string]string{"password": "password", "key": "private key"}[t.Auth] + ".")
	case t.SessionLimit < 60 || t.SessionLimit > 86400:
		return t, nil, httpx.Invalid("Set a session time limit between 1 minute and 24 hours.")
	}
	if t.HostKey != "" {
		key, err := parseHostKey(t.HostKey)
		if err != nil {
			return t, nil, err
		}
		t.HostKey = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
	}
	switch {
	case t.Auth == "certificate":
		t.HasCredential = false
	case in.Credential != nil && *in.Credential != "":
		if t.Auth == "key" {
			if _, err := ssh.ParsePrivateKey([]byte(*in.Credential)); err != nil {
				return t, nil, httpx.Invalid("Paste an unencrypted OpenSSH or PEM private key, like the contents of ~/.ssh/id_ed25519. Halo can't use a key that needs a passphrase.")
			}
		}
		t.HasCredential = true
	case t.Auth != before.Auth || !t.HasCredential:
		return t, nil, httpx.Invalid("Enter the " + map[string]string{"password": "password", "key": "private key"}[t.Auth] + " Halo signs in to the host with. Halo stores it encrypted and never shows it again.")
	}
	for _, f := range []struct {
		label   string
		changed bool
	}{{"outpost", t.OutpostID != before.OutpostID}, {"host", t.Host != before.Host || t.Port != before.Port}, {"account", t.Username != before.Username}, {"sign-in method", t.Auth != before.Auth},
		{"stored credential", t.Auth != "certificate" && in.Credential != nil && *in.Credential != ""}, {"host key", t.HostKey != before.HostKey}, {"session time limit", t.SessionLimit != before.SessionLimit}, {"recording", t.Record != before.Record}} {
		if f.changed {
			changed = append(changed, f.label)
		}
	}
	return t, changed, nil
}

func parseHostKey(raw string) (ssh.PublicKey, error) {
	if strings.HasPrefix(strings.TrimSpace(raw), "@") {
		return nil, httpx.Invalid("Use an unmarked host key. Marked known_hosts entries cannot be used as pins.")
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(raw))
	if err != nil {
		var marker string
		marker, _, key, _, _, err = ssh.ParseKnownHosts([]byte(raw))
		if marker != "" {
			return nil, httpx.Invalid("Use an unmarked host key. Marked known_hosts entries cannot be used as pins.")
		}
	}
	if err != nil {
		return nil, httpx.Invalid("Paste the host's public key, like the output of ssh-keyscan -t ed25519 db1.internal or the contents of /etc/ssh/ssh_host_ed25519_key.pub.")
	}
	if _, isCert := key.(*ssh.Certificate); isCert {
		return nil, httpx.Invalid("That is a host certificate, not a host key. Halo trusts the hosts it signs certificates for already, so leave the host key empty for them.")
	}
	return key, nil
}

func (h *handlers) start(w http.ResponseWriter, r *http.Request) error {
	u, signedIn := httpx.CurrentUser(r)
	sess, cookie := httpx.CurrentSession(r)
	if !signedIn || !cookie {
		return httpx.ErrUnauthenticated
	}
	if sess.ImpersonationID != "" {
		return httpx.ErrImpersonating
	}
	var in struct {
		ApplicationID string `json:"applicationId"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	app, err := h.st.GetApplication(ctx, in.ApplicationID)
	if errors.Is(err, store.ErrNotFound) || err == nil && app.Protocol != "remote" {
		return httpx.NotFound("This remote access application")
	}
	if err != nil {
		return err
	}
	target, err := h.st.RemoteTarget(ctx, app.ID)
	if err != nil {
		return err
	}
	_, system, browser := store.ParseUserAgent(r.UserAgent())
	event := store.SignInEvent{UserID: &u.ID, Email: u.Email, AppID: &app.ID, Result: "failure", Method: sess.Method, IP: httpx.ClientIP(r), Device: browser + " · " + system}
	refuse := func(reason string, e *httpx.Error) error {
		event.Reason = reason
		if err := h.st.RecordSignIn(ctx, event); err != nil {
			return err
		}
		return e
	}
	if app.Status != "active" {
		return refuse("The application is disabled.", httpx.Fail(http.StatusForbidden, "ERR_APP_DISABLED", "Your administrator turned off "+app.Name+". Contact them if you still need it."))
	}
	if !slices.Contains(u.AppIDs, app.ID) {
		return refuse("User is not assigned to this application.", httpx.Fail(http.StatusForbidden, "ERR_NOT_ASSIGNED", "You don't have access to "+app.Name+". Ask your administrator to assign it to you, then open it again."))
	}
	if app.ReauthRequired(sess.CreatedAt) {
		return httpx.Fail(http.StatusUnauthorized, errSignInAgain, app.Name+" asks you to sign in again before you connect. Sign in, then open it again.")
	}
	decision, err := h.policy.Evaluate(ctx, access.Input{User: u, App: &app, Method: sess.Method, IP: event.IP, UserAgent: r.UserAgent(), DeviceToken: httpx.DeviceToken(r), Session: &sess})
	if err != nil {
		return err
	}
	event.ID, event.Risk = decision.EventID, decision.Risk
	reason := decision.Reason
	if decision.Policy != "" {
		reason = "Policy “" + decision.Policy + "”: " + decision.Reason
	}
	_, unmet := decision.StepUp(nil, sess)
	switch {
	case decision.Effect == access.Block:
		return refuse(reason, httpx.Fail(http.StatusForbidden, "ERR_BLOCKED_BY_POLICY", "Your organization's access policy blocked this connection. If you think this is wrong, contact your administrator and mention it: "+reason))
	case decision.Effect == access.RequirePhishingResistant && sess.Method != "passkey" && sess.Method != "security-key":
		return refuse(reason, httpx.Fail(http.StatusForbidden, "ERR_STRONGER_AUTH_REQUIRED", app.Name+" needs a passkey or security key. Sign out of Halo, sign in again with one, then open "+app.Name+" again."))
	case len(decision.Terms) > 0:
		return httpx.Fail(http.StatusForbidden, "ERR_TERMS_REQUIRED", "Accept your organization's terms of use before you connect to "+app.Name+".")
	case unmet != nil:
		reason := "The session does not meet the authentication context “" + unmet.Name + "”."
		return refuse(reason, httpx.Fail(http.StatusForbidden, "ERR_STRONGER_AUTH_REQUIRED", app.Name+" needs a stronger sign-in. Sign out of Halo first. "+unmet.SignInAgain()+" Then open "+app.Name+" again."))
	}
	username := target.Username
	if target.Auth == "certificate" {
		principals, err := sshca.Principals(ctx, h.st, u)
		if err != nil {
			return err
		}
		if len(principals) == 0 {
			return refuse("The person has no SSH principals.", httpx.Fail(http.StatusForbidden, "ERR_NO_PRINCIPALS", "None of your groups maps to a server account, so Halo can't sign you in to "+app.Name+". Ask a security administrator to add an SSH principal mapping for one of your groups."))
		}
		username = cmp.Or(username, loginName(u, principals))
	}
	if !h.hub.Connected(target.OutpostID) {
		return httpx.Fail(http.StatusServiceUnavailable, "ERR_OUTPOST_OFFLINE", "The outpost that reaches "+app.Name+" is not connected to Halo, so Halo can't open a connection. Try again later, or ask your administrator to check the outpost.")
	}
	event.Result = "success"
	var rs store.RemoteSession
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.RecordSignIn(ctx, event); err != nil {
			return err
		}
		rs, err = tx.CreateRemoteSession(ctx, store.RemoteSession{AppID: &app.ID, AppName: app.Name, UserID: &u.ID, Email: u.Email, SessionID: sess.ID, OutpostID: &target.OutpostID, Protocol: target.Protocol,
			Address: net.JoinHostPort(target.Host, strconv.Itoa(target.Port)), Username: username, IP: event.IP, LimitSeconds: target.SessionLimit, Record: target.Record})
		return err
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, rs)
}

func loginName(u store.User, principals []string) string {
	for _, p := range principals {
		if !strings.Contains(p, ":") {
			return p
		}
	}
	if u.POSIX != nil {
		return u.POSIX.Username
	}
	return principals[0]
}

func limitText(seconds int) string {
	d := time.Duration(seconds) * time.Second
	if d < time.Minute {
		return plural(seconds, "second")
	}
	if d%time.Hour == 0 {
		return plural(int(d/time.Hour), "hour")
	}
	return plural(int(d.Round(time.Minute)/time.Minute), "minute")
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}
