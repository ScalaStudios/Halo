package auth_test

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"halo/internal/store"
)

func TestSessionLimit(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	ada := e.withPassword("ada@example.com", "correct horse battery staple", false)
	e.allowPasswords(false)
	signIn := map[string]any{"email": "ada@example.com", "password": "correct horse battery staple"}
	signInWith := func(c *http.Client) {
		t.Helper()
		if status := e.do(c, "POST", "/api/v1/auth/password", signIn, nil); status != http.StatusOK {
			t.Fatalf("sign-in: %d", status)
		}
	}
	active := func(c *http.Client) bool {
		_, status := e.me(c)
		return status == http.StatusOK
	}

	e.org(func(s *store.Settings) { s.SessionLimit = 2 })
	laptop, phone := client(), client()
	signInWith(laptop)
	signInWith(laptop)
	signInWith(phone)
	e.expect(client(), "POST", "/api/v1/auth/password", signIn, http.StatusForbidden, "ERR_SESSION_LIMIT")

	e.org(func(s *store.Settings) { s.SessionLimitAction = "end-oldest" })
	tablet := client()
	signInWith(tablet)
	if active(laptop) || !active(phone) || !active(tablet) {
		t.Fatal("ending the oldest sessions kept the wrong ones")
	}

	kiosks, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Kiosks", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	staff, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Staff", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range []string{kiosks.ID, staff.ID} {
		if err := e.st.AddGroupMember(ctx, g, ada.ID); err != nil {
			t.Fatal(err)
		}
	}
	e.org(func(s *store.Settings) {
		s.SessionLimitGroups = []store.GroupSessionLimit{{GroupID: staff.ID, Limit: 5}, {GroupID: kiosks.ID, Limit: 1}}
	})
	kiosk := client()
	signInWith(kiosk)
	if active(phone) || active(tablet) || !active(kiosk) {
		t.Fatal("the lowest group limit was not applied")
	}

	ended := "ended: " + store.SessionEndedReason
	want := []string{"success: ", "success: ", "success: ", "failure: " + store.SessionLimitReason, ended, "success: ", ended, ended, "success: "}
	if got := e.reasons(ada); !slices.Equal(got, want) {
		t.Fatalf("sign-in log:\n%s", strings.Join(got, "\n"))
	}
}
