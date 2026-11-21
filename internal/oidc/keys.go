package oidc

import (
	"cmp"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"halo/internal/store"
)

var signatureAlgorithms = []jose.SignatureAlgorithm{jose.RS256, jose.ES256, jose.EdDSA}

type signingKey struct {
	id  string
	alg jose.SignatureAlgorithm
	key crypto.Signer
}

func (k signingKey) ID() string                                  { return k.id }
func (k signingKey) SignatureAlgorithm() jose.SignatureAlgorithm { return k.alg }
func (k signingKey) Key() any                                    { return k.key }

type publicKey struct{ signingKey }

func (k publicKey) Algorithm() jose.SignatureAlgorithm { return k.alg }
func (k publicKey) Use() string                        { return "sig" }
func (k publicKey) Key() any                           { return k.key.Public() }

type algorithmsKey struct{}

type algorithms struct {
	id, access jose.SignatureAlgorithm
	signAccess bool
	claims     map[string]any
}

func withAlgorithms(ctx context.Context) context.Context {
	return context.WithValue(ctx, algorithmsKey{}, &algorithms{})
}

func choose(ctx context.Context, id string, granted []store.GrantedResource, aud []string) error {
	a, ok := ctx.Value(algorithmsKey{}).(*algorithms)
	if !ok {
		return nil
	}
	a.id, a.access = jose.SignatureAlgorithm(id), ""
	for _, g := range granted {
		if !slices.Contains(aud, g.Identifier) {
			continue
		}
		if a.access != "" && a.access != jose.SignatureAlgorithm(g.AccessTokenAlg) {
			return oidc.ErrInvalidTarget().WithDescription("The APIs %s sign access tokens with different algorithms, so one token can't be addressed to all of them. Request their scopes in separate token requests.", strings.Join(aud, ", "))
		}
		a.access = jose.SignatureAlgorithm(g.AccessTokenAlg)
	}
	return nil
}

func signingAccessToken(ctx context.Context) {
	if a, ok := ctx.Value(algorithmsKey{}).(*algorithms); ok {
		a.signAccess = true
	}
}

func hookClaims(ctx context.Context) map[string]any {
	claims := map[string]any{}
	if a, ok := ctx.Value(algorithmsKey{}).(*algorithms); ok {
		maps.Copy(claims, a.claims)
	}
	return claims
}

func (s *storage) signingKeys(ctx context.Context, need jose.SignatureAlgorithm) ([]signingKey, error) {
	rows, err := s.st.ListSigningKeys(ctx)
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(rows, func(row store.SigningKey) bool { return row.Algorithm == string(need) }) {
		der, err := NewSigningKey(string(need))
		if err != nil {
			return nil, err
		}
		row, err := s.st.CreateSigningKey(ctx, string(need), der)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	keys := make([]signingKey, len(rows))
	for i, row := range rows {
		parsed, err := x509.ParsePKCS8PrivateKey(row.PrivateKey)
		key, ok := parsed.(crypto.Signer)
		if err != nil || !ok {
			return nil, fmt.Errorf("signing key %s is not a readable private key: %v", row.ID, err)
		}
		keys[i] = signingKey{row.ID, jose.SignatureAlgorithm(row.Algorithm), key}
	}
	return keys, nil
}

func NewSigningKey(algorithm string) ([]byte, error) {
	var key any
	var err error
	switch jose.SignatureAlgorithm(algorithm) {
	case jose.RS256:
		key, err = rsa.GenerateKey(rand.Reader, 2048)
	case jose.ES256:
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case jose.EdDSA:
		_, key, err = ed25519.GenerateKey(rand.Reader)
	default:
		return nil, fmt.Errorf("unsupported signing algorithm %q", algorithm)
	}
	if err != nil {
		return nil, err
	}
	return x509.MarshalPKCS8PrivateKey(key)
}

func (s *storage) SigningKey(ctx context.Context) (op.SigningKey, error) {
	alg := jose.RS256
	if a, ok := ctx.Value(algorithmsKey{}).(*algorithms); ok {
		chosen := a.id
		if a.signAccess {
			chosen, a.signAccess = a.access, false
		}
		alg = cmp.Or(chosen, jose.RS256)
	}
	keys, err := s.signingKeys(ctx, alg)
	if err != nil {
		return nil, err
	}
	return keys[slices.IndexFunc(keys, func(k signingKey) bool { return k.alg == alg })], nil
}

func (s *storage) sign(ctx context.Context, typ jose.ContentType, claims map[string]any) (string, error) {
	key, err := s.SigningKey(ctx)
	if err != nil {
		return "", err
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: key.SignatureAlgorithm(), Key: jose.JSONWebKey{Key: key.Key(), KeyID: key.ID()}}, (&jose.SignerOptions{}).WithType(typ))
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signed, err := signer.Sign(payload)
	if err != nil {
		return "", err
	}
	return signed.CompactSerialize()
}

func (s *storage) SignatureAlgorithms(context.Context) ([]jose.SignatureAlgorithm, error) {
	return signatureAlgorithms, nil
}

func (s *storage) KeySet(ctx context.Context) ([]op.Key, error) {
	keys, err := s.signingKeys(ctx, jose.RS256)
	if err != nil {
		return nil, err
	}
	set := make([]op.Key, len(keys))
	for i, k := range keys {
		set[i] = publicKey{k}
	}
	return set, nil
}
