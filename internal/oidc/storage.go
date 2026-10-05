package oidc

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"halo/internal/access"
	"halo/internal/id"
	"halo/internal/store"
)

type storage struct {
	st     *store.Store
	dev    bool
	policy access.Engine
	issuer string
}

type authRequest struct {
	store.AuthRequest
	audience []string
}

func (a *authRequest) GetID() string                      { return a.ID }
func (a *authRequest) GetACR() string                     { return "" }
func (a *authRequest) GetAMR() []string                   { return a.AMR }
func (a *authRequest) GetAudience() []string              { return a.audience }
func (a *authRequest) GetAuthTime() time.Time             { return deref(a.AuthTime) }
func (a *authRequest) GetClientID() string                { return a.ClientID }
func (a *authRequest) GetNonce() string                   { return a.Nonce }
func (a *authRequest) GetRedirectURI() string             { return a.RedirectURI }
func (a *authRequest) GetResponseType() oidc.ResponseType { return oidc.ResponseType(a.ResponseType) }
func (a *authRequest) GetResponseMode() oidc.ResponseMode { return oidc.ResponseMode(a.ResponseMode) }
func (a *authRequest) GetScopes() []string                { return a.Scopes }
func (a *authRequest) GetState() string                   { return a.State }
func (a *authRequest) GetSubject() string                 { return deref(a.UserID) }
func (a *authRequest) Done() bool                         { return a.AuthRequest.Done && a.UserID != nil }

func (a *authRequest) GetCodeChallenge() *oidc.CodeChallenge {
	if a.CodeChallenge == "" {
		return nil
	}
	return &oidc.CodeChallenge{Challenge: a.CodeChallenge, Method: oidc.CodeChallengeMethod(a.CodeChallengeMethod)}
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func usable(app store.Application, err error) (store.Application, error) {
	if errors.Is(err, store.ErrNotFound) || err == nil && (app.Status != "active" || app.Protocol == "saml") {
		return store.Application{}, oidc.ErrInvalidClient().WithDescription("The application does not exist, is disabled, or does not use OpenID Connect.")
	}
	return app, err
}

func (s *storage) app(ctx context.Context, clientID string) (store.Application, error) {
	return usable(s.st.GetApplicationByClientID(ctx, clientID))
}

func (s *storage) client(ctx context.Context, app store.Application) (client, error) {
	granted, err := s.st.GrantedResources(ctx, app.ClientID)
	if err != nil {
		return client{}, err
	}
	c := client{app: app, dev: s.dev}
	for _, g := range granted {
		c.granted = append(c.granted, g.Scopes...)
	}
	return c, nil
}

func (s *storage) GetClientByClientID(ctx context.Context, clientID string) (op.Client, error) {
	app, err := s.app(ctx, clientID)
	if err != nil {
		return nil, err
	}
	return s.client(ctx, app)
}

func (s *storage) AuthorizeClientIDSecret(ctx context.Context, clientID, clientSecret string) error {
	_, err := usable(s.st.VerifyClientSecret(ctx, clientID, clientSecret))
	return err
}

func (s *storage) ClientCredentials(ctx context.Context, clientID, clientSecret string) (op.Client, error) {
	app, err := usable(s.st.VerifyClientSecret(ctx, clientID, clientSecret))
	if err != nil {
		return nil, err
	}
	return s.client(ctx, app)
}

func (s *storage) ClientCredentialsTokenRequest(ctx context.Context, clientID string, scopes []string) (op.TokenRequest, error) {
	app, err := s.app(ctx, clientID)
	if err != nil {
		return nil, err
	}
	granted, err := s.st.GrantedResources(ctx, clientID)
	if err != nil {
		return nil, err
	}
	scopes = slices.DeleteFunc(scopes, func(scope string) bool { return !slices.Contains(app.Scopes, scope) && !isGranted(granted, scope) })
	if scopes, err = withResources(ctx, granted, scopes); err != nil {
		return nil, err
	}
	audience, _ := audience(granted, clientID, scopes)
	return &oidc.JWTTokenRequest{Subject: clientID, Audience: audience, Scopes: scopes}, nil
}

func (s *storage) CreateAuthRequest(ctx context.Context, req *oidc.AuthRequest, hintSubject string) (op.AuthRequest, error) {
	app, err := s.app(ctx, req.ClientID)
	if err != nil {
		return nil, err
	}
	if (client{app: app}).public() && (req.CodeChallenge == "" || req.CodeChallengeMethod != oidc.CodeChallengeMethodS256) {
		return nil, oidc.ErrInvalidRequest().WithDescription("This application must use PKCE: send code_challenge with code_challenge_method=S256.")
	}
	var maxAge *int
	if req.MaxAge != nil {
		age := int(*req.MaxAge)
		maxAge = &age
	}
	a := store.AuthRequest{
		ID:                  id.New("are"),
		ClientID:            req.ClientID,
		RedirectURI:         req.RedirectURI,
		Scopes:              req.Scopes,
		State:               req.State,
		Nonce:               req.Nonce,
		ResponseType:        string(req.ResponseType),
		ResponseMode:        string(req.ResponseMode),
		CodeChallenge:       req.CodeChallenge,
		CodeChallengeMethod: string(req.CodeChallengeMethod),
		Prompt:              req.Prompt,
		MaxAge:              maxAge,
		LoginHint:           req.LoginHint,
	}
	granted, err := s.st.GrantedResources(ctx, req.ClientID)
	if err != nil {
		return nil, err
	}
	if a.Scopes, err = withResources(ctx, granted, a.Scopes); err != nil {
		return nil, err
	}
	if slices.Contains(req.Prompt, oidc.PromptNone) {
		if err := s.silent(ctx, &a, app, hintSubject); err != nil {
			return nil, err
		}
	}
	created, err := s.st.CreateAuthRequest(ctx, a)
	return s.wrap(ctx, created, err)
}

func (s *storage) wrap(ctx context.Context, a store.AuthRequest, err error) (op.AuthRequest, error) {
	if err != nil {
		return nil, err
	}
	granted, err := s.st.GrantedResources(ctx, a.ClientID)
	if err != nil {
		return nil, err
	}
	aud, _ := audience(granted, a.ClientID, a.Scopes)
	return &authRequest{a, aud}, nil
}

func (s *storage) AuthRequestByID(ctx context.Context, requestID string) (op.AuthRequest, error) {
	a, err := s.st.GetAuthRequest(ctx, requestID)
	return s.wrap(ctx, a, err)
}

func (s *storage) AuthRequestByCode(ctx context.Context, code string) (op.AuthRequest, error) {
	a, err := s.st.GetAuthRequestByCode(ctx, code)
	return s.wrap(ctx, a, err)
}

func (s *storage) SaveAuthCode(ctx context.Context, requestID, code string) error {
	return s.st.SaveAuthCode(ctx, requestID, code)
}

func (s *storage) DeleteAuthRequest(ctx context.Context, requestID string) error {
	return s.st.DeleteAuthRequest(ctx, requestID)
}

func (s *storage) activeUser(ctx context.Context, userID string, app store.Application) (store.User, error) {
	u, err := s.st.GetUser(ctx, userID)
	if errors.Is(err, store.ErrNotFound) || err == nil && (u.Status != "active" || !assigned(u, app)) {
		return store.User{}, oidc.ErrInvalidGrant().WithDescription("The user is no longer active or no longer assigned to this application.")
	}
	return u, err
}

func assigned(u store.User, app store.Application) bool {
	return app.ClientID == CLIClientID || slices.Contains(u.AppIDs, app.ID)
}

func (s *storage) userinfo(ctx context.Context, info *oidc.UserInfo, u store.User, scopes []string) error {
	info.Subject = u.ID
	if slices.Contains(scopes, oidc.ScopeEmail) {
		info.Email = u.Email
		info.AppendClaims("email_verified", u.EmailVerified)
	}
	if slices.Contains(scopes, oidc.ScopeProfile) {
		info.Name, info.PreferredUsername = u.Name, u.Email
		if u.AvatarURL != nil && s.issuer != "" {
			info.Picture = s.issuer + *u.AvatarURL
		}
	}
	if !slices.Contains(scopes, groupsScope) {
		return nil
	}
	groups, err := s.st.ListGroupsRaw(ctx)
	if err != nil {
		return err
	}
	names := []string{}
	for _, g := range groups {
		if slices.Contains(u.GroupIDs, g.ID) {
			names = append(names, g.Name)
		}
	}
	info.AppendClaims(groupsScope, names)
	return nil
}

func (s *storage) SetUserinfoFromScopes(context.Context, *oidc.UserInfo, string, string, []string) error {
	return nil
}

func (s *storage) SetUserinfoFromRequest(ctx context.Context, info *oidc.UserInfo, req op.IDTokenRequest, scopes []string) error {
	u, err := s.st.GetUser(ctx, req.GetSubject())
	if err != nil {
		return err
	}
	var sessionID *string
	switch r := req.(type) {
	case *authRequest:
		sessionID = r.SessionID
	case *refreshRequest:
		sessionID = r.SessionID
	}
	if sessionID != nil {
		info.AppendClaims("sid", *sessionID)
	}
	return s.userinfo(ctx, info, u, scopes)
}

func (s *storage) accessToken(ctx context.Context, tokenID string) (store.OIDCToken, store.User, error) {
	t, err := s.st.GetActiveOIDCToken(ctx, tokenID)
	if err != nil || t.UserID == nil {
		return t, store.User{}, err
	}
	app, err := s.app(ctx, t.ClientID)
	if err != nil {
		return t, store.User{}, err
	}
	u, err := s.activeUser(ctx, *t.UserID, app)
	return t, u, err
}

func (s *storage) SetUserinfoFromToken(ctx context.Context, info *oidc.UserInfo, tokenID, _, _ string) error {
	t, u, err := s.accessToken(ctx, tokenID)
	if err != nil {
		return err
	}
	if t.UserID == nil {
		return errors.New("the access token does not belong to a user")
	}
	return s.userinfo(ctx, info, u, t.Scopes)
}

func (s *storage) SetIntrospectionFromToken(ctx context.Context, resp *oidc.IntrospectionResponse, tokenID, _, clientID string) error {
	t, u, err := s.accessToken(ctx, tokenID)
	if err != nil {
		return err
	}
	if t.ClientID != clientID && !slices.Contains(t.Audience, clientID) {
		return errors.New("the access token was not issued to this application")
	}
	resp.Scope, resp.ClientID, resp.TokenType, resp.Audience, resp.Subject = t.Scopes, t.ClientID, oidc.BearerToken, t.Audience, t.ClientID
	resp.IssuedAt, resp.Expiration, resp.AuthTime = oidc.FromTime(t.CreatedAt), oidc.FromTime(t.ExpiresAt), oidc.FromTime(deref(t.AuthTime))
	if t.UserID == nil {
		return nil
	}
	info := new(oidc.UserInfo)
	if err := s.userinfo(ctx, info, u, t.Scopes); err != nil {
		return err
	}
	resp.SetUserInfo(info)
	return nil
}

func (s *storage) GetPrivateClaimsFromScopes(_ context.Context, _, _ string, scopes []string) (map[string]any, error) {
	return map[string]any{"scope": strings.Join(scopes, " ")}, nil
}

func (s *storage) GetKeyByIDAndClientID(context.Context, string, string) (*jose.JSONWebKey, error) {
	return nil, errors.New("private_key_jwt client authentication is not supported")
}

func (s *storage) ValidateJWTProfileScopes(context.Context, string, []string) ([]string, error) {
	return nil, errors.New("the JWT bearer grant is not supported")
}

func (s *storage) Health(ctx context.Context) error {
	return s.st.Ping(ctx)
}
