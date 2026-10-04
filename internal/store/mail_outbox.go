package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

type OutboxMessage struct {
	ID        string     `json:"id"`
	To        string     `json:"to"`
	Subject   string     `json:"subject"`
	Text      string     `json:"text"`
	Attempts  int        `json:"attempts"`
	LastError string     `json:"lastError"`
	CreatedAt time.Time  `json:"createdAt"`
	SentAt    *time.Time `json:"sentAt"`
}

const outboxColumns = `id, recipient, subject, body_sealed, attempts, last_error, created_at, sent_at`

func (s *Store) scanOutbox(rows pgx.Rows) ([]OutboxMessage, error) {
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (OutboxMessage, error) {
		var m OutboxMessage
		var sealed []byte
		if err := row.Scan(&m.ID, &m.To, &m.Subject, &sealed, &m.Attempts, &m.LastError, &m.CreatedAt, &m.SentAt); err != nil {
			return m, err
		}
		text, err := s.Sealer.Open(sealed)
		m.Text = string(text)
		return m, err
	})
}

func (s *Store) QueueMail(ctx context.Context, to, subject, text string, claim bool) (OutboxMessage, error) {
	m := OutboxMessage{ID: id.New("msg"), To: to, Subject: subject, Text: text, CreatedAt: time.Now()}
	next := m.CreatedAt
	if claim {
		m.Attempts, next = 1, next.Add(time.Minute)
	}
	_, err := s.db.Exec(ctx, `insert into mail_outbox (id, recipient, subject, body_sealed, attempts, created_at, next_attempt_at) values ($1, $2, $3, $4, $5, $6, $7)`,
		m.ID, to, subject, s.Sealer.Seal([]byte(text)), m.Attempts, m.CreatedAt, next)
	return m, err
}

func (s *Store) ClaimDueMail(ctx context.Context, now time.Time, maxAttempts, limit int) ([]OutboxMessage, error) {
	rows, err := s.db.Query(ctx, `update mail_outbox set attempts = attempts + 1, next_attempt_at = $1::timestamptz + interval '1 minute' * power(2, attempts)
		where id in (select id from mail_outbox where sent_at is null and attempts < $2 and next_attempt_at <= $1 and created_at > $1::timestamptz - interval '1 day'
			order by next_attempt_at limit $3 for update skip locked)
		returning `+outboxColumns, now, maxAttempts, limit)
	if err != nil {
		return nil, err
	}
	return s.scanOutbox(rows)
}

func (s *Store) RecordMailDelivery(ctx context.Context, messageID string, failure error) error {
	if failure == nil {
		_, err := s.db.Exec(ctx, `update mail_outbox set sent_at = now(), last_error = '' where id = $1`, messageID)
		return err
	}
	_, err := s.db.Exec(ctx, `update mail_outbox set last_error = $2 where id = $1`, messageID, failure.Error())
	return err
}

func (s *Store) ListMail(ctx context.Context, limit int) ([]OutboxMessage, error) {
	rows, err := s.db.Query(ctx, `select `+outboxColumns+` from mail_outbox order by created_at desc limit $1`, limit)
	if err != nil {
		return nil, err
	}
	return s.scanOutbox(rows)
}
