package store_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"halo/internal/store"
	"halo/internal/testdb"
)

func TestActivatedRoleKeepsEligibilityScope(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	support, _ := st.CreateGroup(ctx, store.NewGroup{Name: "Support", Kind: "assigned"})
	eli, _ := st.CreateUser(ctx, store.NewUser{Email: "eli@example.com", Name: "Eli", Status: "active"})
	inside, _ := st.CreateUser(ctx, store.NewUser{Email: "in@example.com", Name: "In", Status: "active", GroupIDs: []string{support.ID}})
	outside, _ := st.CreateUser(ctx, store.NewUser{Email: "out@example.com", Name: "Out", Status: "active"})

	if err := st.SetUserRoles(ctx, eli.ID, []string{"auditor"}, map[string][]string{"auditor": {support.ID}}); err == nil {
		t.Fatal("an auditor role was limited to groups")
	}
	if _, err := st.SetEligibleRoles(ctx, eli.ID, []string{"helpdesk_admin"}, map[string][]string{"helpdesk_admin": {support.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ActivateRole(ctx, eli.ID, "helpdesk_admin", "Ticket 42", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	u, err := st.GetUser(ctx, eli.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(u.RoleKeys, "helpdesk_admin") || u.HasRole("helpdesk_admin") || !slices.Equal(u.RoleScopes["helpdesk_admin"], []string{support.ID}) {
		t.Fatalf("activation lost its scope: %v %v", u.RoleKeys, u.RoleScopes)
	}
	if !u.Manages(inside, "helpdesk_admin") || u.Manages(outside, "helpdesk_admin") || u.Manages(inside, "user_admin") {
		t.Fatal("scope checks disagree with the eligibility")
	}
	if _, err := st.SetEligibleRoles(ctx, eli.ID, []string{"helpdesk_admin"}, nil); err != nil {
		t.Fatal(err)
	}
	if u, _ = st.GetUser(ctx, eli.ID); !u.HasRole("helpdesk_admin") || !u.Manages(outside, "helpdesk_admin") {
		t.Fatalf("widening the eligibility did not reach the open activation: %v", u.RoleScopes)
	}
}
