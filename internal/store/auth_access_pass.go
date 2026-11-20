package store

import (
	"context"
	"crypto/rand"
	"strings"
	"time"

	"halo/internal/secret"
)

const AccessPassEnrollLifetime = 15 * time.Minute

func (s *Store) CreateAccessPass(ctx context.Context, userID, createdBy string, uses int, expires time.Time) (string, error) {
	raw := strings.ToLower(rand.Text()[:16])
	pass := raw[:4] + "-" + raw[4:8] + "-" + raw[8:12] + "-" + raw[12:]
	_, err := s.db.Exec(ctx, `insert into access_passes (user_id, pass_hash, uses_left, expires_at, created_by) values ($1, $2, $3, $4, nullif($5, ''))
		on conflict (user_id) do update set pass_hash = excluded.pass_hash, uses_left = excluded.uses_left, expires_at = excluded.expires_at,
			created_by = excluded.created_by, created_at = now(), last_used_at = null`,
		userID, s.accessPassHash(pass), uses, expires, createdBy)
	return pass, err
}

func (s *Store) UseAccessPass(ctx context.Context, userID, pass string) (bool, error) {
	var hash []byte
	err := s.db.QueryRow(ctx, `select pass_hash from access_passes where user_id = $1 and uses_left > 0 and expires_at > now()`, userID).Scan(&hash)
	if err != nil {
		return false, notFound(err)
	}
	if !secret.Equal(hash, s.accessPassHash(pass)) {
		return false, nil
	}
	tag, err := s.db.Exec(ctx, `update access_passes set uses_left = uses_left - 1, last_used_at = now() where user_id = $1 and pass_hash = $2 and uses_left > 0 and expires_at > now()`, userID, hash)
	return tag.RowsAffected() == 1, err
}

func (s *Store) accessPassHash(pass string) []byte {
	return secret.MAC(s.codeKey, "access-pass:"+strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(pass))))
}
