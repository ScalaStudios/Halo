package oidc

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"halo/internal/store"
)

func (s *storage) ValidateTokenExchangeRequest(ctx context.Context, req op.TokenExchangeRequest) error {
	app, err := s.app(ctx, req.GetClientID())
	if err != nil {
		return err
	}
	switch {
	case !app.TokenPolicy.Exchange || (client{app: app}).public():
		return oidc.ErrUnauthorizedClient().WithDescription("Token exchange is turned off for %s. Turn it on in the application's authentication settings.", app.Name)
	case req.GetExchangeSubjectTokenType() != oidc.AccessTokenType:
		return oidc.ErrInvalidRequest().WithDescription("Halo exchanges access tokens only. Send subject_token_type=%s.", oidc.AccessTokenType)
	case req.GetExchangeActorTokenIDOrToken() != "" || req.GetExchangeActorTokenType() != "":
		return oidc.ErrInvalidRequest().WithDescription("actor_token is not supported. The authenticated application is the actor.")
	case req.GetRequestedTokenType() == "":
		req.SetRequestedTokenType(oidc.AccessTokenType)
	case req.GetRequestedTokenType() != oidc.AccessTokenType:
		return oidc.ErrInvalidRequest().WithDescription("Halo issues access tokens only. Send requested_token_type=%s or leave it out.", oidc.AccessTokenType)
	}
	subject, err := s.exchangeSubject(ctx, req)
	if err != nil {
		return err
	}
	if accepted, err := s.st.AcceptsExchangeFrom(ctx, app.ID, subject.Audience); err != nil {
		return err
	} else if !accepted {
		return oidc.ErrInvalidRequest().WithDescription("subject_token was not issued for an API that %s accepts tokens from. An administrator can add the API under Token exchange in the application's authentication settings.", app.Name)
	}
	targets := req.GetAudience()
	if len(targets) == 0 {
		return oidc.ErrInvalidRequest().WithDescription("Name the API to call with audience or resource, for example https://billing.example.com.")
	}
	granted, err := s.st.GrantedResources(ctx, app.ClientID)
	if err != nil {
		return err
	}
	var allowed []string
	for _, target := range targets {
		i := slices.IndexFunc(granted, func(g store.GrantedResource) bool { return g.Identifier == target })
		if i < 0 {
			return oidc.ErrInvalidTarget().WithDescription("This application has no access to the API %q. Ask an administrator to grant it on the API resources page.", target)
		}
		allowed = append(allowed, granted[i].Scopes...)
	}
	scopes := slices.Compact(slices.Sorted(slices.Values(req.GetScopes())))
	if len(scopes) == 0 {
		scopes = allowed
	}
	for _, scope := range scopes {
		if !slices.Contains(allowed, scope) {
			return oidc.ErrInvalidScope().WithDescription("%s is not a scope this application holds on %s.", scope, strings.Join(targets, ", "))
		}
	}
	for _, g := range granted {
		if slices.Contains(targets, g.Identifier) && !slices.ContainsFunc(g.Scopes, func(scope string) bool { return slices.Contains(scopes, scope) }) {
			return oidc.ErrInvalidScope().WithDescription("Ask for at least one scope of %s, or leave it out of audience and resource.", g.Identifier)
		}
	}
	req.SetCurrentScopes(scopes)
	return nil
}

func (s *storage) exchangeSubject(ctx context.Context, req op.TokenExchangeRequest) (store.OIDCToken, error) {
	t, _, err := s.accessToken(ctx, req.GetExchangeSubjectTokenIDOrToken())
	var oidcErr *oidc.Error
	if err != nil && !errors.Is(err, store.ErrNotFound) && !errors.As(err, &oidcErr) {
		return t, err
	}
	if err != nil || t.Kind != "access" || t.UserID == nil || *t.UserID != req.GetExchangeSubject() {
		return t, oidc.ErrInvalidRequest().WithDescription("subject_token is not an active access token of a person who can still use the application it was issued to.")
	}
	return t, nil
}

func (s *storage) CreateTokenExchangeRequest(context.Context, op.TokenExchangeRequest) error {
	return nil
}

func (s *storage) GetPrivateClaimsFromTokenExchangeRequest(ctx context.Context, req op.TokenExchangeRequest) (map[string]any, error) {
	signingAccessToken(ctx)
	act := map[string]any{"sub": req.GetClientID()}
	if prior, ok := req.GetExchangeSubjectTokenClaims()["act"]; ok {
		act["act"] = prior
	}
	return map[string]any{"scope": strings.Join(req.GetScopes(), " "), "act": act}, nil
}

func (s *storage) SetUserinfoFromTokenExchangeRequest(context.Context, *oidc.UserInfo, op.TokenExchangeRequest) error {
	return errors.New("token exchange issues access tokens only")
}
