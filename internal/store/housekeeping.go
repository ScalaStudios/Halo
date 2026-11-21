package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) Purge(ctx context.Context) error {
	for _, sql := range []string{
		`delete from auth_ceremonies where expires_at < now()`,
		`delete from oidc_auth_requests where expires_at < now() - interval '1 day'`,
		`delete from oidc_tokens where expires_at < now() - interval '7 days'`,
		`delete from sessions where coalesce(revoked_at, expires_at) < now() - interval '30 days'`,
		`delete from enrollment_tokens where expires_at < now() - interval '30 days'`,
		`delete from mail_outbox where created_at < now() - interval '30 days' and (sent_at is not null or attempts >= 5)`,
		`delete from webhook_deliveries where app_id is not null and created_at < now() - interval '30 days'`,
		`delete from magic_links where expires_at < now() - interval '1 day'`,
		`delete from rate_limit_windows where window_start < now() - interval '1 day'`,
		`delete from sign_ups where verified_at is null and expires_at < now()`,
		`delete from api_keys where credential_id is not null and expires_at < now()`,
		`delete from saml_assertions where expires_at < now()`,
		`delete from forward_auth_sessions where expires_at < now()`,
	} {
		if _, err := s.db.Exec(ctx, sql); err != nil {
			return err
		}
	}
	return s.PruneLogs(ctx, 5000)
}

func (s *Store) PruneLogs(ctx context.Context, batch int) error {
	settings, err := s.CurrentSettings(ctx)
	if err != nil || settings.SignInLogDays == 0 && settings.AuditLogDays == 0 {
		return err
	}
	tag, err := s.db.Exec(ctx, `insert into settings (key, value) values ('logsPrunedAt', to_jsonb(now()))
		on conflict (key) do update set value = excluded.value, updated_at = now() where settings.updated_at <= now() - interval '1 day'`)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	var deleted []string
	for _, l := range []struct {
		table, label string
		days         int
	}{{"sign_in_events", "the sign-in log", settings.SignInLogDays}, {"audit_events", "the audit log", settings.AuditLogDays}} {
		if l.days == 0 {
			continue
		}
		cutoff, n := time.Now().AddDate(0, 0, -l.days), int64(0)
		for {
			tag, err = s.db.Exec(ctx, `delete from `+l.table+` where id in (select id from `+l.table+` where time < $1 limit $2)`, cutoff, batch)
			n += tag.RowsAffected()
			if err != nil || tag.RowsAffected() < int64(batch) {
				break
			}
		}
		noun := "entries"
		if n == 1 {
			noun = "entry"
		}
		if n > 0 {
			deleted = append(deleted, fmt.Sprintf("%d %s older than %d days from %s", n, noun, l.days, l.label))
		}
		if err != nil {
			break
		}
	}
	if len(deleted) > 0 {
		err = errors.Join(err, s.RecordAudit(context.WithoutCancel(ctx), AuditEvent{Action: "log.prune", Summary: "Deleted " + strings.Join(deleted, " and "), TargetType: "settings", TargetLabel: "Log retention"}))
	}
	return err
}
