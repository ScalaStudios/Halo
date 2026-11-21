package kms

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"halo/internal/config"
	"halo/internal/metrics"
)

type Provider interface {
	UnwrapKey(ctx context.Context, wrapped string) ([]byte, error)
	CreateKey(ctx context.Context, algorithm string) (string, error)
	Signer(ctx context.Context, ref string) (crypto.Signer, error)
	Ping(ctx context.Context) error
}

type Keys struct {
	Provider
	Name       string
	Signing    bool
	algorithms []string
	wrap       string
	mu         sync.Mutex
	signers    map[string]crypto.Signer
}

var (
	operations = []string{"decrypt", "create", "read", "sign", "ping", "token", "renew"}
	settings   = map[string]string{config.NameVault: "HALO_VAULT_ADDR", config.NameAWS: "HALO_AWS_KMS_REGION", config.NameGCP: "HALO_GCP_KMS_KEY_RING", config.NameAzure: "HALO_AZURE_KEY_VAULT_URL", config.NamePKCS11: "HALO_PKCS11_MODULE"}
	client     = &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
)

func FromConfig(cfg config.KMS) *Keys {
	name := cfg.Name()
	if name == "" {
		return nil
	}
	for _, op := range operations {
		metrics.KMSErrors.Add(0, op)
	}
	k := &Keys{Name: name, Signing: cfg.Signing, algorithms: []string{"RS256", "ES256", "EdDSA"}, wrap: cfg.WrapCommand(), signers: map[string]crypto.Signer{}}
	switch name {
	case config.NameVault:
		k.Provider = NewVault(cfg)
	case config.NameAWS:
		k.Provider = NewAWS(cfg)
	case config.NameGCP:
		k.Provider = NewGCP(cfg)
		if cfg.GCP.HSM {
			k.algorithms = k.algorithms[:2]
		}
	case config.NameAzure:
		k.Provider = NewAzure(cfg)
		k.algorithms = k.algorithms[:2]
	case config.NamePKCS11:
		k.Provider = NewPKCS11(cfg)
	}
	return k
}

func Location(ref string) string {
	switch {
	case strings.HasPrefix(ref, "arn:"):
		return config.NameAWS
	case strings.HasPrefix(ref, "projects/"):
		return config.NameGCP
	case strings.Contains(ref, "://"):
		return config.NameAzure
	case strings.HasPrefix(ref, "pkcs11:"):
		return config.NamePKCS11
	}
	return config.NameVault
}

func (k *Keys) External() bool {
	return k != nil && k.Signing
}

func (k *Keys) Supports(algorithm string) error {
	if !k.External() || slices.Contains(k.algorithms, algorithm) {
		return nil
	}
	return fmt.Errorf("%s can't sign with %s. Choose %s", k.Name, algorithm, strings.Join(k.algorithms, " or "))
}

func (k *Keys) CreateKey(ctx context.Context, algorithm string) (string, error) {
	if err := k.Supports(algorithm); err != nil {
		return "", err
	}
	return k.Provider.CreateKey(ctx, algorithm)
}

func (k *Keys) Signer(ctx context.Context, ref string) (crypto.Signer, error) {
	where := Location(ref)
	if k == nil {
		return nil, fmt.Errorf("signing key %s is in %s, but %s is not set, so Halo can't use it", ref, where, settings[where])
	}
	if where != k.Name {
		return nil, fmt.Errorf("signing key %s is in %s, but Halo is set up for %s", ref, where, k.Name)
	}
	k.mu.Lock()
	cached, ok := k.signers[ref]
	k.mu.Unlock()
	if ok {
		return cached, nil
	}
	s, err := k.Provider.Signer(ctx, ref)
	if err != nil {
		return nil, err
	}
	k.mu.Lock()
	k.signers[ref] = s
	k.mu.Unlock()
	return s, nil
}

func (k *Keys) KeepRenewed(ctx context.Context) {
	if k == nil {
		return
	}
	if vault, ok := k.Provider.(*Vault); ok {
		vault.KeepRenewed(ctx)
	}
}

func (k *Keys) SecretKey(ctx context.Context, wrapped string) ([]byte, error) {
	key, err := k.UnwrapKey(ctx, wrapped)
	if err != nil {
		return nil, fmt.Errorf("Halo couldn't unwrap the secret key with %s: %w", k.Name, err)
	}
	if len(key) != 32 {
		return nil, errors.New("the unwrapped secret key is not 32 bytes. Wrap a new one with: " + k.wrap)
	}
	return key, nil
}

func measure(operation string, start time.Time, err error) error {
	metrics.KMSDuration.Observe(time.Since(start).Seconds(), operation)
	if err != nil {
		metrics.KMSErrors.Inc(operation)
	}
	return err
}

func observe(operation, provider string, fn func() error) error {
	if err := measure(operation, time.Now(), fn()); err != nil {
		return fmt.Errorf("%s %s: %w", provider, operation, err)
	}
	return nil
}

func newRequest(ctx context.Context, method, target string, in any) (*http.Request, []byte, error) {
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return nil, nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err == nil && in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, body, err
}

func send(req *http.Request, out any) error {
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	limited := io.LimitReader(res.Body, 1<<20)
	if res.StatusCode >= 300 {
		var failure struct {
			Errors      []string
			Type        string `json:"__type"`
			Message     string
			Error       json.RawMessage
			Description string `json:"error_description"`
		}
		_ = json.NewDecoder(limited).Decode(&failure)
		parts := append([]string{res.Status}, failure.Errors...)
		var nested struct{ Message string }
		var code string
		if json.Unmarshal(failure.Error, &nested) == nil && nested.Message != "" {
			parts = append(parts, nested.Message)
		} else if json.Unmarshal(failure.Error, &code) == nil && code != "" {
			parts = append(parts, code)
		}
		for _, part := range []string{failure.Type, failure.Message, failure.Description} {
			if part != "" {
				parts = append(parts, part)
			}
		}
		return errors.New(strings.Join(parts, ": "))
	}
	if text, ok := out.(*string); ok {
		raw, err := io.ReadAll(limited)
		*text = string(raw)
		return err
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(limited).Decode(out)
}

type cached[T any] struct {
	mu      sync.Mutex
	value   T
	refresh time.Time
	fetch   func(context.Context) (T, time.Time, error)
}

func (c *cached[T]) get(ctx context.Context) (T, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Now().Before(c.refresh) {
		return c.value, nil
	}
	start := time.Now()
	value, expires, err := c.fetch(ctx)
	if err = measure("token", start, err); err != nil {
		var zero T
		return zero, fmt.Errorf("credentials: %w", err)
	}
	c.value, c.refresh = value, expires.Add(-min(time.Until(expires)/2, 5*time.Minute))
	return value, nil
}

type signer struct {
	public crypto.PublicKey
	sign   func(digest []byte, hash crypto.Hash) ([]byte, error)
}

func (s *signer) Public() crypto.PublicKey {
	return s.public
}

func (s *signer) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	hash := opts.HashFunc()
	switch s.public.(type) {
	case *rsa.PublicKey:
		if _, pss := opts.(*rsa.PSSOptions); pss {
			return nil, errors.New("Halo signs with PKCS #1 v1.5 RSA signatures only")
		}
	case ed25519.PublicKey:
		if o, ok := opts.(*ed25519.Options); hash != 0 || ok && o.Context != "" {
			return nil, errors.New("Halo signs with pure Ed25519 only")
		}
	}
	if hash != 0 && len(digest) != hash.Size() {
		return nil, fmt.Errorf("the digest is not a %s digest", hash)
	}
	return s.sign(digest, hash)
}

func signingPublicKey(der []byte) (crypto.PublicKey, error) {
	public, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, err
	}
	switch public.(type) {
	case *rsa.PublicKey, *ecdsa.PublicKey, ed25519.PublicKey:
		return public, nil
	}
	return nil, fmt.Errorf("a %T can't sign", public)
}

func asn1Signature(raw []byte) ([]byte, error) {
	if len(raw) != 64 {
		return nil, fmt.Errorf("the ECDSA signature is %d bytes, not 64", len(raw))
	}
	return asn1.Marshal(struct{ R, S *big.Int }{new(big.Int).SetBytes(raw[:32]), new(big.Int).SetBytes(raw[32:])})
}
