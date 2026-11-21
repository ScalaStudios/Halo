package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
	"halo/internal/secret"
)

const sshHostTokenLifetime = time.Hour

type SSHAuthority struct {
	Seed                []byte
	PublicKey           string
	CertificateLifetime int
	CreatedAt           time.Time
}

func authorityTable(host bool) string {
	if host {
		return "ssh_host_authority"
	}
	return "ssh_authority"
}

func (s *Store) GetSSHAuthority(ctx context.Context, host bool) (SSHAuthority, error) {
	var a SSHAuthority
	var sealed []byte
	err := s.db.QueryRow(ctx, `select private_key_sealed, public_key, certificate_lifetime, created_at from `+authorityTable(host)).Scan(&sealed, &a.PublicKey, &a.CertificateLifetime, &a.CreatedAt)
	if err != nil {
		return a, notFound(err)
	}
	a.Seed, err = s.Sealer.Open(sealed)
	return a, err
}

func (s *Store) CreateSSHAuthority(ctx context.Context, host bool, seed []byte, publicKey string) error {
	_, err := s.db.Exec(ctx, `insert into `+authorityTable(host)+` (private_key_sealed, public_key) values ($1, $2) on conflict (id) do nothing`, s.Sealer.Seal(seed), publicKey)
	return err
}

func (s *Store) SetSSHCertificateLifetime(ctx context.Context, host bool, seconds int) error {
	tag, err := s.db.Exec(ctx, `update `+authorityTable(host)+` set certificate_lifetime = $1`, seconds)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

type SSHPrincipalMapping struct {
	ID         string    `json:"id"`
	GroupID    string    `json:"groupId"`
	Principals []string  `json:"principals"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (s *Store) ListSSHPrincipalMappings(ctx context.Context) ([]SSHPrincipalMapping, error) {
	rows, err := s.db.Query(ctx, `select m.id, m.group_id, m.principals, m.created_at, m.updated_at from ssh_principal_mappings m join groups g on g.id = m.group_id order by lower(g.name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	mappings := []SSHPrincipalMapping{}
	for rows.Next() {
		var m SSHPrincipalMapping
		if err := rows.Scan(&m.ID, &m.GroupID, &m.Principals, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		mappings = append(mappings, m)
	}
	return mappings, rows.Err()
}

func (s *Store) SaveSSHPrincipalMapping(ctx context.Context, m SSHPrincipalMapping) (SSHPrincipalMapping, error) {
	if m.ID == "" {
		m.ID = id.New("spm")
	}
	err := s.db.QueryRow(ctx, `insert into ssh_principal_mappings (id, group_id, principals) values ($1, $2, $3)
		on conflict (id) do update set group_id = excluded.group_id, principals = excluded.principals, updated_at = now()
		returning created_at, updated_at`, m.ID, m.GroupID, m.Principals).Scan(&m.CreatedAt, &m.UpdatedAt)
	return m, conflict(err)
}

func (s *Store) DeleteSSHPrincipalMapping(ctx context.Context, mappingID string) error {
	tag, err := s.db.Exec(ctx, `delete from ssh_principal_mappings where id = $1`, mappingID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

type SSHCertificate struct {
	Serial      uint64    `json:"serial"`
	KeyID       string    `json:"keyId"`
	UserID      *string   `json:"userId"`
	UserName    string    `json:"userName"`
	Principals  []string  `json:"principals"`
	Fingerprint string    `json:"fingerprint"`
	KeyType     string    `json:"keyType"`
	IP          string    `json:"ip"`
	ValidAfter  time.Time `json:"validAfter"`
	ValidBefore time.Time `json:"validBefore"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (s *Store) NextSSHSerial(ctx context.Context) (uint64, error) {
	var serial uint64
	err := s.db.QueryRow(ctx, `select nextval('ssh_certificate_serials')`).Scan(&serial)
	return serial, err
}

func (s *Store) RecordSSHCertificate(ctx context.Context, c SSHCertificate) error {
	_, err := s.db.Exec(ctx, `insert into ssh_certificates (serial, key_id, user_id, principals, fingerprint, key_type, ip, valid_after, valid_before) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		c.Serial, c.KeyID, c.UserID, c.Principals, c.Fingerprint, c.KeyType, c.IP, c.ValidAfter, c.ValidBefore)
	return err
}

func (s *Store) ListSSHCertificates(ctx context.Context, limit int) ([]SSHCertificate, error) {
	rows, err := s.db.Query(ctx, `select c.serial, c.key_id, c.user_id, coalesce(u.name, ''), c.principals, c.fingerprint, c.key_type, c.ip, c.valid_after, c.valid_before, c.created_at
		from ssh_certificates c left join users u on u.id = c.user_id order by c.created_at desc, c.serial desc limit $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	certs := []SSHCertificate{}
	for rows.Next() {
		var c SSHCertificate
		if err := rows.Scan(&c.Serial, &c.KeyID, &c.UserID, &c.UserName, &c.Principals, &c.Fingerprint, &c.KeyType, &c.IP, &c.ValidAfter, &c.ValidBefore, &c.CreatedAt); err != nil {
			return nil, err
		}
		certs = append(certs, c)
	}
	return certs, rows.Err()
}

type SSHHost struct {
	ID                  string     `json:"id"`
	Hostnames           []string   `json:"hostnames"`
	PublicKey           *string    `json:"publicKey"`
	Fingerprint         string     `json:"fingerprint"`
	KeyType             string     `json:"keyType"`
	Serial              *uint64    `json:"serial"`
	ValidBefore         *time.Time `json:"validBefore"`
	EnrollmentExpiresAt *time.Time `json:"enrollmentExpiresAt"`
	RevokedAt           *time.Time `json:"revokedAt"`
	CreatedAt           time.Time  `json:"createdAt"`
}

const sshHostColumns = `id, hostnames, public_key, fingerprint, key_type, serial, valid_before, enrollment_expires_at, revoked_at, created_at`

func (s *Store) sshHost(ctx context.Context, query string, args ...any) (SSHHost, error) {
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return SSHHost{}, err
	}
	host, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[SSHHost])
	return host, conflict(notFound(err))
}

func (s *Store) ListSSHHosts(ctx context.Context) ([]SSHHost, error) {
	rows, err := s.db.Query(ctx, `select `+sshHostColumns+` from ssh_hosts order by revoked_at is not null, created_at desc`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[SSHHost])
}

func (s *Store) CreateSSHHost(ctx context.Context, hostnames []string) (SSHHost, error) {
	return s.sshHost(ctx, `insert into ssh_hosts (id, hostnames) values ($1, $2) returning `+sshHostColumns, id.New("hst"), hostnames)
}

func (s *Store) GetSSHHost(ctx context.Context, hostID string) (SSHHost, error) {
	return s.sshHost(ctx, `select `+sshHostColumns+` from ssh_hosts where id = $1 and revoked_at is null`, hostID)
}

func (s *Store) SSHHostByKey(ctx context.Context, publicKey string) (SSHHost, error) {
	return s.sshHost(ctx, `select `+sshHostColumns+` from ssh_hosts where public_key = $1 and revoked_at is null`, publicKey)
}

func (s *Store) IssueSSHHostToken(ctx context.Context, hostID string) (SSHHost, string, error) {
	token := secret.Token(32)
	host, err := s.sshHost(ctx, `update ssh_hosts set enrollment_token_hash = $2, enrollment_expires_at = $3 where id = $1 and revoked_at is null returning `+sshHostColumns,
		hostID, secret.Hash(token), time.Now().Add(sshHostTokenLifetime))
	return host, token, err
}

func (s *Store) ConsumeSSHHostToken(ctx context.Context, token string) (SSHHost, error) {
	return s.sshHost(ctx, `update ssh_hosts set enrollment_token_hash = null, enrollment_expires_at = null
		where enrollment_token_hash = $1 and enrollment_expires_at > now() and revoked_at is null returning `+sshHostColumns, secret.Hash(token))
}

func (s *Store) CertifySSHHost(ctx context.Context, hostID, publicKey, fingerprint, keyType string, serial uint64, validBefore time.Time) error {
	_, err := s.db.Exec(ctx, `update ssh_hosts set public_key = $2, fingerprint = $3, key_type = $4, serial = $5, valid_before = $6 where id = $1`, hostID, publicKey, fingerprint, keyType, serial, validBefore)
	return conflict(err)
}

func (s *Store) RevokeSSHHost(ctx context.Context, hostID string) (SSHHost, error) {
	return s.sshHost(ctx, `update ssh_hosts set revoked_at = now(), enrollment_token_hash = null, enrollment_expires_at = null where id = $1 and revoked_at is null returning `+sshHostColumns, hostID)
}
