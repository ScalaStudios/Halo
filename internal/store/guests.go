package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type GuestExpiry struct {
	Guest     NamedRef
	SponsorID *string
	ExpiresAt time.Time
}

func (s *Store) SetGuestAccess(ctx context.Context, userID, sponsorID string, expires time.Time) error {
	tag, err := s.db.Exec(ctx, `update users set sponsor_id = $2, expires_at = $3, expiry_notice_at = case when expires_at = $3 then expiry_notice_at end, updated_at = now()
		where id = $1 and guest`, userID, sponsorID, expires)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ClaimGuestExpiryNotices(ctx context.Context, now, before time.Time) ([]GuestExpiry, error) {
	return s.guestExpiries(ctx, `update users set expiry_notice_at = $1 where guest and expiry_notice_at is null and expires_at > $1 and expires_at <= $2
		and status in ('active', 'invited') returning id, name, email, sponsor_id, expires_at`, now, before)
}

func (s *Store) ClaimExpiredGuests(ctx context.Context, now time.Time) ([]GuestExpiry, error) {
	expired, err := s.guestExpiries(ctx, `update users set status = 'suspended', updated_at = $1 where guest and expires_at <= $1 and status in ('active', 'invited')
		returning id, name, email, sponsor_id, expires_at`, now)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(expired))
	for i, g := range expired {
		ids[i] = g.Guest.ID
	}
	return expired, s.signal(ctx, accountDisabled, nil, `select id, '', '', '' from users where id = any($1)`, ids)
}

func (s *Store) guestExpiries(ctx context.Context, query string, args ...any) ([]GuestExpiry, error) {
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (GuestExpiry, error) {
		var g GuestExpiry
		return g, row.Scan(&g.Guest.ID, &g.Guest.Name, &g.Guest.Email, &g.SponsorID, &g.ExpiresAt)
	})
}
