package oidc

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	"halo/internal/store"
)

const (
	dpopWindow    = time.Minute
	dpopTokenType = "DPoP"
)

var dpopAlgorithms = []jose.SignatureAlgorithm{jose.ES256, jose.ES384, jose.ES512, jose.PS256, jose.PS384, jose.PS512, jose.RS256, jose.EdDSA}

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
	values := r.Header.Values(oidc.DPoPHeader)
	if len(values) != 1 {
		return "", invalidProof("Send exactly one DPoP header.")
	}
	jws, err := jose.ParseSigned(values[0], dpopAlgorithms)
	if err != nil || len(jws.Signatures) != 1 {
		return "", invalidProof("The DPoP proof must be a JWT signed with ES256, ES384, ES512, PS256, PS384, PS512, RS256 or EdDSA.")
	}
	header := jws.Signatures[0].Protected
	jwk := header.JSONWebKey
	if typ, _ := header.ExtraHeaders[jose.HeaderType].(string); typ != string(oidc.DPoPProofType) || jwk == nil || !jwk.Valid() || !jwk.IsPublic() || oidc.ValidateDPoPKeyStrength(jwk.Key) != nil {
		return "", invalidProof("The DPoP proof needs the typ dpop+jwt and the public key that signed it in jwk.")
	}
	payload, err := jws.Verify(jwk.Key)
	if err != nil {
		return "", invalidProof("The DPoP proof signature does not match the key in its jwk header.")
	}
	var claims struct {
		ID              string `json:"jti"`
		Method          string `json:"htm"`
		URI             string `json:"htu"`
		IssuedAt        *int64 `json:"iat"`
		AccessTokenHash string `json:"ath"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.ID == "" || len(claims.ID) > 256 || claims.IssuedAt == nil {
		return "", invalidProof("The DPoP proof needs jti, htm, htu and iat claims.")
	}
	if claims.Method != r.Method {
		return "", invalidProof("The DPoP proof's htm must be " + r.Method + ".")
	}
	want, _ := url.Parse(s.issuer + r.URL.Path)
	if htu, err := url.Parse(claims.URI); err != nil || want == nil || !strings.EqualFold(htu.Scheme, want.Scheme) || !strings.EqualFold(htu.Host, want.Host) || htu.Path != want.Path {
		return "", invalidProof("The DPoP proof's htu must be " + s.issuer + r.URL.Path + ".")
	}
	issuedAt := time.Unix(*claims.IssuedAt, 0)
	if time.Since(issuedAt).Abs() > dpopWindow {
		return "", invalidProof("The DPoP proof's iat must be within a minute of the current time. Create a new proof for every request.")
	}
	if accessToken != "" {
		hash := sha256.Sum256([]byte(accessToken))
		if claims.AccessTokenHash != base64.RawURLEncoding.EncodeToString(hash[:]) {
			return "", invalidProof("The DPoP proof's ath must be the base64url SHA-256 hash of the access token.")
		}
	}
	jkt, err := oidc.JWKThumbprint(jwk)
	if err != nil {
		return "", invalidProof("Halo could not compute the thumbprint of the DPoP proof's jwk.")
	}
	if err := s.st.UseDPoPProof(r.Context(), jkt, claims.ID, issuedAt.Add(dpopWindow)); errors.Is(err, store.ErrConflict) {
		return "", invalidProof("The DPoP proof was already used. Create a new proof with a new jti for every request.")
	} else if err != nil {
		return "", err
	}
	return jkt, nil
}
