package store_test

import (
	"context"
	"errors"
	"testing"

	"halo/internal/store"
	"halo/internal/testdb"
)

func TestIssuerChangeDropsLinkedIdentities(t *testing.T) {
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
	p, err := st.SaveIdentityProvider(ctx, store.IdentityProvider{Kind: "oidc", Name: "Acme", Issuer: "https://a.example.com", ClientID: "halo", Scopes: []string{}, AllowedDomains: []string{}, JITGroupIDs: []string{}}, "secret")
	must(err)
	must(st.UseFederatedIdentity(ctx, p.ID, u.ID, "u1", u.Email))

	p.Name, p.Enabled = "Acme SSO", !p.Enabled
	p, err = st.SaveIdentityProvider(ctx, p, "")
	must(err)
	if _, err := st.FindFederatedIdentity(ctx, p.ID, "u1"); err != nil {
		t.Fatalf("a change that keeps the issuer must keep the link: %v", err)
	}

	p.Issuer = "https://b.example.com"
	_, err = st.SaveIdentityProvider(ctx, p, "")
	must(err)
	if _, err := st.FindFederatedIdentity(ctx, p.ID, "u1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("a link from the old issuer must be dropped: %v", err)
	}
}
