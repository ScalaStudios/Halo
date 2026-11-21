package store

import (
	"context"

	"halo/internal/secret"
)

func (s *Store) LDAPApplication(ctx context.Context, appID string) (Application, error) {
	return s.protocolApplication(ctx, appID, "ldap")
}

func (s *Store) RotateLDAPSecret(ctx context.Context, appID string) (string, error) {
	value := "hlb_" + secret.Token(32)
	tag, err := s.db.Exec(ctx, `update applications set ldap_secret_sealed = $2, updated_at = now() where id = $1 and protocol = 'ldap'`, appID, s.Sealer.Seal([]byte(value)))
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return value, err
}

func (s *Store) LDAPSecretMatches(ctx context.Context, appID, value string) (bool, error) {
	var sealed []byte
	err := s.db.QueryRow(ctx, `select ldap_secret_sealed from applications where id = $1 and protocol = 'ldap' and ldap_secret_sealed is not null`, appID).Scan(&sealed)
	if err != nil {
		return false, notFound(err)
	}
	plain, err := s.Sealer.Open(sealed)
	return err == nil && secret.Equal(plain, []byte(value)), err
}
