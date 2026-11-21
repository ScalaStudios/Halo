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
	EndpointID     string     `json:"endpointId,omitempty"`
	RuleID         string     `json:"ruleId,omitempty"`
	AppID          string     `json:"appId,omitempty"`
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

const deliveryColumns = `id, coalesce(endpoint_id, ''), coalesce(rule_id, ''), coalesce(app_id, ''), event_id, event_type, body, status, attempts, next_attempt_at, last_attempt_at, response_status, error, created_at`

func (s *Store) ListWebhookDeliveries(ctx context.Context, webhookID string, limit int) ([]WebhookDelivery, error) {
	rows, err := s.db.Query(ctx, `select `+deliveryColumns+` from webhook_deliveries where endpoint_id = $1 or rule_id = $1 or app_id = $1 order by created_at desc, id desc limit $2`, webhookID, limit)
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

var eventSources = []struct{ name, table, columns string }{
	{"audit", "audit_events", `id, action, time, jsonb_build_object('id', id, 'time', time, 'actorId', actor_id, 'action', action, 'summary', summary,
		'targetType', target_type, 'targetId', target_id, 'target', target_label, 'ip', ip, 'changes', changes)`},
	{"sign_in", "sign_in_events", `id, 'sign_in.' || result, time, jsonb_build_object('id', id, 'time', time, 'userId', user_id, 'email', email, 'appId', app_id,
		'result', result, 'method', method, 'ip', ip, 'location', location, 'device', device, 'risk', risk, 'reason', reason, 'challenge', challenge, 'impersonatorId', impersonator_id, 'hook', hook)`},
}

func (s *Store) PollWebhookEvents(ctx context.Context, before time.Time, limit int) ([]WebhookEvent, error) {
	return s.pollEvents(ctx, "", before, limit)
}

func (s *Store) pollEvents(ctx context.Context, cursor string, before time.Time, limit int) ([]WebhookEvent, error) {
	var events []WebhookEvent
	for _, source := range eventSources {
		var cursorTime time.Time
		var cursorID string
		if err := s.db.QueryRow(ctx, `select time, id from webhook_cursors where source = $1 for update`, cursor+source.name).Scan(&cursorTime, &cursorID); err != nil {
			return nil, err
		}
		rows, err := s.db.Query(ctx, `select `+source.columns+` from `+source.table+` where (time, id) > ($1, $2) and time < $3 order by time, id limit $4`, cursorTime, cursorID, before, limit)
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
		if _, err := s.db.Exec(ctx, `update webhook_cursors set time = $2, id = $3 where source = $1`, cursor+source.name, last.Time, last.ID); err != nil {
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
	return s.claimDeliveries(ctx, `endpoint_id in (select id from webhook_endpoints where enabled) or rule_id in (select id from notification_rules where enabled)`, now, lease, limit)
}

func (s *Store) ClaimLogoutDeliveries(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]WebhookDelivery, error) {
	return s.claimDeliveries(ctx, `app_id in (select id from applications where backchannel_logout_uri <> '')`, now, lease, limit)
}

func (s *Store) claimDeliveries(ctx context.Context, target string, now time.Time, lease time.Duration, limit int) ([]WebhookDelivery, error) {
	rows, err := s.db.Query(ctx, `update webhook_deliveries set next_attempt_at = $2 where id in (
		select id from webhook_deliveries where status = 'pending' and next_attempt_at <= $1 and (`+target+`)
		order by next_attempt_at limit $3 for update skip locked) returning `+deliveryColumns, now, now.Add(lease), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[WebhookDelivery])
}

func (s *Store) RecordWebhookAttempt(ctx context.Context, deliveryID, status string, responseStatus *int, message string, at, next time.Time) error {
	_, err := s.db.Exec(ctx, `update webhook_deliveries set status = $2, attempts = attempts + 1, response_status = $3, error = $4, last_attempt_at = $5, next_attempt_at = $6 where id = $1`,
		deliveryID, status, responseStatus, message, at, next)
	return err
}

type NotificationRule struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Destination   string      `json:"destination"`
	Host          string      `json:"host"`
	Events        []string    `json:"events"`
	MinRisk       string      `json:"minRisk"`
	Role          string      `json:"role"`
	Threshold     int         `json:"threshold"`
	WindowMinutes int         `json:"windowMinutes"`
	QuietMinutes  int         `json:"quietMinutes"`
	Enabled       bool        `json:"enabled"`
	CreatedAt     time.Time   `json:"createdAt"`
	UpdatedAt     time.Time   `json:"updatedAt"`
	LastSentAt    *time.Time  `json:"lastSentAt"`
	LastStatus    *string     `json:"lastStatus"`
	Hits          []time.Time `json:"-"`
	URLSealed     []byte      `json:"-"`
	SecretSealed  []byte      `json:"-"`
}

type NewNotificationRule struct {
	Name          string
	Destination   string
	URL           string
	Host          string
	Events        []string
	MinRisk       string
	Role          string
	Threshold     int
	WindowMinutes int
	QuietMinutes  int
	Enabled       bool
}

const ruleColumns = `r.id, r.name, r.destination, r.host, r.events, r.min_risk, r.role, r.threshold, r.window_minutes, r.quiet_minutes, r.enabled, r.created_at, r.updated_at, r.last_sent_at,
	(select d.status from webhook_deliveries d where d.rule_id = r.id and d.last_attempt_at is not null order by d.last_attempt_at desc limit 1),
	r.hits, r.url_sealed, r.secret_sealed`

func (s *Store) rules(ctx context.Context, where string, args ...any) ([]NotificationRule, error) {
	rows, err := s.db.Query(ctx, `select `+ruleColumns+` from notification_rules r `+where+` order by r.created_at`, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[NotificationRule])
}

func (s *Store) ListNotificationRules(ctx context.Context) ([]NotificationRule, error) {
	return s.rules(ctx, ``)
}

func (s *Store) ActiveNotificationRules(ctx context.Context) ([]NotificationRule, error) {
	return s.rules(ctx, `where r.enabled`)
}

func (s *Store) GetNotificationRule(ctx context.Context, ruleID string) (NotificationRule, error) {
	rules, err := s.rules(ctx, `where r.id = $1`, ruleID)
	if err == nil && len(rules) == 0 {
		return NotificationRule{}, ErrNotFound
	}
	if err != nil {
		return NotificationRule{}, err
	}
	return rules[0], nil
}

func (s *Store) CreateNotificationRule(ctx context.Context, in NewNotificationRule) (string, string, error) {
	ruleID, ruleSecret := id.New("ntf"), "whsec_"+secret.Token(24)
	_, err := s.db.Exec(ctx, `insert into notification_rules (id, name, destination, host, url_sealed, secret_sealed, events, min_risk, role, threshold, window_minutes, quiet_minutes, enabled)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		ruleID, in.Name, in.Destination, in.Host, s.Sealer.Seal([]byte(in.URL)), s.Sealer.Seal([]byte(ruleSecret)), in.Events, in.MinRisk, in.Role, in.Threshold, in.WindowMinutes, in.QuietMinutes, in.Enabled)
	return ruleID, ruleSecret, err
}

func (s *Store) UpdateNotificationRule(ctx context.Context, ruleID string, in NewNotificationRule) error {
	var sealed []byte
	if in.URL != "" {
		sealed = s.Sealer.Seal([]byte(in.URL))
	}
	tag, err := s.db.Exec(ctx, `update notification_rules set name = $2, destination = $3, host = coalesce(nullif($4, ''), host), url_sealed = coalesce($5, url_sealed), events = $6,
		min_risk = $7, role = $8, threshold = $9, window_minutes = $10, quiet_minutes = $11, enabled = $12, hits = '{}', updated_at = now() where id = $1`,
		ruleID, in.Name, in.Destination, in.Host, sealed, in.Events, in.MinRisk, in.Role, in.Threshold, in.WindowMinutes, in.QuietMinutes, in.Enabled)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) DeleteNotificationRule(ctx context.Context, ruleID string) error {
	tag, err := s.db.Exec(ctx, `delete from notification_rules where id = $1`, ruleID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) SaveNotificationRuleState(ctx context.Context, ruleID string, hits []time.Time, lastSentAt *time.Time) error {
	_, err := s.db.Exec(ctx, `update notification_rules set hits = $2, last_sent_at = $3 where id = $1`, ruleID, hits, lastSentAt)
	return err
}

func (s *Store) QueueNotification(ctx context.Context, ruleID string, e WebhookEvent, body []byte, next time.Time) (string, error) {
	deliveryID := id.New("whd")
	_, err := s.db.Exec(ctx, `insert into webhook_deliveries (id, rule_id, event_id, event_type, body, next_attempt_at) values ($1, $2, $3, $4, $5, $6) on conflict (rule_id, event_id) do nothing`,
		deliveryID, ruleID, e.ID, e.Type, string(body), next)
	return deliveryID, err
}

type ApplicationHook struct {
	AppID        string   `json:"appId"`
	Host         string   `json:"host"`
	Claims       []string `json:"claims"`
	FailOpen     bool     `json:"failOpen"`
	URLSealed    []byte   `json:"-"`
	SecretSealed []byte   `json:"-"`
}

func (s *Store) GetApplicationHook(ctx context.Context, appID string) (ApplicationHook, error) {
	rows, err := s.db.Query(ctx, `select app_id, host, claims, fail_open, url_sealed, secret_sealed from application_hooks where app_id = $1`, appID)
	if err != nil {
		return ApplicationHook{}, err
	}
	h, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[ApplicationHook])
	return h, notFound(err)
}

func (s *Store) SaveApplicationHook(ctx context.Context, h ApplicationHook) error {
	_, err := s.db.Exec(ctx, `insert into application_hooks (app_id, host, url_sealed, secret_sealed, claims, fail_open) values ($1, $2, $3, $4, $5, $6)
		on conflict (app_id) do update set host = $2, url_sealed = $3, secret_sealed = $4, claims = $5, fail_open = $6, updated_at = now()`,
		h.AppID, h.Host, h.URLSealed, h.SecretSealed, h.Claims, h.FailOpen)
	return err
}

func (s *Store) DeleteApplicationHook(ctx context.Context, appID string) error {
	tag, err := s.db.Exec(ctx, `delete from application_hooks where app_id = $1`, appID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
