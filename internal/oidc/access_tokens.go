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
		jws, err := jose.ParseSigned(token, []jose.SignatureAlgorithm{jose.RS256})
		if err != nil || len(jws.Signatures) != 1 {
			return store.User{}, errAccessToken
		}
		keys, err := s.signingKeys(ctx)
		if err != nil {
			return store.User{}, err
		}
		i := slices.IndexFunc(keys, func(k signingKey) bool { return k.id == jws.Signatures[0].Header.KeyID })
		if i < 0 {
			return store.User{}, errAccessToken
		}
		payload, err := jws.Verify(&keys[i].key.PublicKey)
		if err != nil {
			return store.User{}, errAccessToken
		}
		var claims struct {
			ID      string `json:"jti"`
			Subject string `json:"sub"`
		}
		if err := json.Unmarshal(payload, &claims); err != nil {
			return store.User{}, errAccessToken
		}
		t, err := st.GetActiveOIDCToken(ctx, claims.ID)
		if err != nil || t.Kind != "access" || t.ClientID != CLIClientID || t.UserID == nil || *t.UserID != claims.Subject {
			return store.User{}, errAccessToken
		}
		return st.GetUser(ctx, claims.Subject)
	}
}
