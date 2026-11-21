package store

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
	"halo/internal/secret"
)

const (
	SessionLifetime    = 12 * time.Hour
	SessionLimitReason = "Reached the session limit."
	SessionEndedReason = "Ended the oldest session to stay within the session limit."
)

var ErrSessionLimit = errors.New("session limit reached")

type NewSession struct {
	UserID       string
	Method       string
	SecondFactor bool
	IP           string
	Location     string
	UserAgent    string
	DeviceHash   []byte
}

func (s *Store) CreateSession(ctx context.Context, in NewSession) (Session, string, error) {
	settings, err := s.Settings(ctx)
	if err != nil {
		return Session{}, "", err
	}
	token := secret.Token(32)
	now := time.Now()
	sess := Session{
		ID:           id.New("ses"),
		UserID:       in.UserID,
		IP:           in.IP,
		Location:     in.Location,
		Method:       in.Method,
		SecondFactor: in.SecondFactor,
		CreatedAt:    now,
		LastActiveAt: now,
		ExpiresAt:    now.Add(settings.SessionLifetime()),
	}
	sess.Device, sess.OS, sess.Browser = ParseUserAgent(in.UserAgent)
	_, err = s.db.Exec(ctx, `insert into sessions (id, token_hash, user_id, method, second_factor, ip, location, user_agent, created_at, last_active_at, expires_at, device_hash) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9, $10, $11)`,
		sess.ID, secret.Hash(token), in.UserID, in.Method, in.SecondFactor, in.IP, in.Location, in.UserAgent, now, sess.ExpiresAt, in.DeviceHash)
	if err != nil {
		return Session{}, "", err
	}
	return sess, token, nil
}

const sessionColumns = `id, user_id, method, second_factor, ip, location, user_agent, created_at, last_active_at, expires_at, revoked_at, coalesce(client_id, ''), coalesce(impersonation_id, ''), reauthenticated_at`

func scanSession(row rowScanner) (Session, error) {
	var sess Session
	var ua string
	err := row.Scan(&sess.ID, &sess.UserID, &sess.Method, &sess.SecondFactor, &sess.IP, &sess.Location, &ua, &sess.CreatedAt, &sess.LastActiveAt, &sess.ExpiresAt, &sess.RevokedAt, &sess.Client, &sess.ImpersonationID, &sess.ReauthenticatedAt)
	sess.Device, sess.OS, sess.Browser = ParseUserAgent(ua)
	return sess, err
}

func (s Session) AMR() []string {
	switch {
	case s.Method == "passkey" || s.Method == "security-key":
		return []string{"hwk", "mfa"}
	case s.Method == "federated":
		return []string{"fed"}
	case s.Method == "certificate":
		return []string{"swk"}
	case s.Method == "password" && s.SecondFactor:
		return []string{"pwd", "otp", "mfa"}
	case s.Method == "password":
		return []string{"pwd"}
	}
	return []string{"otp"}
}

func (s Session) StrongSignInAt() time.Time {
	if s.ReauthenticatedAt != nil {
		return *s.ReauthenticatedAt
	}
	if s.Client == "" && slices.Contains([]string{"passkey", "security-key", "recovery-codes"}, s.Method) {
		return s.CreatedAt
	}
	return time.Time{}
}

func (s *Store) MarkReauthenticated(ctx context.Context, sessionID string) error {
	_, err := s.db.Exec(ctx, `update sessions set reauthenticated_at = now() where id = $1`, sessionID)
	return err
}

func (s *Store) SessionByToken(ctx context.Context, token string) (Session, error) {
	sess, err := scanSession(s.db.QueryRow(ctx, `select `+sessionColumns+` from sessions where token_hash = $1 and revoked_at is null and expires_at > now()`, secret.Hash(token)))
	if err != nil {
		return Session{}, notFound(err)
	}
	if time.Since(sess.LastActiveAt) > time.Minute {
		_, _ = s.db.Exec(ctx, `update sessions set last_active_at = now() where id = $1`, sess.ID)
	}
	return sess, nil
}

func (s *Store) ListSessions(ctx context.Context, userID string) ([]Session, error) {
	query := `select ` + sessionColumns + ` from sessions where revoked_at is null and expires_at > now()`
	args := []any{}
	if userID != "" {
		query += ` and user_id = $1`
		args = append(args, userID)
	}
	rows, err := s.db.Query(ctx, query+` order by last_active_at desc`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := []Session{}
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

func (s *Store) RevokeSession(ctx context.Context, sessionID, userID string) error {
	query := `update sessions set revoked_at = now() where id = $1 and revoked_at is null`
	args := []any{sessionID}
	if userID != "" {
		query += ` and user_id = $2`
		args = append(args, userID)
	}
	tag, err := s.db.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return s.revokeTokensForSessions(ctx, []string{sessionID})
}

func (s *Store) MakeRoomForSession(ctx context.Context, u User, cli bool, replacing string) error {
	settings, err := s.Settings(ctx)
	if err != nil {
		return err
	}
	limit := settings.SessionLimitFor(u)
	if limit == 0 || cli && !settings.SessionLimitCLI {
		return nil
	}
	if _, err := s.db.Exec(ctx, `select 1 from users where id = $1 for update`, u.ID); err != nil {
		return err
	}
	if _, err := s.revokeSessions(ctx, `id = $1 and user_id = $2`, replacing, u.ID); err != nil {
		return err
	}
	rows, err := s.db.Query(ctx, `select `+sessionColumns+` from sessions where user_id = $1 and (client_id is not null) = $2 and impersonation_id is null and revoked_at is null and expires_at > now() order by created_at`, u.ID, cli)
	if err != nil {
		return err
	}
	sessions, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Session, error) { return scanSession(row) })
	if err != nil || len(sessions) < limit {
		return err
	}
	if settings.SessionLimitAction != "end-oldest" {
		return ErrSessionLimit
	}
	ended := sessions[:len(sessions)-limit+1]
	ids := make([]string, len(ended))
	for i, sess := range ended {
		ids[i] = sess.ID
	}
	if _, err := s.revokeSessions(ctx, `id = any($1)`, ids); err != nil {
		return err
	}
	for _, sess := range ended {
		err := s.RecordSignIn(ctx, SignInEvent{Result: "ended", Method: sess.Method, UserID: &u.ID, Email: u.Email, IP: sess.IP, Location: sess.Location, Device: sess.Browser + " · " + sess.OS, Reason: SessionEndedReason})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) RevokeUserSessions(ctx context.Context, userID, exceptSessionID string) (int, error) {
	return s.revokeSessions(ctx, `user_id = $1 and id <> $2`, userID, exceptSessionID)
}

func (s *Store) revokeTokensForSessions(ctx context.Context, sessionIDs []string) error {
	if len(sessionIDs) == 0 {
		return nil
	}
	if err := s.queueLogouts(ctx, `s.id = any($1)`, sessionIDs); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `update oidc_tokens set revoked_at = now() where session_id = any($1) and revoked_at is null`, sessionIDs)
	return err
}

func (s *Store) QueueExpiredSessionLogouts(ctx context.Context) error {
	return s.queueLogouts(ctx, `s.revoked_at is null and s.client_id is null and s.expires_at <= now() and s.expires_at > now() - interval '1 hour'`)
}

func (s *Store) queueLogouts(ctx context.Context, where string, args ...any) error {
	rows, err := s.db.Query(ctx, `select distinct a.id, s.id, jsonb_build_object('sub', s.user_id, 'sid', s.id)::text from sessions s
		join oidc_tokens t on t.session_id = s.id join applications a on a.client_id = t.client_id where a.backchannel_logout_uri <> '' and `+where, args...)
	if err != nil {
		return err
	}
	logouts, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct{ AppID, SessionID, Body string }])
	if err != nil {
		return err
	}
	for _, l := range logouts {
		if _, err := s.db.Exec(ctx, `insert into webhook_deliveries (id, app_id, event_id, event_type, body) values ($1, $2, $3, 'backchannel_logout', $4) on conflict (app_id, event_id) do nothing`,
			id.New("whd"), l.AppID, l.SessionID, l.Body); err != nil {
			return err
		}
	}
	return nil
}

var (
	browserPattern = regexp.MustCompile(`(Firefox|Edg|Chrome|kubelogin)/v?(\d+(?:\.\d+)?)`)
	safariPattern  = regexp.MustCompile(`Version/(\d+(?:\.\d+)?).*Safari/`)
)

var cliPlatforms = map[string][2]string{"linux": {"Linux computer", "Linux"}, "darwin": {"Mac", "macOS"}, "windows": {"Windows PC", "Windows"}}

func ParseUserAgent(ua string) (device, os, browser string) {
	if platform, ok := strings.CutPrefix(ua, "Halo CLI ("); ok {
		p, found := cliPlatforms[strings.Split(platform, "/")[0]]
		if !found {
			p = [2]string{"Unknown device", "Unknown"}
		}
		return p[0], p[1], "Halo CLI"
	}
	switch {
	case strings.Contains(ua, "iPhone"):
		device, os = "iPhone", "iOS"
	case strings.Contains(ua, "iPad"):
		device, os = "iPad", "iPadOS"
	case strings.Contains(ua, "Android"):
		device, os = "Android phone", "Android"
	case strings.Contains(ua, "Mac OS X"):
		device, os = "Mac", "macOS"
	case strings.Contains(ua, "Windows"):
		device, os = "Windows PC", "Windows"
	case strings.Contains(ua, "Linux"):
		device, os = "Linux computer", "Linux"
	default:
		device, os = "Unknown device", "Unknown"
	}
	names := map[string]string{"Firefox": "Firefox", "Edg": "Edge", "Chrome": "Chrome", "kubelogin": "kubelogin"}
	found := map[string]string{}
	for _, m := range browserPattern.FindAllStringSubmatch(ua, -1) {
		found[names[m[1]]] = m[2]
	}
	for _, name := range []string{"kubelogin", "Firefox", "Edge", "Chrome"} {
		if version, ok := found[name]; ok {
			if name != "kubelogin" {
				version = strings.Split(version, ".")[0]
			}
			return device, os, name + " " + version
		}
	}
	if m := safariPattern.FindStringSubmatch(ua); m != nil {
		return device, os, "Safari " + strings.Split(m[1], ".")[0]
	}
	return device, os, "Unknown browser"
}
