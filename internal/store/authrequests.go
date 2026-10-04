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
	UserID              *string
	SessionID           *string
	AuthTime            *time.Time
	AMR                 []string
	Done                bool
	Code                *string
	CreatedAt           time.Time
	ExpiresAt           time.Time
}

const AuthRequestLifetime = 15 * time.Minute

const authRequestColumns = `id, client_id, redirect_uri, scopes, state, nonce, response_type, response_mode, code_challenge, code_challenge_method, prompt, max_age, login_hint, user_id, session_id, auth_time, amr, done, code, created_at, expires_at`

func scanAuthRequest(row rowScanner) (AuthRequest, error) {
	var a AuthRequest
	err := row.Scan(&a.ID, &a.ClientID, &a.RedirectURI, &a.Scopes, &a.State, &a.Nonce, &a.ResponseType, &a.ResponseMode, &a.CodeChallenge, &a.CodeChallengeMethod,
		&a.Prompt, &a.MaxAge, &a.LoginHint, &a.UserID, &a.SessionID, &a.AuthTime, &a.AMR, &a.Done, &a.Code, &a.CreatedAt, &a.ExpiresAt)
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
	row := s.db.QueryRow(ctx, `insert into oidc_auth_requests (id, client_id, redirect_uri, scopes, state, nonce, response_type, response_mode, code_challenge, code_challenge_method, prompt, max_age, login_hint, user_id, session_id, auth_time, amr, done, expires_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19) returning `+authRequestColumns,
		a.ID, a.ClientID, a.RedirectURI, a.Scopes, a.State, a.Nonce, a.ResponseType, a.ResponseMode, a.CodeChallenge, a.CodeChallengeMethod, a.Prompt, a.MaxAge, a.LoginHint, a.UserID, a.SessionID, a.AuthTime, a.AMR, a.Done, a.ExpiresAt)
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

func (s *Store) SaveAuthCode(ctx context.Context, requestID, code string) error {
	tag, err := s.db.Exec(ctx, `update oidc_auth_requests set code = $2 where id = $1`, requestID, code)
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

func (s *Store) CompleteAuthRequest(ctx context.Context, requestID, userID, sessionID string, amr []string, at time.Time) error {
	tag, err := s.db.Exec(ctx, `update oidc_auth_requests set user_id = $2, session_id = $3, amr = $4, auth_time = $5, done = true where id = $1 and expires_at > now() and not done`,
		requestID, userID, sessionID, amr, at)
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
