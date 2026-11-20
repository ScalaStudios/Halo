package api_test

import (
	"context"
	"net/http/httptest"
	"slices"
	"testing"

	"halo/internal/store"
)

func TestGroupOwnersManageOnlyTheirGroups(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "uma@example.com", "user_admin")
	owner, ownerToken := person(t, st, "olive@example.com")
	_, stranger := person(t, st, "sam@example.com")
	pat, _ := person(t, st, "pat@example.com")
	team, _ := st.CreateGroup(ctx, store.NewGroup{Name: "Team", Kind: "assigned"})
	other, _ := st.CreateGroup(ctx, store.NewGroup{Name: "Other", Kind: "assigned"})
	rule := `user.email endsWith "@example.com"`
	dynamic, _ := st.CreateGroup(ctx, store.NewGroup{Name: "Everyone", Kind: "dynamic", Rule: &rule})

	expect(t, call(t, h, ownerToken, "PUT", "/api/v1/groups/"+team.ID+"/owners", map[string]any{"userIds": []string{owner.ID}}), 403)
	expect(t, call(t, h, admin, "PUT", "/api/v1/groups/"+dynamic.ID+"/owners", map[string]any{"userIds": []string{owner.ID}}), 409)
	rec := call(t, h, admin, "PUT", "/api/v1/groups/"+team.ID+"/owners", map[string]any{"userIds": []string{owner.ID, owner.ID}})
	expect(t, rec, 200)
	if g := decode[store.Group](t, rec); len(g.Owners) != 1 || g.Owners[0].ID != owner.ID {
		t.Fatalf("owners: %+v", g.Owners)
	}

	expect(t, call(t, h, ownerToken, "POST", "/api/v1/groups/"+team.ID+"/members", map[string]any{"userId": pat.ID}), 204)
	expect(t, call(t, h, ownerToken, "POST", "/api/v1/groups/"+team.ID+"/members", map[string]any{"userId": owner.ID}), 204)
	expect(t, call(t, h, ownerToken, "DELETE", "/api/v1/groups/"+team.ID+"/members/"+owner.ID, nil), 204)
	added, err := st.GetUser(ctx, pat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(added.GroupIDs, team.ID) || len(added.Roles) != 0 {
		t.Fatalf("pat after owner add: groups %v roles %v", added.GroupIDs, added.Roles)
	}

	expect(t, call(t, h, ownerToken, "POST", "/api/v1/groups/"+other.ID+"/members", map[string]any{"userId": pat.ID}), 403)
	expect(t, call(t, h, ownerToken, "POST", "/api/v1/groups/"+dynamic.ID+"/members", map[string]any{"userId": pat.ID}), 403)
	expect(t, call(t, h, ownerToken, "DELETE", "/api/v1/groups/"+dynamic.ID+"/members/"+pat.ID, nil), 403)
	expect(t, call(t, h, stranger, "POST", "/api/v1/groups/"+team.ID+"/members", map[string]any{"userId": pat.ID}), 403)
	expect(t, call(t, h, stranger, "DELETE", "/api/v1/groups/"+team.ID+"/members/"+pat.ID, nil), 403)
	expect(t, call(t, h, ownerToken, "PATCH", "/api/v1/groups/"+team.ID, map[string]any{"joinPolicy": "open"}), 403)
	expect(t, call(t, h, ownerToken, "DELETE", "/api/v1/groups/"+team.ID, nil), 403)
	expect(t, call(t, h, ownerToken, "PUT", "/api/v1/users/"+pat.ID+"/roles", map[string]any{"roles": []string{"user_admin"}}), 403)
	expect(t, call(t, h, ownerToken, "PUT", "/api/v1/users/"+owner.ID+"/roles", map[string]any{"roles": []string{"global_admin"}}), 403)
	expect(t, call(t, h, ownerToken, "GET", "/api/v1/groups/"+team.ID+"/members", nil), 403)
	expect(t, call(t, h, stranger, "GET", "/api/v1/me/owned-groups/"+team.ID, nil), 404)

	expect(t, call(t, h, ownerToken, "DELETE", "/api/v1/groups/"+team.ID+"/members/"+pat.ID, nil), 204)
	if u, _ := st.GetUser(ctx, owner.ID); len(u.Roles) != 0 {
		t.Fatalf("owner gained roles: %v", u.Roles)
	}

	events, err := st.ListAudit(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	byOwner := map[string]int{}
	for _, e := range events {
		if e.ActorID != nil && *e.ActorID == owner.ID && e.TargetID == team.ID {
			byOwner[e.Action]++
		}
	}
	if byOwner["group.member.add"] != 2 || byOwner["group.member.remove"] != 2 {
		t.Fatalf("owner audit entries: %v", byOwner)
	}
}

func TestSelfServiceGroupMembership(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "uma@example.com", "user_admin")
	owner, ownerToken := person(t, st, "olive@example.com")
	pat, patToken := person(t, st, "pat@example.com")
	_, stranger := person(t, st, "sam@example.com")
	team, _ := st.CreateGroup(ctx, store.NewGroup{Name: "Team", Kind: "assigned"})
	open, _ := st.CreateGroup(ctx, store.NewGroup{Name: "Book club", Kind: "assigned"})
	closed, _ := st.CreateGroup(ctx, store.NewGroup{Name: "Payroll", Kind: "assigned"})
	rule := `user.department == "Finance"`
	dynamic, _ := st.CreateGroup(ctx, store.NewGroup{Name: "Finance", Kind: "dynamic", Rule: &rule})

	expect(t, call(t, h, admin, "PATCH", "/api/v1/groups/"+team.ID, map[string]any{"joinPolicy": "request"}), 422)
	expect(t, call(t, h, admin, "PATCH", "/api/v1/groups/"+team.ID, map[string]any{"joinPolicy": "anyone"}), 422)
	expect(t, call(t, h, admin, "PATCH", "/api/v1/groups/"+dynamic.ID, map[string]any{"joinPolicy": "open"}), 422)
	expect(t, call(t, h, admin, "PUT", "/api/v1/groups/"+team.ID+"/owners", map[string]any{"userIds": []string{owner.ID}}), 200)
	expect(t, call(t, h, admin, "PATCH", "/api/v1/groups/"+team.ID, map[string]any{"joinPolicy": "request"}), 200)
	expect(t, call(t, h, admin, "PUT", "/api/v1/groups/"+team.ID+"/owners", map[string]any{"userIds": []string{}}), 422)
	expect(t, call(t, h, admin, "PATCH", "/api/v1/groups/"+open.ID, map[string]any{"joinPolicy": "open"}), 200)

	joinable := decode[[]struct {
		ID        string `json:"id"`
		Requested bool   `json:"requested"`
	}](t, ok(t, call(t, h, patToken, "GET", "/api/v1/me/joinable-groups", nil)))
	if len(joinable) != 2 || joinable[0].ID != team.ID || joinable[0].Requested || joinable[1].ID != open.ID {
		t.Fatalf("joinable: %+v", joinable)
	}

	expect(t, call(t, h, patToken, "POST", "/api/v1/me/groups/"+closed.ID+"/join", nil), 403)
	expect(t, call(t, h, patToken, "POST", "/api/v1/me/groups/"+dynamic.ID+"/join", nil), 403)
	for range 2 {
		if got := decode[map[string]string](t, ok(t, call(t, h, patToken, "POST", "/api/v1/me/groups/"+team.ID+"/join", nil)))["status"]; got != "requested" {
			t.Fatalf("join on request: %s", got)
		}
	}
	if got := decode[map[string]string](t, ok(t, call(t, h, patToken, "POST", "/api/v1/me/groups/"+open.ID+"/join", nil)))["status"]; got != "joined" {
		t.Fatalf("join open group: %s", got)
	}

	owned := decode[[]struct {
		ID           string `json:"id"`
		RequestCount int    `json:"requestCount"`
	}](t, ok(t, call(t, h, ownerToken, "GET", "/api/v1/me/owned-groups", nil)))
	if len(owned) != 1 || owned[0].ID != team.ID || owned[0].RequestCount != 1 {
		t.Fatalf("owned: %+v", owned)
	}
	detail := decode[struct {
		Requests []store.JoinRequest `json:"requests"`
		People   []store.NamedRef    `json:"people"`
	}](t, ok(t, call(t, h, ownerToken, "GET", "/api/v1/me/owned-groups/"+team.ID, nil)))
	if len(detail.Requests) != 1 || detail.Requests[0].User.ID != pat.ID || len(detail.People) != 4 {
		t.Fatalf("owned detail: %+v", detail)
	}

	approve := "/api/v1/groups/" + team.ID + "/join-requests/" + pat.ID + "/approve"
	deny := "/api/v1/groups/" + team.ID + "/join-requests/" + pat.ID + "/deny"
	expect(t, call(t, h, stranger, "POST", approve, nil), 403)
	expect(t, call(t, h, patToken, "POST", approve, nil), 403)
	expect(t, call(t, h, ownerToken, "POST", deny, nil), 204)
	expect(t, call(t, h, ownerToken, "POST", approve, nil), 404)
	ok(t, call(t, h, patToken, "POST", "/api/v1/me/groups/"+team.ID+"/join", nil))
	expect(t, call(t, h, ownerToken, "POST", approve, nil), 204)

	u, err := st.GetUser(ctx, pat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(u.GroupIDs, team.ID) || !slices.Contains(u.GroupIDs, open.ID) || slices.Contains(u.GroupIDs, closed.ID) || len(u.Roles) != 0 {
		t.Fatalf("pat after joining: groups %v roles %v", u.GroupIDs, u.Roles)
	}

	sam := call(t, h, stranger, "POST", "/api/v1/me/groups/"+team.ID+"/join", nil)
	if got := decode[map[string]string](t, ok(t, sam))["status"]; got != "requested" {
		t.Fatalf("sam join: %s", got)
	}
	ok(t, call(t, h, admin, "PATCH", "/api/v1/groups/"+team.ID, map[string]any{"joinPolicy": "open"}))
	stale := decode[[]struct {
		ID        string `json:"id"`
		Requested bool   `json:"requested"`
	}](t, ok(t, call(t, h, stranger, "GET", "/api/v1/me/joinable-groups", nil)))
	if len(stale) == 0 || stale[0].ID != team.ID || stale[0].Requested {
		t.Fatalf("requests survived the switch to open: %+v", stale)
	}

	events, err := st.ListAudit(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	count := map[string]int{}
	for _, e := range events {
		count[e.Action]++
	}
	if count["group.join.request"] != 3 || count["group.join.deny"] != 1 || count["group.member.add"] != 2 {
		t.Fatalf("audit: %v", count)
	}
}

func ok(t *testing.T, rec *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	expect(t, rec, 200)
	return rec
}
