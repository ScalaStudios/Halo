package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

type TOTPSecret struct {
	ID        string
	Sealed    []byte
	Confirmed bool
	LastStep  int64
}

func (s *Store) CreateTOTPSecret(ctx context.Context, userID, label string, sealed []byte) (string, error) {
	if _, err := s.db.Exec(ctx, `delete from totp_secrets where user_id = $1 and not confirmed`, userID); err != nil {
		return "", err
	}
	secretID := id.New("mth")
	_, err := s.db.Exec(ctx, `insert into totp_secrets (id, user_id, label, secret_sealed) values ($1, $2, $3, $4)`, secretID, userID, label, sealed)
	return secretID, err
}

func (s *Store) ListTOTPSecrets(ctx context.Context, userID string) ([]TOTPSecret, error) {
	rows, err := s.db.Query(ctx, `select id, secret_sealed, confirmed, last_step from totp_secrets where user_id = $1 order by created_at`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (TOTPSecret, error) {
		var t TOTPSecret
		return t, row.Scan(&t.ID, &t.Sealed, &t.Confirmed, &t.LastStep)
	})
}

func (s *Store) ConfirmTOTPSecret(ctx context.Context, secretID, label string, step int64) (Method, error) {
	m := Method{ID: secretID, Kind: "totp", Label: label}
	err := s.db.QueryRow(ctx, `update totp_secrets set confirmed = true, label = $2, last_step = $3 where id = $1 and not confirmed returning created_at`, secretID, label, step).Scan(&m.AddedAt)
	return m, notFound(err)
}

func (s *Store) AdvanceTOTP(ctx context.Context, secretID string, step int64) (bool, error) {
	tag, err := s.db.Exec(ctx, `update totp_secrets set last_step = $2, last_used_at = now() where id = $1 and last_step < $2`, secretID, step)
	return tag.RowsAffected() == 1, err
}

func (s *Store) RecentFailures(ctx context.Context, userID, email string, methods []string, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `select count(*) from sign_in_events where result = 'failure' and method = any($3) and time > $4
		and (user_id = $1 or (user_id is null and lower(email) = lower($2)))`, userID, email, methods, since).Scan(&n)
	return n, err
}
