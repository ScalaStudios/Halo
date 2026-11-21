package store

import (
	"context"
	"time"
)

type AuthRequest struct {
	ID                  string
	ClientID            string
	RedirectURI         string
	Scopes              []string
	State               string
	Nonce               string
	ResponseType        string
	ResponseMode        string
	CodeChallenge       string
	CodeChallengeMethod string
	Prompt              []string
	MaxAge              *int
	LoginHint           string
	DomainHint          string
	UserID              *string
	SessionID           *string
	AuthTime            *time.Time
	AMR                 []string
	ACRValues           []string
	ACR                 string
	Done                bool
	Code                *string
	Consent             *string
	CreatedAt           time.Time
	ExpiresAt           time.Time
	DPoPJKT             string
}

const AuthRequestLifetime = 15 * time.Minute

const authRequestColumns = `id, client_id, redirect_uri, scopes, state, nonce, response_type, response_mode, code_challenge, code_challenge_method, prompt, max_age, login_hint, domain_hint, user_id, session_id, auth_time, amr, acr_values, acr, done, code, consent, created_at, expires_at, dpop_jkt`

func scanAuthRequest(row rowScanner) (AuthRequest, error) {
	var a AuthRequest
	err := row.Scan(&a.ID, &a.ClientID, &a.RedirectURI, &a.Scopes, &a.State, &a.Nonce, &a.ResponseType, &a.ResponseMode, &a.CodeChallenge, &a.CodeChallengeMethod,
		&a.Prompt, &a.MaxAge, &a.LoginHint, &a.DomainHint, &a.UserID, &a.SessionID, &a.AuthTime, &a.AMR, &a.ACRValues, &a.ACR, &a.Done, &a.Code, &a.Consent, &a.CreatedAt, &a.ExpiresAt, &a.DPoPJKT)
	return a, err
}

func (s *Store) CreateAuthRequest(ctx context.Context, a AuthRequest) (AuthRequest, error) {
	if a.ExpiresAt.IsZero() {
		a.ExpiresAt = time.Now().Add(AuthRequestLifetime)
	}
	if a.Prompt == nil {
		a.Prompt = []string{}
	}
	if a.AMR == nil {
		a.AMR = []string{}
	}
	row := s.db.QueryRow(ctx, `insert into oidc_auth_requests (id, client_id, redirect_uri, scopes, state, nonce, response_type, response_mode, code_challenge, code_challenge_method, prompt, max_age, login_hint, domain_hint, user_id, session_id, auth_time, amr, acr_values, acr, done, expires_at, dpop_jkt)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23) returning `+authRequestColumns,
		a.ID, a.ClientID, a.RedirectURI, a.Scopes, a.State, a.Nonce, a.ResponseType, a.ResponseMode, a.CodeChallenge, a.CodeChallengeMethod, a.Prompt, a.MaxAge, a.LoginHint, a.DomainHint, a.UserID, a.SessionID, a.AuthTime, a.AMR, nonNil(a.ACRValues), a.ACR, a.Done, a.ExpiresAt, a.DPoPJKT)
	return scanAuthRequest(row)
}

func (s *Store) GetAuthRequest(ctx context.Context, requestID string) (AuthRequest, error) {
	a, err := scanAuthRequest(s.db.QueryRow(ctx, `select `+authRequestColumns+` from oidc_auth_requests where id = $1 and expires_at > now()`, requestID))
	return a, notFound(err)
}

func (s *Store) GetAuthRequestByCode(ctx context.Context, code string) (AuthRequest, error) {
	a, err := scanAuthRequest(s.db.QueryRow(ctx, `select `+authRequestColumns+` from oidc_auth_requests where code = $1 and expires_at > now()`, code))
	return a, notFound(err)
}

func (s *Store) SaveAuthCode(ctx context.Context, requestID, code string, lifetime time.Duration) error {
	tag, err := s.db.Exec(ctx, `update oidc_auth_requests set code = $2, expires_at = least(expires_at, $3) where id = $1`, requestID, code, time.Now().Add(lifetime))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteAuthRequest(ctx context.Context, requestID string) error {
	_, err := s.db.Exec(ctx, `delete from oidc_auth_requests where id = $1`, requestID)
	return err
}

func (s *Store) CompleteAuthRequest(ctx context.Context, requestID, userID, sessionID string, amr []string, at time.Time, acr string) error {
	tag, err := s.db.Exec(ctx, `update oidc_auth_requests set user_id = $2, session_id = $3, amr = $4, auth_time = $5, acr = $6, done = true where id = $1 and expires_at > now() and not done`,
		requestID, userID, sessionID, amr, at, acr)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DecideConsent(ctx context.Context, requestID, sessionID, decision string) error {
	tag, err := s.db.Exec(ctx, `update oidc_auth_requests set consent = $3 where id = $1 and session_id = $2 and done and consent is null and expires_at > now()`, requestID, sessionID, decision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func AuthorizeCallbackPath(requestID string) string {
	return "/oauth2/authorize/callback?id=" + requestID
}

var PushedRequestLifetime = time.Minute

func (s *Store) PushAuthRequest(ctx context.Context, requestHash []byte, clientID, params string) error {
	_, err := s.db.Exec(ctx, `insert into oidc_pushed_requests (request_hash, client_id, params, expires_at) values ($1, $2, $3, $4)`, requestHash, clientID, params, time.Now().Add(PushedRequestLifetime))
	return err
}

func (s *Store) TakePushedAuthRequest(ctx context.Context, requestHash []byte, clientID string) (string, error) {
	var params string
	err := s.db.QueryRow(ctx, `delete from oidc_pushed_requests where request_hash = $1 and client_id = $2 and expires_at > now() returning params`, requestHash, clientID).Scan(&params)
	return params, notFound(err)
}
