package store

import (
	"context"
	"time"
)

type OIDCToken struct {
	ID          string
	Kind        string
	ClientID    string
	UserID      *string
	SessionID   *string
	RefreshHash []byte
	RefreshID   *string
	Scopes      []string
	Audience    []string
	AMR         []string
	AuthTime    *time.Time
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

const oidcTokenColumns = `id, kind, client_id, user_id, session_id, refresh_id, scopes, audience, amr, auth_time, created_at, expires_at`

func scanOIDCToken(row rowScanner) (OIDCToken, error) {
	var t OIDCToken
	err := row.Scan(&t.ID, &t.Kind, &t.ClientID, &t.UserID, &t.SessionID, &t.RefreshID, &t.Scopes, &t.Audience, &t.AMR, &t.AuthTime, &t.CreatedAt, &t.ExpiresAt)
	return t, notFound(err)
}

func (s *Store) CreateOIDCTokens(ctx context.Context, tokens ...OIDCToken) error {
	return s.Tx(ctx, func(tx *Store) error {
		for _, t := range tokens {
			_, err := tx.db.Exec(ctx, `insert into oidc_tokens (id, kind, client_id, user_id, session_id, refresh_token_hash, refresh_id, scopes, audience, amr, auth_time, expires_at)
				values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
				t.ID, t.Kind, t.ClientID, t.UserID, t.SessionID, t.RefreshHash, t.RefreshID, nonNil(t.Scopes), nonNil(t.Audience), nonNil(t.AMR), t.AuthTime, t.ExpiresAt)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) GetActiveOIDCToken(ctx context.Context, tokenID string) (OIDCToken, error) {
	return scanOIDCToken(s.db.QueryRow(ctx, `select `+oidcTokenColumns+` from oidc_tokens where id = $1 and revoked_at is null and expires_at > now()`, tokenID))
}

func (s *Store) GetActiveRefreshToken(ctx context.Context, hash []byte) (OIDCToken, error) {
	return scanOIDCToken(s.db.QueryRow(ctx, `select `+oidcTokenColumns+` from oidc_tokens where kind = 'refresh' and refresh_token_hash = $1 and revoked_at is null and expires_at > now()`, hash))
}

func (s *Store) RevokeOIDCToken(ctx context.Context, tokenID string) error {
	return s.Tx(ctx, func(tx *Store) error {
		tag, err := tx.db.Exec(ctx, `update oidc_tokens set revoked_at = now() where id = $1 and revoked_at is null`, tokenID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		_, err = tx.db.Exec(ctx, `update oidc_tokens set revoked_at = now() where refresh_id = $1 and revoked_at is null`, tokenID)
		return err
	})
}

func (s *Store) RevokeClientTokens(ctx context.Context, userID, clientID string) error {
	_, err := s.db.Exec(ctx, `update oidc_tokens set revoked_at = now() where user_id = $1 and client_id = $2 and revoked_at is null`, userID, clientID)
	return err
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
