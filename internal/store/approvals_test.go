package store_test

import (
	"context"
	"testing"
	"time"

	"halo/internal/store"
	"halo/internal/testdb"
)

func TestApprovalTargetLockHoldsOffEveryWriter(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, store.NewUser{Email: "cara@example.com", Name: "Cara", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetEligibleRoles(ctx, u.ID, []string{"auditor"}); err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateAccessPolicy(ctx, store.AccessPolicy{Name: "Everyone", Enabled: true, Mode: "enforce", Effect: "require-phishing-resistant", Conditions: store.PolicyConditions{AllUsers: true, AllApps: true}})
	if err != nil {
		t.Fatal(err)
	}
	idp, err := st.SaveIdentityProvider(ctx, store.IdentityProvider{Kind: "github", Name: "GitHub", Issuer: "https://github.com", ClientID: "halo", Scopes: []string{}, AllowedDomains: []string{}, JITGroupIDs: []string{}}, "secret")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		target string
		write  func(context.Context) error
	}{
		{"user", func(ctx context.Context) error { return st.SetUserRoles(ctx, u.ID, nil) }},
		{"user", func(ctx context.Context) error { _, err := st.SetEligibleRoles(ctx, u.ID, nil); return err }},
		{"user", func(ctx context.Context) error {
			_, err := st.ActivateRole(ctx, u.ID, "auditor", "incident", time.Now().Add(time.Hour))
			return err
		}},
		{"user", func(ctx context.Context) error { u.Name = "Renamed"; return st.UpdateUserProfile(ctx, u) }},
		{"policy", func(ctx context.Context) error { _, err := st.UpdateAccessPolicy(ctx, p); return err }},
		{"identity_provider", func(ctx context.Context) error { _, err := st.SaveIdentityProvider(ctx, idp, "rotated"); return err }},
		{"method", func(ctx context.Context) error { return st.SetMethodEnabled(ctx, "password", true) }},
		{"settings", func(ctx context.Context) error { return st.SaveSettings(ctx, store.DefaultSettings) }},
	} {
		err := st.Tx(ctx, func(tx *store.Store) error {
			if err := tx.LockApprovalTarget(ctx, c.target); err != nil {
				return err
			}
			short, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
			defer cancel()
			if err := c.write(short); err == nil || short.Err() == nil {
				t.Errorf("a %s write must wait for the approval lock: %v", c.target, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if st.LockApprovalTarget(ctx, "unknown") == nil {
		t.Fatal("an unknown target must not go unlocked")
	}
}
