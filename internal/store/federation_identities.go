package store

import (
	"context"
	"time"

	"halo/internal/id"
	"halo/internal/secret"
)

const FederationStateLifetime = 10 * time.Minute

type FederatedIdentity struct {
	ID              string     `json:"id"`
	ProviderID      string     `json:"providerId"`
	ProviderName    string     `json:"providerName"`
	ProviderKind    string     `json:"providerKind"`
	ProviderEnabled bool       `json:"-"`
	UserID          string     `json:"-"`
	Email           string     `json:"email"`
	CreatedAt       time.Time  `json:"createdAt"`
	LastUsedAt      *time.Time `json:"lastUsedAt"`
}

const identityColumns = `f.id, f.provider_id, p.name, p.kind, p.enabled, f.user_id, f.email, f.created_at, f.last_used_at from federated_identities f join identity_providers p on p.id = f.provider_id`

func scanIdentity(row rowScanner) (FederatedIdentity, error) {
	var f FederatedIdentity
	err := row.Scan(&f.ID, &f.ProviderID, &f.ProviderName, &f.ProviderKind, &f.ProviderEnabled, &f.UserID, &f.Email, &f.CreatedAt, &f.LastUsedAt)
	return f, err
}

func (s *Store) FindFederatedIdentity(ctx context.Context, providerID, subject string) (FederatedIdentity, error) {
	f, err := scanIdentity(s.db.QueryRow(ctx, `select `+identityColumns+` where f.provider_id = $1 and f.subject = $2`, providerID, subject))
	return f, notFound(err)
}

func (s *Store) ListUserIdentities(ctx context.Context, userID string) ([]FederatedIdentity, error) {
	rows, err := s.db.Query(ctx, `select `+identityColumns+` where f.user_id = $1 order by f.created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	identities := []FederatedIdentity{}
	for rows.Next() {
		f, err := scanIdentity(rows)
		if err != nil {
			return nil, err
		}
		identities = append(identities, f)
	}
	return identities, rows.Err()
}

func (s *Store) UseFederatedIdentity(ctx context.Context, providerID, userID, subject, email string) error {
	tag, err := s.db.Exec(ctx, `insert into federated_identities (id, provider_id, user_id, subject, email, last_used_at) values ($1, $2, $3, $4, $5, now())
		on conflict (provider_id, subject) do update set email = excluded.email, last_used_at = now() where federated_identities.user_id = excluded.user_id`,
		id.New("fid"), providerID, userID, subject, email)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

func (s *Store) DeleteUserIdentity(ctx context.Context, userID, identityID string) error {
	tag, err := s.db.Exec(ctx, `delete from federated_identities where id = $1 and user_id = $2`, identityID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type FederationState struct {
	ProviderID   string
	CodeVerifier string
	Nonce        string
	AuthRequest  string
	Next         string
}

func (s *Store) CreateFederationState(ctx context.Context, state string, f FederationState) error {
	if _, err := s.db.Exec(ctx, `delete from federation_states where expires_at < now()`); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `insert into federation_states (state_hash, provider_id, code_verifier, nonce, auth_request, next, expires_at) values ($1, $2, $3, $4, $5, $6, $7)`,
		secret.Hash(state), f.ProviderID, f.CodeVerifier, f.Nonce, f.AuthRequest, f.Next, time.Now().Add(FederationStateLifetime))
	return err
}

func (s *Store) TakeFederationState(ctx context.Context, state string) (FederationState, error) {
	var f FederationState
	err := s.db.QueryRow(ctx, `delete from federation_states where state_hash = $1 and expires_at > now() returning provider_id, code_verifier, nonce, auth_request, next`, secret.Hash(state)).
		Scan(&f.ProviderID, &f.CodeVerifier, &f.Nonce, &f.AuthRequest, &f.Next)
	return f, notFound(err)
}
