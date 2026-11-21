package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	jose "github.com/go-jose/go-jose/v4"

	"halo/internal/httpx"
	"halo/internal/store"
)

var errAccessToken = errors.New("the access token is not a valid, active Halo CLI token")

func AccessTokenResolver(st *store.Store) httpx.AccessTokenResolver {
	s := &storage{st: st}
	return func(ctx context.Context, token string) (store.User, error) {
		t, subject, err := s.accessTokenRow(ctx, token)
		if err != nil {
			return store.User{}, err
		}
		if t.ClientID != CLIClientID || t.UserID == nil || *t.UserID != subject {
			return store.User{}, errAccessToken
		}
		return st.GetUser(ctx, subject)
	}
}

func (s *storage) accessTokenRow(ctx context.Context, token string) (store.OIDCToken, string, error) {
	jws, err := jose.ParseSigned(token, signatureAlgorithms)
	if err != nil || len(jws.Signatures) != 1 {
		return store.OIDCToken{}, "", errAccessToken
	}
	keys, err := s.signingKeys(ctx, jose.RS256)
	if err != nil {
		return store.OIDCToken{}, "", err
	}
	i := slices.IndexFunc(keys, func(k signingKey) bool { return k.id == jws.Signatures[0].Header.KeyID })
	if i < 0 {
		return store.OIDCToken{}, "", errAccessToken
	}
	payload, err := jws.Verify(keys[i].key.Public())
	if err != nil {
		return store.OIDCToken{}, "", errAccessToken
	}
	var claims struct {
		ID      string `json:"jti"`
		Subject string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return store.OIDCToken{}, "", errAccessToken
	}
	t, err := s.st.GetActiveOIDCToken(ctx, claims.ID)
	if err != nil || t.Kind != "access" || t.DPoPJKT != "" {
		return store.OIDCToken{}, "", errAccessToken
	}
	return t, claims.Subject, nil
}
