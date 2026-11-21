package store

import (
	"context"
	"strings"
	"time"

	"halo/internal/id"
	"halo/internal/secret"
)

type Device struct {
	ID          string            `json:"id"`
	UserID      string            `json:"userId"`
	Name        string            `json:"name"`
	OS          string            `json:"os"`
	Browser     string            `json:"browser"`
	Trust       string            `json:"trust"`
	FirstSeenAt time.Time         `json:"firstSeenAt"`
	LastSeenAt  time.Time         `json:"lastSeenAt"`
	LastIP      string            `json:"lastIp"`
	SignIns     int               `json:"signIns"`
	Compliance  *DeviceCompliance `json:"compliance"`
	Current     bool              `json:"current,omitempty"`
	TokenHash   []byte            `json:"-"`
}

func DeviceName(userAgent string) string {
	device, _, browser := ParseUserAgent(userAgent)
	if strings.HasPrefix(browser, "Unknown") {
		return device
	}
	return strings.Fields(browser)[0] + " on " + device
}

const deviceQuery = `select d.id, d.user_id, d.token_hash, d.user_agent, d.trust, d.first_seen_at, d.last_seen_at, d.last_ip, d.sign_in_count,
	s.id, s.name, r.external_id, r.name, r.state, r.reported_at, r.reported_at < now() - make_interval(mins => s.max_age_minutes)
	from devices d left join compliance_records r on r.source_id = d.compliance_source_id and r.external_id = d.compliance_external_id
	left join compliance_sources s on s.id = r.source_id`

func scanDevice(row rowScanner) (Device, error) {
	var d Device
	var ua string
	var sourceID, source, externalID, record, state *string
	var reportedAt *time.Time
	var stale *bool
	err := row.Scan(&d.ID, &d.UserID, &d.TokenHash, &ua, &d.Trust, &d.FirstSeenAt, &d.LastSeenAt, &d.LastIP, &d.SignIns, &sourceID, &source, &externalID, &record, &state, &reportedAt, &stale)
	d.Name = DeviceName(ua)
	_, d.OS, d.Browser = ParseUserAgent(ua)
	if err == nil && sourceID != nil {
		d.Compliance = &DeviceCompliance{State: *state, SourceID: *sourceID, Source: *source, ExternalID: *externalID, Record: *record, ReportedAt: *reportedAt, Stale: *stale}
		if *stale {
			d.Compliance.State = "unknown"
		}
	}
	return d, err
}

func (s *Store) FindDevice(ctx context.Context, userID string, tokenHash []byte) (*Device, int, error) {
	rows, err := s.db.Query(ctx, deviceQuery+` where d.user_id = $1`, userID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var found *Device
	known := 0
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, 0, err
		}
		if secret.Equal(d.TokenHash, tokenHash) {
			found = &d
		}
		known++
	}
	return found, known, rows.Err()
}

func (s *Store) TouchDevice(ctx context.Context, userID string, tokenHash []byte, userAgent, ip string, signedIn bool) (string, error) {
	count := 0
	if signedIn {
		count = 1
	}
	var deviceID string
	err := s.db.QueryRow(ctx, `insert into devices (id, user_id, token_hash, user_agent, last_ip, sign_in_count) values ($1, $2, $3, $4, $5, $6)
		on conflict (user_id, token_hash) do update set user_agent = excluded.user_agent, last_ip = excluded.last_ip, last_seen_at = now(),
		sign_in_count = devices.sign_in_count + excluded.sign_in_count returning id`,
		id.New("dev"), userID, tokenHash, userAgent, ip, count).Scan(&deviceID)
	return deviceID, err
}

func (s *Store) ListDevices(ctx context.Context, userID string) ([]Device, error) {
	rows, err := s.db.Query(ctx, deviceQuery+` where $1 = '' or d.user_id = $1 order by d.last_seen_at desc`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	devices := []Device{}
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

func (s *Store) GetDevice(ctx context.Context, deviceID string) (Device, error) {
	d, err := scanDevice(s.db.QueryRow(ctx, deviceQuery+` where d.id = $1`, deviceID))
	return d, notFound(err)
}

func (s *Store) SetDeviceTrust(ctx context.Context, deviceID, trust string) error {
	tag, err := s.db.Exec(ctx, `update devices set trust = $2 where id = $1`, deviceID, trust)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RecentSignInIPs(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `select distinct ip from (select ip from sign_in_events where user_id = $1 and result = 'success' and method <> 'token' and impersonator_id is null order by time desc limit 200) recent`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ips := []string{}
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return nil, err
		}
		ips = append(ips, ip)
	}
	return ips, rows.Err()
}

type RiskEvent struct {
	ID         string     `json:"id"`
	Time       time.Time  `json:"time"`
	Type       string     `json:"type"`
	Level      string     `json:"level"`
	Status     string     `json:"status"`
	UserID     *string    `json:"userId"`
	IP         string     `json:"ip"`
	DeviceID   *string    `json:"deviceId"`
	Device     string     `json:"device"`
	SignInID   *string    `json:"signInId"`
	Detail     string     `json:"detail"`
	ResolvedBy *string    `json:"resolvedBy"`
	ResolvedAt *time.Time `json:"resolvedAt"`
}

func (s *Store) RecordRiskEvent(ctx context.Context, e RiskEvent) error {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	_, err := s.db.Exec(ctx, `insert into risk_events (id, time, type, level, user_id, ip, device_id, sign_in_id, detail) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		id.New("rsk"), e.Time, e.Type, e.Level, e.UserID, e.IP, e.DeviceID, e.SignInID, e.Detail)
	return err
}

const riskQuery = `select r.id, r.time, r.type, r.level, r.status, r.user_id, r.ip, r.device_id, coalesce(d.user_agent, ''), s.id, r.detail, r.resolved_by, r.resolved_at
	from risk_events r left join devices d on d.id = r.device_id left join sign_in_events s on s.id = r.sign_in_id`

func scanRisk(row rowScanner) (RiskEvent, error) {
	var e RiskEvent
	var ua string
	err := row.Scan(&e.ID, &e.Time, &e.Type, &e.Level, &e.Status, &e.UserID, &e.IP, &e.DeviceID, &ua, &e.SignInID, &e.Detail, &e.ResolvedBy, &e.ResolvedAt)
	if e.DeviceID != nil {
		e.Device = DeviceName(ua)
	}
	return e, err
}

func (s *Store) ListRiskEvents(ctx context.Context) ([]RiskEvent, error) {
	rows, err := s.db.Query(ctx, riskQuery+` order by r.time desc limit 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []RiskEvent{}
	for rows.Next() {
		e, err := scanRisk(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *Store) GetRiskEvent(ctx context.Context, eventID string) (RiskEvent, error) {
	e, err := scanRisk(s.db.QueryRow(ctx, riskQuery+` where r.id = $1`, eventID))
	return e, notFound(err)
}

func (s *Store) SetRiskEventStatus(ctx context.Context, eventID, status, actorID string) error {
	tag, err := s.db.Exec(ctx, `update risk_events set status = $2, resolved_by = $3, resolved_at = now() where id = $1`, eventID, status, actorID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
