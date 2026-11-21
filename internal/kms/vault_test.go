package kms_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"strings"
	"testing"

	"halo/internal/kms"
	"halo/internal/kms/kmstest"
	"halo/internal/metrics"
)

func TestVaultTransit(t *testing.T) {
	ctx := context.Background()
	fake := kmstest.New(t)
	keys := kms.FromConfig(fake.Config())

	secretKey := make([]byte, 32)
	_, _ = rand.Read(secretKey)
	if got, err := keys.SecretKey(ctx, fake.Wrap(secretKey)); err != nil || !bytes.Equal(got, secretKey) {
		t.Fatalf("unwrap round trip: %x, %v", got, err)
	}
	if _, err := keys.SecretKey(ctx, fake.Wrap([]byte("short"))); err == nil {
		t.Fatal("a wrapped key that is not 32 bytes must be refused")
	}

	message := []byte("header.payload")
	digest := sha256.Sum256(message)
	for _, alg := range []string{"RS256", "ES256", "EdDSA"} {
		ref, err := keys.CreateKey(ctx, alg)
		if err != nil || !fake.Has(ref) {
			t.Fatalf("%s: create %q: %v", alg, ref, err)
		}
		signer, err := keys.Signer(ctx, ref)
		if err != nil {
			t.Fatalf("%s: %v", alg, err)
		}
		var ok bool
		switch public := signer.Public().(type) {
		case *rsa.PublicKey:
			signature, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
			ok = err == nil && rsa.VerifyPKCS1v15(public, crypto.SHA256, digest[:], signature) == nil
		case *ecdsa.PublicKey:
			signature, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
			ok = err == nil && ecdsa.VerifyASN1(public, digest[:], signature)
		case ed25519.PublicKey:
			signature, err := signer.Sign(rand.Reader, message, crypto.Hash(0))
			ok = err == nil && ed25519.Verify(public, message, signature)
		}
		if !ok {
			t.Fatalf("%s: the signature from Vault does not verify with its public key", alg)
		}
	}

	exported, _ := keys.CreateKey(ctx, "ES256")
	fake.AllowExport(exported)
	if _, err := keys.Signer(ctx, exported); err == nil || !strings.Contains(err.Error(), "exported") {
		t.Fatalf("an exportable key must be refused: %v", err)
	}
	if _, err := keys.Signer(ctx, "../../sys/seal"); err == nil {
		t.Fatal("a key reference that is not a key name must be refused")
	}

	wrongToken := fake.Config()
	wrongToken.Token = "other-token"
	failures := metrics.KMSErrors.Value("read")
	if _, err := kms.FromConfig(wrongToken).Signer(ctx, exported); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("a refused token: %v", err)
	}
	if metrics.KMSErrors.Value("read") != failures+1 {
		t.Fatal("a failed Vault request must count in halo_kms_errors_total")
	}

	fake.Close()
	if _, err := keys.CreateKey(ctx, "RS256"); err == nil {
		t.Fatal("creating a key with Vault down must fail")
	}
	if _, err := keys.SecretKey(ctx, fake.Wrap(secretKey)); err == nil {
		t.Fatal("unwrapping with Vault down must fail")
	}
	var none *kms.Keys
	if none.External() {
		t.Fatal("without Vault, keys are local")
	}
	if _, err := none.Signer(ctx, "halo-key_x"); err == nil || !strings.Contains(err.Error(), "HALO_VAULT_ADDR") {
		t.Fatalf("an external key without Vault configured: %v", err)
	}
}
