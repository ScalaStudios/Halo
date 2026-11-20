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

func (s *Store) CreateMagicLink(ctx context.Context, userID, authRequest, code string, expires time.Time) (string, error) {
	token := secret.Token(32)
	var codeHash []byte
	if code != "" {
		codeHash = s.emailCodeHash(code)
	}
	_, err := s.db.Exec(ctx, `insert into magic_links (token_hash, user_id, auth_request, code_hash, expires_at) values ($1, $2, nullif($3, ''), $4, $5)`,
		secret.Hash(token), userID, authRequest, codeHash, expires)
	return token, err
}

func (s *Store) UseEmailCode(ctx context.Context, userID, code string, attempts int) (bool, error) {
	var tokenHash, codeHash []byte
	err := s.db.QueryRow(ctx, `update magic_links set attempts = attempts + 1
		where token_hash = (select token_hash from magic_links where user_id = $1 and code_hash is not null order by created_at desc limit 1)
			and code_used_at is null and expires_at > now() and attempts < $2
		returning token_hash, code_hash`, userID, attempts).Scan(&tokenHash, &codeHash)
	if err != nil {
		return false, notFound(err)
	}
	if !secret.Equal(codeHash, s.emailCodeHash(code)) {
		return false, nil
	}
	tag, err := s.db.Exec(ctx, `update magic_links set code_used_at = now() where token_hash = $1 and code_used_at is null`, tokenHash)
	return tag.RowsAffected() == 1, err
}

func (s *Store) emailCodeHash(code string) []byte {
	return secret.MAC(s.codeKey, "email-code:"+code)
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
