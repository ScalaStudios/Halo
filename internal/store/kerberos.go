package store

import (
	"context"
	"time"
)

type KerberosRealm struct {
	Realm  string `json:"realm"`
	Domain string `json:"domain"`
}

type KerberosSource struct {
	ServicePrincipal string          `json:"servicePrincipal"`
	Realms           []KerberosRealm `json:"realms"`
	ZoneIDs          []string        `json:"zoneIds"`
	Keytab           []byte          `json:"-"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}

func (s *Store) KerberosSource(ctx context.Context) (KerberosSource, error) {
	var k KerberosSource
	var sealed []byte
	err := s.db.QueryRow(ctx, `select service_principal, realms, zone_ids, keytab_sealed, updated_at from kerberos_source`).Scan(&k.ServicePrincipal, &k.Realms, &k.ZoneIDs, &sealed, &k.UpdatedAt)
	if err != nil {
		return k, notFound(err)
	}
	k.Keytab, err = s.Sealer.Open(sealed)
	return k, err
}

func (s *Store) SaveKerberosSource(ctx context.Context, k KerberosSource) error {
	_, err := s.db.Exec(ctx, `insert into kerberos_source (service_principal, realms, zone_ids, keytab_sealed) values ($1, $2, $3, $4)
		on conflict (singleton) do update set service_principal = excluded.service_principal, realms = excluded.realms, zone_ids = excluded.zone_ids, keytab_sealed = excluded.keytab_sealed, updated_at = now()`,
		k.ServicePrincipal, k.Realms, k.ZoneIDs, s.Sealer.Seal(k.Keytab))
	return err
}

func (s *Store) DeleteKerberosSource(ctx context.Context) error {
	tag, err := s.db.Exec(ctx, `delete from kerberos_source`)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) KerberosPrincipalUser(ctx context.Context, principal string) (string, error) {
	var userID string
	err := s.db.QueryRow(ctx, `select user_id from kerberos_principals where principal = $1`, principal).Scan(&userID)
	return userID, notFound(err)
}

func (s *Store) LinkKerberosPrincipal(ctx context.Context, principal, userID string) error {
	_, err := s.db.Exec(ctx, `insert into kerberos_principals (principal, user_id) values ($1, $2)`, principal, userID)
	return conflict(err)
}

func (s *Store) UseKerberosAuthenticator(ctx context.Context, digest []byte, expiresAt time.Time) error {
	tag, err := s.db.Exec(ctx, `insert into kerberos_authenticators (digest, expires_at) values ($1, $2) on conflict do nothing`, digest, expiresAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}
