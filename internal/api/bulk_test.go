package api_test

import (
	"context"
	"strings"
	"testing"

	"halo/internal/httpx"
	"halo/internal/store"
)

type bulkResponse struct {
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Results   []struct {
		ID    string       `json:"id"`
		OK    bool         `json:"ok"`
		Error *httpx.Error `json:"error"`
	} `json:"results"`
}

func (b bulkResponse) codes() map[string]string {
	codes := map[string]string{}
	for _, r := range b.Results {
		codes[r.ID] = "ok"
		if r.Error != nil {
			codes[r.ID] = r.Error.Code
		}
	}
	return codes
}

func status(t *testing.T, st *store.Store, userID string) string {
	t.Helper()
	u, err := st.GetUser(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	return u.Status
}

func TestBulkSuspendChecksEachPerson(t *testing.T) {
	st, h := setup(t)
	uma, admin := person(t, st, "uma@example.com", "user_admin")
	_, helpdesk := person(t, st, "hana@example.com", "helpdesk_admin")
	_, global := person(t, st, "gia@example.com", "global_admin")
	pat, _ := person(t, st, "pat@example.com")
	auditor, _ := person(t, st, "audrey@example.com", "auditor")
	boss, _ := person(t, st, "bo@example.com", "global_admin")
	nora, _ := person(t, st, "nora@example.com")

	expect(t, call(t, h, helpdesk, "POST", "/api/v1/users/bulk/suspend", map[string]any{"userIds": []string{pat.ID}}), 403)
	rec := call(t, h, admin, "POST", "/api/v1/users/bulk/suspend", map[string]any{"userIds": []string{pat.ID, auditor.ID, boss.ID, uma.ID, "usr_missing", pat.ID}})
	expect(t, rec, 200)
	out := decode[bulkResponse](t, rec)
	want := map[string]string{pat.ID: "ok", auditor.ID: "ERR_FORBIDDEN", boss.ID: "ERR_FORBIDDEN", uma.ID: "ERR_INVALID", "usr_missing": "ERR_NOT_FOUND"}
	if out.Succeeded != 1 || out.Failed != 4 || len(out.Results) != 5 {
		t.Fatalf("results: %+v", out)
	}
	for id, code := range out.codes() {
		if want[id] != code {
			t.Errorf("%s: want %s, got %s", id, want[id], code)
		}
	}
	if status(t, st, pat.ID) != "suspended" || status(t, st, auditor.ID) != "active" || status(t, st, boss.ID) != "active" || status(t, st, uma.ID) != "active" {
		t.Fatal("only the regular person should be suspended")
	}
	audit, _ := st.ListAudit(context.Background(), 100)
	suspended := 0
	for _, e := range audit {
		if e.Action == "user.suspend" {
			suspended++
		}
	}
	if suspended != 1 {
		t.Fatalf("want one suspend audit event, got %d", suspended)
	}

	rec = call(t, h, global, "POST", "/api/v1/users/bulk/suspend", map[string]any{"userIds": []string{auditor.ID, boss.ID}})
	expect(t, rec, 200)
	if out := decode[bulkResponse](t, rec); out.Succeeded != 2 {
		t.Fatalf("global administrator: %+v", out)
	}
	rec = call(t, h, admin, "POST", "/api/v1/users/bulk/restore", map[string]any{"userIds": []string{pat.ID, auditor.ID, nora.ID}})
	expect(t, rec, 200)
	codes := decode[bulkResponse](t, rec).codes()
	if codes[pat.ID] != "ok" || codes[auditor.ID] != "ERR_FORBIDDEN" || codes[nora.ID] != "ERR_CONFLICT" || status(t, st, pat.ID) == "suspended" {
		t.Fatalf("restore: %+v", codes)
	}
	session(t, st, pat.ID)
	rec = call(t, h, helpdesk, "POST", "/api/v1/users/bulk/revoke-sessions", map[string]any{"userIds": []string{pat.ID, "usr_missing"}})
	expect(t, rec, 200)
	if codes := decode[bulkResponse](t, rec).codes(); codes[pat.ID] != "ok" || codes["usr_missing"] != "ERR_NOT_FOUND" {
		t.Fatalf("revoke: %+v", codes)
	}
	if sessions, _ := st.ListSessions(context.Background(), pat.ID); len(sessions) != 0 {
		t.Fatalf("sessions left: %+v", sessions)
	}
}

func TestBulkGroupMembershipFollowsOwnership(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "uma@example.com", "user_admin")
	owner, ownerToken := person(t, st, "olive@example.com")
	_, stranger := person(t, st, "sam@example.com")
	pat, _ := person(t, st, "pat@example.com")
	nora, _ := person(t, st, "nora@example.com")
	team, _ := st.CreateGroup(ctx, store.NewGroup{Name: "Team", Kind: "assigned"})
	rule := `user.email endsWith "@example.com"`
	dynamic, _ := st.CreateGroup(ctx, store.NewGroup{Name: "Everyone", Kind: "dynamic", Rule: &rule})
	if err := st.SetGroupOwners(ctx, team.ID, []string{owner.ID}); err != nil {
		t.Fatal(err)
	}
	add := "/api/v1/groups/" + team.ID + "/members/bulk/add"
	body := map[string]any{"userIds": []string{pat.ID, owner.ID, "usr_missing"}}

	expect(t, call(t, h, stranger, "POST", add, body), 403)
	expect(t, call(t, h, admin, "POST", "/api/v1/groups/"+dynamic.ID+"/members/bulk/add", body), 409)
	rec := call(t, h, ownerToken, "POST", add, body)
	expect(t, rec, 200)
	if codes := decode[bulkResponse](t, rec).codes(); codes[pat.ID] != "ok" || codes[owner.ID] != "ok" || codes["usr_missing"] != "ERR_INVALID" {
		t.Fatalf("add: %+v", codes)
	}
	rec = call(t, h, ownerToken, "POST", "/api/v1/groups/"+team.ID+"/members/bulk/remove", map[string]any{"userIds": []string{pat.ID, nora.ID}})
	expect(t, rec, 200)
	if codes := decode[bulkResponse](t, rec).codes(); codes[pat.ID] != "ok" || codes[nora.ID] != "ERR_NOT_FOUND" {
		t.Fatalf("remove: %+v", codes)
	}
	members, _ := st.GetGroup(ctx, team.ID)
	if members.MemberCount != 1 {
		t.Fatalf("members: %+v", members)
	}
}

func TestBulkRequestsAreLimited(t *testing.T) {
	st, h := setup(t)
	_, admin := person(t, st, "uma@example.com", "user_admin")
	ids := make([]string, 501)
	for i := range ids {
		ids[i] = "usr_" + strings.Repeat("x", i)
	}
	expect(t, call(t, h, admin, "POST", "/api/v1/users/bulk/suspend", map[string]any{"userIds": ids}), 422)
	for range 19 {
		expect(t, call(t, h, admin, "POST", "/api/v1/users/bulk/suspend", map[string]any{"userIds": []string{}}), 422)
	}
	rec := call(t, h, admin, "POST", "/api/v1/users/bulk/restore", map[string]any{"userIds": []string{}})
	expect(t, rec, 429)
	if !strings.Contains(rec.Body.String(), "ERR_RATE_LIMITED") || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("rate limit: %s", rec.Body)
	}
	expect(t, upload(t, h, admin, "?dryRun=true", "email,name\nsam@example.com,Sam\n"), 429)
}
