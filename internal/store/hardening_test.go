package store_test

import (
	"context"
	"crypto/rand"
	"slices"
	"testing"
	"time"

	"halo/internal/secret"
	"halo/internal/store"
	"halo/internal/testdb"
)

func TestRotateSecretKey(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	u, err := st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada", Status: "active"})
	must(err)
	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Wiki", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://wiki.example.com/cb"}})
	must(err)
	_, err = st.CreateTOTPSecret(ctx, u.ID, "Phone", st.Sealer.Seal([]byte("totp-seed")))
	must(err)
	_, err = st.CreateSigningKey(ctx, "RS256", []byte("oidc-key"))
	must(err)
	_, err = st.QueueMail(ctx, "ada@example.com", "Hello", "mail body", false)
	must(err)
	must(st.CreateSAMLKey(ctx, []byte("saml-key"), []byte("saml-cert")))
	must(st.SaveAppProvisioning(ctx, app.ID, "https://wiki.example.com/scim", st.Sealer.Seal([]byte("scim-token")), true))
	idp, err := st.SaveIdentityProvider(ctx, store.IdentityProvider{Kind: "oidc", Name: "Acme", Issuer: "https://idp.example.com", ClientID: "halo", Scopes: []string{}, AllowedDomains: []string{}, JITGroupIDs: []string{}}, "client-secret")
	must(err)
	webhookID, _, err := st.CreateWebhook(ctx, store.NewWebhook{URL: "https://hooks.example.com", Events: []string{"user.create"}, Enabled: true})
	must(err)
	must(st.CreateSSHAuthority(ctx, []byte("ssh-seed"), "ssh-ed25519 AAAA"))
	must(st.ReplaceRecoveryCodes(ctx, u.ID, [][]byte{st.RecoveryCodeHash("abcd")}))

	key := make([]byte, 32)
	_, _ = rand.Read(key)
	next, err := secret.NewSealer(key)
	must(err)
	rotation, err := st.RotateSecretKey(ctx, next)
	must(err)
	if len(rotation.Resealed) != 8 || slices.ContainsFunc(rotation.Resealed, func(c store.ResealedColumn) bool { return c.Rows != 1 }) {
		t.Fatalf("resealed: %+v", rotation.Resealed)
	}
	if !slices.Equal(rotation.RecoveryCodeUsers, []string{"ada@example.com"}) {
		t.Fatalf("recovery code users: %v", rotation.RecoveryCodeUsers)
	}
	if _, err := st.ListSigningKeys(ctx); err == nil {
		t.Fatal("the old key must no longer open signing keys")
	}

	rotated, err := st.WithKey(key)
	must(err)
	totp, err := rotated.ListTOTPSecrets(ctx, u.ID)
	must(err)
	keys, err := rotated.ListSigningKeys(ctx)
	must(err)
	mail, err := rotated.ListMail(ctx, 10)
	must(err)
	saml, err := rotated.ActiveSAMLKey(ctx)
	must(err)
	provisioning, err := rotated.GetAppProvisioning(ctx, app.ID)
	must(err)
	provider, err := rotated.GetIdentityProvider(ctx, idp.ID)
	must(err)
	webhook, err := rotated.GetWebhook(ctx, webhookID)
	must(err)
	ssh, err := rotated.GetSSHAuthority(ctx)
	must(err)
	for want, sealed := range map[string][]byte{"totp-seed": totp[0].Sealed, "scim-token": provisioning.TokenSealed, "client-secret": provider.SecretSealed} {
		if plain, err := rotated.Sealer.Open(sealed); err != nil || string(plain) != want {
			t.Fatalf("open %s with the new key: %q %v", want, plain, err)
		}
	}
	if _, err := rotated.Sealer.Open(webhook.Sealed); err != nil {
		t.Fatalf("webhook secret: %v", err)
	}
	if string(keys[0].PrivateKey) != "oidc-key" || mail[0].Text != "mail body" || string(saml.PrivateKey) != "saml-key" || string(ssh.Seed) != "ssh-seed" {
		t.Fatalf("rotated values: %q %q %q %q", keys[0].PrivateKey, mail[0].Text, saml.PrivateKey, ssh.Seed)
	}
	if _, err := st.RotateSecretKey(ctx, next); err == nil {
		t.Fatal("rotating again with the old key must fail without changing anything")
	}
	if _, err := rotated.ListSigningKeys(ctx); err != nil {
		t.Fatalf("a failed second rotation must leave the values sealed with the new key: %v", err)
	}
}

func TestSigningKeyLifecycle(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	first, err := st.CreateSigningKey(ctx, "RS256", []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.CreateSigningKey(ctx, "RS256", []byte("second"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RetireSigningKeys(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	info, err := st.ListSigningKeyInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(info) != 2 || info[0].ID != second.ID || !info[0].Active || info[0].RetiresAt != nil || info[1].ID != first.ID || info[1].Active || info[1].RetiredAt != nil ||
		info[1].RetiresAt == nil || !info[1].RetiresAt.Equal(info[0].CreatedAt.Add(store.SigningKeyOverlap)) {
		t.Fatalf("keys during the overlap: %+v", info)
	}
	if keys, err := st.ListSigningKeys(ctx); err != nil || len(keys) != 2 || keys[0].ID != second.ID {
		t.Fatalf("published keys during the overlap: %+v %v", keys, err)
	}
	if err := st.RetireSigningKeys(ctx, time.Now().Add(store.SigningKeyOverlap+time.Minute)); err != nil {
		t.Fatal(err)
	}
	if keys, err := st.ListSigningKeys(ctx); err != nil || len(keys) != 1 || keys[0].ID != second.ID {
		t.Fatalf("published keys after the overlap: %+v %v", keys, err)
	}
	info, err = st.ListSigningKeyInfo(ctx)
	if err != nil || len(info) != 2 || info[1].RetiredAt == nil || info[1].RetiresAt != nil {
		t.Fatalf("retired key: %+v %v", info, err)
	}
}

func TestSetBasedCounts(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	rule := `user.department == "engineering"`
	eng, err := st.CreateGroup(ctx, store.NewGroup{Name: "Engineering", Kind: "dynamic", Rule: &rule})
	if err != nil {
		t.Fatal(err)
	}
	ops, err := st.CreateGroup(ctx, store.NewGroup{Name: "Ops", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	for i, u := range []store.NewUser{
		{Email: "a@example.com", Name: "A", Department: "Engineering", Status: "active", GroupIDs: []string{ops.ID}},
		{Email: "b@example.com", Name: "B", Department: "ENGINEERING", Status: "active"},
		{Email: "c@example.com", Name: "C", Department: "Sales", Status: "active", GroupIDs: []string{ops.ID}},
		{Email: "d@example.com", Name: "D", Department: "Engineering", Status: "deprovisioned", GroupIDs: []string{ops.ID}},
	} {
		created, err := st.CreateUser(ctx, u)
		if err != nil {
			t.Fatal(err)
		}
		want := map[int][]string{0: {ops.ID, eng.ID}, 1: {eng.ID}, 2: {ops.ID}, 3: {ops.ID, eng.ID}}[i]
		if !slices.Equal(created.GroupIDs, want) {
			t.Fatalf("%s groups: %v, want %v", u.Email, created.GroupIDs, want)
		}
	}
	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Wiki", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://wiki.example.com/cb"}, GroupIDs: []string{eng.ID, ops.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if app.UserCount != 3 {
		t.Fatalf("user count: %d, want 3", app.UserCount)
	}
	groups, err := st.ListGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range groups {
		if want := map[string]int{eng.ID: 2, ops.ID: 2}[g.ID]; g.MemberCount != want {
			t.Fatalf("%s members: %d, want %d", g.Name, g.MemberCount, want)
		}
	}
	users, err := st.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(users, func(u store.User) bool { return u.Email == "b@example.com" && slices.Equal(u.AppIDs, []string{app.ID}) }) {
		t.Fatalf("app assignment through a dynamic group: %+v", users)
	}
}
