package kms

import (
	"context"
	"crypto"
	"errors"
	"fmt"

	"halo/internal/config"
)

type Provider interface {
	UnwrapKey(ctx context.Context, wrapped string) ([]byte, error)
	CreateKey(ctx context.Context, algorithm string) (string, error)
	Signer(ctx context.Context, ref string) (crypto.Signer, error)
	Ping(ctx context.Context) error
}

type Keys struct {
	Provider
	Signing bool
}

func FromConfig(cfg config.Vault) *Keys {
	if cfg.Address == "" {
		return nil
	}
	return &Keys{Provider: NewVault(cfg), Signing: cfg.Signing}
}

func (k *Keys) External() bool {
	return k != nil && k.Signing
}

func (k *Keys) Signer(ctx context.Context, ref string) (crypto.Signer, error) {
	if k == nil {
		return nil, fmt.Errorf("signing key %s is in Vault Transit, but HALO_VAULT_ADDR is not set, so Halo can't use it. Set HALO_VAULT_ADDR and HALO_VAULT_TOKEN", ref)
	}
	return k.Provider.Signer(ctx, ref)
}

func (k *Keys) SecretKey(ctx context.Context, wrapped string) ([]byte, error) {
	key, err := k.UnwrapKey(ctx, wrapped)
	if err != nil {
		return nil, fmt.Errorf("Halo couldn't unwrap the secret key with Vault Transit: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("the unwrapped secret key is not 32 bytes. Wrap a new one with: vault write -field=ciphertext transit/encrypt/<key> plaintext=\"$(openssl rand -base64 32)\"")
	}
	return key, nil
}
