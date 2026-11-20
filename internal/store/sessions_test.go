package store_test

import (
	"strings"
	"testing"
	"time"

	"halo/internal/store"
)

func TestSessionLimitFor(t *testing.T) {
	s := store.Settings{SessionLimit: 4, SessionLimitGroups: []store.GroupSessionLimit{{GroupID: "grp_staff", Limit: 10}, {GroupID: "grp_kiosk", Limit: 1}}}
	for groups, want := range map[string]int{"": 4, "grp_other": 4, "grp_staff": 10, "grp_kiosk": 1, "grp_staff grp_kiosk": 1} {
		u := store.User{}
		for g := range strings.FieldsSeq(groups) {
			u.GroupIDs = append(u.GroupIDs, g)
		}
		if got := s.SessionLimitFor(u); got != want {
			t.Errorf("groups %q: limit %d, want %d", groups, got, want)
		}
	}
}

func TestStrongSignInAt(t *testing.T) {
	signedIn, confirmed := time.Now().Add(-time.Hour), time.Now()
	for _, c := range []struct {
		session store.Session
		want    time.Time
	}{
		{store.Session{Method: "passkey", CreatedAt: signedIn}, signedIn},
		{store.Session{Method: "security-key", CreatedAt: signedIn}, signedIn},
		{store.Session{Method: "recovery-codes", CreatedAt: signedIn}, signedIn},
		{store.Session{Method: "totp", CreatedAt: signedIn}, time.Time{}},
		{store.Session{Method: "password", SecondFactor: true, CreatedAt: signedIn}, time.Time{}},
		{store.Session{Method: "passkey", Client: "halo-cli", CreatedAt: signedIn}, time.Time{}},
		{store.Session{Method: "magic-link", CreatedAt: signedIn, ReauthenticatedAt: &confirmed}, confirmed},
	} {
		if got := c.session.StrongSignInAt(); !got.Equal(c.want) {
			t.Errorf("%s session: strong sign-in at %v, want %v", c.session.Method, got, c.want)
		}
	}
}
