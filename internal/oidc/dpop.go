package oidc

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"

	"halo/internal/dpop"
	"halo/internal/store"
)

const dpopTokenType = "DPoP"

type dpopKey struct{}

func withDPoP(ctx context.Context, jkt string) context.Context {
	return context.WithValue(ctx, dpopKey{}, jkt)
}

func dpopJKT(ctx context.Context) string {
	jkt, _ := ctx.Value(dpopKey{}).(string)
	return jkt
}

func invalidProof(description string) *oidc.Error {
	return &oidc.Error{ErrorType: "invalid_dpop_proof", Description: description}
}

func checkThumbprint(jkt string) error {
	if hash, err := base64.RawURLEncoding.DecodeString(jkt); jkt != "" && (err != nil || len(hash) != sha256.Size) {
		return oidc.ErrInvalidRequest().WithDescription("dpop_jkt must be the base64url SHA-256 JWK thumbprint of the DPoP key.")
	}
	return nil
}

func confirm(ctx context.Context, claims map[string]any) {
	if jkt := dpopJKT(ctx); jkt != "" {
		claims["cnf"] = map[string]any{"jkt": jkt}
	}
}

func (s *storage) proof(r *http.Request, accessToken string) (string, error) {
	target, _ := url.Parse(s.issuer + r.URL.Path)
	proof, err := dpop.Verify(r, target, accessToken, time.Now())
	if err != nil {
		return "", invalidProof(err.Error())
	}
	if err := s.st.UseDPoPProof(r.Context(), proof.Thumbprint, proof.ID, proof.ExpiresAt); errors.Is(err, store.ErrConflict) {
		return "", invalidProof("The DPoP proof was already used. Create a new proof with a new jti for every request.")
	} else if err != nil {
		return "", err
	}
	return proof.Thumbprint, nil
}
