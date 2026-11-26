package main

import (
	"context"
	"testing"

	"halo/internal/testdb"
)

func TestBootstrapReissuesSetupLink(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	first, firstToken, _, err := bootstrapAdmin(ctx, st, "sam@example.com", "Sam Lee")
	if err != nil {
		t.Fatal(err)
	}
	again, secondToken, _, err := bootstrapAdmin(ctx, st, "Sam@Example.com", "Sam Lee")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID || secondToken == firstToken {
		t.Fatalf("second run must reissue a link for the same person: %s %s", first.ID, again.ID)
	}
	if _, _, err := st.PeekEnrollmentToken(ctx, firstToken); err == nil {
		t.Fatal("the first link must stop working")
	}
	if u, _, err := st.PeekEnrollmentToken(ctx, secondToken); err != nil || u.ID != first.ID {
		t.Fatalf("the new link must work: %v", err)
	}
	if _, _, _, err := bootstrapAdmin(ctx, st, "other@example.com", "Other"); err == nil {
		t.Fatal("a different email must not create a second administrator while the first is pending")
	}
	if err := st.SetUserStatus(ctx, first.ID, "active"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := bootstrapAdmin(ctx, st, "sam@example.com", "Sam Lee"); err == nil {
		t.Fatal("bootstrap must refuse once the administrator is no longer invited")
	}
	users, err := st.ListUsers(ctx)
	if err != nil || len(users) != 1 {
		t.Fatalf("users: %v %v", users, err)
	}
}
