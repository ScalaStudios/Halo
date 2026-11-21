package store

import (
	"context"
	"time"

	"halo/internal/secret"
)

const ForwardAuthLifetime = time.Hour

func (s *Store) ForwardAuthApplication(ctx context.Context, appID string) (Application, error) {
	return s.protocolApplication(ctx, appID, "forward-auth")
}

func (s *Store) CreateForwardAuthCode(ctx context.Context, appID, sessionID, state, returnTo string) (string, error) {
	code := secret.Token(32)
	_, err := s.db.Exec(ctx, `insert into forward_auth_sessions (token_hash, app_id, session_id, state_hash, return_to, expires_at) values ($1, $2, $3, $4, $5, now() + interval '1 minute')`,
		secret.Hash(code), appID, sessionID, secret.Hash(state), returnTo)
	return code, err
}

func (s *Store) ExchangeForwardAuthCode(ctx context.Context, appID, code, state, origin string) (string, string, time.Time, error) {
	token := secret.Token(32)
	var returnTo string
	var expires time.Time
	err := s.db.QueryRow(ctx, `update forward_auth_sessions f set token_hash = $4, exchanged = true, expires_at = least($5, s.expires_at) from sessions s
		where f.token_hash = $1 and f.app_id = $2 and f.state_hash = $3 and starts_with(f.return_to, $6) and not f.exchanged and f.expires_at > now() and s.id = f.session_id and s.revoked_at is null and s.expires_at > now()
		returning f.return_to, f.expires_at`, secret.Hash(code), appID, secret.Hash(state), secret.Hash(token), time.Now().Add(ForwardAuthLifetime), origin+"/").Scan(&returnTo, &expires)
	return token, returnTo, expires, notFound(err)
}

func (s *Store) ForwardAuthSession(ctx context.Context, appID, token string) (Session, error) {
	sess, err := scanSession(s.db.QueryRow(ctx, `select `+sessionColumns+` from sessions where id = (select session_id from forward_auth_sessions where token_hash = $1 and app_id = $2 and exchanged and expires_at > now())
		and revoked_at is null and expires_at > now()`, secret.Hash(token), appID))
	return sess, notFound(err)
}

func (s *Store) EndForwardAuthSession(ctx context.Context, appID, token string) error {
	_, err := s.db.Exec(ctx, `delete from forward_auth_sessions where token_hash = $1 and app_id = $2`, secret.Hash(token), appID)
	return err
}
