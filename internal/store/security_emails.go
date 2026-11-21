package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) ClaimSecurityEmail(ctx context.Context, userID, kind, subject string, now time.Time, window time.Duration) (bool, error) {
	var claimed bool
	err := s.db.QueryRow(ctx, `insert into security_email_sends (user_id, kind, subject, sent_at)
		select $1, $2, $3, $4 where not exists (select 1 from security_email_opt_outs where user_id = $1 and kind = $2)
		on conflict (user_id, kind, subject) do update set sent_at = excluded.sent_at where security_email_sends.sent_at <= $5
		returning true`, userID, kind, subject, now, now.Add(-window)).Scan(&claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return claimed, err
}

func (s *Store) SecurityEmailOptOuts(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `select kind from security_email_opt_outs where user_id = $1 order by kind`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func (s *Store) SetSecurityEmailOptOuts(ctx context.Context, userID string, kinds []string) error {
	return s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.db.Exec(ctx, `delete from security_email_opt_outs where user_id = $1`, userID); err != nil {
			return err
		}
		_, err := tx.db.Exec(ctx, `insert into security_email_opt_outs (user_id, kind) select $1, unnest($2::text[]) on conflict do nothing`, userID, kinds)
		return err
	})
}
