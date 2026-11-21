package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"halo/internal/httpx"
	"halo/internal/store"
)

const (
	PrivateKeyJWT     = "private_key_jwt"
	ClientSecretBasic = "client_secret_basic"

	assertionLifetime = time.Hour
	assertionLeeway   = time.Minute
	fapiCodeLifetime  = time.Minute
	clientKeyLimit    = 20
)

var (
	JWKSClient          = httpx.GuardedClient()
	assertionAlgorithms = []jose.SignatureAlgorithm{jose.RS256, jose.RS384, jose.RS512, jose.PS256, jose.PS384, jose.PS512, jose.ES256, jose.ES384, jose.ES512, jose.EdDSA}
	fapiAlgorithms      = []jose.SignatureAlgorithm{jose.PS256, jose.ES256, jose.EdDSA}
)

type assertedKey struct{}

func assertedClient(ctx context.Context) string {
	clientID, _ := ctx.Value(assertedKey{}).(string)
	return clientID
}

func ClientKeys(raw json.RawMessage) ([]jose.JSONWebKey, error) {
	var set jose.JSONWebKeySet
	if err := json.Unmarshal(raw, &set); err != nil {
		return nil, errors.New("jwks must be a JSON Web Key Set: an object with a keys array")
	}
	if len(set.Keys) == 0 || len(set.Keys) > clientKeyLimit {
		return nil, fmt.Errorf("jwks must hold between 1 and %d keys", clientKeyLimit)
	}
	if slices.ContainsFunc(set.Keys, func(k jose.JSONWebKey) bool { return !publicSigningKey(k) }) {
		return nil, errors.New("jwks may only hold public RSA keys of 2048 bits or more, P-256, P-384 or P-521 EC keys, or Ed25519 keys, for signing")
	}
	return set.Keys, nil
}

func publicSigningKey(k jose.JSONWebKey) bool {
	return k.Valid() && k.IsPublic() && (k.Use == "" || k.Use == oidc.KeyUseSignature) && oidc.ValidateDPoPKeyStrength(k.Key) == nil
}

func (s *storage) application(ctx context.Context, clientID string) bool {
	_, err := s.st.GetApplicationByClientID(ctx, clientID)
	return clientID == "" || !errors.Is(err, store.ErrNotFound)
}

func (s *storage) authenticateClient(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			op.RequestError(w, r, oidc.ErrInvalidRequest().WithDescription("Halo could not read the request body. Send it as application/x-www-form-urlencoded."), nil)
			return
		}
		if !r.Form.Has("client_assertion") && !r.Form.Has("client_assertion_type") {
			next(w, r)
			return
		}
		clientID, err := s.verifyAssertion(r)
		var oidcErr *oidc.Error
		if err != nil && !errors.As(err, &oidcErr) {
			err = oidc.ErrServerError().WithDescription("Halo could not check client_assertion. Try again; if it keeps failing, check the server log.").WithParent(err)
		}
		if err != nil {
			op.RequestError(w, r, err, nil)
			return
		}
		for _, form := range []url.Values{r.Form, r.PostForm} {
			form.Del("client_assertion")
			form.Del("client_assertion_type")
			form.Set("client_id", clientID)
		}
		r.SetBasicAuth(url.QueryEscape(clientID), "")
		next(w, r.WithContext(context.WithValue(r.Context(), assertedKey{}, clientID)))
	}
}

func (s *storage) verifyAssertion(r *http.Request) (string, error) {
	ctx, form := r.Context(), r.Form
	invalid := func(format string, args ...any) error {
		return oidc.ErrInvalidClient().WithDescription(format, args...)
	}
	if form.Get("client_assertion_type") != oidc.ClientAssertionTypeJWTAssertion {
		return "", invalid("Set client_assertion_type to %s.", oidc.ClientAssertionTypeJWTAssertion)
	}
	if r.Header.Get("Authorization") != "" || form.Has("client_secret") {
		return "", oidc.ErrInvalidRequest().WithDescription("Authenticate with one method: send client_assertion without a client secret or an Authorization header.")
	}
	tok, err := jwt.ParseSigned(form.Get("client_assertion"), assertionAlgorithms)
	var unverified jwt.Claims
	if err != nil || len(tok.Headers) != 1 || tok.UnsafeClaimsWithoutVerification(&unverified) != nil {
		return "", invalid("client_assertion must be a JWT signed with RS256, RS384, RS512, PS256, PS384, PS512, ES256, ES384, ES512 or EdDSA. Halo refuses none and HMAC algorithms.")
	}
	clientID := unverified.Issuer
	if id := form.Get("client_id"); id != "" && id != clientID {
		return "", invalid("client_id must match the iss claim of client_assertion.")
	}
	app, err := s.app(ctx, clientID)
	if err != nil {
		return "", err
	}
	if app.TokenEndpointAuthMethod != PrivateKeyJWT || (client{app: app}).public() {
		return "", invalid("%s authenticates with its client secret, not with client_assertion.", app.Name)
	}
	header := tok.Headers[0]
	alg := jose.SignatureAlgorithm(header.Algorithm)
	if app.FAPI2Profile && !slices.Contains(fapiAlgorithms, alg) {
		return "", invalid("%s follows the FAPI 2.0 security profile, so client_assertion must be signed with PS256, ES256 or EdDSA.", app.Name)
	}
	key, err := s.assertionKey(ctx, app, header.KeyID, alg)
	if err != nil {
		return "", err
	}
	var claims jwt.Claims
	var audience struct {
		Value json.RawMessage `json:"aud"`
	}
	if err := tok.Claims(key.Key, &claims, &audience); errors.Is(err, jose.ErrCryptoFailure) {
		return "", invalid("The signature of client_assertion does not match the key %q that %s registered.", key.KeyID, app.Name)
	} else if err != nil {
		return "", invalid("client_assertion has claims of the wrong type.")
	}
	now := time.Now()
	switch {
	case claims.Subject != clientID:
		return "", invalid("The iss and sub claims of client_assertion must both be the client_id.")
	case claims.ID == "" || len(claims.ID) > 256:
		return "", invalid("client_assertion needs a jti claim of at most 256 characters, unique for every request.")
	case claims.Expiry == nil:
		return "", invalid("client_assertion needs an exp claim.")
	case claims.Expiry.Time().After(now.Add(assertionLifetime)):
		return "", invalid("client_assertion must expire within an hour.")
	}
	switch err := claims.ValidateWithLeeway(jwt.Expected{Time: now}, assertionLeeway); {
	case errors.Is(err, jwt.ErrExpired):
		return "", invalid("client_assertion expired at %s.", claims.Expiry.Time().UTC().Format(time.RFC3339))
	case errors.Is(err, jwt.ErrNotValidYet):
		return "", invalid("client_assertion is not valid before %s.", claims.NotBefore.Time().UTC().Format(time.RFC3339))
	case err != nil:
		return "", invalid("client_assertion was issued in the future. Check the client's clock.")
	}
	var single string
	if app.FAPI2Profile {
		if json.Unmarshal(audience.Value, &single) != nil || single != s.issuer {
			return "", invalid("%s follows the FAPI 2.0 security profile, so the aud claim of client_assertion must be the string %s.", app.Name, s.issuer)
		}
	} else if !slices.ContainsFunc([]string{s.issuer, s.issuer + "/oauth2/token", s.issuer + r.URL.Path}, claims.Audience.Contains) {
		return "", invalid("The aud claim of client_assertion must be %s or %s/oauth2/token.", s.issuer, s.issuer)
	}
	if err := s.st.UseClientAssertion(ctx, clientID, claims.ID, claims.Expiry.Time().Add(assertionLeeway)); errors.Is(err, store.ErrConflict) {
		return "", invalid("client_assertion was already used. Sign a new one with a new jti for every request.")
	} else if err != nil {
		return "", err
	}
	return clientID, nil
}

func (s *storage) assertionKey(ctx context.Context, app store.Application, kid string, alg jose.SignatureAlgorithm) (jose.JSONWebKey, error) {
	if app.JWKSURI == "" {
		keys, err := ClientKeys(app.JWKS)
		if err != nil {
			return jose.JSONWebKey{}, oidc.ErrInvalidClient().WithDescription("%s has no usable public keys. Register its keys in Halo.", app.Name)
		}
		if key, err := oidc.FindMatchingKey(kid, oidc.KeyUseSignature, string(alg), keys...); err == nil {
			return key, nil
		}
		return jose.JSONWebKey{}, oidc.ErrInvalidClient().WithDescription("client_assertion is signed with a key that %s has not registered.", app.Name)
	}
	for _, stale := range []bool{false, true} {
		keys, err := s.keys.get(ctx, app.JWKSURI, stale, func(ctx context.Context) ([]jose.JSONWebKey, error) {
			if !httpx.ValidIssuer(app.JWKSURI, s.dev) {
				return nil, fmt.Errorf("jwks_uri %q is not an https URL", app.JWKSURI)
			}
			return fetchKeySet(ctx, JWKSClient, app.JWKSURI)
		})
		if err != nil {
			return jose.JSONWebKey{}, oidc.ErrInvalidClient().WithDescription("Halo could not fetch the public keys of %s from its jwks_uri. Try again in a few minutes.", app.Name).WithParent(err)
		}
		if key, err := oidc.FindMatchingKey(kid, oidc.KeyUseSignature, string(alg), keys...); err == nil && publicSigningKey(key) {
			return key, nil
		}
	}
	return jose.JSONWebKey{}, oidc.ErrInvalidClient().WithDescription("client_assertion is signed with a key that %s does not publish at its jwks_uri.", app.Name)
}
