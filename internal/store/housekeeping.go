package store

import "context"

func (s *Store) Purge(ctx context.Context) error {
	for _, sql := range []string{
		`delete from auth_ceremonies where expires_at < now()`,
		`delete from oidc_auth_requests where expires_at < now() - interval '1 day'`,
		`delete from oidc_tokens where expires_at < now() - interval '7 days'`,
		`delete from sessions where coalesce(revoked_at, expires_at) < now() - interval '30 days'`,
		`delete from enrollment_tokens where expires_at < now() - interval '30 days'`,
		`delete from mail_outbox where created_at < now() - interval '30 days' and (sent_at is not null or attempts >= 5)`,
		`delete from magic_links where expires_at < now() - interval '1 day'`,
	} {
		if _, err := s.db.Exec(ctx, sql); err != nil {
			return err
		}
	}
	return nil
}
