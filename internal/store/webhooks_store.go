package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
	"halo/internal/secret"
)

type Webhook struct {
	ID           string     `json:"id"`
	URL          string     `json:"url"`
	Description  string     `json:"description"`
	Events       []string   `json:"events"`
	Enabled      bool       `json:"enabled"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	LastStatus   *string    `json:"lastStatus"`
	LastDelivery *time.Time `json:"lastDeliveryAt"`
	Sealed       []byte     `json:"-"`
}

type NewWebhook struct {
	URL         string
	Description string
	Events      []string
	Enabled     bool
}

const webhookColumns = `w.id, w.url, w.description, w.events, w.enabled, w.created_at, w.updated_at,
	(select d.status from webhook_deliveries d where d.endpoint_id = w.id and d.last_attempt_at is not null order by d.last_attempt_at desc limit 1),
	(select max(d.last_attempt_at) from webhook_deliveries d where d.endpoint_id = w.id),
	w.secret_sealed`

func (s *Store) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	rows, err := s.db.Query(ctx, `select `+webhookColumns+` from webhook_endpoints w order by w.created_at`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Webhook])
}

func (s *Store) GetWebhook(ctx context.Context, webhookID string) (Webhook, error) {
	rows, err := s.db.Query(ctx, `select `+webhookColumns+` from webhook_endpoints w where w.id = $1`, webhookID)
	if err != nil {
		return Webhook{}, err
	}
	w, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Webhook])
	return w, notFound(err)
}

func (s *Store) CreateWebhook(ctx context.Context, in NewWebhook) (string, string, error) {
	webhookID, webhookSecret := id.New("whk"), "whsec_"+secret.Token(24)
	_, err := s.db.Exec(ctx, `insert into webhook_endpoints (id, url, description, events, secret_sealed, enabled) values ($1, $2, $3, $4, $5, $6)`,
		webhookID, in.URL, in.Description, in.Events, s.Sealer.Seal([]byte(webhookSecret)), in.Enabled)
	return webhookID, webhookSecret, err
}

func (s *Store) UpdateWebhook(ctx context.Context, webhookID string, in NewWebhook) error {
	tag, err := s.db.Exec(ctx, `update webhook_endpoints set url = $2, description = $3, events = $4, enabled = $5, updated_at = now() where id = $1`,
		webhookID, in.URL, in.Description, in.Events, in.Enabled)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) DeleteWebhook(ctx context.Context, webhookID string) error {
	tag, err := s.db.Exec(ctx, `delete from webhook_endpoints where id = $1`, webhookID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

type WebhookDelivery struct {
	ID             string     `json:"id"`
	EndpointID     string     `json:"endpointId"`
	EventID        string     `json:"eventId"`
	EventType      string     `json:"eventType"`
	Body           string     `json:"body"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	NextAttemptAt  time.Time  `json:"nextAttemptAt"`
	LastAttemptAt  *time.Time `json:"lastAttemptAt"`
	ResponseStatus *int       `json:"responseStatus"`
	Error          string     `json:"error"`
	CreatedAt      time.Time  `json:"createdAt"`
}

const deliveryColumns = `id, endpoint_id, event_id, event_type, body, status, attempts, next_attempt_at, last_attempt_at, response_status, error, created_at`

func (s *Store) ListWebhookDeliveries(ctx context.Context, webhookID string, limit int) ([]WebhookDelivery, error) {
	rows, err := s.db.Query(ctx, `select `+deliveryColumns+` from webhook_deliveries where endpoint_id = $1 order by created_at desc, id desc limit $2`, webhookID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[WebhookDelivery])
}

func (s *Store) GetWebhookDelivery(ctx context.Context, deliveryID string) (WebhookDelivery, error) {
	rows, err := s.db.Query(ctx, `select `+deliveryColumns+` from webhook_deliveries where id = $1`, deliveryID)
	if err != nil {
		return WebhookDelivery{}, err
	}
	d, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[WebhookDelivery])
	return d, notFound(err)
}

type WebhookEvent struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	Time time.Time       `json:"time"`
	Data json.RawMessage `json:"data"`
}

func (s *Store) QueueWebhookDelivery(ctx context.Context, endpointID string, e WebhookEvent, next time.Time) (string, error) {
	e.Time = e.Time.UTC()
	body, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	deliveryID := id.New("whd")
	_, err = s.db.Exec(ctx, `insert into webhook_deliveries (id, endpoint_id, event_id, event_type, body, next_attempt_at) values ($1, $2, $3, $4, $5, $6) on conflict (endpoint_id, event_id) do nothing`,
		deliveryID, endpointID, e.ID, e.Type, string(body), next)
	return deliveryID, err
}

func (s *Store) PollWebhookEvents(ctx context.Context, before time.Time, limit int) ([]WebhookEvent, error) {
	var events []WebhookEvent
	for _, source := range []struct{ name, query string }{
		{"audit", `select id, action, time, jsonb_build_object('id', id, 'time', time, 'actorId', actor_id, 'action', action, 'summary', summary,
			'targetType', target_type, 'targetId', target_id, 'target', target_label, 'ip', ip) from audit_events`},
		{"sign_in", `select id, 'sign_in.' || result, time, jsonb_build_object('id', id, 'time', time, 'userId', user_id, 'email', email, 'appId', app_id,
			'result', result, 'method', method, 'ip', ip, 'location', location, 'device', device, 'risk', risk, 'reason', reason) from sign_in_events`},
	} {
		var cursorTime time.Time
		var cursorID string
		if err := s.db.QueryRow(ctx, `select time, id from webhook_cursors where source = $1 for update`, source.name).Scan(&cursorTime, &cursorID); err != nil {
			return nil, err
		}
		rows, err := s.db.Query(ctx, source.query+` where (time, id) > ($1, $2) and time < $3 order by time, id limit $4`, cursorTime, cursorID, before, limit)
		if err != nil {
			return nil, err
		}
		batch, err := pgx.CollectRows(rows, pgx.RowToStructByPos[WebhookEvent])
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			continue
		}
		last := batch[len(batch)-1]
		if _, err := s.db.Exec(ctx, `update webhook_cursors set time = $2, id = $3 where source = $1`, source.name, last.Time, last.ID); err != nil {
			return nil, err
		}
		events = append(events, batch...)
	}
	return events, nil
}

func (s *Store) ActiveWebhooks(ctx context.Context) ([]Webhook, error) {
	rows, err := s.db.Query(ctx, `select `+webhookColumns+` from webhook_endpoints w where w.enabled`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Webhook])
}

func (s *Store) ClaimWebhookDeliveries(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]WebhookDelivery, error) {
	rows, err := s.db.Query(ctx, `update webhook_deliveries set next_attempt_at = $2 where id in (
		select id from webhook_deliveries where status = 'pending' and next_attempt_at <= $1 and endpoint_id in (select id from webhook_endpoints where enabled)
		order by next_attempt_at limit $3 for update skip locked) returning `+deliveryColumns, now, now.Add(lease), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[WebhookDelivery])
}

func (s *Store) WebhookEventFamilies(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `select distinct split_part(action, '.', 1) from audit_events`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func (s *Store) RecordWebhookAttempt(ctx context.Context, deliveryID, status string, responseStatus *int, message string, at, next time.Time) error {
	_, err := s.db.Exec(ctx, `update webhook_deliveries set status = $2, attempts = attempts + 1, response_status = $3, error = $4, last_attempt_at = $5, next_attempt_at = $6 where id = $1`,
		deliveryID, status, responseStatus, message, at, next)
	return err
}
