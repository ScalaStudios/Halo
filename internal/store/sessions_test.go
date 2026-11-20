package store_test

import (
	"strings"
	"testing"

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
