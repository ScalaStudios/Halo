package store

import (
	"context"

	"halo/internal/id"
)

func (s *Store) ReplaceRecoveryCodes(ctx context.Context, userID string, hashes [][]byte) error {
	return s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.db.Exec(ctx, `delete from recovery_codes where user_id = $1`, userID); err != nil {
			return err
		}
		for _, hash := range hashes {
			if _, err := tx.db.Exec(ctx, `insert into recovery_codes (id, user_id, code_hash) values ($1, $2, $3)`, id.New("rec"), userID, hash); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) UseRecoveryCode(ctx context.Context, userID string, hash []byte) error {
	tag, err := s.db.Exec(ctx, `update recovery_codes set used_at = now() where user_id = $1 and code_hash = $2 and used_at is null`, userID, hash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
