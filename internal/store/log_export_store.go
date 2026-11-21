package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type LogExport struct {
	Protocol      string     `json:"protocol"`
	Endpoint      string     `json:"endpoint"`
	AllowPrivate  bool       `json:"allowPrivate"`
	HeadersSet    bool       `json:"headersSet"`
	Failures      int        `json:"failures"`
	NextAttemptAt time.Time  `json:"nextAttemptAt"`
	LastSentAt    *time.Time `json:"lastSentAt"`
	LastError     string     `json:"lastError"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	HeadersSealed []byte     `json:"-"`
}

const logCursor = "log_"

func (s *Store) GetLogExport(ctx context.Context) (LogExport, error) {
	rows, err := s.db.Query(ctx, `select protocol, endpoint, allow_private, headers_sealed is not null, failures, next_attempt_at, last_sent_at, last_error, updated_at, headers_sealed from log_export`)
	if err != nil {
		return LogExport{}, err
	}
	x, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[LogExport])
	return x, notFound(err)
}

func (s *Store) SaveLogExport(ctx context.Context, x LogExport, headers string) error {
	var sealed []byte
	if headers != "" {
		sealed = s.Sealer.Seal([]byte(headers))
	}
	return s.Tx(ctx, func(tx *Store) error {
		tag, err := tx.db.Exec(ctx, `insert into log_export (protocol, endpoint, allow_private, headers_sealed) values ($1, $2, $3, $4) on conflict (singleton) do nothing`,
			x.Protocol, x.Endpoint, x.AllowPrivate, sealed)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 1 {
			_, err = tx.db.Exec(ctx, `update webhook_cursors set time = now(), id = '' where source like 'log\_%'`)
			return err
		}
		_, err = tx.db.Exec(ctx, `update log_export set protocol = $1, endpoint = $2, allow_private = $3, headers_sealed = $4, failures = 0, next_attempt_at = now(), updated_at = now()`,
			x.Protocol, x.Endpoint, x.AllowPrivate, sealed)
		return err
	})
}

func (s *Store) DeleteLogExport(ctx context.Context) error {
	tag, err := s.db.Exec(ctx, `delete from log_export`)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) RecordLogExport(ctx context.Context, sentAt *time.Time, message string, failures int, next time.Time) error {
	_, err := s.db.Exec(ctx, `update log_export set last_sent_at = coalesce($1, last_sent_at), last_error = $2, failures = $3, next_attempt_at = $4`, sentAt, message, failures, next)
	return err
}

func (s *Store) PollLogEvents(ctx context.Context, before time.Time, limit int) ([]WebhookEvent, error) {
	return s.pollEvents(ctx, logCursor, before, limit)
}

func (s *Store) queued(ctx context.Context, cursor, table string) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `select count(*) from `+table+` e, webhook_cursors c where c.source = $1 and (e.time, e.id) > (c.time, c.id)`, cursor).Scan(&n)
	return n, err
}

func (s *Store) LogQueue(ctx context.Context) (int, error) {
	total := 0
	for _, source := range eventSources {
		n, err := s.queued(ctx, logCursor+source.name, source.table)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

func (s *Store) TrimLogQueue(ctx context.Context, keep int) (int, error) {
	dropped := 0
	for _, source := range eventSources {
		n, err := s.queued(ctx, logCursor+source.name, source.table)
		if err != nil {
			return 0, err
		}
		if n <= keep {
			continue
		}
		if _, err := s.db.Exec(ctx, `with skipped as (
			select e.time, e.id from `+source.table+` e, webhook_cursors c where c.source = $1 and (e.time, e.id) > (c.time, c.id) order by e.time desc, e.id desc offset $2 limit 1)
			update webhook_cursors c set time = skipped.time, id = skipped.id from skipped where c.source = $1`, logCursor+source.name, keep); err != nil {
			return 0, err
		}
		dropped += n - keep
	}
	return dropped, nil
}
