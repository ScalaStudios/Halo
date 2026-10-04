package store

import (
	"context"
	"time"

	"halo/internal/secret"
)

type MagicLink struct {
	UserID      string
	AuthRequest string
	ExpiresAt   time.Time
	UsedAt      *time.Time
}

func (s *Store) CreateMagicLink(ctx context.Context, userID, authRequest string, expires time.Time) (string, error) {
	token := secret.Token(32)
	_, err := s.db.Exec(ctx, `insert into magic_links (token_hash, user_id, auth_request, expires_at) values ($1, $2, nullif($3, ''), $4)`,
		secret.Hash(token), userID, authRequest, expires)
	return token, err
}

func (s *Store) CountMagicLinks(ctx context.Context, userID string, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `select count(*) from magic_links where user_id = $1 and created_at > $2`, userID, since).Scan(&n)
	return n, err
}

func (s *Store) GetMagicLink(ctx context.Context, token string) (MagicLink, error) {
	var l MagicLink
	err := s.db.QueryRow(ctx, `select user_id, coalesce(auth_request, ''), expires_at, used_at from magic_links where token_hash = $1`, secret.Hash(token)).
		Scan(&l.UserID, &l.AuthRequest, &l.ExpiresAt, &l.UsedAt)
	return l, notFound(err)
}

func (s *Store) UseMagicLink(ctx context.Context, token string) error {
	tag, err := s.db.Exec(ctx, `update magic_links set used_at = now() where token_hash = $1 and used_at is null and expires_at > now()`, secret.Hash(token))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func MagicLinkPath(token string) string {
	return "/sign-in/magic?token=" + token
}
