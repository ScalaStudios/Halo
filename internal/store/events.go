package store

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"halo/internal/id"
	"halo/internal/metrics"
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
	_, err := s.db.Exec(ctx, `insert into sign_in_events (id, time, user_id, email, app_id, result, method, ip, location, device, risk, reason, challenge, impersonator_id, hook) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		e.ID, e.Time, e.UserID, e.Email, e.AppID, e.Result, e.Method, e.IP, e.Location, e.Device, e.Risk, e.Reason, e.Challenge, e.ImpersonatorID, e.Hook)
	if err == nil {
		metrics.SignIns.Inc(e.Method, e.Result, e.Risk)
	}
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
	rows, err := s.db.Query(ctx, `select id, time, user_id, email, app_id, result, method, ip, location, device, risk, reason, challenge, impersonator_id, hook from sign_in_events
		where ($1 = '' or user_id = $1) and ($2 = '' or app_id = $2) order by time desc limit $3`, f.UserID, f.AppID, f.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []SignInEvent{}
	for rows.Next() {
		var e SignInEvent
		if err := rows.Scan(&e.ID, &e.Time, &e.UserID, &e.Email, &e.AppID, &e.Result, &e.Method, &e.IP, &e.Location, &e.Device, &e.Risk, &e.Reason, &e.Challenge, &e.ImpersonatorID, &e.Hook); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

type SignInCount struct {
	SignIns      int        `json:"signIns"`
	People       int        `json:"people"`
	LastSignInAt *time.Time `json:"lastSignInAt"`
}

func (s *Store) SignInsByMethod(ctx context.Context, from, to time.Time) (map[string]SignInCount, error) {
	return s.countSignIns(ctx, "method", from, to)
}

func (s *Store) SignInsByApp(ctx context.Context, from, to time.Time) (map[string]SignInCount, error) {
	return s.countSignIns(ctx, "app_id", from, to)
}

func (s *Store) countSignIns(ctx context.Context, key string, from, to time.Time) (map[string]SignInCount, error) {
	rows, err := s.db.Query(ctx, `select e.key, sum(e.n)::bigint, count(*), max(e.last) from (
			select coalesce(`+key+`, '') as key, user_id, count(*) as n, max(time) as last from sign_in_events
			where result = 'success' and impersonator_id is null and time >= $1 and time < $2 group by 1, 2
		) e join users u on u.id = e.user_id and u.kind = 'person' group by e.key`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]SignInCount{}
	for rows.Next() {
		var k string
		var c SignInCount
		if err := rows.Scan(&k, &c.SignIns, &c.People, &c.LastSignInAt); err != nil {
			return nil, err
		}
		counts[k] = c
	}
	return counts, rows.Err()
}

func (s *Store) RecordAudit(ctx context.Context, e AuditEvent) error {
	if e.ID == "" {
		e.ID = id.New("aud")
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	if e.Changes == nil {
		e.Changes = []Change{}
	}
	_, err := s.db.Exec(ctx, `insert into audit_events (id, time, actor_id, action, summary, target_type, target_id, target_label, ip, changes) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		e.ID, e.Time, e.ActorID, e.Action, e.Summary, e.TargetType, e.TargetID, e.TargetLabel, e.IP, e.Changes)
	return err
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	rows, err := s.db.Query(ctx, `select id, time, actor_id, action, summary, target_type, target_id, target_label, ip, changes from audit_events order by time desc limit $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []AuditEvent{}
	for rows.Next() {
		var e AuditEvent
		if err := rows.Scan(&e.ID, &e.Time, &e.ActorID, &e.Action, &e.Summary, &e.TargetType, &e.TargetID, &e.TargetLabel, &e.IP, &e.Changes); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

var secretField = regexp.MustCompile(`(?i)secret|hash|seed|sealed|privatekey|password$`)

func Diff(before, after any) []Change {
	old, updated := map[string]json.RawMessage{}, map[string]json.RawMessage{}
	flatten("", before, old)
	flatten("", after, updated)
	fields := append(slices.Collect(maps.Keys(old)), slices.Collect(maps.Keys(updated))...)
	slices.Sort(fields)
	changes := []Change{}
	for _, field := range slices.Compact(fields) {
		name := field[strings.LastIndex(field, ".")+1:]
		switch {
		case name == "updatedAt" || bytes.Equal(old[field], updated[field]):
		case secretField.MatchString(name):
			changes = append(changes, Change{Field: field, Redacted: true})
		default:
			changes = append(changes, Change{Field: field, Before: old[field], After: updated[field]})
		}
	}
	return changes
}

func flatten(prefix string, value any, out map[string]json.RawMessage) {
	raw, _ := json.Marshal(value)
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		out[prefix] = raw
		return
	}
	for key, child := range fields {
		if prefix != "" {
			key = prefix + "." + key
		}
		flatten(key, child, out)
	}
}
