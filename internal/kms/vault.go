package kms

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"halo/internal/config"
	"halo/internal/id"
	"halo/internal/metrics"
)

var (
	keyTypes   = map[string]string{"RS256": "rsa-2048", "ES256": "ecdsa-p256", "EdDSA": "ed25519"}
	hashes     = map[crypto.Hash]string{crypto.SHA256: "sha2-256", crypto.SHA384: "sha2-384", crypto.SHA512: "sha2-512"}
	keyName    = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	operations = []string{"decrypt", "create", "read", "sign", "ping"}
)

type Vault struct {
	cfg     config.Vault
	client  *http.Client
	mu      sync.Mutex
	signers map[string]*vaultSigner
}

func NewVault(cfg config.Vault) *Vault {
	for _, op := range operations {
		metrics.KMSErrors.Add(0, op)
	}
	return &Vault{cfg: cfg, signers: map[string]*vaultSigner{}, client: &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (v *Vault) UnwrapKey(ctx context.Context, wrapped string) ([]byte, error) {
	var out struct{ Data struct{ Plaintext string } }
	if err := v.call(ctx, "decrypt", http.MethodPost, v.cfg.Mount+"/decrypt/"+v.cfg.WrappingKey, map[string]string{"ciphertext": wrapped}, &out); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(out.Data.Plaintext)
}

func (v *Vault) CreateKey(ctx context.Context, algorithm string) (string, error) {
	kind, ok := keyTypes[algorithm]
	if !ok {
		return "", fmt.Errorf("Vault Transit has no key type for %s", algorithm)
	}
	name := "halo-" + id.New("key")
	return name, v.call(ctx, "create", http.MethodPost, v.cfg.Mount+"/keys/"+name, map[string]string{"type": kind}, nil)
}

func (v *Vault) Signer(ctx context.Context, ref string) (crypto.Signer, error) {
	v.mu.Lock()
	cached, ok := v.signers[ref]
	v.mu.Unlock()
	if ok {
		return cached, nil
	}
	if !keyName.MatchString(ref) {
		return nil, fmt.Errorf("%q is not a Vault Transit key name", ref)
	}
	var out struct {
		Data struct {
			Type                 string
			Exportable           bool
			AllowPlaintextBackup bool `json:"allow_plaintext_backup"`
			Keys                 map[string]struct {
				PublicKey string `json:"public_key"`
			}
		}
	}
	if err := v.call(ctx, "read", http.MethodGet, v.cfg.Mount+"/keys/"+ref, nil, &out); err != nil {
		return nil, err
	}
	if out.Data.Exportable || out.Data.AllowPlaintextBackup {
		return nil, fmt.Errorf("Vault Transit lets key %s be exported, so Halo won't sign with it", ref)
	}
	public, err := publicKey(out.Data.Type, out.Data.Keys["1"].PublicKey)
	if err != nil {
		return nil, fmt.Errorf("Vault Transit key %s: %w", ref, err)
	}
	s := &vaultSigner{vault: v, name: ref, public: public}
	v.mu.Lock()
	v.signers[ref] = s
	v.mu.Unlock()
	return s, nil
}

func (v *Vault) Ping(ctx context.Context) error {
	return v.call(ctx, "ping", http.MethodGet, "auth/token/lookup-self", nil, nil)
}

func publicKey(kind, encoded string) (crypto.PublicKey, error) {
	if kind == "ed25519" {
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return nil, errors.New("the public key is not an Ed25519 key")
		}
		return ed25519.PublicKey(raw), nil
	}
	block, _ := pem.Decode([]byte(encoded))
	if block == nil {
		return nil, fmt.Errorf("a %q key has no PEM public key", kind)
	}
	public, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	switch public.(type) {
	case *rsa.PublicKey, *ecdsa.PublicKey:
		return public, nil
	}
	return nil, fmt.Errorf("a %q key can't sign", kind)
}

func (v *Vault) call(ctx context.Context, operation, method, path string, in, out any) error {
	start := time.Now()
	err := v.send(ctx, method, path, in, out)
	metrics.KMSDuration.Observe(time.Since(start).Seconds(), operation)
	if err != nil {
		metrics.KMSErrors.Inc(operation)
		return fmt.Errorf("vault %s: %w", operation, err)
	}
	return nil
}

func (v *Vault) send(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, v.cfg.Address+"/v1/"+path, body)
	if err != nil {
		return err
	}
	if v.cfg.Token != "" {
		req.Header.Set("X-Vault-Token", v.cfg.Token)
	}
	if v.cfg.Namespace != "" {
		req.Header.Set("X-Vault-Namespace", v.cfg.Namespace)
	}
	res, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	limited := io.LimitReader(res.Body, 1<<20)
	if res.StatusCode >= 300 {
		var failure struct{ Errors []string }
		_ = json.NewDecoder(limited).Decode(&failure)
		return errors.New(strings.Join(append([]string{res.Status}, failure.Errors...), ": "))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(limited).Decode(out)
}

type vaultSigner struct {
	vault  *Vault
	name   string
	public crypto.PublicKey
}

func (s *vaultSigner) Public() crypto.PublicKey {
	return s.public
}

func (s *vaultSigner) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	in := map[string]any{"input": base64.StdEncoding.EncodeToString(digest), "key_version": 1}
	path := s.vault.cfg.Mount + "/sign/" + s.name
	if opts.HashFunc() != 0 {
		hash, ok := hashes[opts.HashFunc()]
		if !ok {
			return nil, fmt.Errorf("Vault Transit can't sign a %s digest", opts.HashFunc())
		}
		path += "/" + hash
		in["prehashed"] = true
	}
	if _, ok := s.public.(*rsa.PublicKey); ok {
		if _, pss := opts.(*rsa.PSSOptions); pss {
			return nil, errors.New("Halo signs with PKCS #1 v1.5 RSA signatures only")
		}
		in["signature_algorithm"] = "pkcs1v15"
	}
	var out struct{ Data struct{ Signature string } }
	if err := s.vault.call(context.Background(), "sign", http.MethodPost, path, in, &out); err != nil {
		return nil, err
	}
	parts := strings.Split(out.Data.Signature, ":")
	if len(parts) != 3 || parts[0] != "vault" {
		return nil, errors.New("vault sign: the response holds no signature")
	}
	return base64.StdEncoding.DecodeString(parts[2])
}
