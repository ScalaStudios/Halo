package store

import "context"

type Password struct {
	Hash       string
	MustChange bool
}

func (s *Store) GetPassword(ctx context.Context, userID string) (Password, error) {
	var p Password
	err := s.db.QueryRow(ctx, `select hash, must_change from passwords where user_id = $1`, userID).Scan(&p.Hash, &p.MustChange)
	return p, notFound(err)
}

func (s *Store) SetPassword(ctx context.Context, userID, hash string, mustChange bool) error {
	_, err := s.db.Exec(ctx, `insert into passwords (user_id, hash, must_change) values ($1, $2, $3)
		on conflict (user_id) do update set hash = excluded.hash, must_change = excluded.must_change, created_at = now(), last_used_at = null`, userID, hash, mustChange)
	return err
}

func (s *Store) TouchPassword(ctx context.Context, userID string) error {
	_, err := s.db.Exec(ctx, `update passwords set last_used_at = now() where user_id = $1`, userID)
	return err
}
