package dpop

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

const proofWindow = time.Minute

var dpopAlgorithms = []jose.SignatureAlgorithm{jose.ES256, jose.ES384, jose.ES512, jose.PS256, jose.PS384, jose.PS512, jose.RS256, jose.EdDSA}

func Algorithms() []jose.SignatureAlgorithm {
	return slices.Clone(dpopAlgorithms)
}

type Proof struct {
	Thumbprint string
	ID         string
	ExpiresAt  time.Time
}

func Verify(r *http.Request, target *url.URL, accessToken string, now time.Time) (Proof, error) {
	values := r.Header.Values(oidc.DPoPHeader)
	if len(values) != 1 {
		return Proof{}, errors.New("Send exactly one DPoP header.")
	}
	jws, err := jose.ParseSignedCompact(values[0], dpopAlgorithms)
	if err != nil || len(jws.Signatures) != 1 {
		return Proof{}, errors.New("The DPoP proof must be a JWT signed with ES256, ES384, ES512, PS256, PS384, PS512, RS256 or EdDSA.")
	}
	header := jws.Signatures[0].Protected
	jwk := header.JSONWebKey
	if typ, _ := header.ExtraHeaders[jose.HeaderType].(string); typ != string(oidc.DPoPProofType) || jwk == nil || !jwk.Valid() || !jwk.IsPublic() || oidc.ValidateDPoPKeyStrength(jwk.Key) != nil {
		return Proof{}, errors.New("The DPoP proof needs the typ dpop+jwt and the public key that signed it in jwk.")
	}
	payload, err := jws.Verify(jwk.Key)
	if err != nil {
		return Proof{}, errors.New("The DPoP proof signature does not match the key in its jwk header.")
	}
	var claims struct {
		ID              string `json:"jti"`
		Method          string `json:"htm"`
		URI             string `json:"htu"`
		IssuedAt        *int64 `json:"iat"`
		AccessTokenHash string `json:"ath"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.ID == "" || len(claims.ID) > 256 || claims.IssuedAt == nil {
		return Proof{}, errors.New("The DPoP proof needs jti, htm, htu and iat claims.")
	}
	if claims.Method != r.Method {
		return Proof{}, errors.New("The DPoP proof's htm must be " + r.Method + ".")
	}
	if htu, err := url.Parse(claims.URI); err != nil || target == nil || !strings.EqualFold(htu.Scheme, target.Scheme) || !strings.EqualFold(htu.Host, target.Host) || htu.Path != target.Path {
		return Proof{}, errors.New("The DPoP proof's htu must match the request URI.")
	}
	issuedAt := time.Unix(*claims.IssuedAt, 0)
	if now.Sub(issuedAt).Abs() > proofWindow {
		return Proof{}, errors.New("The DPoP proof's iat must be within a minute of the current time. Create a new proof for every request.")
	}
	if accessToken != "" {
		hash := sha256.Sum256([]byte(accessToken))
		if claims.AccessTokenHash != base64.RawURLEncoding.EncodeToString(hash[:]) {
			return Proof{}, errors.New("The DPoP proof's ath must be the base64url SHA-256 hash of the access token.")
		}
	}
	jkt, err := oidc.JWKThumbprint(jwk)
	if err != nil {
		return Proof{}, errors.New("Halo could not compute the thumbprint of the DPoP proof's jwk.")
	}
	return Proof{Thumbprint: jkt, ID: claims.ID, ExpiresAt: issuedAt.Add(proofWindow)}, nil
}
