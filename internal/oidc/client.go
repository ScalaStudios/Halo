package oidc

import (
	"slices"
	"strings"
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"halo/internal/store"
)

const CLIClientID = "halo-cli"

type client struct {
	app     store.Application
	dev     bool
	granted []string
}

func (c client) GetID() string                        { return c.app.ClientID }
func (c client) RedirectURIs() []string               { return c.app.RedirectURIs }
func (c client) PostLogoutRedirectURIs() []string     { return c.app.PostLogoutURIs }
func (c client) IDTokenLifetime() time.Duration       { return seconds(c.app.TokenPolicy.IDTokenTTL) }
func (c client) DevMode() bool                        { return c.dev }
func (c client) IDTokenUserinfoClaimsAssertion() bool { return true }
func (c client) ClockSkew() time.Duration             { return 0 }
func (c client) public() bool                         { return c.app.Type == "spa" || c.app.Type == "native" }

func (c client) RestrictAdditionalIdTokenScopes() func([]string) []string     { return keepScopes }
func (c client) RestrictAdditionalAccessTokenScopes() func([]string) []string { return keepScopes }

func (c client) ResponseTypes() []oidc.ResponseType {
	return []oidc.ResponseType{oidc.ResponseTypeCode}
}

func (c client) IsScopeAllowed(scope string) bool {
	return scope == groupsScope && slices.Contains(c.app.Scopes, scope) || slices.Contains(c.granted, scope)
}

func (c client) ApplicationType() op.ApplicationType {
	switch c.app.Type {
	case "spa":
		return op.ApplicationTypeUserAgent
	case "native":
		return op.ApplicationTypeNative
	}
	return op.ApplicationTypeWeb
}

func (c client) AuthMethod() oidc.AuthMethod {
	if c.public() {
		return oidc.AuthMethodNone
	}
	return oidc.AuthMethodBasic
}

func (c client) GrantTypes() []oidc.GrantType {
	grants := []oidc.GrantType{oidc.GrantTypeCode, oidc.GrantTypeRefreshToken}
	if c.app.Type == "service" {
		grants = append(grants, oidc.GrantTypeClientCredentials)
	}
	if c.app.ClientID == CLIClientID {
		grants = append(grants, oidc.GrantTypeDeviceCode)
	}
	return grants
}

func (c client) AccessTokenType() op.AccessTokenType {
	if c.app.Type == "service" || c.app.ClientID == CLIClientID || len(c.granted) > 0 {
		return op.AccessTokenTypeJWT
	}
	return op.AccessTokenTypeBearer
}

func (c client) LoginURL(requestID string) string {
	if strings.HasPrefix(requestID, silentPrefix+"_") {
		return store.AuthorizeCallbackPath(requestID)
	}
	return "/sign-in?authRequest=" + requestID
}

func keepScopes(scopes []string) []string { return scopes }

func seconds(n int) time.Duration { return time.Duration(n) * time.Second }
