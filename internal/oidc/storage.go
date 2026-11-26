package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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
	keys   keyCache
}

type authRequest struct {
	store.AuthRequest
	audience []string
}

func (a *authRequest) GetID() string                      { return a.ID }
func (a *authRequest) GetACR() string                     { return a.ACR }
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
	if errors.Is(err, store.ErrNotFound) || err == nil && (app.Status != "active" || app.Protocol != "oidc" && app.Protocol != "oauth") {
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
	_, err := s.authenticated(ctx, clientID, clientSecret)
	return err
}

func (s *storage) ClientCredentials(ctx context.Context, clientID, clientSecret string) (op.Client, error) {
	app, err := s.authenticated(ctx, clientID, clientSecret)
	if err != nil {
		return nil, err
	}
	return s.client(ctx, app)
}

func (s *storage) authenticated(ctx context.Context, clientID, clientSecret string) (store.Application, error) {
	if asserted := assertedClient(ctx); asserted != "" {
		if asserted != clientID {
			return store.Application{}, oidc.ErrInvalidClient().WithDescription("client_id must match the iss claim of client_assertion.")
		}
		return s.app(ctx, clientID)
	}
	app, err := usable(s.st.VerifyClientSecret(ctx, clientID, clientSecret))
	if err == nil && (app.TokenEndpointAuthMethod == PrivateKeyJWT || app.FAPI2Profile) {
		return store.Application{}, oidc.ErrInvalidClient().WithDescription("%s authenticates with private_key_jwt. Send client_assertion instead of the client secret.", app.Name)
	}
	return app, err
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
	if scopes, err = withResources(ctx, granted, allowedScopes(app, granted, scopes)); err != nil {
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
	if app.ClientID == CLIClientID {
		return nil, oidc.ErrUnauthorizedClient().WithDescription("The Halo CLI signs in with the device flow only.")
	}
	if err := checkPKCE(app, req); err != nil {
		return nil, err
	}
	if _, pushed := ctx.Value(pushedKey{}).(bool); (app.RequirePAR || app.FAPI2Profile) && !pushed {
		return nil, oidc.ErrInvalidRequest().WithDescription("This application must push its authorization request to /oauth2/par first and send only client_id and request_uri here.")
	}
	var maxAge *int
	if req.MaxAge != nil {
		age := int(*req.MaxAge)
		maxAge = &age
	}
	if app.MaxAuthAge > 0 && (maxAge == nil || *maxAge > app.MaxAuthAge) {
		maxAge = &app.MaxAuthAge
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
	values, essential := req.ACRValues, false
	if r, ok := ctx.Value(requestKey{}).(*http.Request); ok {
		a.DomainHint, a.DPoPJKT = r.Form.Get("domain_hint"), r.Form.Get("dpop_jkt")
		if err := checkThumbprint(a.DPoPJKT); err != nil {
			return nil, err
		}
		values, essential = requestedACR(r.Form.Get("claims"), values)
	}
	known, err := s.st.RequestedAuthContexts(ctx, values)
	if err != nil {
		return nil, err
	}
	if essential && len(known) == 0 {
		return nil, oidc.ErrAccessDenied().WithDescription("Halo has none of the requested authentication contexts: %s.", strings.Join(values, ", "))
	}
	for _, c := range known {
		a.ACRValues = append(a.ACRValues, c.ID)
	}
	granted, err := s.st.GrantedResources(ctx, req.ClientID)
	if err != nil {
		return nil, err
	}
	if a.Scopes, err = withResources(ctx, granted, allowedScopes(app, granted, a.Scopes)); err != nil {
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

func checkPKCE(app store.Application, req *oidc.AuthRequest) error {
	if ((client{app: app}).public() || app.RequirePKCE || app.FAPI2Profile) && (req.CodeChallenge == "" || req.CodeChallengeMethod != oidc.CodeChallengeMethodS256) {
		return oidc.ErrInvalidRequest().WithDescription("This application must use PKCE: send code_challenge with code_challenge_method=S256.")
	}
	return nil
}

func requestedACR(claims string, values []string) ([]string, bool) {
	var parsed struct {
		IDToken struct {
			ACR *struct {
				Essential bool     `json:"essential"`
				Value     string   `json:"value"`
				Values    []string `json:"values"`
			} `json:"acr"`
		} `json:"id_token"`
	}
	if json.Unmarshal([]byte(claims), &parsed) != nil || parsed.IDToken.ACR == nil {
		return values, false
	}
	acr := parsed.IDToken.ACR
	requested := slices.DeleteFunc(append(acr.Values, acr.Value), func(v string) bool { return v == "" })
	if acr.Essential && len(requested) > 0 {
		return requested, true
	}
	return append(requested, values...), false
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
	a, err := s.st.GetAuthRequest(ctx, requestID)
	if err != nil {
		return err
	}
	app, err := s.app(ctx, a.ClientID)
	if err != nil {
		return err
	}
	lifetime := store.AuthRequestLifetime
	if app.FAPI2Profile {
		lifetime = fapiCodeLifetime
	}
	return s.st.SaveAuthCode(ctx, requestID, code, lifetime)
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

func (s *storage) userinfo(ctx context.Context, info *oidc.UserInfo, u store.User, scopes []string, clientID string) error {
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
	var app store.Application
	if clientID != "" {
		var err error
		if app, err = s.st.GetApplicationByClientID(ctx, clientID); err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}
	mappings, roles := app.ClaimMappings, app.RoleKeys(u)
	names := []string{}
	if slices.Contains(scopes, groupsScope) || len(mappings) > 0 {
		groups, err := s.st.ListGroupsRaw(ctx)
		if err != nil {
			return err
		}
		for _, g := range groups {
			if slices.Contains(u.GroupIDs, g.ID) {
				names = append(names, g.Name)
			}
		}
	}
	if slices.Contains(scopes, groupsScope) {
		info.AppendClaims(groupsScope, names)
	}
	if len(roles) > 0 {
		info.AppendClaims("roles", roles)
	}
	for _, m := range mappings {
		if slices.Contains(ReservedClaims, m.Name) {
			continue
		}
		switch m.Name {
		case "email":
			info.Email = ""
		case "name":
			info.Name = ""
		case "preferred_username":
			info.PreferredUsername = ""
		case "picture":
			info.Picture = ""
		}
		delete(info.Claims, m.Name)
		if v := u.ClaimValue(m.Source, names, roles); v != nil {
			info.AppendClaims(m.Name, v)
		}
	}
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
	for name, value := range hookClaims(ctx) {
		info.AppendClaims(name, value)
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
	return s.userinfo(ctx, info, u, scopes, req.GetClientID())
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
	if t.SubjectID != nil {
		_, u, err := s.accessToken(ctx, *t.SubjectID)
		return t, u, err
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
	if t.DPoPJKT != dpopJKT(ctx) {
		return oidc.ErrInvalidRequest().WithDescription("The access token is bound to a DPoP key. Send it as Authorization: DPoP with a DPoP proof signed by that key.")
	}
	return s.userinfo(ctx, info, u, t.Scopes, t.ClientID)
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
	if t.DPoPJKT != "" {
		resp.TokenType, resp.Claims = dpopTokenType, map[string]any{"cnf": map[string]any{"jkt": t.DPoPJKT}}
	}
	if t.UserID == nil {
		return nil
	}
	info := new(oidc.UserInfo)
	if err := s.userinfo(ctx, info, u, t.Scopes, ""); err != nil {
		return err
	}
	resp.SetUserInfo(info)
	return nil
}

func (s *storage) GetPrivateClaimsFromScopes(ctx context.Context, userID, clientID string, scopes []string) (map[string]any, error) {
	signingAccessToken(ctx)
	claims := hookClaims(ctx)
	claims["scope"] = strings.Join(scopes, " ")
	confirm(ctx, claims)
	app, err := s.st.GetApplicationByClientID(ctx, clientID)
	if errors.Is(err, store.ErrNotFound) || err == nil && len(app.Roles) == 0 {
		return claims, nil
	}
	if err != nil {
		return nil, err
	}
	u, err := s.st.GetUser(ctx, userID)
	if errors.Is(err, store.ErrNotFound) {
		return claims, nil
	}
	if err != nil {
		return nil, err
	}
	roles := app.RoleKeys(u)
	if len(roles) > 0 {
		claims["roles"] = roles
	}
	for _, m := range app.ClaimMappings {
		if m.Name == "roles" {
			delete(claims, m.Name)
		}
		if m.Source == "user.roles" && len(roles) > 0 && !slices.Contains(ReservedClaims, m.Name) {
			claims[m.Name] = roles
		}
	}
	return claims, nil
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
