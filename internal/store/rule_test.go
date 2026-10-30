package store_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"halo/internal/store"
	"halo/internal/testdb"
)

func TestRuleGroups(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	group := func(name string, rule string) store.Group {
		t.Helper()
		in := store.NewGroup{Name: name, Kind: "assigned"}
		if rule != "" {
			in.Kind, in.Rule = "dynamic", &rule
		}
		g, err := st.CreateGroup(ctx, in)
		must(err)
		return g
	}
	contractors := group("Contractors", "")
	person := func(email, department, title, status string, groups ...string) store.User {
		t.Helper()
		u, err := st.CreateUser(ctx, store.NewUser{Email: email, Name: email, Department: department, Title: title, Status: status, GroupIDs: groups})
		must(err)
		return u
	}
	ada := person("ada@lunarlabs.dev", "Engineering", "Engineer", "active")
	bob := person("bob@LunarLabs.dev", "Operations", "Contractor", "active", contractors.ID)
	cy := person("cy@lunarlabs.dev", "Sales", "Rep", "suspended")
	di := person("di@example.com", "Engineering", "Lead", "active")
	di.ManagerID = &ada.ID
	must(st.UpdateUserProfile(ctx, di))

	match := func(rule string) []string {
		t.Helper()
		ids, err := st.MatchRule(ctx, "", rule)
		must(err)
		var emails []string
		for _, u := range []store.User{ada, bob, cy, di} {
			if slices.Contains(ids, u.ID) {
				emails = append(emails, strings.Split(u.Email, "@")[0])
			}
		}
		return emails
	}
	staff := group("Staff", `user.emailDomain == "lunarlabs.dev" and not user.memberOf("Contractors")`)
	for rule, want := range map[string][]string{
		`user.email endsWith "@lunarlabs.dev" and user.status == "active"`:                    {"ada", "bob"},
		`user.department in ["Engineering", "Operations"] and not user.title == "Contractor"`: {"ada", "di"},
		`user.memberOf("Staff") or user.memberOf("Contractors")`:                              {"ada", "bob", "cy"},
		`user.department == "engineering"`:                                                    {"ada", "di"},
		`true`:                                                                                {"ada", "bob", "cy", "di"},
		`user.manager == "ADA@lunarlabs.dev"`:                                                 {"di"},
		`user.name startsWith "c" or user.title contains "ea"`:                                {"cy", "di"},
		`user.title == "x' or '1'='1" or user.department == "%"`:                              nil,
		`user.email contains "%" or user.email endsWith "_" or user.email startsWith "\"`:     nil,
	} {
		if got := match(rule); !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", rule, got, want)
		}
	}

	everyone := group("Everyone", "true")
	group("Lunar Labs", `user.memberOf("Staff") or user.memberOf("Contractors")`)
	groups, err := st.ListGroups(ctx)
	must(err)
	counts := map[string]int{}
	for _, g := range groups {
		counts[g.Name] = g.MemberCount
	}
	if counts["Staff"] != 2 || counts["Everyone"] != 4 || counts["Lunar Labs"] != 3 || counts["Contractors"] != 1 {
		t.Fatalf("member counts: %v", counts)
	}
	reloaded, err := st.GetUser(ctx, bob.ID)
	must(err)
	if slices.Contains(reloaded.GroupIDs, staff.ID) || !slices.Contains(reloaded.GroupIDs, everyone.ID) {
		t.Fatalf("bob's groups: %v", reloaded.GroupIDs)
	}

	if err := st.CheckRule(ctx, staff.ID, `user.memberOf("Lunar Labs")`); err == nil || !strings.Contains(err.Error(), "makes a loop") {
		t.Fatalf("cycle through Lunar Labs: %v", err)
	}
	if err := st.CheckRule(ctx, everyone.ID, `user.memberOf("Lunar Labs")`); err != nil {
		t.Fatalf("no cycle: %v", err)
	}
}
