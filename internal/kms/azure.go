package kms

import (
	"cmp"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"halo/internal/config"
	"halo/internal/id"
)

var (
	azureHashes = map[crypto.Hash]string{crypto.SHA256: "256", crypto.SHA384: "384", crypto.SHA512: "512"}
	azureKeyID  = regexp.MustCompile(`^/keys/[0-9A-Za-z-]+/[0-9a-f]{32}$`)
)

type Azure struct {
	cfg         config.AzureKeyVault
	vault       *url.URL
	resource    string
	wrappingKey string
	token       *cached[string]
}

type jsonWebKey struct {
	Kty, Crv, N, E, X, Y string
}

func NewAzure(cfg config.KMS) *Azure {
	vault, _ := url.Parse(cfg.Azure.URL)
	_, suffix, _ := strings.Cut(vault.Hostname(), ".")
	a := &Azure{cfg: cfg.Azure, vault: vault, resource: "https://" + suffix, wrappingKey: cfg.WrappingKey}
	a.token = &cached[string]{fetch: a.fetchToken}
	return a
}

func (a *Azure) UnwrapKey(ctx context.Context, wrapped string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(wrapped)
	if err != nil {
		return nil, errors.New("the wrapped key is not base64")
	}
	var out struct{ Value string }
	in := map[string]string{"alg": "RSA-OAEP-256", "value": base64.RawURLEncoding.EncodeToString(raw)}
	if err := a.call(ctx, "decrypt", http.MethodPost, a.cfg.URL+"/keys/"+a.wrappingKey+"/unwrapkey", in, &out); err != nil {
		return nil, err
	}
	return base64URL(out.Value)
}

func (a *Azure) CreateKey(ctx context.Context, algorithm string) (string, error) {
	in := map[string]any{"key_ops": []string{"sign", "verify"}}
	switch algorithm {
	case "RS256":
		in["kty"], in["key_size"] = "RSA", 2048
	case "ES256":
		in["kty"], in["crv"] = "EC", "P-256"
	default:
		return "", fmt.Errorf("Azure Key Vault has no key type for %s", algorithm)
	}
	if a.cfg.HSM {
		in["kty"] = in["kty"].(string) + "-HSM"
	}
	name := "halo-" + strings.ReplaceAll(id.New("key"), "_", "-")
	var out struct{ Key struct{ Kid string } }
	if err := a.call(ctx, "create", http.MethodPost, a.cfg.URL+"/keys/"+name+"/create", in, &out); err != nil {
		return "", err
	}
	if !a.ours(out.Key.Kid) {
		return "", fmt.Errorf("Azure Key Vault returned %q, which is not a key in %s", out.Key.Kid, a.cfg.URL)
	}
	return out.Key.Kid, nil
}

func (a *Azure) Signer(ctx context.Context, ref string) (crypto.Signer, error) {
	if !a.ours(ref) {
		return nil, fmt.Errorf("%q is not a key version in %s", ref, a.cfg.URL)
	}
	var out struct {
		Key        jsonWebKey
		Attributes struct{ Enabled, Exportable bool }
	}
	if err := a.call(ctx, "read", http.MethodGet, ref, nil, &out); err != nil {
		return nil, err
	}
	if !out.Attributes.Enabled || out.Attributes.Exportable {
		return nil, fmt.Errorf("Azure Key Vault key %s is disabled or exportable, so Halo won't sign with it", ref)
	}
	public, err := out.Key.public()
	if err != nil {
		return nil, fmt.Errorf("Azure Key Vault key %s: %w", ref, err)
	}
	return &signer{public: public, sign: func(digest []byte, hash crypto.Hash) ([]byte, error) {
		suffix, ok := azureHashes[hash]
		alg := "RS" + suffix
		if _, ec := public.(*ecdsa.PublicKey); ec {
			alg, ok = "ES256", hash == crypto.SHA256
		}
		if !ok {
			return nil, fmt.Errorf("Azure Key Vault key %s can't sign a %s digest", ref, hash)
		}
		var out struct{ Value string }
		if err := a.call(context.Background(), "sign", http.MethodPost, ref+"/sign", map[string]string{"alg": alg, "value": base64.RawURLEncoding.EncodeToString(digest)}, &out); err != nil {
			return nil, err
		}
		signature, err := base64URL(out.Value)
		if err != nil || len(signature) == 0 {
			return nil, errors.New("azure key vault sign: the response holds no signature")
		}
		if alg == "ES256" {
			return asn1Signature(signature)
		}
		return signature, nil
	}}, nil
}

func (a *Azure) Ping(ctx context.Context) error {
	return a.call(ctx, "ping", http.MethodGet, a.cfg.URL+"/keys?maxresults=1", nil, nil)
}

func (a *Azure) ours(kid string) bool {
	u, err := url.Parse(kid)
	return err == nil && u.Scheme == a.vault.Scheme && strings.EqualFold(u.Host, a.vault.Host) && u.RawQuery == "" && u.User == nil && azureKeyID.MatchString(u.Path)
}

func (a *Azure) call(ctx context.Context, operation, method, target string, in, out any) error {
	return observe(operation, "azure key vault", func() error {
		token, err := a.token.get(ctx)
		if err != nil {
			return err
		}
		separator := "?"
		if strings.Contains(target, "?") {
			separator = "&"
		}
		req, _, err := newRequest(ctx, method, target+separator+"api-version=7.4", in)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return send(req, out)
	})
}

func (a *Azure) fetchToken(ctx context.Context) (string, time.Time, error) {
	var req *http.Request
	var err error
	switch {
	case a.cfg.ClientSecret != "" || a.cfg.FederatedTokenFile != "":
		form := url.Values{"grant_type": {"client_credentials"}, "client_id": {a.cfg.ClientID}, "scope": {a.resource + "/.default"}, "client_secret": {a.cfg.ClientSecret}}
		if a.cfg.FederatedTokenFile != "" {
			assertion, err := os.ReadFile(a.cfg.FederatedTokenFile)
			if err != nil {
				return "", time.Time{}, err
			}
			form.Del("client_secret")
			form.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
			form.Set("client_assertion", strings.TrimSpace(string(assertion)))
		}
		authority := cmp.Or(a.cfg.AuthorityHost, "https://login.microsoftonline.com")
		if req, err = http.NewRequestWithContext(ctx, http.MethodPost, authority+"/"+url.PathEscape(a.cfg.TenantID)+"/oauth2/v2.0/token", strings.NewReader(form.Encode())); err != nil {
			return "", time.Time{}, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	case a.cfg.IdentityEndpoint != "":
		query := url.Values{"api-version": {"2019-08-01"}, "resource": {a.resource}}
		if a.cfg.ClientID != "" {
			query.Set("client_id", a.cfg.ClientID)
		}
		if req, err = http.NewRequestWithContext(ctx, http.MethodGet, a.cfg.IdentityEndpoint+"?"+query.Encode(), nil); err != nil {
			return "", time.Time{}, err
		}
		req.Header.Set("X-IDENTITY-HEADER", a.cfg.IdentityHeader)
	default:
		query := url.Values{"api-version": {"2018-02-01"}, "resource": {a.resource}}
		if a.cfg.ClientID != "" {
			query.Set("client_id", a.cfg.ClientID)
		}
		if req, err = http.NewRequestWithContext(ctx, http.MethodGet, cmp.Or(a.cfg.MetadataEndpoint, "http://169.254.169.254")+"/metadata/identity/oauth2/token?"+query.Encode(), nil); err != nil {
			return "", time.Time{}, err
		}
		req.Header.Set("Metadata", "true")
	}
	var out struct {
		AccessToken string          `json:"access_token"`
		ExpiresIn   json.RawMessage `json:"expires_in"`
		ExpiresOn   json.RawMessage `json:"expires_on"`
	}
	if err := send(req, &out); err != nil {
		return "", time.Time{}, err
	}
	number := func(raw json.RawMessage) int64 {
		n, _ := strconv.ParseInt(strings.Trim(string(raw), `"`), 10, 64)
		return n
	}
	expires := time.Now().Add(time.Duration(number(out.ExpiresIn)) * time.Second)
	if out.ExpiresIn == nil {
		expires = time.Unix(number(out.ExpiresOn), 0)
	}
	if out.AccessToken == "" {
		return "", time.Time{}, errors.New("the token response holds no access token")
	}
	return out.AccessToken, expires, nil
}

func (k jsonWebKey) public() (crypto.PublicKey, error) {
	decode := func(field string) *big.Int {
		raw, _ := base64URL(field)
		return new(big.Int).SetBytes(raw)
	}
	switch strings.TrimSuffix(k.Kty, "-HSM") {
	case "RSA":
		n, e := decode(k.N), decode(k.E)
		if n.BitLen() < 2048 || !e.IsInt64() || e.Int64() < 3 || e.Int64() > 1<<31-1 {
			return nil, errors.New("the RSA public key is not usable")
		}
		return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
	case "EC":
		x, _ := base64URL(k.X)
		y, _ := base64URL(k.Y)
		if k.Crv != "P-256" || len(x) != 32 || len(y) != 32 {
			return nil, fmt.Errorf("the %s key is not a P-256 key", k.Crv)
		}
		return ecdsa.ParseUncompressedPublicKey(elliptic.P256(), append(append([]byte{4}, x...), y...))
	}
	return nil, fmt.Errorf("a %s key can't sign", k.Kty)
}

func base64URL(value string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(value, "="))
}
