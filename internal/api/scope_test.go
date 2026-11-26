package api_test

import (
	"context"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"halo/internal/store"
)

func scopedAdmin(t *testing.T, st *store.Store, email, role string, groups ...string) (store.User, string) {
	t.Helper()
	u, token := person(t, st, email)
	if err := st.SetUserRoles(context.Background(), u.ID, []string{role}, map[string][]string{role: groups}); err != nil {
		t.Fatal(err)
	}
	return u, token
}

func member(t *testing.T, st *store.Store, email string, groups ...string) store.User {
	t.Helper()
	u, err := st.CreateUser(context.Background(), store.NewUser{Email: email, Name: strings.Split(email, "@")[0], Status: "active", GroupIDs: groups})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func ids[T any](items []T, id func(T) string) []string {
	out := []string{}
	for _, item := range items {
		out = append(out, id(item))
	}
	slices.Sort(out)
	return out
}

func TestScopedUserAdminReachesOnlyTheirGroups(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	support, _ := st.CreateGroup(ctx, store.NewGroup{Name: "Support", Kind: "assigned"})
	sales, _ := st.CreateGroup(ctx, store.NewGroup{Name: "Sales", Kind: "assigned"})
	sam := member(t, st, "sam@example.com", support.ID)
	otto := member(t, st, "otto@example.com", sales.ID)
	loner := member(t, st, "lone@example.com")
	_, sue := scopedAdmin(t, st, "sue@example.com", "user_admin", support.ID)
	_, uma := person(t, st, "uma@example.com", "user_admin")

	listed := decode[[]store.User](t, expect200(t, call(t, h, sue, "GET", "/api/v1/users", nil)))
	if got := ids(listed, func(u store.User) string { return u.ID }); !slices.Equal(got, []string{sam.ID}) {
		t.Fatalf("scoped list: %v", got)
	}
	everyone := decode[[]store.User](t, expect200(t, call(t, h, uma, "GET", "/api/v1/users", nil)))
	if len(everyone) != 5 {
		t.Fatalf("organization-wide list: %d", len(everyone))
	}
	groups := decode[[]store.Group](t, expect200(t, call(t, h, sue, "GET", "/api/v1/groups", nil)))
	if got := ids(groups, func(g store.Group) string { return g.ID }); !slices.Equal(got, []string{support.ID}) {
		t.Fatalf("scoped groups: %v", got)
	}

	expect(t, call(t, h, sue, "GET", "/api/v1/users/"+sam.ID, nil), 200)
	for _, path := range []string{"/api/v1/users/" + otto.ID, "/api/v1/users/" + otto.ID + "/sessions", "/api/v1/users/" + loner.ID + "/sign-ins", "/api/v1/groups/" + sales.ID, "/api/v1/groups/" + sales.ID + "/members"} {
		expect(t, call(t, h, sue, "GET", path, nil), 404)
	}

	for _, c := range []struct{ method, path string }{
		{"PATCH", "/api/v1/users/" + otto.ID},
		{"POST", "/api/v1/users/" + otto.ID + "/suspend"},
		{"POST", "/api/v1/users/" + otto.ID + "/restore"},
		{"POST", "/api/v1/users/" + otto.ID + "/revoke-sessions"},
		{"POST", "/api/v1/users/" + loner.ID + "/reset-authentication"},
		{"PUT", "/api/v1/users/" + otto.ID + "/password"},
		{"POST", "/api/v1/users/" + otto.ID + "/access-pass"},
		{"DELETE", "/api/v1/users/" + otto.ID + "/methods/password"},
	} {
		rec := call(t, h, sue, c.method, c.path, map[string]any{"title": "Changed", "password": "correct horse battery staple 42"})
		expect(t, rec, 403)
		if !strings.Contains(rec.Body.String(), "ERR_OUT_OF_SCOPE") {
			t.Fatalf("%s %s: %s", c.method, c.path, rec.Body)
		}
	}
	if u, _ := st.GetUser(ctx, otto.ID); u.Title != "" || u.Status != "active" {
		t.Fatalf("out-of-scope person changed: %+v", u)
	}

	expect(t, call(t, h, sue, "PATCH", "/api/v1/users/"+sam.ID, map[string]any{"name": "Samuel"}), 200)
	expect(t, call(t, h, sue, "PATCH", "/api/v1/users/"+sam.ID, map[string]any{"managerId": otto.ID}), 422)
	expect(t, call(t, h, sue, "POST", "/api/v1/users/"+sam.ID+"/suspend", nil), 200)
	out := decode[bulkResponse](t, expect200(t, call(t, h, sue, "POST", "/api/v1/users/bulk/restore", map[string]any{"userIds": []string{sam.ID, otto.ID}})))
	if codes := out.codes(); codes[sam.ID] != "ok" || codes[otto.ID] != "ERR_OUT_OF_SCOPE" {
		t.Fatalf("bulk: %v", codes)
	}
	out = decode[bulkResponse](t, expect200(t, call(t, h, sue, "POST", "/api/v1/users/bulk/suspend", map[string]any{"userIds": []string{otto.ID, loner.ID}})))
	if out.Succeeded != 0 || status(t, st, otto.ID) != "active" || status(t, st, loner.ID) != "active" {
		t.Fatalf("bulk suspend outside scope: %+v", out)
	}

	expect(t, call(t, h, sue, "POST", "/api/v1/users", map[string]any{"email": "new@example.com", "name": "New", "groupIds": []string{sales.ID}}), 403)
	expect(t, call(t, h, sue, "POST", "/api/v1/users", map[string]any{"email": "new@example.com", "name": "New"}), 403)
	if _, err := st.GetUserByEmail(ctx, "new@example.com"); err == nil {
		t.Fatal("an invitation outside the scope created the person")
	}
	expect(t, call(t, h, sue, "POST", "/api/v1/users", map[string]any{"email": "new@example.com", "name": "New", "groupIds": []string{support.ID}}), 201)

	csv := "email,name,groups,manager email\nin@example.com,In,Support,\nout@example.com,Out,Sales,\nnone@example.com,None,,\nboss@example.com,Boss,Support,otto@example.com\n"
	preview := decode[importResponse](t, expect200(t, upload(t, h, sue, "?dryRun=true", csv)))
	want := map[int]string{2: "", 3: "no group named", 4: "ERR_OUT_OF_SCOPE", 5: "Nobody with the email otto@example.com"}
	for _, row := range preview.Rows {
		problem := want[row.Row]
		if problem == "" && row.Error != nil || problem != "" && (row.Error == nil || !strings.Contains(row.Error.Code+" "+row.Error.Message, problem)) {
			t.Errorf("import row %d: want %q, got %+v", row.Row, problem, row.Error)
		}
	}

	expect(t, call(t, h, sue, "GET", "/api/v1/audit", nil), 403)
	expect(t, call(t, h, sue, "POST", "/api/v1/groups", map[string]any{"name": "Mine", "kind": "assigned"}), 403)
	expect(t, call(t, h, sue, "POST", "/api/v1/groups/"+support.ID+"/members", map[string]any{"userId": otto.ID}), 403)
	expect(t, call(t, h, uma, "POST", "/api/v1/users/"+otto.ID+"/suspend", nil), 200)
}

func TestScopeFollowsRuleBasedGroups(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	rule := `user.department == "Support"`
	desk, _ := st.CreateGroup(ctx, store.NewGroup{Name: "Support desk", Kind: "dynamic", Rule: &rule})
	_, hana := scopedAdmin(t, st, "hana@example.com", "helpdesk_admin", desk.ID)
	_, uma := scopedAdmin(t, st, "uma@example.com", "user_admin", desk.ID)
	in, _ := st.CreateUser(ctx, store.NewUser{Email: "in@example.com", Name: "In", Department: "Support", Status: "active"})
	out, _ := st.CreateUser(ctx, store.NewUser{Email: "out@example.com", Name: "Out", Department: "Sales", Status: "active"})

	expect(t, call(t, h, hana, "PATCH", "/api/v1/users/"+in.ID, map[string]any{"title": "Lead"}), 200)
	expect(t, call(t, h, hana, "PATCH", "/api/v1/users/"+in.ID, map[string]any{"name": "Renamed"}), 403)
	expect(t, call(t, h, hana, "PATCH", "/api/v1/users/"+out.ID, map[string]any{"title": "Lead"}), 403)
	expect(t, call(t, h, hana, "POST", "/api/v1/users/"+in.ID+"/reset-authentication", nil), 200)
	expect(t, call(t, h, hana, "POST", "/api/v1/users/"+in.ID+"/suspend", nil), 403)
	expect(t, call(t, h, hana, "POST", "/api/v1/users", map[string]any{"email": "x@example.com", "name": "X", "department": "Support"}), 403)
	expect(t, call(t, h, uma, "POST", "/api/v1/users", map[string]any{"email": "y@example.com", "name": "Y", "department": "Sales"}), 403)
	expect(t, call(t, h, uma, "POST", "/api/v1/users", map[string]any{"email": "x@example.com", "name": "X", "department": "Support"}), 201)

	leads := `user.title == "Lead"`
	finance, _ := st.CreateGroup(ctx, store.NewGroup{Name: "Finance leads", Kind: "dynamic", Rule: &leads})
	expect(t, call(t, h, uma, "POST", "/api/v1/users", map[string]any{"email": "z@example.com", "name": "Z", "department": "Support", "title": "Lead"}), 403)
	if _, err := st.GetUserByEmail(ctx, "z@example.com"); err != store.ErrNotFound {
		t.Fatalf("a refused invitation must not create the person: %v", err)
	}
	expect(t, call(t, h, hana, "PATCH", "/api/v1/users/"+in.ID, map[string]any{"title": "Analyst"}), 200)
	before, _ := st.GetUser(ctx, in.ID)
	expect(t, call(t, h, hana, "PATCH", "/api/v1/users/"+in.ID, map[string]any{"title": "Lead"}), 403)
	if after, _ := st.GetUser(ctx, in.ID); slices.Contains(after.GroupIDs, finance.ID) || !slices.Equal(after.GroupIDs, before.GroupIDs) {
		t.Fatalf("a limited administrator must not move someone into a rule-based group outside their scope: %v", after.GroupIDs)
	}
}

func TestScopedRolesCannotEscalate(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	support, _ := st.CreateGroup(ctx, store.NewGroup{Name: "Support", Kind: "assigned"})
	sales, _ := st.CreateGroup(ctx, store.NewGroup{Name: "Sales", Kind: "assigned"})
	sam := member(t, st, "sam@example.com", support.ID)
	otto := member(t, st, "otto@example.com", sales.ID)
	sue, token := scopedAdmin(t, st, "sue@example.com", "user_admin", support.ID)
	if err := st.AddGroupMember(ctx, support.ID, sue.ID); err != nil {
		t.Fatal(err)
	}
	peer, _ := scopedAdmin(t, st, "pia@example.com", "helpdesk_admin", support.ID)
	if err := st.AddGroupMember(ctx, support.ID, peer.ID); err != nil {
		t.Fatal(err)
	}
	gia, global := person(t, st, "gia@example.com", "global_admin")

	expect(t, call(t, h, token, "PUT", "/api/v1/users/"+sam.ID+"/roles", map[string]any{"roles": []string{"user_admin"}, "scopes": map[string]any{"user_admin": []string{support.ID}}}), 403)
	expect(t, call(t, h, token, "PUT", "/api/v1/users/"+sue.ID+"/roles", map[string]any{"roles": []string{"user_admin"}, "scopes": map[string]any{"user_admin": []string{}}}), 403)
	expect(t, call(t, h, token, "PUT", "/api/v1/users/"+sam.ID+"/eligible-roles", map[string]any{"roles": []string{"user_admin"}}), 403)
	expect(t, call(t, h, token, "POST", "/api/v1/users", map[string]any{"email": "a@example.com", "name": "A", "groupIds": []string{support.ID}, "roles": []string{"auditor"}}), 403)
	expect(t, call(t, h, token, "PATCH", "/api/v1/users/"+peer.ID, map[string]any{"title": "Changed"}), 403)
	expect(t, call(t, h, token, "PATCH", "/api/v1/users/"+sue.ID, map[string]any{"department": "Everything"}), 403)

	for _, body := range []map[string]any{
		{"roles": []string{"auditor"}, "scopes": map[string]any{"auditor": []string{support.ID}}},
		{"roles": []string{"user_admin"}, "scopes": map[string]any{"user_admin": []string{"grp_missing"}}},
		{"roles": []string{"auditor"}, "scopes": map[string]any{"user_admin": []string{support.ID}}},
	} {
		expect(t, call(t, h, global, "PUT", "/api/v1/users/"+sue.ID+"/roles", body), 422)
	}

	rec := call(t, h, global, "PUT", "/api/v1/users/"+sue.ID+"/roles", map[string]any{"roles": []string{"user_admin", "auditor"}})
	got := decode[store.User](t, expect200(t, rec))
	if !slices.Equal(got.RoleScopes["user_admin"], []string{support.ID}) || len(got.RoleScopes) != 1 {
		t.Fatalf("leaving scopes out must keep them: %+v", got.RoleScopes)
	}
	events, err := st.ListAudit(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(events, func(e store.AuditEvent) bool { return e.Action == "role.update" }); i < 0 || !strings.Contains(events[i].Summary, "User administrator for Support") {
		t.Fatalf("audit: %+v", events)
	}
	expect(t, call(t, h, token, "GET", "/api/v1/users/"+otto.ID, nil), 200)
	expect(t, call(t, h, token, "POST", "/api/v1/users/"+otto.ID+"/suspend", nil), 403)

	got = decode[store.User](t, expect200(t, call(t, h, global, "PUT", "/api/v1/users/"+sue.ID+"/roles", map[string]any{"roles": []string{"user_admin"}, "scopes": map[string]any{"user_admin": []string{}}})))
	if len(got.RoleScopes) != 0 {
		t.Fatalf("an empty scope makes the role organization-wide: %+v", got.RoleScopes)
	}
	expect(t, call(t, h, token, "POST", "/api/v1/users/"+otto.ID+"/suspend", nil), 200)

	mix, both := person(t, st, "max@example.com")
	if err := st.SetUserRoles(ctx, mix.ID, []string{"user_admin", "helpdesk_admin"}, map[string][]string{"user_admin": {support.ID}}); err != nil {
		t.Fatal(err)
	}
	expect(t, call(t, h, both, "PATCH", "/api/v1/users/"+otto.ID, map[string]any{"title": "Rep"}), 200)
	expect(t, call(t, h, both, "PATCH", "/api/v1/users/"+otto.ID, map[string]any{"name": "Otto Renamed"}), 403)
	expect(t, call(t, h, both, "PATCH", "/api/v1/users/"+sam.ID, map[string]any{"name": "Sam Renamed"}), 200)

	expect(t, call(t, h, global, "PUT", "/api/v1/users/"+gia.ID+"/roles", map[string]any{"roles": []string{"global_admin", "user_admin"}, "scopes": map[string]any{"user_admin": []string{support.ID}}}), 403)

	guest, err := st.CreateUser(ctx, store.NewUser{Email: "g@partner.example", Name: "Guest", Status: "active", Guest: true, SponsorID: &gia.ID, ExpiresAt: &sam.CreatedAt})
	if err != nil {
		t.Fatal(err)
	}
	rec = call(t, h, global, "DELETE", "/api/v1/groups/"+support.ID, nil)
	expect(t, rec, 409)
	if !strings.Contains(rec.Body.String(), "administrator role of pia") {
		t.Fatalf("group delete: %s", rec.Body)
	}

	expect(t, call(t, h, global, "PUT", "/api/v1/users/"+guest.ID+"/eligible-roles", map[string]any{"roles": []string{"helpdesk_admin"}, "scopes": map[string]any{"helpdesk_admin": []string{support.ID}}}), 422)
}

func expect200(t *testing.T, rec *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	expect(t, rec, 200)
	return rec
}
