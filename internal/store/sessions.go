package store

import (
	"context"
	"regexp"
	"strings"
	"time"

	"halo/internal/id"
	"halo/internal/secret"
)

const SessionLifetime = 12 * time.Hour

type NewSession struct {
	UserID     string
	Method     string
	IP         string
	Location   string
	UserAgent  string
	DeviceHash []byte
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
		CreatedAt:    now,
		LastActiveAt: now,
		ExpiresAt:    now.Add(settings.SessionLifetime()),
	}
	sess.Device, sess.OS, sess.Browser = ParseUserAgent(in.UserAgent)
	_, err = s.db.Exec(ctx, `insert into sessions (id, token_hash, user_id, method, ip, location, user_agent, created_at, last_active_at, expires_at, device_hash) values ($1, $2, $3, $4, $5, $6, $7, $8, $8, $9, $10)`,
		sess.ID, secret.Hash(token), in.UserID, in.Method, in.IP, in.Location, in.UserAgent, now, sess.ExpiresAt, in.DeviceHash)
	if err != nil {
		return Session{}, "", err
	}
	return sess, token, nil
}

const sessionColumns = `id, user_id, method, ip, location, user_agent, created_at, last_active_at, expires_at, revoked_at, coalesce(client_id, '')`

func scanSession(row rowScanner) (Session, error) {
	var sess Session
	var ua string
	err := row.Scan(&sess.ID, &sess.UserID, &sess.Method, &sess.IP, &sess.Location, &ua, &sess.CreatedAt, &sess.LastActiveAt, &sess.ExpiresAt, &sess.RevokedAt, &sess.Client)
	sess.Device, sess.OS, sess.Browser = ParseUserAgent(ua)
	return sess, err
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

func (s *Store) RevokeUserSessions(ctx context.Context, userID, exceptSessionID string) (int, error) {
	return s.revokeSessions(ctx, `user_id = $1 and id <> $2`, userID, exceptSessionID)
}

func (s *Store) revokeTokensForSessions(ctx context.Context, sessionIDs []string) error {
	if len(sessionIDs) == 0 {
		return nil
	}
	_, err := s.db.Exec(ctx, `update oidc_tokens set revoked_at = now() where session_id = any($1) and revoked_at is null`, sessionIDs)
	return err
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
