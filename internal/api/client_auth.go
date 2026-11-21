package api

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"

	"halo/internal/httpx"
	"halo/internal/oidc"
	"halo/internal/store"
	"halo/internal/webhooks"
)

type clientAuthChange struct {
	TokenEndpointAuthMethod *string         `json:"tokenEndpointAuthMethod"`
	JWKS                    json.RawMessage `json:"jwks"`
	JWKSURI                 *string         `json:"jwksUri"`
	FAPI2Profile            *bool           `json:"fapi2Profile"`
}

func clientKeys(method string, jwks json.RawMessage, jwksURI string) error {
	if method != oidc.PrivateKeyJWT {
		return nil
	}
	if len(jwks) > 0 == (jwksURI != "") {
		return httpx.Invalid("Register the application's public keys either as a JWKS or as a JWKS URL, not both and not neither.")
	}
	if jwksURI != "" {
		_, err := webhooks.PublicURL(jwksURI, "JWKS", "Halo only fetches public keys from")
		return err
	}
	if _, err := oidc.ClientKeys(jwks); err != nil {
		return httpx.Invalid("The JWKS is not usable: " + err.Error() + ".")
	}
	return nil
}

func compactJSON(raw json.RawMessage) json.RawMessage {
	var out bytes.Buffer
	if len(raw) == 0 || json.Compact(&out, raw) != nil {
		return raw
	}
	return out.Bytes()
}

func (c clientAuthChange) apply(app store.Application, next *store.Application) ([]string, error) {
	requested := c.TokenEndpointAuthMethod != nil || c.JWKS != nil || c.JWKSURI != nil || c.FAPI2Profile != nil
	confidential := (app.Protocol == "oidc" || app.Protocol == "oauth") && (app.Type == "web" || app.Type == "service") && app.ClientID != oidc.CLIClientID
	if requested && !confidential {
		return nil, httpx.Invalid(app.Name + " has no client secret to replace, so it can't use private_key_jwt or the FAPI 2.0 profile. Only web and service applications can.")
	}
	changed := []string{}
	if c.TokenEndpointAuthMethod != nil && *c.TokenEndpointAuthMethod != app.TokenEndpointAuthMethod {
		if *c.TokenEndpointAuthMethod != oidc.ClientSecretBasic && *c.TokenEndpointAuthMethod != oidc.PrivateKeyJWT {
			return nil, httpx.Invalid(`Set tokenEndpointAuthMethod to "client_secret_basic" or "private_key_jwt".`)
		}
		next.TokenEndpointAuthMethod = *c.TokenEndpointAuthMethod
		changed = append(changed, "client authentication")
	}
	if c.JWKS != nil {
		var keys json.RawMessage
		if string(c.JWKS) != "null" {
			keys = compactJSON(c.JWKS)
		}
		if !bytes.Equal(keys, app.JWKS) {
			next.JWKS = keys
			changed = append(changed, "public keys")
		}
	}
	if c.JWKSURI != nil && strings.TrimSpace(*c.JWKSURI) != app.JWKSURI {
		next.JWKSURI = strings.TrimSpace(*c.JWKSURI)
		changed = append(changed, "JWKS URL")
	}
	if c.FAPI2Profile != nil && *c.FAPI2Profile != app.FAPI2Profile {
		next.FAPI2Profile = *c.FAPI2Profile
		changed = append(changed, "FAPI 2.0 profile")
	}
	if next.FAPI2Profile {
		more, err := fapi2(app, next)
		if err != nil {
			return nil, err
		}
		changed = append(changed, more...)
	}
	if err := clientKeys(next.TokenEndpointAuthMethod, next.JWKS, next.JWKSURI); err != nil && len(changed) > 0 {
		return nil, err
	}
	return changed, nil
}

func fapi2(app store.Application, next *store.Application) ([]string, error) {
	switch {
	case app.FAPI2Profile && (app.RequirePAR && !next.RequirePAR || app.RequirePKCE && !next.RequirePKCE || app.RequireDPoP && !next.RequireDPoP || app.TokenEndpointAuthMethod == oidc.PrivateKeyJWT && next.TokenEndpointAuthMethod != oidc.PrivateKeyJWT || app.TokenPolicy.IDTokenAlg != "RS256" && next.TokenPolicy.IDTokenAlg == "RS256"):
		return nil, httpx.Invalid(app.Name + " follows the FAPI 2.0 security profile, which requires pushed requests, PKCE, DPoP, private_key_jwt and ES256 or EdDSA signatures. Turn the profile off first.")
	case next.TokenEndpointAuthMethod != oidc.PrivateKeyJWT:
		return nil, httpx.Invalid("The FAPI 2.0 security profile needs private_key_jwt client authentication. Register the application's public keys and switch it to private_key_jwt, in this change or before.")
	case app.Type == "web" && slices.ContainsFunc(next.RedirectURIs, func(uri string) bool { return !strings.HasPrefix(uri, "https://") }):
		return nil, httpx.Invalid("The FAPI 2.0 security profile allows only https redirect URIs for web applications. Remove the http:// ones first.")
	}
	changed := []string{}
	for _, f := range []struct {
		label string
		field *bool
		on    bool
	}{{"pushed authorization requirement", &next.RequirePAR, app.Type == "web"}, {"PKCE requirement", &next.RequirePKCE, app.Type == "web"}, {"DPoP requirement", &next.RequireDPoP, true}} {
		if f.on && !*f.field {
			*f.field = true
			changed = append(changed, f.label)
		}
	}
	if next.TokenPolicy.IDTokenAlg == "RS256" {
		next.TokenPolicy.IDTokenAlg = "ES256"
		changed = append(changed, "ID token signing algorithm")
	}
	return changed, nil
}
