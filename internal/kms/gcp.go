package kms

import (
	"cmp"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"halo/internal/config"
	"halo/internal/id"
)

var (
	gcpAlgorithms = map[string]string{"RS256": "RSA_SIGN_PKCS1_2048_SHA256", "ES256": "EC_SIGN_P256_SHA256", "EdDSA": "EC_SIGN_ED25519"}
	gcpVersion    = regexp.MustCompile(`^projects/[a-z0-9-]+/locations/[a-z0-9-]+/keyRings/[A-Za-z0-9_-]+/cryptoKeys/[A-Za-z0-9_-]+/cryptoKeyVersions/[0-9]+$`)
)

type GCP struct {
	cfg         config.GCPKMS
	endpoint    string
	wrappingKey string
	token       *cached[string]
}

func NewGCP(cfg config.KMS) *GCP {
	g := &GCP{cfg: cfg.GCP, endpoint: cmp.Or(cfg.GCP.Endpoint, "https://cloudkms.googleapis.com"), wrappingKey: cfg.WrappingKey}
	g.token = &cached[string]{fetch: g.fetchToken}
	return g
}

func (g *GCP) UnwrapKey(ctx context.Context, wrapped string) ([]byte, error) {
	var out struct{ Plaintext []byte }
	err := g.call(ctx, "decrypt", http.MethodPost, g.cfg.KeyRing+"/cryptoKeys/"+g.wrappingKey+":decrypt", map[string]string{"ciphertext": wrapped}, &out)
	return out.Plaintext, err
}

func (g *GCP) CreateKey(ctx context.Context, algorithm string) (string, error) {
	kind, ok := gcpAlgorithms[algorithm]
	if !ok {
		return "", fmt.Errorf("Google Cloud KMS has no algorithm for %s", algorithm)
	}
	level := "SOFTWARE"
	if g.cfg.HSM {
		level = "HSM"
	}
	name := "halo-" + id.New("key")
	in := map[string]any{"purpose": "ASYMMETRIC_SIGN", "versionTemplate": map[string]string{"algorithm": kind, "protectionLevel": level}}
	if err := g.call(ctx, "create", http.MethodPost, g.cfg.KeyRing+"/cryptoKeys?cryptoKeyId="+name, in, nil); err != nil {
		return "", err
	}
	ref := g.cfg.KeyRing + "/cryptoKeys/" + name + "/cryptoKeyVersions/1"
	for range 120 {
		var version struct{ State string }
		if err := g.call(ctx, "create", http.MethodGet, ref, nil, &version); err != nil {
			return "", err
		}
		if version.State == "ENABLED" {
			return ref, nil
		}
		if version.State != "PENDING_GENERATION" {
			return "", fmt.Errorf("Google Cloud KMS key %s is %s", ref, version.State)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return "", fmt.Errorf("Google Cloud KMS took over a minute to generate key %s", ref)
}

func (g *GCP) Signer(ctx context.Context, ref string) (crypto.Signer, error) {
	if !gcpVersion.MatchString(ref) || !strings.HasPrefix(ref, g.cfg.KeyRing+"/cryptoKeys/") {
		return nil, fmt.Errorf("%q is not a key version in %s", ref, g.cfg.KeyRing)
	}
	var version struct{ State, Algorithm, ImportJob string }
	if err := g.call(ctx, "read", http.MethodGet, ref, nil, &version); err != nil {
		return nil, err
	}
	if version.State != "ENABLED" || version.ImportJob != "" || !slices.Contains(slices.Collect(maps.Values(gcpAlgorithms)), version.Algorithm) {
		return nil, fmt.Errorf("Google Cloud KMS key %s is %s with algorithm %s, so Halo won't sign with it. Halo signs only with enabled keys that Google Cloud KMS generated", ref, version.State, version.Algorithm)
	}
	var out struct{ Pem string }
	if err := g.call(ctx, "read", http.MethodGet, ref+"/publicKey", nil, &out); err != nil {
		return nil, err
	}
	block, _ := pem.Decode([]byte(out.Pem))
	if block == nil {
		return nil, fmt.Errorf("Google Cloud KMS key %s has no PEM public key", ref)
	}
	public, err := signingPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("Google Cloud KMS key %s: %w", ref, err)
	}
	return &signer{public: public, sign: func(digest []byte, hash crypto.Hash) ([]byte, error) {
		in := map[string]any{"data": digest}
		if _, ok := public.(ed25519.PublicKey); !ok {
			if hash != crypto.SHA256 {
				return nil, fmt.Errorf("Google Cloud KMS key %s signs SHA-256 digests only, not %s", ref, hash)
			}
			in = map[string]any{"digest": map[string][]byte{"sha256": digest}}
		}
		var out struct{ Signature []byte }
		if err := g.call(context.Background(), "sign", http.MethodPost, ref+":asymmetricSign", in, &out); err != nil {
			return nil, err
		}
		if len(out.Signature) == 0 {
			return nil, errors.New("google cloud kms sign: the response holds no signature")
		}
		return out.Signature, nil
	}}, nil
}

func (g *GCP) Ping(ctx context.Context) error {
	return g.call(ctx, "ping", http.MethodGet, g.cfg.KeyRing, nil, nil)
}

func (g *GCP) call(ctx context.Context, operation, method, path string, in, out any) error {
	return observe(operation, "google cloud kms", func() error {
		token, err := g.token.get(ctx)
		if err != nil {
			return err
		}
		req, _, err := newRequest(ctx, method, g.endpoint+"/v1/"+path, in)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return send(req, out)
	})
}

func (g *GCP) fetchToken(ctx context.Context) (string, time.Time, error) {
	var req *http.Request
	var err error
	if g.cfg.CredentialsFile != "" {
		assertion, tokenURI, err := g.assertion()
		if err != nil {
			return "", time.Time{}, err
		}
		form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
		if req, err = http.NewRequestWithContext(ctx, http.MethodPost, tokenURI, strings.NewReader(form.Encode())); err != nil {
			return "", time.Time{}, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		host := cmp.Or(g.cfg.MetadataHost, "metadata.google.internal")
		if req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://"+host+"/computeMetadata/v1/instance/service-accounts/default/token", nil); err != nil {
			return "", time.Time{}, err
		}
		req.Header.Set("Metadata-Flavor", "Google")
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := send(req, &out); err != nil {
		return "", time.Time{}, err
	}
	if out.AccessToken == "" {
		return "", time.Time{}, errors.New("the token response holds no access token")
	}
	return out.AccessToken, time.Now().Add(time.Duration(out.ExpiresIn) * time.Second), nil
}

func (g *GCP) assertion() (string, string, error) {
	raw, err := os.ReadFile(g.cfg.CredentialsFile)
	if err != nil {
		return "", "", err
	}
	var account struct {
		Type         string
		ClientEmail  string `json:"client_email"`
		PrivateKey   string `json:"private_key"`
		PrivateKeyID string `json:"private_key_id"`
		TokenURI     string `json:"token_uri"`
	}
	if err := json.Unmarshal(raw, &account); err != nil || account.Type != "service_account" {
		return "", "", errors.New("GOOGLE_APPLICATION_CREDENTIALS must point at the JSON key file of a service account")
	}
	tokenURI := cmp.Or(account.TokenURI, "https://oauth2.googleapis.com/token")
	if u, err := url.Parse(tokenURI); err != nil || u.Scheme != "https" && !(u.Scheme == "http" && config.Loopback(u.Hostname())) {
		return "", "", fmt.Errorf("the service account's token_uri %q is not an https address", tokenURI)
	}
	block, _ := pem.Decode([]byte(account.PrivateKey))
	if block == nil {
		return "", "", errors.New("the service account key file has no PEM private key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	key, ok := parsed.(*rsa.PrivateKey)
	if err != nil || !ok {
		return "", "", fmt.Errorf("the service account key is not an RSA key: %v", err)
	}
	encode := func(v any) string {
		data, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(data)
	}
	now := time.Now().Unix()
	unsigned := encode(map[string]string{"alg": "RS256", "typ": "JWT", "kid": account.PrivateKeyID}) + "." +
		encode(map[string]any{"iss": account.ClientEmail, "scope": "https://www.googleapis.com/auth/cloudkms", "aud": tokenURI, "iat": now, "exp": now + 3600})
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(nil, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), tokenURI, nil
}
