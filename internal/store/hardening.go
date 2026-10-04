package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
	"halo/internal/secret"
)

func (s *Store) MarkEmailVerified(ctx context.Context, userID string) error {
	_, err := s.db.Exec(ctx, `update users set email_verified = true where id = $1`, userID)
	return err
}

func (s *Store) MarkEnrollmentEmailed(ctx context.Context, token string) error {
	_, err := s.db.Exec(ctx, `update enrollment_tokens set emailed = true where token_hash = $1`, secret.Hash(token))
	return err
}

func (s *Store) RevokeDeviceSessions(ctx context.Context, userID string, deviceHash []byte) (int, error) {
	return s.revokeSessions(ctx, `user_id = $1 and device_hash = $2`, userID, deviceHash)
}

func (s *Store) revokeSessions(ctx context.Context, where string, args ...any) (int, error) {
	rows, err := s.db.Query(ctx, `update sessions set revoked_at = now() where revoked_at is null and `+where+` returning id`, args...)
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, err
	}
	return len(ids), s.revokeTokensForSessions(ctx, ids)
}

func (s *Store) CreateDeviceSession(ctx context.Context, deviceCodeHash []byte, expires time.Time) (string, error) {
	sessionID := id.New("ses")
	err := s.db.QueryRow(ctx, `insert into sessions (id, token_hash, user_id, method, ip, user_agent, client_id, expires_at)
		select $1, $2, user_id, method, ip, user_agent, client_id, $4 from device_authorizations where device_code_hash = $3 and user_id is not null and method is not null
		returning id`, sessionID, secret.Hash(secret.Token(32)), deviceCodeHash, expires).Scan(&sessionID)
	return sessionID, notFound(err)
}

func (s *Store) TouchClientSession(ctx context.Context, sessionID string, expires time.Time) error {
	_, err := s.db.Exec(ctx, `update sessions set last_active_at = now(), expires_at = greatest(expires_at, $2) where id = $1 and client_id is not null and revoked_at is null`, sessionID, expires)
	return err
}

func (s *Store) EndClientSession(ctx context.Context, sessionID string) error {
	_, err := s.revokeSessions(ctx, `id = $1 and client_id is not null`, sessionID)
	return err
}

func (s *Store) CountDeviceAuthorizations(ctx context.Context, ip string, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `select count(*) from device_authorizations where ip = $1 and created_at > $2`, ip, since).Scan(&n)
	return n, err
}
