package store

import (
	"cmp"
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
	"halo/internal/secret"
)

const (
	outpostTokenLifetime   = 24 * time.Hour
	RemoteConnectWindow    = time.Minute
	remoteTargetColumns    = `app_id, outpost_id, protocol, host, port, username, auth, credential_sealed is not null, host_key, session_limit, record, rdp_security`
	outpostColumns         = `id, name, enrolled_at, enrollment_expires_at, last_seen_at, last_ip, version, created_at, (select count(*) from remote_targets t where t.outpost_id = outposts.id)`
	remoteSessionColumns   = `id, time, app_id, app_name, user_id, email, session_id, outpost_id, protocol, address, username, ip, limit_seconds, record, status, started_at, ended_at, end_reason, bytes_in, bytes_out, recording_sealed is not null, terminated_by`
	remoteSessionsByNewest = ` order by time desc limit $1`
)

type Outpost struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	EnrolledAt          *time.Time `json:"enrolledAt"`
	EnrollmentExpiresAt *time.Time `json:"enrollmentExpiresAt"`
	LastSeenAt          *time.Time `json:"lastSeenAt"`
	LastIP              string     `json:"lastIp"`
	Version             string     `json:"version"`
	CreatedAt           time.Time  `json:"createdAt"`
	Targets             int        `json:"targets"`
}

type RemoteTarget struct {
	AppID         string `json:"-"`
	OutpostID     string `json:"outpostId"`
	Protocol      string `json:"protocol"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	Auth          string `json:"auth"`
	HasCredential bool   `json:"hasCredential"`
	HostKey       string `json:"hostKey"`
	SessionLimit  int    `json:"sessionLimit"`
	Record        bool   `json:"record"`
	RDPSecurity   string `json:"rdpSecurity"`
}

type RemoteSession struct {
	ID           string     `json:"id"`
	Time         time.Time  `json:"createdAt"`
	AppID        *string    `json:"applicationId"`
	AppName      string     `json:"applicationName"`
	UserID       *string    `json:"userId"`
	Email        string     `json:"email"`
	SessionID    string     `json:"-"`
	OutpostID    *string    `json:"outpostId"`
	Protocol     string     `json:"protocol"`
	Address      string     `json:"address"`
	Username     string     `json:"username"`
	IP           string     `json:"ip"`
	LimitSeconds int        `json:"limitSeconds"`
	Record       bool       `json:"record"`
	Status       string     `json:"status"`
	StartedAt    *time.Time `json:"startedAt"`
	EndedAt      *time.Time `json:"endedAt"`
	EndReason    string     `json:"endReason"`
	BytesIn      int64      `json:"bytesIn"`
	BytesOut     int64      `json:"bytesOut"`
	Recorded     bool       `json:"recorded"`
	TerminatedBy *string    `json:"terminatedBy"`
}

func (s *Store) outpost(ctx context.Context, query string, args ...any) (Outpost, error) {
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return Outpost{}, err
	}
	o, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Outpost])
	return o, conflict(notFound(err))
}

func (s *Store) ListOutposts(ctx context.Context) ([]Outpost, error) {
	rows, err := s.db.Query(ctx, `select `+outpostColumns+` from outposts order by lower(name)`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Outpost])
}

func (s *Store) GetOutpost(ctx context.Context, outpostID string) (Outpost, error) {
	return s.outpost(ctx, `select `+outpostColumns+` from outposts where id = $1`, outpostID)
}

func (s *Store) CreateOutpost(ctx context.Context, name string) (Outpost, string, error) {
	token := "hle_" + secret.Token(32)
	o, err := s.outpost(ctx, `insert into outposts (id, name, enrollment_token_hash, enrollment_expires_at) values ($1, $2, $3, $4) returning `+outpostColumns,
		id.New("out"), name, secret.Hash(token), time.Now().Add(outpostTokenLifetime))
	return o, token, err
}

func (s *Store) IssueOutpostToken(ctx context.Context, outpostID string) (Outpost, string, error) {
	token := "hle_" + secret.Token(32)
	o, err := s.outpost(ctx, `update outposts set enrollment_token_hash = $2, enrollment_expires_at = $3 where id = $1 returning `+outpostColumns,
		outpostID, secret.Hash(token), time.Now().Add(outpostTokenLifetime))
	return o, token, err
}

func (s *Store) EnrollOutpost(ctx context.Context, token, ip, version string) (Outpost, string, error) {
	key := "hlo_" + secret.Token(32)
	o, err := s.outpost(ctx, `update outposts set key_hash = $2, enrollment_token_hash = null, enrollment_expires_at = null, enrolled_at = now(), last_ip = $3, version = $4
		where enrollment_token_hash = $1 and enrollment_expires_at > now() returning `+outpostColumns, secret.Hash(token), secret.Hash(key), ip, version)
	return o, key, err
}

func (s *Store) OutpostByKey(ctx context.Context, key string) (Outpost, error) {
	return s.outpost(ctx, `select `+outpostColumns+` from outposts where key_hash = $1`, secret.Hash(key))
}

func (s *Store) TouchOutpost(ctx context.Context, outpostID, ip, version string) error {
	_, err := s.db.Exec(ctx, `update outposts set last_seen_at = now(), last_ip = $2, version = $3 where id = $1`, outpostID, ip, version)
	return err
}

func (s *Store) DeleteOutpost(ctx context.Context, outpostID string) (Outpost, error) {
	o, err := s.GetOutpost(ctx, outpostID)
	if err != nil {
		return o, err
	}
	if o.Targets > 0 {
		return o, ErrConflict
	}
	_, err = s.db.Exec(ctx, `delete from outposts where id = $1`, outpostID)
	return o, err
}

func (s *Store) RemoteTarget(ctx context.Context, appID string) (RemoteTarget, error) {
	return s.remoteTarget(ctx, appID, "")
}

func (s *Store) RemoteTargetForUpdate(ctx context.Context, appID string) (RemoteTarget, error) {
	return s.remoteTarget(ctx, appID, " for update")
}

func (s *Store) remoteTarget(ctx context.Context, appID, lock string) (RemoteTarget, error) {
	rows, err := s.db.Query(ctx, `select `+remoteTargetColumns+` from remote_targets where app_id = $1`+lock, appID)
	if err != nil {
		return RemoteTarget{}, err
	}
	t, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[RemoteTarget])
	return t, notFound(err)
}

func (s *Store) SaveRemoteTarget(ctx context.Context, t RemoteTarget, credential *string) error {
	var sealed []byte
	if credential != nil && *credential != "" && t.Auth != "certificate" {
		sealed = s.Sealer.Seal([]byte(*credential))
	}
	_, err := s.db.Exec(ctx, `insert into remote_targets (app_id, outpost_id, protocol, host, port, username, auth, credential_sealed, host_key, session_limit, record, rdp_security)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $13)
		on conflict (app_id) do update set outpost_id = excluded.outpost_id, protocol = excluded.protocol, host = excluded.host, port = excluded.port, username = excluded.username,
			auth = excluded.auth, credential_sealed = case when excluded.auth = 'certificate' then null when $12 then excluded.credential_sealed else remote_targets.credential_sealed end,
			host_key = excluded.host_key, session_limit = excluded.session_limit, record = excluded.record, rdp_security = excluded.rdp_security`,
		t.AppID, t.OutpostID, t.Protocol, t.Host, t.Port, t.Username, t.Auth, sealed, t.HostKey, t.SessionLimit, t.Record, credential != nil && *credential != "", cmp.Or(t.RDPSecurity, "nla"))
	return err
}

func (s *Store) RemoteCredential(ctx context.Context, appID string) (string, error) {
	var sealed []byte
	if err := s.db.QueryRow(ctx, `select credential_sealed from remote_targets where app_id = $1 and credential_sealed is not null`, appID).Scan(&sealed); err != nil {
		return "", notFound(err)
	}
	plain, err := s.Sealer.Open(sealed)
	return string(plain), err
}

func (s *Store) remoteSession(ctx context.Context, query string, args ...any) (RemoteSession, error) {
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return RemoteSession{}, err
	}
	rs, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[RemoteSession])
	return rs, notFound(err)
}

func (s *Store) CreateRemoteSession(ctx context.Context, rs RemoteSession) (RemoteSession, error) {
	return s.remoteSession(ctx, `insert into remote_sessions (id, app_id, app_name, user_id, email, session_id, outpost_id, protocol, address, username, ip, limit_seconds, record)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) returning `+remoteSessionColumns,
		id.New("rs"), rs.AppID, rs.AppName, rs.UserID, rs.Email, rs.SessionID, rs.OutpostID, rs.Protocol, rs.Address, rs.Username, rs.IP, rs.LimitSeconds, rs.Record)
}

func (s *Store) ActivateRemoteSession(ctx context.Context, remoteID, userID, sessionID string) (RemoteSession, error) {
	return s.remoteSession(ctx, `update remote_sessions set status = 'active', started_at = now(), seen_at = now()
		where id = $1 and user_id = $2 and session_id = $3 and status = 'pending' and terminated_by is null and time > $4 returning `+remoteSessionColumns,
		remoteID, userID, sessionID, time.Now().Add(-RemoteConnectWindow))
}

func (s *Store) TouchRemoteSession(ctx context.Context, remoteID string) error {
	_, err := s.db.Exec(ctx, `update remote_sessions set seen_at = now() where id = $1`, remoteID)
	return err
}

func (s *Store) RemoteSessionState(ctx context.Context, remoteID string) (name, status string, err error) {
	err = s.db.QueryRow(ctx, `select coalesce(u.name, ''), r.status from remote_sessions r left join users u on u.id = r.terminated_by where r.id = $1`, remoteID).Scan(&name, &status)
	return name, status, notFound(err)
}

func (s *Store) CancelPendingRemoteSession(ctx context.Context, remoteID, reason string) (bool, error) {
	tag, err := s.db.Exec(ctx, `update remote_sessions set status = 'ended', ended_at = now(), end_reason = $2 where id = $1 and status = 'pending'`, remoteID, reason)
	return tag.RowsAffected() > 0, err
}

func (s *Store) EndStaleRemoteSessions(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `update remote_sessions set status = 'ended', ended_at = now(),
		end_reason = case when status = 'pending' then 'The browser never opened the connection.' else 'The Halo server running the session stopped.' end
		where status = 'pending' and time < $1 or status = 'active' and seen_at < $2`, time.Now().Add(-2*RemoteConnectWindow), time.Now().Add(-2*time.Minute))
	return err
}

func (s *Store) EndRemoteSession(ctx context.Context, remoteID, reason string, bytesIn, bytesOut int64, recording []byte) error {
	var sealed []byte
	if len(recording) > 0 {
		sealed = s.Sealer.Seal(recording)
	}
	_, err := s.db.Exec(ctx, `update remote_sessions set status = 'ended', ended_at = coalesce(ended_at, now()), end_reason = $2, bytes_in = $3, bytes_out = $4, recording_sealed = $5 where id = $1`,
		remoteID, reason, bytesIn, bytesOut, sealed)
	return err
}

func (s *Store) MarkRemoteSessionTerminated(ctx context.Context, remoteID, actorID string) (RemoteSession, error) {
	return s.remoteSession(ctx, `update remote_sessions set terminated_by = $2 where id = $1 and status <> 'ended' returning `+remoteSessionColumns, remoteID, actorID)
}

func (s *Store) GetRemoteSession(ctx context.Context, remoteID string) (RemoteSession, error) {
	return s.remoteSession(ctx, `select `+remoteSessionColumns+` from remote_sessions where id = $1`, remoteID)
}

func (s *Store) ListRemoteSessions(ctx context.Context, appID string, limit int) ([]RemoteSession, error) {
	query, args := `select `+remoteSessionColumns+` from remote_sessions`+remoteSessionsByNewest, []any{limit}
	if appID != "" {
		query, args = `select `+remoteSessionColumns+` from remote_sessions where app_id = $2`+remoteSessionsByNewest, []any{limit, appID}
	}
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[RemoteSession])
}

func (s *Store) RemoteRecording(ctx context.Context, remoteID string) ([]byte, error) {
	var sealed []byte
	if err := s.db.QueryRow(ctx, `select recording_sealed from remote_sessions where id = $1 and recording_sealed is not null`, remoteID).Scan(&sealed); err != nil {
		return nil, notFound(err)
	}
	return s.Sealer.Open(sealed)
}
