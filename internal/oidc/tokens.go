package oidc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"halo/internal/id"
	"halo/internal/secret"
	"halo/internal/store"
)

type refreshRequest struct {
	store.OIDCToken
	scopes []string
}

func (r *refreshRequest) GetAMR() []string                 { return r.AMR }
func (r *refreshRequest) GetAudience() []string            { return r.Audience }
func (r *refreshRequest) GetAuthTime() time.Time           { return deref(r.AuthTime) }
func (r *refreshRequest) GetClientID() string              { return r.ClientID }
func (r *refreshRequest) GetScopes() []string              { return r.scopes }
func (r *refreshRequest) GetSubject() string               { return deref(r.UserID) }
func (r *refreshRequest) SetCurrentScopes(scopes []string) { r.scopes = scopes }

func (s *storage) grant(ctx context.Context, req op.TokenRequest) (store.OIDCToken, store.Application, error) {
	t := store.OIDCToken{ID: id.New("atk"), Kind: "access", Scopes: req.GetScopes()}
	user := true
	switch r := req.(type) {
	case *authRequest:
		t.ClientID, t.SessionID, t.AMR, t.AuthTime = r.ClientID, r.SessionID, r.AMR, r.AuthTime
	case *refreshRequest:
		t.ClientID, t.SessionID, t.AMR, t.AuthTime = r.ClientID, r.SessionID, r.AMR, r.AuthTime
	case *op.DeviceAuthorizationState:
		t.ClientID, t.AMR, t.AuthTime = r.ClientID, r.AMR, &r.AuthTime
	case *oidc.JWTTokenRequest:
		t.ClientID, user = r.Subject, false
	default:
		return t, store.Application{}, fmt.Errorf("unsupported token request %T", req)
	}
	app, err := s.app(ctx, t.ClientID)
	if err != nil {
		return t, app, err
	}
	if user {
		subject := req.GetSubject()
		if _, err := s.activeUser(ctx, subject, app); err != nil {
			return t, app, err
		}
		t.UserID = &subject
	}
	granted, err := s.st.GrantedResources(ctx, t.ClientID)
	if err != nil {
		return t, app, err
	}
	var ttl int
	if t.Audience, ttl = audience(granted, t.ClientID, t.Scopes); ttl == 0 {
		ttl = app.TokenPolicy.AccessTokenTTL
	}
	t.ExpiresAt = time.Now().Add(seconds(ttl))
	switch req.(type) {
	case *op.DeviceAuthorizationState:
		t.SessionID, err = s.deviceSession(ctx, app)
	case *refreshRequest:
		if t.SessionID != nil {
			err = s.st.TouchClientSession(ctx, *t.SessionID, time.Now().Add(seconds(app.TokenPolicy.RefreshTokenTTL)))
		}
	}
	return t, app, err
}

func (s *storage) deviceSession(ctx context.Context, app store.Application) (*string, error) {
	r, ok := ctx.Value(requestKey{}).(*http.Request)
	if !ok {
		return nil, nil
	}
	ttl := max(app.TokenPolicy.RefreshTokenTTL, app.TokenPolicy.AccessTokenTTL)
	sessionID, err := s.st.CreateDeviceSession(ctx, secret.Hash(r.PostForm.Get("device_code")), time.Now().Add(seconds(ttl)))
	return &sessionID, err
}

func (s *storage) CreateAccessToken(ctx context.Context, req op.TokenRequest) (string, time.Time, error) {
	access, _, err := s.grant(ctx, req)
	if err != nil {
		return "", time.Time{}, err
	}
	return access.ID, access.ExpiresAt, s.st.CreateOIDCTokens(ctx, access)
}

func (s *storage) CreateAccessAndRefreshTokens(ctx context.Context, req op.TokenRequest, current string) (string, string, time.Time, error) {
	access, app, err := s.grant(ctx, req)
	if err != nil {
		return "", "", time.Time{}, err
	}
	prev, refreshing := req.(*refreshRequest)
	if refreshing && !app.TokenPolicy.Rotation {
		access.RefreshID = &prev.ID
		return access.ID, current, access.ExpiresAt, s.st.CreateOIDCTokens(ctx, access)
	}
	value := secret.Token(32)
	refresh := access
	refresh.ID, refresh.Kind, refresh.RefreshHash = id.New("rtk"), "refresh", secret.Hash(value)
	refresh.ExpiresAt = time.Now().Add(seconds(app.TokenPolicy.RefreshTokenTTL))
	access.RefreshID = &refresh.ID
	err = s.st.Tx(ctx, func(tx *store.Store) error {
		if refreshing {
			refresh.Scopes = prev.Scopes
			if err := tx.RevokeOIDCToken(ctx, prev.ID); errors.Is(err, store.ErrNotFound) {
				return oidc.ErrInvalidGrant().WithDescription("The refresh token was already used.")
			} else if err != nil {
				return err
			}
		}
		return tx.CreateOIDCTokens(ctx, refresh, access)
	})
	if err != nil {
		return "", "", time.Time{}, err
	}
	return access.ID, value, access.ExpiresAt, nil
}

func (s *storage) TokenRequestByRefreshToken(ctx context.Context, value string) (op.RefreshTokenRequest, error) {
	t, err := s.st.GetActiveRefreshToken(ctx, secret.Hash(value))
	if err != nil {
		return nil, err
	}
	granted, err := s.st.GrantedResources(ctx, t.ClientID)
	if err != nil {
		return nil, err
	}
	t.Audience, _ = audience(granted, t.ClientID, t.Scopes)
	return &refreshRequest{t, t.Scopes}, nil
}

func (s *storage) GetRefreshTokenInfo(ctx context.Context, _ string, value string) (string, string, error) {
	t, err := s.st.GetActiveRefreshToken(ctx, secret.Hash(value))
	if errors.Is(err, store.ErrNotFound) {
		return "", "", op.ErrInvalidRefreshToken
	}
	if err != nil {
		return "", "", err
	}
	return deref(t.UserID), t.ID, nil
}

func (s *storage) RevokeToken(ctx context.Context, tokenOrID, _, clientID string) *oidc.Error {
	t, err := s.st.GetActiveOIDCToken(ctx, tokenOrID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return oidc.ErrServerError().WithParent(err)
	}
	if t.ClientID != clientID {
		return oidc.ErrInvalidClient().WithDescription("The token was not issued to this application.")
	}
	if err := s.st.RevokeOIDCToken(ctx, t.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		return oidc.ErrServerError().WithParent(err)
	}
	if t.Kind == "refresh" && t.SessionID != nil {
		if err := s.st.EndClientSession(ctx, *t.SessionID); err != nil {
			return oidc.ErrServerError().WithParent(err)
		}
	}
	return nil
}

func (s *storage) TerminateSession(ctx context.Context, userID, clientID string) error {
	return s.st.RevokeClientTokens(ctx, userID, clientID)
}
