package store

import (
	"context"
	"time"

	"halo/internal/id"
	"halo/internal/secret"
)

type FederatedCredential struct {
	ID               string            `json:"id"`
	ServiceAccountID string            `json:"serviceAccountId"`
	Name             string            `json:"name"`
	Issuer           string            `json:"issuer"`
	Subject          string            `json:"subject"`
	Audience         string            `json:"audience"`
	Conditions       map[string]string `json:"conditions"`
	Scopes           []string          `json:"scopes"`
	CreatedBy        *string           `json:"createdBy"`
	CreatedAt        time.Time         `json:"createdAt"`
	LastUsedAt       *time.Time        `json:"lastUsedAt"`
}

func (s *Store) ListFederatedCredentials(ctx context.Context, serviceAccountID string) ([]FederatedCredential, error) {
	rows, err := s.db.Query(ctx, `select id, service_account_id, name, issuer, subject, audience, conditions, scopes, created_by, created_at, last_used_at
		from federated_credentials where $1 = '' or service_account_id = $1 order by created_at, id`, serviceAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	credentials := []FederatedCredential{}
	for rows.Next() {
		var c FederatedCredential
		if err := rows.Scan(&c.ID, &c.ServiceAccountID, &c.Name, &c.Issuer, &c.Subject, &c.Audience, &c.Conditions, &c.Scopes, &c.CreatedBy, &c.CreatedAt, &c.LastUsedAt); err != nil {
			return nil, err
		}
		credentials = append(credentials, c)
	}
	return credentials, rows.Err()
}

func (s *Store) CreateFederatedCredential(ctx context.Context, c FederatedCredential) (FederatedCredential, error) {
	c.ID = id.New("fic")
	err := s.db.QueryRow(ctx, `insert into federated_credentials (id, service_account_id, name, issuer, subject, audience, conditions, scopes, created_by) values ($1, $2, $3, $4, $5, $6, $7, $8, $9) returning created_at`,
		c.ID, c.ServiceAccountID, c.Name, c.Issuer, c.Subject, c.Audience, c.Conditions, c.Scopes, c.CreatedBy).Scan(&c.CreatedAt)
	return c, conflict(err)
}

func (s *Store) DeleteFederatedCredential(ctx context.Context, serviceAccountID, credentialID string) (FederatedCredential, error) {
	var c FederatedCredential
	err := s.db.QueryRow(ctx, `delete from federated_credentials where id = $1 and service_account_id = $2 returning id, name, issuer, subject`, credentialID, serviceAccountID).Scan(&c.ID, &c.Name, &c.Issuer, &c.Subject)
	return c, notFound(err)
}

func (s *Store) IssueFederatedToken(ctx context.Context, c FederatedCredential, expiresAt time.Time) (string, error) {
	value := "hlk_" + secret.Token(32)
	_, err := s.db.Exec(ctx, `insert into api_keys (id, service_account_id, label, prefix, key_hash, scopes, expires_at, credential_id) values ($1, $2, $3, $4, $5, $6, $7, $8)`,
		id.New("key"), c.ServiceAccountID, c.Name, value[:12], secret.Hash(value), c.Scopes, expiresAt, c.ID)
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(ctx, `update federated_credentials set last_used_at = now() where id = $1`, c.ID)
	return value, err
}
