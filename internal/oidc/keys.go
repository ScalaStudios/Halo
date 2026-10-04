package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"fmt"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/op"
)

type signingKey struct {
	id  string
	key *rsa.PrivateKey
}

func (k signingKey) ID() string                                  { return k.id }
func (k signingKey) SignatureAlgorithm() jose.SignatureAlgorithm { return jose.RS256 }
func (k signingKey) Key() any                                    { return k.key }

type publicKey struct{ signingKey }

func (k publicKey) Algorithm() jose.SignatureAlgorithm { return jose.RS256 }
func (k publicKey) Use() string                        { return "sig" }
func (k publicKey) Key() any                           { return &k.key.PublicKey }

func (s *storage) signingKeys(ctx context.Context) ([]signingKey, error) {
	rows, err := s.st.ListSigningKeys(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		der, err := NewSigningKey()
		if err != nil {
			return nil, err
		}
		row, err := s.st.CreateSigningKey(ctx, string(jose.RS256), der)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	keys := make([]signingKey, len(rows))
	for i, row := range rows {
		parsed, err := x509.ParsePKCS8PrivateKey(row.PrivateKey)
		key, ok := parsed.(*rsa.PrivateKey)
		if err != nil || !ok {
			return nil, fmt.Errorf("signing key %s is not a readable RSA key: %v", row.ID, err)
		}
		keys[i] = signingKey{row.ID, key}
	}
	return keys, nil
}

func NewSigningKey() ([]byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	return x509.MarshalPKCS8PrivateKey(key)
}

func (s *storage) SigningKey(ctx context.Context) (op.SigningKey, error) {
	keys, err := s.signingKeys(ctx)
	if err != nil {
		return nil, err
	}
	return keys[0], nil
}

func (s *storage) SignatureAlgorithms(context.Context) ([]jose.SignatureAlgorithm, error) {
	return []jose.SignatureAlgorithm{jose.RS256}, nil
}

func (s *storage) KeySet(ctx context.Context) ([]op.Key, error) {
	keys, err := s.signingKeys(ctx)
	if err != nil {
		return nil, err
	}
	set := make([]op.Key, len(keys))
	for i, k := range keys {
		set[i] = publicKey{k}
	}
	return set, nil
}
