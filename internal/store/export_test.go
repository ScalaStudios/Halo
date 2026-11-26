package store

import "context"

func (s *Store) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := s.db.Exec(ctx, sql, args...)
	return err
}
