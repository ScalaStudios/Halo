package kms

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"halo/internal/config"
	"halo/internal/id"
)

var (
	vaultKeyTypes = map[string]string{"RS256": "rsa-2048", "ES256": "ecdsa-p256", "EdDSA": "ed25519"}
	vaultHashes   = map[crypto.Hash]string{crypto.SHA256: "sha2-256", crypto.SHA384: "sha2-384", crypto.SHA512: "sha2-512"}
	keyName       = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

type Vault struct {
	cfg         config.Vault
	wrappingKey string
}

func NewVault(cfg config.KMS) *Vault {
	return &Vault{cfg: cfg.Vault, wrappingKey: cfg.WrappingKey}
}

func (v *Vault) UnwrapKey(ctx context.Context, wrapped string) ([]byte, error) {
	var out struct{ Data struct{ Plaintext string } }
	if err := v.call(ctx, "decrypt", http.MethodPost, v.cfg.Mount+"/decrypt/"+v.wrappingKey, map[string]string{"ciphertext": wrapped}, &out); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(out.Data.Plaintext)
}

func (v *Vault) CreateKey(ctx context.Context, algorithm string) (string, error) {
	kind, ok := vaultKeyTypes[algorithm]
	if !ok {
		return "", fmt.Errorf("Vault Transit has no key type for %s", algorithm)
	}
	name := "halo-" + id.New("key")
	return name, v.call(ctx, "create", http.MethodPost, v.cfg.Mount+"/keys/"+name, map[string]string{"type": kind}, nil)
}

func (v *Vault) Signer(ctx context.Context, ref string) (crypto.Signer, error) {
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
	public, err := vaultPublicKey(out.Data.Type, out.Data.Keys["1"].PublicKey)
	if err != nil {
		return nil, fmt.Errorf("Vault Transit key %s: %w", ref, err)
	}
	return &signer{public: public, sign: func(digest []byte, hash crypto.Hash) ([]byte, error) {
		return v.sign(ref, public, digest, hash)
	}}, nil
}

func (v *Vault) Ping(ctx context.Context) error {
	return v.call(ctx, "ping", http.MethodGet, "auth/token/lookup-self", nil, nil)
}

func (v *Vault) KeepRenewed(ctx context.Context) {
	if v.cfg.Token == "" {
		return
	}
	method, path := http.MethodGet, "auth/token/lookup-self"
	var body any
	var wait time.Duration
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		var out struct {
			Data struct {
				TTL       int
				Renewable bool
			}
			Auth struct {
				LeaseDuration int `json:"lease_duration"`
				Renewable     bool
			}
		}
		if err := v.call(ctx, "renew", method, path, body, &out); err != nil {
			slog.Error("Halo couldn't renew its Vault token and tries again in 10 seconds", "error", err)
			wait = 10 * time.Second
			continue
		}
		ttl := out.Data.TTL + out.Auth.LeaseDuration
		if !out.Data.Renewable && !out.Auth.Renewable || ttl <= 0 {
			if method == http.MethodPost {
				slog.Error("Vault won't renew Halo's token any further, so requests to Vault fail once it expires. Give Halo a new token", "ttl_seconds", ttl)
			}
			return
		}
		method, path, body, wait = http.MethodPost, "auth/token/renew-self", map[string]string{}, time.Duration(ttl)*time.Second*2/3
	}
}

func (v *Vault) sign(name string, public crypto.PublicKey, digest []byte, hash crypto.Hash) ([]byte, error) {
	in := map[string]any{"input": base64.StdEncoding.EncodeToString(digest), "key_version": 1}
	path := v.cfg.Mount + "/sign/" + name
	if hash != 0 {
		suffix, ok := vaultHashes[hash]
		if !ok {
			return nil, fmt.Errorf("Vault Transit can't sign a %s digest", hash)
		}
		path += "/" + suffix
		in["prehashed"] = true
	}
	if _, ok := public.(*rsa.PublicKey); ok {
		in["signature_algorithm"] = "pkcs1v15"
	}
	var out struct{ Data struct{ Signature string } }
	if err := v.call(context.Background(), "sign", http.MethodPost, path, in, &out); err != nil {
		return nil, err
	}
	parts := strings.Split(out.Data.Signature, ":")
	if len(parts) != 3 || parts[0] != "vault" {
		return nil, errors.New("vault sign: the response holds no signature")
	}
	return base64.StdEncoding.DecodeString(parts[2])
}

func vaultPublicKey(kind, encoded string) (crypto.PublicKey, error) {
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
	public, err := signingPublicKey(block.Bytes)
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
	return observe(operation, "vault", func() error {
		req, _, err := newRequest(ctx, method, v.cfg.Address+"/v1/"+path, in)
		if err != nil {
			return err
		}
		if v.cfg.Token != "" {
			req.Header.Set("X-Vault-Token", v.cfg.Token)
		}
		if v.cfg.Namespace != "" {
			req.Header.Set("X-Vault-Namespace", v.cfg.Namespace)
		}
		return send(req, out)
	})
}
