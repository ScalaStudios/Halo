package store

import (
	"context"
	"time"

	"halo/internal/id"
)

func (s *Store) RecordSignIn(ctx context.Context, e SignInEvent) error {
	if e.ID == "" {
		e.ID = id.New("evt")
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	if e.Risk == "" {
		e.Risk = "none"
	}
	_, err := s.db.Exec(ctx, `insert into sign_in_events (id, time, user_id, email, app_id, result, method, ip, location, device, risk, reason) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		e.ID, e.Time, e.UserID, e.Email, e.AppID, e.Result, e.Method, e.IP, e.Location, e.Device, e.Risk, e.Reason)
	return err
}

type SignInFilter struct {
	UserID string
	AppID  string
	Limit  int
}

func (s *Store) ListSignIns(ctx context.Context, f SignInFilter) ([]SignInEvent, error) {
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 500
	}
	rows, err := s.db.Query(ctx, `select id, time, user_id, email, app_id, result, method, ip, location, device, risk, reason from sign_in_events
		where ($1 = '' or user_id = $1) and ($2 = '' or app_id = $2) order by time desc limit $3`, f.UserID, f.AppID, f.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []SignInEvent{}
	for rows.Next() {
		var e SignInEvent
		if err := rows.Scan(&e.ID, &e.Time, &e.UserID, &e.Email, &e.AppID, &e.Result, &e.Method, &e.IP, &e.Location, &e.Device, &e.Risk, &e.Reason); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *Store) RecordAudit(ctx context.Context, e AuditEvent) error {
	if e.ID == "" {
		e.ID = id.New("aud")
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	_, err := s.db.Exec(ctx, `insert into audit_events (id, time, actor_id, action, summary, target_type, target_id, target_label, ip) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		e.ID, e.Time, e.ActorID, e.Action, e.Summary, e.TargetType, e.TargetID, e.TargetLabel, e.IP)
	return err
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	rows, err := s.db.Query(ctx, `select id, time, actor_id, action, summary, target_type, target_id, target_label, ip from audit_events order by time desc limit $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []AuditEvent{}
	for rows.Next() {
		var e AuditEvent
		if err := rows.Scan(&e.ID, &e.Time, &e.ActorID, &e.Action, &e.Summary, &e.TargetType, &e.TargetID, &e.TargetLabel, &e.IP); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
