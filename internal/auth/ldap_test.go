package auth_test

import (
	"context"
	"fmt"
	"testing"

	"halo/internal/access"
	"halo/internal/auth"
	"halo/internal/httpx"
	"halo/internal/store"
)

func TestPasswordBindSprayFromOneAddress(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.allowPasswords(false)
	ada := e.withPassword("ada@example.com", "correct horse battery staple", false)
	group, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Storage", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.AddGroupMember(ctx, group.ID, ada.ID); err != nil {
		t.Fatal(err)
	}
	app, err := e.st.CreateApplication(ctx, store.NewApplication{Name: "NAS", Protocol: "ldap", Type: "web", GroupIDs: []string{group.ID}})
	if err != nil {
		t.Fatal(err)
	}
	deps := httpx.Deps{Store: e.st, Policy: access.AllowAll{}}
	bind := func(email, password, ip string) string {
		t.Helper()
		userID, err := auth.PasswordBind(ctx, deps, app, email, password, ip)
		if err != nil {
			t.Fatal(err)
		}
		return userID
	}

	if bind("ada@example.com", "correct horse battery staple", "198.51.100.4") != ada.ID {
		t.Fatal("a correct bind was refused")
	}
	for i := range 20 {
		if bind(fmt.Sprintf("victim%d@example.com", i), "Summer2026!Summer2026!", "203.0.113.7") != "" {
			t.Fatal("a sprayed guess was accepted")
		}
	}
	if bind("ada@example.com", "correct horse battery staple", "203.0.113.7") != "" {
		t.Fatal("a bind from a spraying address was accepted")
	}
	events, err := e.st.ListSignIns(ctx, store.SignInFilter{UserID: ada.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 || events[0].Reason != store.SprayedReason || events[0].IP != "203.0.113.7" || events[0].Result != "failure" {
		t.Fatalf("throttled bind is not in the sign-in log: %+v", events)
	}
	if bind("ada@example.com", "correct horse battery staple", "198.51.100.4") != ada.ID {
		t.Fatal("a different address must not be throttled")
	}
}
