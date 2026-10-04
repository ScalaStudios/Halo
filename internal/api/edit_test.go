package api_test

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"testing"

	"halo/internal/store"
)

func lastAudit(t *testing.T, st *store.Store) store.AuditEvent {
	t.Helper()
	events, err := st.ListAudit(context.Background(), 1)
	if err != nil || len(events) != 1 {
		t.Fatalf("audit: %+v %v", events, err)
	}
	return events[0]
}

func TestUpdateUserProfile(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, userAdmin := person(t, st, "uma@example.com", "user_admin")
	_, helpdesk := person(t, st, "hal@example.com", "helpdesk_admin")
	_, appAdmin := person(t, st, "abe@example.com", "app_admin")
	_, global := person(t, st, "gia@example.com", "global_admin")
	auditor, _ := person(t, st, "audrey@example.com", "auditor")
	sam, _ := person(t, st, "sam@example.com")
	person(t, st, "lee@example.com")
	engineering, err := st.CreateGroup(ctx, store.NewGroup{Name: "Engineering", Kind: "dynamic", Rule: new(`user.department == "Engineering"`)})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/users/" + sam.ID

	rec := call(t, h, userAdmin, "PATCH", path, map[string]any{"name": " Sam Lee ", "email": "sam.lee@example.com", "department": "Engineering", "location": "Lisbon"})
	expect(t, rec, 200)
	got := decode[store.User](t, rec)
	if got.Name != "Sam Lee" || got.Email != "sam.lee@example.com" || got.Department != "Engineering" || got.Location != "Lisbon" || !slices.Contains(got.GroupIDs, engineering.ID) {
		t.Fatalf("profile not updated: %+v", got)
	}
	if e := lastAudit(t, st); e.Action != "user.update" || e.Summary != "Updated name, email, department, location" || e.TargetID != sam.ID || e.TargetLabel != "Sam Lee" {
		t.Fatalf("audit: %+v", e)
	}

	expect(t, call(t, h, userAdmin, "PATCH", path, map[string]any{"title": "", "name": "Sam Lee"}), 200)
	if e := lastAudit(t, st); e.Summary != "Updated name, email, department, location" {
		t.Fatalf("a no-op update must not be audited: %+v", e)
	}
	expect(t, call(t, h, userAdmin, "PATCH", path, map[string]any{"email": "not-an-email"}), 422)
	expect(t, call(t, h, userAdmin, "PATCH", path, map[string]any{"email": "LEE@example.com"}), 409)
	expect(t, call(t, h, userAdmin, "PATCH", path, map[string]any{"name": "  "}), 422)
	expect(t, call(t, h, userAdmin, "PATCH", path, map[string]any{"title": strings.Repeat("x", 201)}), 422)
	expect(t, call(t, h, userAdmin, "PATCH", path, map[string]any{"status": "active"}), 400)
	expect(t, call(t, h, userAdmin, "PATCH", "/api/v1/users/usr_missing", map[string]any{"title": "x"}), 404)

	expect(t, call(t, h, helpdesk, "PATCH", path, map[string]any{"title": "Staff engineer", "department": "Platform"}), 200)
	rec = call(t, h, helpdesk, "PATCH", path, map[string]any{"name": "Samuel"})
	expect(t, rec, 403)
	if !strings.Contains(rec.Body.String(), "title, department and location only") {
		t.Fatalf("helpdesk message: %s", rec.Body)
	}
	expect(t, call(t, h, helpdesk, "PATCH", path, map[string]any{"name": "Sam Lee", "title": "Principal engineer"}), 200)
	expect(t, call(t, h, appAdmin, "PATCH", path, map[string]any{"title": "x"}), 403)
	expect(t, call(t, h, userAdmin, "PATCH", "/api/v1/users/"+auditor.ID, map[string]any{"title": "Auditor"}), 403)
	expect(t, call(t, h, global, "PATCH", "/api/v1/users/"+auditor.ID, map[string]any{"title": "Auditor"}), 200)
}

func TestUpdateUserManager(t *testing.T) {
	st, h := setup(t)
	_, admin := person(t, st, "uma@example.com", "user_admin")
	ceo, _ := person(t, st, "ceo@example.com")
	vp, _ := person(t, st, "vp@example.com")
	ic, _ := person(t, st, "ic@example.com")
	set := func(u store.User, managerID string) int {
		return call(t, h, admin, "PATCH", "/api/v1/users/"+u.ID, map[string]any{"managerId": managerID}).Code
	}

	if set(vp, ceo.ID) != 200 || set(ic, vp.ID) != 200 {
		t.Fatal("setting managers failed")
	}
	if e := lastAudit(t, st); e.Summary != "Updated manager" || e.TargetID != ic.ID {
		t.Fatalf("audit: %+v", e)
	}
	rec := call(t, h, admin, "PATCH", "/api/v1/users/"+ceo.ID, map[string]any{"managerId": ic.ID})
	expect(t, rec, 422)
	if !strings.Contains(rec.Body.String(), "would create a loop") {
		t.Fatalf("cycle message: %s", rec.Body)
	}
	if set(ceo, ceo.ID) != 422 || set(ceo, "usr_missing") != 422 {
		t.Fatal("self and unknown managers must be rejected")
	}
	rec = call(t, h, admin, "PATCH", "/api/v1/users/"+ic.ID, map[string]any{"managerId": ""})
	expect(t, rec, 200)
	if decode[store.User](t, rec).ManagerID != nil {
		t.Fatal("manager not cleared")
	}
	if set(ceo, ic.ID) != 200 {
		t.Fatal("no loop remains once the chain is broken")
	}

	rec = call(t, h, admin, "POST", "/api/v1/users", map[string]any{"email": "new@example.com", "name": "New Hire", "title": "Engineer", "department": "Engineering", "location": "Berlin", "managerId": vp.ID})
	expect(t, rec, 201)
	invited := decode[struct {
		User store.User `json:"user"`
	}](t, rec).User
	if invited.ManagerID == nil || *invited.ManagerID != vp.ID || invited.Department != "Engineering" || invited.Title != "Engineer" || invited.Location != "Berlin" {
		t.Fatalf("invite profile: %+v", invited)
	}
	expect(t, call(t, h, admin, "POST", "/api/v1/users", map[string]any{"email": "other@example.com", "name": "Other", "managerId": "usr_missing"}), 422)
}

func TestGroupEditMembersAndDelete(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "uma@example.com", "user_admin")
	_, auditor := person(t, st, "audrey@example.com", "auditor")
	sam, _ := person(t, st, "sam@example.com")
	if _, err := st.CreateGroup(ctx, store.NewGroup{Name: "Platform", Kind: "assigned"}); err != nil {
		t.Fatal(err)
	}
	ops, _ := st.CreateGroup(ctx, store.NewGroup{Name: "Ops", Kind: "assigned"})
	berlin, _ := st.CreateGroup(ctx, store.NewGroup{Name: "Berlin", Kind: "dynamic", Rule: new(`user.location == "Berlin"`)})
	if err := st.AddGroupMember(ctx, ops.ID, sam.ID); err != nil {
		t.Fatal(err)
	}
	sam.Location = "Lisbon"
	if err := st.UpdateUserProfile(ctx, sam); err != nil {
		t.Fatal(err)
	}

	rec := call(t, h, auditor, "GET", "/api/v1/groups/"+ops.ID+"/members", nil)
	expect(t, rec, 200)
	if members := decode[[]store.User](t, rec); len(members) != 1 || members[0].ID != sam.ID {
		t.Fatalf("assigned members: %+v", members)
	}
	rec = call(t, h, auditor, "GET", "/api/v1/groups/"+berlin.ID+"/members?rule="+url.QueryEscape(`user.location == "Lisbon"`), nil)
	expect(t, rec, 200)
	if members := decode[[]store.User](t, rec); len(members) != 1 || members[0].ID != sam.ID {
		t.Fatalf("rule preview: %+v", members)
	}
	expect(t, call(t, h, auditor, "GET", "/api/v1/groups/"+berlin.ID+"/members?rule=nonsense", nil), 422)
	expect(t, call(t, h, auditor, "GET", "/api/v1/groups/"+ops.ID+"/members?rule="+url.QueryEscape(`user.location == "Lisbon"`), nil), 422)
	expect(t, call(t, h, auditor, "GET", "/api/v1/groups/grp_missing", nil), 404)

	expect(t, call(t, h, auditor, "PATCH", "/api/v1/groups/"+ops.ID, map[string]any{"name": "Operations"}), 403)
	rec = call(t, h, admin, "PATCH", "/api/v1/groups/"+ops.ID, map[string]any{"name": "Operations", "description": "Runs production"})
	expect(t, rec, 200)
	if g := decode[store.Group](t, rec); g.Name != "Operations" || g.Description != "Runs production" || g.MemberCount != 1 {
		t.Fatalf("group not updated: %+v", g)
	}
	if e := lastAudit(t, st); e.Action != "group.update" || e.Summary != "Updated name, description" || e.TargetLabel != "Operations" {
		t.Fatalf("audit: %+v", e)
	}
	expect(t, call(t, h, admin, "PATCH", "/api/v1/groups/"+ops.ID, map[string]any{"name": "platform"}), 409)
	expect(t, call(t, h, admin, "PATCH", "/api/v1/groups/"+ops.ID, map[string]any{"kind": "dynamic"}), 422)
	expect(t, call(t, h, admin, "PATCH", "/api/v1/groups/"+ops.ID, map[string]any{"rule": `user.location == "Lisbon"`}), 422)
	expect(t, call(t, h, admin, "PATCH", "/api/v1/groups/"+berlin.ID, map[string]any{"rule": "user.location = Lisbon"}), 422)
	rec = call(t, h, admin, "PATCH", "/api/v1/groups/"+berlin.ID, map[string]any{"name": "Lisbon", "rule": ` user.location == "Lisbon" `})
	expect(t, rec, 200)
	if g := decode[store.Group](t, rec); g.Rule == nil || *g.Rule != `user.location == "Lisbon"` || g.MemberCount != 1 {
		t.Fatalf("rule not updated: %+v", g)
	}

	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", GroupIDs: []string{ops.ID}})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := st.SaveAccessPackage(ctx, "", store.AccessPackageInput{Name: "On-call", GroupIDs: []string{ops.ID}})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := st.CreateAccessPolicy(ctx, store.AccessPolicy{Name: "Ops need passkeys", Enabled: true, Mode: "report", Effect: "block", Conditions: store.PolicyConditions{AllUsers: true, ExcludeGroupIDs: []string{ops.ID}, AllApps: true}})
	if err != nil {
		t.Fatal(err)
	}
	rec = call(t, h, admin, "DELETE", "/api/v1/groups/"+ops.ID, nil)
	expect(t, rec, 409)
	for _, want := range []string{"the access package On-call", "the application Grafana", "the policy Ops need passkeys"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("conflict message missing %q: %s", want, rec.Body)
		}
	}
	if err := errors.Join(st.SetApplicationGroups(ctx, app.ID, nil), st.DeleteAccessPackage(ctx, pkg), st.DeleteAccessPolicy(ctx, policy.ID)); err != nil {
		t.Fatal(err)
	}
	expect(t, call(t, h, auditor, "DELETE", "/api/v1/groups/"+ops.ID, nil), 403)
	expect(t, call(t, h, admin, "DELETE", "/api/v1/groups/"+ops.ID, nil), 204)
	if e := lastAudit(t, st); e.Action != "group.delete" || e.Summary != "Deleted the assigned group, which had 1 member" {
		t.Fatalf("audit: %+v", e)
	}
	expect(t, call(t, h, admin, "DELETE", "/api/v1/groups/"+ops.ID, nil), 404)
}

func TestUpdateApplication(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	owner, admin := person(t, st, "abe@example.com", "app_admin")
	_, userAdmin := person(t, st, "uma@example.com", "user_admin")
	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://grafana.example.com/login"}})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/applications/" + app.ID

	expect(t, call(t, h, userAdmin, "PATCH", path, map[string]any{"name": "Dashboards"}), 403)
	rec := call(t, h, admin, "PATCH", path, map[string]any{
		"name":           "Dashboards",
		"homepage":       "https://grafana.example.com",
		"redirectUris":   []string{"https://grafana.example.com/login", " https://grafana.example.com/login ", "http://localhost:3000/login"},
		"postLogoutUris": []string{"https://grafana.example.com/"},
		"scopes":         []string{"openid", "email", "offline_access"},
		"tokenPolicy":    map[string]any{"accessTokenTtl": 300, "refreshTokenTtl": 7 * 86400, "rotation": false},
		"owner":          owner.ID,
	})
	expect(t, rec, 200)
	got := decode[store.Application](t, rec)
	if got.Name != "Dashboards" || len(got.RedirectURIs) != 2 || len(got.PostLogoutURIs) != 1 || !slices.Equal(got.Scopes, []string{"openid", "email", "offline_access"}) ||
		got.TokenPolicy != (store.TokenPolicy{AccessTokenTTL: 300, RefreshTokenTTL: 7 * 86400, IDTokenTTL: 900, Rotation: false}) || got.Owner == nil || *got.Owner != owner.ID {
		t.Fatalf("application not updated: %+v", got)
	}
	want := "Updated name, homepage, redirect URIs, post-logout URIs, scopes, refresh token rotation, token lifetimes, owner"
	if e := lastAudit(t, st); e.Action != "application.update" || e.Summary != want || e.TargetLabel != "Dashboards" {
		t.Fatalf("audit: %+v", e)
	}

	for _, body := range []map[string]any{
		{"name": ""},
		{"homepage": "grafana"},
		{"redirectUris": []string{}},
		{"redirectUris": []string{"http://grafana.example.com/login"}},
		{"postLogoutUris": []string{"https://grafana.example.com/#done"}},
		{"scopes": []string{"email"}},
		{"scopes": []string{"openid", "two words"}},
		{"tokenPolicy": map[string]any{"accessTokenTtl": 59}},
		{"tokenPolicy": map[string]any{"accessTokenTtl": 86401}},
		{"tokenPolicy": map[string]any{"idTokenTtl": 0}},
		{"tokenPolicy": map[string]any{"refreshTokenTtl": 90*86400 + 1}},
		{"tokenPolicy": map[string]any{"refreshTokenTtl": -1}},
		{"owner": "usr_missing"},
	} {
		if rec := call(t, h, admin, "PATCH", path, body); rec.Code != 422 {
			t.Errorf("%v: want 422, got %d: %s", body, rec.Code, rec.Body)
		}
	}
	rec = call(t, h, admin, "PATCH", path, map[string]any{"tokenPolicy": map[string]any{"refreshTokenTtl": 0}, "owner": ""})
	expect(t, rec, 200)
	if got := decode[store.Application](t, rec); got.TokenPolicy.RefreshTokenTTL != 0 || got.Owner != nil {
		t.Fatalf("refresh tokens and owner not cleared: %+v", got)
	}

	saml, err := st.CreateApplication(ctx, store.NewApplication{Name: "Slack", Protocol: "saml", Type: "web", ClientID: "https://slack.com", RedirectURIs: []string{"https://example.slack.com/sso/saml"}})
	if err != nil {
		t.Fatal(err)
	}
	expect(t, call(t, h, admin, "PATCH", "/api/v1/applications/"+saml.ID, map[string]any{"scopes": []string{"openid"}}), 422)
	rec = call(t, h, admin, "PATCH", "/api/v1/applications/"+saml.ID, map[string]any{"name": "Slack Enterprise", "description": "Chat"})
	expect(t, rec, 200)
	if got := decode[store.Application](t, rec); got.Name != "Slack Enterprise" || !slices.Equal(got.RedirectURIs, []string{"https://example.slack.com/sso/saml"}) {
		t.Fatalf("saml app: %+v", got)
	}
}
