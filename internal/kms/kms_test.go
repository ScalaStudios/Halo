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
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"halo/internal/config"
	"halo/internal/kms"
	"halo/internal/kms/kmstest"
	"halo/internal/metrics"
)

type fake interface {
	Wrap([]byte) string
	Has(string) bool
	Close()
}

var (
	everything = []string{"RS256", "ES256", "EdDSA"}
	noEdDSA    = []string{"RS256", "ES256"}
	bearer     = regexp.MustCompile(`^Bearer [A-Za-z0-9._-]+$`)
	sigV4      = regexp.MustCompile(`^AWS4-HMAC-SHA256 Credential=[A-Z]+/[0-9]{8}/eu-central-1/kms/aws4_request, SignedHeaders=content-type;host;x-amz-date;(x-amz-security-token;)?x-amz-target, Signature=[0-9a-f]{64}$`)
)

func TestProviders(t *testing.T) {
	for _, c := range []struct {
		name       string
		start      func(*testing.T) (fake, config.KMS)
		algorithms []string
		auth       *regexp.Regexp
	}{
		{"vault", func(t *testing.T) (fake, config.KMS) { f := kmstest.New(t); return f, f.Config() }, everything, nil},
		{"aws access key", func(t *testing.T) (fake, config.KMS) { f := kmstest.NewAWS(t); return f, f.Config() }, everything, sigV4},
		{"aws instance role", func(t *testing.T) (fake, config.KMS) { f := kmstest.NewAWS(t); return f, f.InstanceRoleConfig() }, everything, sigV4},
		{"gcp service account", func(t *testing.T) (fake, config.KMS) { f := kmstest.NewGCP(t); return f, f.Config() }, everything, bearer},
		{"gcp metadata server", func(t *testing.T) (fake, config.KMS) { f := kmstest.NewGCP(t); return f, f.MetadataConfig() }, everything, bearer},
		{"gcp hsm", func(t *testing.T) (fake, config.KMS) { f := kmstest.NewGCP(t); f.HSM = true; return f, f.Config() }, noEdDSA, bearer},
		{"azure client secret", func(t *testing.T) (fake, config.KMS) { f := kmstest.NewAzure(t); return f, f.Config() }, noEdDSA, bearer},
		{"azure workload identity", func(t *testing.T) (fake, config.KMS) { f := kmstest.NewAzure(t); return f, f.WorkloadIdentityConfig(t) }, noEdDSA, bearer},
		{"azure managed identity", func(t *testing.T) (fake, config.KMS) { f := kmstest.NewAzure(t); return f, f.ManagedIdentityConfig() }, noEdDSA, bearer},
		{"azure app service identity", func(t *testing.T) (fake, config.KMS) { f := kmstest.NewAzure(t); return f, f.AppServiceConfig() }, noEdDSA, bearer},
		{"azure hsm", func(t *testing.T) (fake, config.KMS) { f := kmstest.NewAzure(t); f.HSM = true; return f, f.Config() }, noEdDSA, bearer},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			f, cfg := c.start(t)
			keys := kms.FromConfig(cfg)

			secretKey := make([]byte, 32)
			_, _ = rand.Read(secretKey)
			if got, err := keys.SecretKey(ctx, f.Wrap(secretKey)); err != nil || !bytes.Equal(got, secretKey) {
				t.Fatalf("unwrap round trip: %x, %v", got, err)
			}
			if _, err := keys.SecretKey(ctx, f.Wrap([]byte("short"))); err == nil {
				t.Fatal("a wrapped key that is not 32 bytes must be refused")
			}
			if err := keys.Ping(ctx); err != nil {
				t.Fatal(err)
			}

			var signers []crypto.Signer
			for _, alg := range everything {
				ref, err := keys.CreateKey(ctx, alg)
				if !slices.Contains(c.algorithms, alg) {
					if err == nil || !strings.Contains(err.Error(), "can't sign with "+alg) {
						t.Fatalf("%s: %s must be refused: %q %v", keys.Name, alg, ref, err)
					}
					continue
				}
				if err != nil || !f.Has(ref) || kms.Location(ref) != keys.Name {
					t.Fatalf("%s: create %q: %v", alg, ref, err)
				}
				signer, err := keys.Signer(ctx, ref)
				if err != nil {
					t.Fatalf("%s: %v", alg, err)
				}
				if !verifies(signer) {
					t.Fatalf("%s: the signature does not verify with the public key", alg)
				}
				signers = append(signers, signer)
			}
			if auth, ok := f.(interface{ Authorization() string }); ok && !c.auth.MatchString(auth.Authorization()) {
				t.Fatalf("authorization header %q", auth.Authorization())
			}
			if _, err := keys.Signer(ctx, "halo-key_01JBVAULT"); err == nil && keys.Name != config.NameVault {
				t.Fatal("a key in another key manager must be refused")
			}

			failures := metrics.KMSErrors.Value("sign")
			f.Close()
			if _, err := signers[0].Sign(rand.Reader, digest[:], crypto.SHA256); err == nil {
				t.Fatal("signing with the key manager down must fail")
			}
			if metrics.KMSErrors.Value("sign") != failures+1 {
				t.Fatal("a failed request must count in halo_kms_errors_total")
			}
			if _, err := keys.CreateKey(ctx, "RS256"); err == nil {
				t.Fatal("creating a key with the key manager down must fail")
			}
			if err := keys.Ping(ctx); err == nil {
				t.Fatal("the startup check with the key manager down must fail")
			}
			if _, err := kms.FromConfig(cfg).SecretKey(ctx, f.Wrap(secretKey)); err == nil {
				t.Fatal("unwrapping with the key manager down must fail")
			}
		})
	}
}

var message = []byte("header.payload")
var digest = sha256.Sum256(message)

func verifies(signer crypto.Signer) bool {
	switch public := signer.Public().(type) {
	case *rsa.PublicKey:
		signature, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
		return err == nil && rsa.VerifyPKCS1v15(public, crypto.SHA256, digest[:], signature) == nil
	case *ecdsa.PublicKey:
		signature, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
		return err == nil && ecdsa.VerifyASN1(public, digest[:], signature)
	case ed25519.PublicKey:
		signature, err := signer.Sign(rand.Reader, message, crypto.Hash(0))
		return err == nil && ed25519.Verify(public, message, signature)
	}
	return false
}

func TestProviderRefusals(t *testing.T) {
	ctx := context.Background()
	aws := kmstest.NewAWS(t)
	keys := kms.FromConfig(aws.Config())
	ref, err := keys.CreateKey(ctx, "EdDSA")
	if err != nil {
		t.Fatal(err)
	}
	signer, err := keys.Signer(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := signer.Sign(rand.Reader, make([]byte, 4097), crypto.Hash(0)); err == nil || !strings.Contains(err.Error(), "4096 bytes") {
		t.Fatalf("AWS KMS signs at most 4096 bytes with Ed25519: %v", err)
	}
	if _, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256); err == nil {
		t.Fatal("an Ed25519 key must refuse a prehashed signature")
	}
	imported, _ := keys.CreateKey(ctx, "ES256")
	aws.Import(imported)
	if _, err := keys.Signer(ctx, imported); err == nil || !strings.Contains(err.Error(), "EXTERNAL") {
		t.Fatalf("a key with imported key material must be refused: %v", err)
	}
	if _, err := keys.Signer(ctx, "arn:aws:kms:eu-central-1:111122223333:alias/../key"); err == nil {
		t.Fatal("a reference that is not a key ARN must be refused")
	}
	wrong := aws.Config()
	wrong.AWS.SecretAccessKey = "not-the-secret"
	if _, err := kms.FromConfig(wrong).SecretKey(ctx, aws.Wrap(make([]byte, 32))); err == nil || !strings.Contains(err.Error(), "signature we calculated") {
		t.Fatalf("a wrong secret access key: %v", err)
	}

	azure := kmstest.NewAzure(t)
	keys = kms.FromConfig(azure.Config())
	exportable, _ := keys.CreateKey(ctx, "RS256")
	azure.AllowExport(exportable)
	if _, err := keys.Signer(ctx, exportable); err == nil || !strings.Contains(err.Error(), "exportable") {
		t.Fatalf("an exportable key must be refused: %v", err)
	}
	elsewhere := strings.Replace(exportable, "127.0.0.1", "localhost", 1)
	if _, err := keys.Signer(ctx, elsewhere); err == nil || !strings.Contains(err.Error(), "is not a key version in") {
		t.Fatalf("a key in another vault must be refused before the token is sent: %v", err)
	}
	pss, _ := keys.CreateKey(ctx, "RS256")
	signer, _ = keys.Signer(ctx, pss)
	if _, err := signer.Sign(rand.Reader, digest[:], &rsa.PSSOptions{Hash: crypto.SHA256}); err == nil {
		t.Fatal("PSS signatures must be refused")
	}

	gcp := kmstest.NewGCP(t)
	keys = kms.FromConfig(gcp.Config())
	if _, err := keys.Signer(ctx, "projects/acme/locations/europe-west1/keyRings/other/cryptoKeys/k/cryptoKeyVersions/1"); err == nil {
		t.Fatal("a key outside the configured key ring must be refused")
	}
	if _, err := keys.Signer(ctx, aws.URL); err == nil || !strings.Contains(err.Error(), "is in Azure Key Vault, but Halo is set up for Google Cloud KMS") {
		t.Fatalf("a key in another key manager: %v", err)
	}

	var none *kms.Keys
	if none.External() || none.Supports("EdDSA") != nil {
		t.Fatal("without a key manager, keys are local and every algorithm works")
	}
	if _, err := none.Signer(ctx, "halo-key_x"); err == nil || !strings.Contains(err.Error(), "HALO_VAULT_ADDR") {
		t.Fatalf("an external key without Vault configured: %v", err)
	}
	if _, err := none.Signer(ctx, imported); err == nil || !strings.Contains(err.Error(), "HALO_AWS_KMS_REGION") {
		t.Fatalf("an external key without AWS KMS configured: %v", err)
	}
}

func TestVaultTransit(t *testing.T) {
	ctx := context.Background()
	fake := kmstest.New(t)
	keys := kms.FromConfig(fake.Config())
	exported, _ := keys.CreateKey(ctx, "ES256")
	fake.AllowExport(exported)
	if _, err := keys.Signer(ctx, exported); err == nil || !strings.Contains(err.Error(), "exported") {
		t.Fatalf("an exportable key must be refused: %v", err)
	}
	if _, err := keys.Signer(ctx, "../../sys/seal"); err == nil {
		t.Fatal("a key reference that is not a key name must be refused")
	}
	wrongToken := fake.Config()
	wrongToken.Vault.Token = "other-token"
	failures := metrics.KMSErrors.Value("read")
	if _, err := kms.FromConfig(wrongToken).Signer(ctx, exported); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("a refused token: %v", err)
	}
	if metrics.KMSErrors.Value("read") != failures+1 {
		t.Fatal("a failed Vault request must count in halo_kms_errors_total")
	}
}

func TestVaultTokenRenewal(t *testing.T) {
	fake := kmstest.New(t)
	fake.ExpireToken(2 * time.Second)
	vault := kms.NewVault(fake.Config())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go vault.KeepRenewed(ctx)
	time.Sleep(3500 * time.Millisecond)
	if err := vault.Ping(ctx); err != nil || fake.Renewals() < 2 {
		t.Fatalf("the token must stay valid past its TTL: %d renewals, %v", fake.Renewals(), err)
	}
	cancel()
	time.Sleep(2500 * time.Millisecond)
	if err := vault.Ping(context.Background()); err == nil {
		t.Fatal("without renewal the fake must expire the token")
	}
}
