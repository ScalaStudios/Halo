package api

import (
	"context"
	"slices"

	"halo/internal/httpx"
	"halo/internal/oidc"
	"halo/internal/store"
)

func clientPolicies(ctx context.Context, tx *store.Store, protocol, appType, clientID string) ([]store.ClientPolicy, error) {
	if protocol != "oidc" && protocol != "oauth" || clientID == oidc.CLIClientID {
		return nil, nil
	}
	if err := tx.LockApprovalTarget(ctx, "settings", ""); err != nil {
		return nil, err
	}
	settings, err := tx.CurrentSettings(ctx)
	if err != nil {
		return nil, err
	}
	var policies []store.ClientPolicy
	for _, policy := range settings.ClientPolicies {
		if policy.ApplicationType == "all" || policy.ApplicationType == appType {
			policies = append(policies, policy)
		}
	}
	return policies, nil
}

func applyClientPolicyDefaults(app *store.Application, policies []store.ClientPolicy) {
	for _, policy := range policies {
		if app.Type != "service" {
			app.RequirePAR = app.RequirePAR || policy.RequirePAR
			app.RequirePKCE = app.RequirePKCE || policy.RequirePKCE
		}
		app.RequireDPoP = app.RequireDPoP || policy.RequireDPoP
		if policy.MaxAccessTokenTTL > 0 {
			app.TokenPolicy.AccessTokenTTL = min(app.TokenPolicy.AccessTokenTTL, policy.MaxAccessTokenTTL)
		}
		if policy.MaxIDTokenTTL > 0 {
			app.TokenPolicy.IDTokenTTL = min(app.TokenPolicy.IDTokenTTL, policy.MaxIDTokenTTL)
		}
		if policy.MaxRefreshTokenTTL > 0 {
			app.TokenPolicy.RefreshTokenTTL = min(app.TokenPolicy.RefreshTokenTTL, policy.MaxRefreshTokenTTL)
		}
	}
}

func checkClientPolicies(app store.Application, policies []store.ClientPolicy) error {
	method := app.TokenEndpointAuthMethod
	public := app.Type == "spa" || app.Type == "native"
	if public {
		method = "none"
	}
	for _, policy := range policies {
		const prefix = "The organization’s OAuth client policy "
		switch {
		case len(policy.AllowedAuthMethods) > 0 && !slices.Contains(policy.AllowedAuthMethods, method):
			return httpx.Invalid(prefix + "does not allow this client authentication method.")
		case app.Type != "service" && policy.RequirePAR && !app.RequirePAR:
			return httpx.Invalid(prefix + "requires pushed authorization requests.")
		case app.Type != "service" && !public && policy.RequirePKCE && !app.RequirePKCE:
			return httpx.Invalid(prefix + "requires PKCE.")
		case policy.RequireDPoP && !app.RequireDPoP:
			return httpx.Invalid(prefix + "requires DPoP.")
		case policy.MaxAccessTokenTTL > 0 && app.TokenPolicy.AccessTokenTTL > policy.MaxAccessTokenTTL:
			return httpx.Invalid(prefix + "limits the access token lifetime.")
		case policy.MaxIDTokenTTL > 0 && app.TokenPolicy.IDTokenTTL > policy.MaxIDTokenTTL:
			return httpx.Invalid(prefix + "limits the ID token lifetime.")
		case policy.MaxRefreshTokenTTL > 0 && app.TokenPolicy.RefreshTokenTTL > policy.MaxRefreshTokenTTL:
			return httpx.Invalid(prefix + "limits the refresh token lifetime.")
		}
	}
	return nil
}
