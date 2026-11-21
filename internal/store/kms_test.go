package store_test

import (
	"context"
	"crypto/rand"
	"testing"

	"halo/internal/kms"
	"halo/internal/kms/kmstest"
	"halo/internal/secret"
	"halo/internal/testdb"
)

func TestRotateSecretKeyWithExternalKeys(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	fake := kmstest.New(t)
	st.Keys = kms.FromConfig(fake.Config())
	refs := map[string]string{}
	for _, alg := range []string{"ES256", "RS256", "EdDSA"} {
		ref, err := st.Keys.CreateKey(ctx, alg)
		must(err)
		refs[alg] = ref
	}
	_, err := st.CreateSigningKey(ctx, "ES256", nil, refs["ES256"])
	must(err)
	_, err = st.CreateSigningKey(ctx, "RS256", []byte("local-key"), "")
	must(err)
	must(st.CreateSAMLKey(ctx, nil, []byte("saml-cert"), refs["RS256"]))
	must(st.CreateSSHAuthority(ctx, false, nil, "ssh-ed25519 AAAA", refs["EdDSA"]))
	must(st.CreateSSHAuthority(ctx, true, nil, "ssh-ed25519 BBBB", refs["EdDSA"]))

	key := make([]byte, 32)
	_, _ = rand.Read(key)
	next, err := secret.NewSealer(key)
	must(err)
	rotation, err := st.RotateSecretKey(ctx, next)
	must(err)
	for _, c := range rotation.Resealed {
		if want := map[string]int{"signing_keys.private_key_sealed": 1}[c.Name]; c.Rows != want {
			t.Fatalf("%s: resealed %d rows, want %d", c.Name, c.Rows, want)
		}
	}

	rotated, err := st.WithKey(key)
	must(err)
	rotated.Keys = st.Keys
	keys, err := rotated.ListSigningKeys(ctx)
	must(err)
	external, local := keys[1], keys[0]
	if external.ExternalKey != refs["ES256"] || external.PrivateKey != nil || string(local.PrivateKey) != "local-key" || local.ExternalKey != "" {
		t.Fatalf("after rotation: %+v", keys)
	}
	must(rotated.CheckExternalKeys(ctx))
	rotated.Keys = nil
	if err := rotated.CheckExternalKeys(ctx); err == nil {
		t.Fatal("keys in Vault without Vault configured must stop the server from starting")
	}
}
