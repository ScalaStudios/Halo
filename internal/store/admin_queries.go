package store

import (
	"context"
	"time"
)

func (s *Store) GetSession(ctx context.Context, sessionID string) (Session, error) {
	sess, err := scanSession(s.db.QueryRow(ctx, `select `+sessionColumns+` from sessions where id = $1 and revoked_at is null and expires_at > now()`, sessionID))
	if err != nil {
		return Session{}, notFound(err)
	}
	return sess, nil
}

func (s *Store) ListHighRiskSignIns(ctx context.Context, since time.Time) ([]SignInEvent, error) {
	rows, err := s.db.Query(ctx, `select id, time, user_id, email, app_id, result, method, ip, location, device, risk, reason from sign_in_events
		where risk = 'high' and time > $1 order by time desc limit 100`, since)
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
