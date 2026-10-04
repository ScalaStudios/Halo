package store

import (
	"context"
	"time"

	"halo/internal/id"
)

type SSHAuthority struct {
	Seed                []byte
	PublicKey           string
	CertificateLifetime int
	CreatedAt           time.Time
}

func (s *Store) GetSSHAuthority(ctx context.Context) (SSHAuthority, error) {
	var a SSHAuthority
	var sealed []byte
	err := s.db.QueryRow(ctx, `select private_key_sealed, public_key, certificate_lifetime, created_at from ssh_authority`).Scan(&sealed, &a.PublicKey, &a.CertificateLifetime, &a.CreatedAt)
	if err != nil {
		return a, notFound(err)
	}
	a.Seed, err = s.Sealer.Open(sealed)
	return a, err
}

func (s *Store) CreateSSHAuthority(ctx context.Context, seed []byte, publicKey string) error {
	_, err := s.db.Exec(ctx, `insert into ssh_authority (private_key_sealed, public_key) values ($1, $2) on conflict (id) do nothing`, s.Sealer.Seal(seed), publicKey)
	return err
}

func (s *Store) SetSSHCertificateLifetime(ctx context.Context, seconds int) error {
	tag, err := s.db.Exec(ctx, `update ssh_authority set certificate_lifetime = $1`, seconds)
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
