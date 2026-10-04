package store

import (
	"context"
	"time"

	"halo/internal/secret"
)

const EnrollmentLifetime = 7 * 24 * time.Hour

func (s *Store) CreateEnrollmentToken(ctx context.Context, userID, purpose string, createdBy *string) (string, time.Time, error) {
	settings, err := s.Settings(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	token := secret.Token(32)
	expires := time.Now().Add(time.Duration(settings.InviteDays) * 24 * time.Hour)
	_, err = s.db.Exec(ctx, `update enrollment_tokens set used_at = now() where user_id = $1 and used_at is null`, userID)
	if err != nil {
		return "", time.Time{}, err
	}
	_, err = s.db.Exec(ctx, `insert into enrollment_tokens (token_hash, user_id, purpose, created_by, expires_at) values ($1, $2, $3, $4, $5)`,
		secret.Hash(token), userID, purpose, createdBy, expires)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func (s *Store) PeekEnrollmentToken(ctx context.Context, token string) (User, string, error) {
	var userID, purpose string
	err := s.db.QueryRow(ctx, `select user_id, purpose from enrollment_tokens where token_hash = $1 and used_at is null and expires_at > now()`, secret.Hash(token)).Scan(&userID, &purpose)
	if err != nil {
		return User{}, "", notFound(err)
	}
	u, err := s.GetUser(ctx, userID)
	return u, purpose, err
}

func (s *Store) ConsumeEnrollmentToken(ctx context.Context, token string) (string, bool, error) {
	var userID string
	var emailed bool
	err := s.db.QueryRow(ctx, `update enrollment_tokens set used_at = now() where token_hash = $1 and used_at is null and expires_at > now() returning user_id, emailed`, secret.Hash(token)).Scan(&userID, &emailed)
	if err != nil {
		return "", false, notFound(err)
	}
	return userID, emailed, nil
}

func EnrollPath(token string) string {
	return "/enroll?token=" + token
}
