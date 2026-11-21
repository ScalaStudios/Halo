package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

const (
	caep = "https://schemas.openid.net/secevent/caep/event-type/"
	risc = "https://schemas.openid.net/secevent/risc/event-type/"

	sessionRevoked         = caep + "session-revoked"
	credentialChange       = caep + "credential-change"
	deviceComplianceChange = caep + "device-compliance-change"
	accountDisabled        = risc + "account-disabled"
	accountEnabled         = risc + "account-enabled"
	accountPurged          = risc + "account-purged"
	IdentifierChanged      = risc + "identifier-changed"

	PushDelivery = "urn:ietf:rfc:8935"
	PollDelivery = "urn:ietf:rfc:8936"

	signalUser   = `select id, '', '', '' from users where id = $1`
	isSignal     = `event_type like 'https://schemas.openid.net/secevent/%'`
	signalFields = `id, app_id, description, events_requested, delivery_method, endpoint_url, status, reason, created_at, updated_at, authorization_sealed`
)

var SignalEvents = []string{sessionRevoked, credentialChange, deviceComplianceChange, accountDisabled, accountEnabled, accountPurged, IdentifierChanged}

var credentialTypes = map[string]string{"passkey": "fido2-platform", "security-key": "fido2-roaming", "totp": "app", "password": "password"}

type SignalReceiver struct {
	AppID         string        `json:"appId"`
	SubjectFormat string        `json:"subjectFormat"`
	CreatedAt     time.Time     `json:"createdAt"`
	Stream        *SignalStream `json:"stream"`
}

type SignalStream struct {
	ID                  string    `json:"id"`
	AppID               string    `json:"appId"`
	Description         string    `json:"description"`
	EventsRequested     []string  `json:"eventsRequested"`
	DeliveryMethod      string    `json:"deliveryMethod"`
	EndpointURL         string    `json:"endpointUrl"`
	Status              string    `json:"status"`
	Reason              string    `json:"reason"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
	AuthorizationSealed []byte    `json:"-"`
}

type Signal struct {
	User    string         `json:"user,omitempty"`
	Email   string         `json:"email,omitempty"`
	Session string         `json:"session,omitempty"`
	Device  string         `json:"device,omitempty"`
	Txn     string         `json:"txn,omitempty"`
	Event   map[string]any `json:"event"`
}

func (s *Store) GetSignalReceiver(ctx context.Context, appID string) (SignalReceiver, error) {
	var r SignalReceiver
	if err := s.db.QueryRow(ctx, `select app_id, subject_format, created_at from ssf_receivers where app_id = $1`, appID).Scan(&r.AppID, &r.SubjectFormat, &r.CreatedAt); err != nil {
		return r, notFound(err)
	}
	rows, err := s.db.Query(ctx, `select `+signalFields+` from ssf_streams where app_id = $1`, appID)
	if err != nil {
		return r, err
	}
	streams, err := pgx.CollectRows(rows, pgx.RowToStructByPos[SignalStream])
	if len(streams) == 1 {
		r.Stream = &streams[0]
	}
	return r, err
}

func (s *Store) SaveSignalReceiver(ctx context.Context, appID, format string) error {
	_, err := s.db.Exec(ctx, `insert into ssf_receivers (app_id, subject_format) values ($1, $2) on conflict (app_id) do update set subject_format = excluded.subject_format`, appID, format)
	return err
}

func (s *Store) DeleteSignalReceiver(ctx context.Context, appID string) error {
	if err := s.dropSignals(ctx, appID, `true`); err != nil {
		return err
	}
	tag, err := s.db.Exec(ctx, `delete from ssf_receivers where app_id = $1`, appID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) CreateSignalStream(ctx context.Context, st SignalStream) (SignalStream, error) {
	rows, err := s.db.Query(ctx, `insert into ssf_streams (id, app_id, description, events_requested, delivery_method, endpoint_url, authorization_sealed) values ($1, $2, $3, $4, $5, $6, $7) returning `+signalFields,
		id.New("sst"), st.AppID, st.Description, nonNil(st.EventsRequested), st.DeliveryMethod, st.EndpointURL, st.AuthorizationSealed)
	if err != nil {
		return st, err
	}
	created, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[SignalStream])
	return created, conflict(err)
}

func (s *Store) UpdateSignalStream(ctx context.Context, st SignalStream) error {
	_, err := s.db.Exec(ctx, `update ssf_streams set description = $2, events_requested = $3, delivery_method = $4, endpoint_url = $5, authorization_sealed = $6, updated_at = now() where id = $1`,
		st.ID, st.Description, nonNil(st.EventsRequested), st.DeliveryMethod, st.EndpointURL, st.AuthorizationSealed)
	return err
}

func (s *Store) SetSignalStreamStatus(ctx context.Context, st SignalStream, status, reason string) error {
	if status == "disabled" {
		if err := s.dropSignals(ctx, st.AppID, `status = 'pending'`); err != nil {
			return err
		}
	}
	_, err := s.db.Exec(ctx, `update ssf_streams set status = $2, reason = $3, updated_at = now() where id = $1`, st.ID, status, reason)
	return err
}

func (s *Store) DeleteSignalStream(ctx context.Context, st SignalStream) error {
	if err := s.dropSignals(ctx, st.AppID, `true`); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `delete from ssf_streams where id = $1`, st.ID)
	return err
}

func (s *Store) dropSignals(ctx context.Context, appID, where string) error {
	_, err := s.db.Exec(ctx, `delete from webhook_deliveries where app_id = $1 and `+isSignal+` and `+where, appID)
	return err
}

func (s *Store) QueueSignal(ctx context.Context, appID, event string, sig Signal) error {
	body, err := json.Marshal(sig)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `insert into webhook_deliveries (id, app_id, event_id, event_type, body) values ($1, $2, $3, $4, $5)`, id.New("whd"), appID, id.New("set"), event, string(body))
	return err
}

func (s *Store) LastSignal(ctx context.Context, appID, event string) (time.Time, error) {
	var at time.Time
	err := s.db.QueryRow(ctx, `select coalesce(max(created_at), 'epoch') from webhook_deliveries where app_id = $1 and event_type = $2`, appID, event).Scan(&at)
	return at, err
}

func (s *Store) ListSignalDeliveries(ctx context.Context, appID string, limit int) ([]WebhookDelivery, error) {
	return s.signalDeliveries(ctx, `true order by created_at desc, id desc`, appID, limit)
}

func (s *Store) PendingSignals(ctx context.Context, appID string, limit int) ([]WebhookDelivery, error) {
	return s.signalDeliveries(ctx, `status = 'pending' order by created_at, id`, appID, limit)
}

func (s *Store) signalDeliveries(ctx context.Context, where, appID string, limit int) ([]WebhookDelivery, error) {
	rows, err := s.db.Query(ctx, `select `+deliveryColumns+` from webhook_deliveries where app_id = $1 and `+isSignal+` and `+where+` limit $2`, appID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[WebhookDelivery])
}

func (s *Store) ClaimSignalDeliveries(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]WebhookDelivery, error) {
	return s.claimDeliveries(ctx, isSignal+` and app_id in (select st.app_id from ssf_streams st join applications a on a.id = st.app_id
		where st.status = 'enabled' and a.status = 'active' and st.delivery_method = '`+PushDelivery+`')`, now, lease, limit)
}

func (s *Store) AckSignals(ctx context.Context, appID string, acks []string, errs map[string]string) error {
	const ack = `update webhook_deliveries set status = $3, attempts = attempts + 1, last_attempt_at = now(), error = $4 where app_id = $1 and ` + isSignal + ` and status = 'pending' and event_id = any($2)`
	if _, err := s.db.Exec(ctx, ack, appID, acks, "delivered", ""); err != nil {
		return err
	}
	for jti, message := range errs {
		if _, err := s.db.Exec(ctx, ack, appID, []string{jti}, "failed", message); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) signal(ctx context.Context, event string, claims map[string]any, targets string, args ...any) error {
	rows, err := s.db.Query(ctx, `select st.app_id from ssf_streams st join ssf_receivers r on r.app_id = st.app_id join applications a on a.id = st.app_id
		where a.status = 'active' and st.status <> 'disabled' and $1 = any(st.events_requested) and ($1 <> $2 or r.subject_format = 'email')`, event, IdentifierChanged)
	if err != nil {
		return err
	}
	apps, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || len(apps) == 0 {
		return err
	}
	rows, err = s.db.Query(ctx, `select u.id, coalesce(nullif(t.email, ''), u.email), t.session, t.device from (`+targets+`) t(user_id, session, device, email)
		join users u on u.id = t.user_id and u.kind = 'person'`, args...)
	if err != nil {
		return err
	}
	signals, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Signal, error) {
		var sig Signal
		return sig, row.Scan(&sig.User, &sig.Email, &sig.Session, &sig.Device)
	})
	if err != nil {
		return err
	}
	if claims == nil {
		claims = map[string]any{}
	}
	claims["event_timestamp"] = time.Now().Unix()
	for _, sig := range signals {
		sig.Txn, sig.Event = id.New("txn"), claims
		for _, app := range apps {
			if err := s.QueueSignal(ctx, app, event, sig); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) credentialChanged(ctx context.Context, userID, kind, change, name string) error {
	credential, ok := credentialTypes[kind]
	if !ok {
		return nil
	}
	claims := map[string]any{"credential_type": credential, "change_type": change}
	if name != "" {
		claims["friendly_name"] = name
	}
	return s.signal(ctx, credentialChange, claims, signalUser, userID)
}

func (s *Store) emailChanged(ctx context.Context, userID, previous, email string) error {
	if strings.EqualFold(previous, email) {
		return nil
	}
	return s.signal(ctx, IdentifierChanged, map[string]any{"new-value": email}, `select id, '', '', $2::text from users where id = $1`, userID, previous)
}
