package store

import (
	"cmp"
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

const (
	LockedOutReason = "Too many failed attempts."
	SprayedReason   = "Too many failed passwords from this IP address."
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
	var userID string
	if err := s.db.QueryRow(ctx, `update totp_secrets set confirmed = true, label = $2, last_step = $3 where id = $1 and not confirmed returning created_at, user_id`, secretID, label, step).Scan(&m.AddedAt, &userID); err != nil {
		return m, notFound(err)
	}
	return m, s.credentialChanged(ctx, userID, "totp", "create", label)
}

func (s *Store) AdvanceTOTP(ctx context.Context, secretID string, step int64) (bool, error) {
	tag, err := s.db.Exec(ctx, `update totp_secrets set last_step = $2, last_used_at = now() where id = $1 and last_step < $2`, secretID, step)
	return tag.RowsAffected() == 1, err
}

func (s *Store) RecentFailures(ctx context.Context, userID, email string, methods []string, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `select count(*) from sign_in_events where result = 'failure' and challenge not in ('failed', 'unavailable') and hook <> 'failed_closed' and method = any($3) and time > $4
		and reason not in ($5, $6) and (user_id = $1 or (user_id is null and lower(email) = lower($2)))`, userID, email, methods, since, LockedOutReason, SprayedReason).Scan(&n)
	return n, err
}

func (s *Store) RecentFailuresFromIP(ctx context.Context, ip string, methods []string, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `select count(*) from sign_in_events where ip = $1 and result = 'failure' and challenge <> 'unavailable' and hook <> 'failed_closed' and method = any($2) and time > $3
		and reason not in ($4, $5)`, ip, methods, since, LockedOutReason, SprayedReason).Scan(&n)
	return n, err
}

func (s *Store) RecentMismatches(ctx context.Context, userID, ip string, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `select count(*) from sign_in_events where user_id = $1 and ip = $2 and mismatch and time > $3`, userID, ip, since).Scan(&n)
	return n, err
}

func (s *Store) ReserveAttempt(ctx context.Context, userID, email string, methods []string, since time.Time, threshold int) (string, error) {
	subject := cmp.Or(userID, "email:"+strings.ToLower(email))
	reservation := ""
	err := s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.db.Exec(ctx, `select pg_advisory_xact_lock(hashtextextended('halo:sign-in:' || $1, 0))`, subject); err != nil {
			return err
		}
		failures, err := tx.RecentFailures(ctx, userID, email, methods, since)
		if err != nil {
			return err
		}
		var pending int
		if err := tx.db.QueryRow(ctx, `select count(*) from sign_in_attempts where subject = $1 and time > $2`, subject, since).Scan(&pending); err != nil {
			return err
		}
		if failures+pending >= threshold {
			return nil
		}
		reservation = id.New("att")
		_, err = tx.db.Exec(ctx, `insert into sign_in_attempts (id, subject) values ($1, $2)`, reservation, subject)
		return err
	})
	return reservation, err
}

func (s *Store) ReleaseAttempt(ctx context.Context, reservation string) error {
	_, err := s.db.Exec(ctx, `delete from sign_in_attempts where id = $1`, reservation)
	return err
}
