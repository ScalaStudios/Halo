//go:build pkcs11

package kms_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/miekg/pkcs11"

	"halo/internal/config"
	"halo/internal/kms"
)

func TestPKCS11(t *testing.T) {
	module := os.Getenv("HALO_TEST_PKCS11_MODULE")
	if module == "" {
		t.Skip("set HALO_TEST_PKCS11_MODULE to a PKCS #11 module with a token labelled halo and user PIN 1234, such as SoftHSM")
	}
	ctx := context.Background()
	cfg := config.KMS{PKCS11: config.PKCS11{Module: module, TokenLabel: "halo", PIN: "1234"}, WrappingKey: "halo-secret-key", Signing: true}
	public := wrappingKey(t, module)
	wrongPIN := cfg
	wrongPIN.PKCS11.PIN = "0000"
	if err := kms.FromConfig(wrongPIN).Ping(ctx); err == nil || !strings.Contains(err.Error(), "PIN") {
		t.Fatalf("a wrong PIN must fail the startup check: %v", err)
	}
	keys := kms.FromConfig(cfg)
	if err := keys.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	secretKey := make([]byte, 32)
	_, _ = rand.Read(secretKey)
	sealed, _ := rsa.EncryptOAEP(sha256.New(), rand.Reader, public, secretKey, nil)
	got, err := keys.SecretKey(ctx, base64.StdEncoding.EncodeToString(sealed))
	switch {
	case err != nil && strings.Contains(err.Error(), "CKR_ARGUMENTS_BAD"):
		t.Log("the module refuses RSA-OAEP with SHA-256, as SoftHSM 2.6 does, so unwrapping is not checked")
	case err != nil || !bytes.Equal(got, secretKey):
		t.Fatalf("unwrap round trip: %x, %v", got, err)
	}

	for _, alg := range everything {
		ref, err := keys.CreateKey(ctx, alg)
		if err != nil || kms.Location(ref) != config.NamePKCS11 {
			t.Fatalf("%s: create %q: %v", alg, ref, err)
		}
		signer, err := keys.Signer(ctx, ref)
		if err != nil {
			t.Fatalf("%s: %v", alg, err)
		}
		if !verifies(signer) {
			t.Fatalf("%s: the signature from the token does not verify with its public key", alg)
		}
	}
	if _, err := keys.Signer(ctx, "pkcs11:object=halo-secret-key"); err == nil {
		t.Fatal("a key Halo didn't create must be refused")
	}
}

func wrappingKey(t *testing.T, module string) *rsa.PublicKey {
	t.Helper()
	m := pkcs11.New(module)
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}
	defer m.Destroy()
	defer m.Finalize()
	slots, _ := m.GetSlotList(true)
	for _, slot := range slots {
		info, _ := m.GetTokenInfo(slot)
		if strings.TrimSpace(info.Label) != "halo" {
			continue
		}
		s, err := m.OpenSession(slot, pkcs11.CKF_SERIAL_SESSION|pkcs11.CKF_RW_SESSION)
		if err != nil {
			t.Fatal(err)
		}
		defer m.CloseSession(s)
		if err := m.Login(s, pkcs11.CKU_USER, "1234"); err != nil {
			t.Fatal(err)
		}
		pub, _, err := m.GenerateKeyPair(s, []*pkcs11.Mechanism{pkcs11.NewMechanism(pkcs11.CKM_RSA_PKCS_KEY_PAIR_GEN, nil)},
			[]*pkcs11.Attribute{pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PUBLIC_KEY), pkcs11.NewAttribute(pkcs11.CKA_TOKEN, true), pkcs11.NewAttribute(pkcs11.CKA_LABEL, "halo-secret-key"), pkcs11.NewAttribute(pkcs11.CKA_ENCRYPT, true), pkcs11.NewAttribute(pkcs11.CKA_MODULUS_BITS, 2048), pkcs11.NewAttribute(pkcs11.CKA_PUBLIC_EXPONENT, []byte{1, 0, 1})},
			[]*pkcs11.Attribute{pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PRIVATE_KEY), pkcs11.NewAttribute(pkcs11.CKA_TOKEN, true), pkcs11.NewAttribute(pkcs11.CKA_LABEL, "halo-secret-key"), pkcs11.NewAttribute(pkcs11.CKA_DECRYPT, true), pkcs11.NewAttribute(pkcs11.CKA_SENSITIVE, true), pkcs11.NewAttribute(pkcs11.CKA_EXTRACTABLE, false)})
		if err != nil {
			t.Fatal(err)
		}
		attributes, err := m.GetAttributeValue(s, pub, []*pkcs11.Attribute{pkcs11.NewAttribute(pkcs11.CKA_MODULUS, nil), pkcs11.NewAttribute(pkcs11.CKA_PUBLIC_EXPONENT, nil)})
		if err != nil {
			t.Fatal(err)
		}
		_ = m.Logout(s)
		return &rsa.PublicKey{N: new(big.Int).SetBytes(attributes[0].Value), E: int(new(big.Int).SetBytes(attributes[1].Value).Int64())}
	}
	t.Fatal("no token labelled halo")
	return nil
}
