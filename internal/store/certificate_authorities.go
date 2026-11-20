package store

import (
	"context"
	"strings"
	"time"

	"halo/internal/id"
)

type CertificateAuthority struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Certificates string    `json:"-"`
	Mapping      string    `json:"mapping"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (s *Store) ListCertificateAuthorities(ctx context.Context) ([]CertificateAuthority, error) {
	rows, err := s.db.Query(ctx, `select id, name, certificates, mapping, created_at from certificate_authorities order by created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cas := []CertificateAuthority{}
	for rows.Next() {
		var ca CertificateAuthority
		if err := rows.Scan(&ca.ID, &ca.Name, &ca.Certificates, &ca.Mapping, &ca.CreatedAt); err != nil {
			return nil, err
		}
		cas = append(cas, ca)
	}
	return cas, rows.Err()
}

func (s *Store) CreateCertificateAuthority(ctx context.Context, ca CertificateAuthority) (CertificateAuthority, error) {
	ca.ID, ca.Name = id.New("ca"), strings.TrimSpace(ca.Name)
	err := s.db.QueryRow(ctx, `insert into certificate_authorities (id, name, certificates, mapping) values ($1, $2, $3, $4) returning created_at`,
		ca.ID, ca.Name, ca.Certificates, ca.Mapping).Scan(&ca.CreatedAt)
	return ca, conflict(err)
}

func (s *Store) DeleteCertificateAuthority(ctx context.Context, caID string) (CertificateAuthority, error) {
	var ca CertificateAuthority
	err := s.db.QueryRow(ctx, `delete from certificate_authorities where id = $1 returning id, name, certificates, mapping, created_at`, caID).
		Scan(&ca.ID, &ca.Name, &ca.Certificates, &ca.Mapping, &ca.CreatedAt)
	return ca, notFound(err)
}
