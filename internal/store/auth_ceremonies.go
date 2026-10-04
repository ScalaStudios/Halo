package store

import (
	"context"
	"time"

	"halo/internal/id"
)

const CeremonyLifetime = 5 * time.Minute

func (s *Store) CreateCeremony(ctx context.Context, kind string, userID *string, data any) (string, error) {
	if _, err := s.db.Exec(ctx, `delete from auth_ceremonies where expires_at < now()`); err != nil {
		return "", err
	}
	ceremonyID := id.New("cer")
	_, err := s.db.Exec(ctx, `insert into auth_ceremonies (id, kind, user_id, data, expires_at) values ($1, $2, $3, $4, $5)`,
		ceremonyID, kind, userID, data, time.Now().Add(CeremonyLifetime))
	return ceremonyID, err
}

func (s *Store) TakeCeremony(ctx context.Context, ceremonyID, kind string, data any) (*string, error) {
	var userID *string
	err := s.db.QueryRow(ctx, `delete from auth_ceremonies where id = $1 and kind = $2 and expires_at > now() returning user_id, data`, ceremonyID, kind).Scan(&userID, data)
	return userID, notFound(err)
}
