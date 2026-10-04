package store

import (
	"context"
	"time"

	"halo/internal/id"
)

type IdentityProvider struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	Name           string    `json:"name"`
	Issuer         string    `json:"issuer"`
	ClientID       string    `json:"clientId"`
	HasSecret      bool      `json:"hasSecret"`
	Scopes         []string  `json:"scopes"`
	Enabled        bool      `json:"enabled"`
	ShowOnSignIn   bool      `json:"showOnSignIn"`
	AllowedDomains []string  `json:"allowedDomains"`
	JIT            bool      `json:"jit"`
	JITGroupIDs    []string  `json:"jitGroupIds"`
	LinkedCount    int       `json:"linkedCount"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	SecretSealed   []byte    `json:"-"`
}

const providerColumns = `id, kind, name, issuer, client_id, client_secret_sealed, scopes, enabled, show_on_sign_in, allowed_domains, jit, jit_group_ids, created_at, updated_at,
	(select count(*) from federated_identities f where f.provider_id = identity_providers.id)`

func scanProvider(row rowScanner) (IdentityProvider, error) {
	var p IdentityProvider
	err := row.Scan(&p.ID, &p.Kind, &p.Name, &p.Issuer, &p.ClientID, &p.SecretSealed, &p.Scopes, &p.Enabled, &p.ShowOnSignIn, &p.AllowedDomains, &p.JIT, &p.JITGroupIDs, &p.CreatedAt, &p.UpdatedAt, &p.LinkedCount)
	p.HasSecret = len(p.SecretSealed) > 0
	return p, err
}

func (s *Store) ListIdentityProviders(ctx context.Context) ([]IdentityProvider, error) {
	rows, err := s.db.Query(ctx, `select `+providerColumns+` from identity_providers order by lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	providers := []IdentityProvider{}
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		providers = append(providers, p)
	}
	return providers, rows.Err()
}

func (s *Store) GetIdentityProvider(ctx context.Context, providerID string) (IdentityProvider, error) {
	p, err := scanProvider(s.db.QueryRow(ctx, `select `+providerColumns+` from identity_providers where id = $1`, providerID))
	return p, notFound(err)
}

func (s *Store) SaveIdentityProvider(ctx context.Context, p IdentityProvider, clientSecret string) (IdentityProvider, error) {
	var sealed []byte
	if clientSecret != "" {
		sealed = s.Sealer.Seal([]byte(clientSecret))
	}
	if p.ID == "" {
		p.ID = id.New("idp")
		_, err := s.db.Exec(ctx, `insert into identity_providers (id, kind, name, issuer, client_id, client_secret_sealed, scopes, enabled, show_on_sign_in, allowed_domains, jit, jit_group_ids)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			p.ID, p.Kind, p.Name, p.Issuer, p.ClientID, sealed, p.Scopes, p.Enabled, p.ShowOnSignIn, p.AllowedDomains, p.JIT, p.JITGroupIDs)
		if err != nil {
			return IdentityProvider{}, conflict(err)
		}
		return s.GetIdentityProvider(ctx, p.ID)
	}
	tag, err := s.db.Exec(ctx, `update identity_providers set name = $2, issuer = $3, client_id = $4, client_secret_sealed = coalesce($5, client_secret_sealed), scopes = $6, enabled = $7,
		show_on_sign_in = $8, allowed_domains = $9, jit = $10, jit_group_ids = $11, updated_at = now() where id = $1`,
		p.ID, p.Name, p.Issuer, p.ClientID, sealed, p.Scopes, p.Enabled, p.ShowOnSignIn, p.AllowedDomains, p.JIT, p.JITGroupIDs)
	if err != nil {
		return IdentityProvider{}, conflict(err)
	}
	if tag.RowsAffected() == 0 {
		return IdentityProvider{}, ErrNotFound
	}
	return s.GetIdentityProvider(ctx, p.ID)
}

func (s *Store) DeleteIdentityProvider(ctx context.Context, providerID string) error {
	tag, err := s.db.Exec(ctx, `delete from identity_providers where id = $1`, providerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SeedFederation(ctx context.Context) error {
	_, err := s.SaveIdentityProvider(ctx, IdentityProvider{
		Kind:           "google",
		Name:           "Google Workspace",
		Issuer:         "https://accounts.google.com",
		ClientID:       "replace-me.apps.googleusercontent.com",
		Scopes:         []string{"openid", "email", "profile"},
		ShowOnSignIn:   true,
		AllowedDomains: []string{"example.com"},
		JITGroupIDs:    []string{},
	}, "")
	return err
}
