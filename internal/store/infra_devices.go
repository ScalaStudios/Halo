package store

import (
	"context"
	"time"
)

type DeviceAuthorization struct {
	UserCode  string
	ClientID  string
	Scopes    []string
	IP        string
	UserAgent string
	State     string
	UserID    *string
	AMR       []string
	AuthTime  *time.Time
	CreatedAt time.Time
	ExpiresAt time.Time
}

const deviceAuthorizationColumns = `user_code, client_id, scopes, ip, user_agent, state, user_id, amr, auth_time, created_at, expires_at`

func scanDeviceAuthorization(row rowScanner) (DeviceAuthorization, error) {
	var d DeviceAuthorization
	err := row.Scan(&d.UserCode, &d.ClientID, &d.Scopes, &d.IP, &d.UserAgent, &d.State, &d.UserID, &d.AMR, &d.AuthTime, &d.CreatedAt, &d.ExpiresAt)
	return d, notFound(err)
}

func (s *Store) CreateDeviceAuthorization(ctx context.Context, codeHash []byte, d DeviceAuthorization) error {
	return s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.db.Exec(ctx, `delete from device_authorizations where expires_at < now() - interval '1 hour'`); err != nil {
			return err
		}
		_, err := tx.db.Exec(ctx, `insert into device_authorizations (device_code_hash, user_code, client_id, scopes, ip, user_agent, expires_at) values ($1, $2, $3, $4, $5, $6, $7)`,
			codeHash, d.UserCode, d.ClientID, nonNil(d.Scopes), d.IP, d.UserAgent, d.ExpiresAt)
		return conflict(err)
	})
}

func (s *Store) PendingDeviceAuthorization(ctx context.Context, userCode string) (DeviceAuthorization, error) {
	return scanDeviceAuthorization(s.db.QueryRow(ctx, `select `+deviceAuthorizationColumns+` from device_authorizations where user_code = $1 and state = 'pending' and expires_at > now()`, userCode))
}

func (s *Store) DecideDeviceAuthorization(ctx context.Context, userCode string, approve bool, userID, method string, amr []string, authTime time.Time) error {
	state := "denied"
	if approve {
		state = "approved"
	}
	tag, err := s.db.Exec(ctx, `update device_authorizations set state = $2, user_id = $3, amr = $4, auth_time = $5, method = $6 where user_code = $1 and state = 'pending' and expires_at > now()`,
		userCode, state, userID, nonNil(amr), authTime, method)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) PollDeviceAuthorization(ctx context.Context, codeHash []byte, clientID string, interval time.Duration) (DeviceAuthorization, error) {
	d, err := scanDeviceAuthorization(s.db.QueryRow(ctx, `select `+deviceAuthorizationColumns+` from device_authorizations where device_code_hash = $1 and client_id = $2`, codeHash, clientID))
	if err != nil {
		return d, err
	}
	if d.State == "pending" {
		var tooFast bool
		err := s.db.QueryRow(ctx, `update device_authorizations n set last_polled_at = now() from device_authorizations o where n.device_code_hash = $1 and o.device_code_hash = $1
			returning coalesce(o.last_polled_at > now() - make_interval(secs => $2), false)`, codeHash, interval.Seconds()).Scan(&tooFast)
		if err == nil && tooFast {
			err = ErrSlowDown
		}
		return d, err
	}
	if d.State != "approved" {
		return d, nil
	}
	tag, err := s.db.Exec(ctx, `update device_authorizations set state = 'used' where device_code_hash = $1 and state = 'approved'`, codeHash)
	if err == nil && tag.RowsAffected() == 0 {
		d.State = "used"
	}
	return d, err
}
