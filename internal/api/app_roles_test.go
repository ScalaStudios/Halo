package api_test

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"halo/internal/store"
)

func TestApplicationRoles(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	staff, err := st.CreateGroup(ctx, store.NewGroup{Name: "Staff", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	_, appAdmin := person(t, st, "aya@example.com", "app_admin")
	_, userAdmin := person(t, st, "uma@example.com", "user_admin")
	pat, _ := person(t, st, "pat@example.com")
	wiki, err := st.CreateApplication(ctx, store.NewApplication{Name: "Wiki", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://wiki.example.com/cb"}, GroupIDs: []string{staff.ID}})
	if err != nil {
		t.Fatal(err)
	}
	status, err := st.CreateApplication(ctx, store.NewApplication{Name: "Status", Protocol: "bookmark", Type: "web", Homepage: "https://status.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	roles := "/api/v1/applications/" + wiki.ID + "/roles"
	body := map[string]any{"key": "editor", "name": " Editor ", "description": "Edits pages", "groupIds": []string{staff.ID}, "userIds": []string{pat.ID, pat.ID}}

	expect(t, call(t, h, userAdmin, "POST", roles, body), 403)
	expect(t, call(t, h, appAdmin, "POST", "/api/v1/applications/"+status.ID+"/roles", body), 422)
	for _, bad := range []map[string]any{
		{"key": "1editor", "name": "Editor"},
		{"key": "edit or", "name": "Editor"},
		{"key": "editor", "name": ""},
		{"key": "editor", "name": "Editor", "groupIds": []string{"grp_missing"}},
		{"key": "editor", "name": "Editor", "userIds": []string{"usr_missing"}},
	} {
		expect(t, call(t, h, appAdmin, "POST", roles, bad), 422)
	}
	rec := call(t, h, appAdmin, "POST", roles, body)
	expect(t, rec, 201)
	role := decode[store.AppRole](t, rec)
	if role.Key != "editor" || role.Name != "Editor" || !slices.Equal(role.GroupIDs, []string{staff.ID}) || !slices.Equal(role.UserIDs, []string{pat.ID}) {
		t.Fatalf("created: %+v", role)
	}
	if e := lastAudit(t, st); e.Action != "application.role.create" || e.TargetID != wiki.ID || e.Summary != "Added the role Editor (editor), held by Staff, pat" {
		t.Fatalf("audit: %+v", e)
	}
	expect(t, call(t, h, appAdmin, "POST", roles, map[string]any{"key": "editor", "name": "Again"}), 409)

	path := roles + "/" + role.ID
	expect(t, call(t, h, userAdmin, "PATCH", path, map[string]any{"name": "Writer"}), 403)
	expect(t, call(t, h, appAdmin, "PATCH", path, map[string]any{"key": "writer"}), 422)
	rec = call(t, h, appAdmin, "PATCH", path, map[string]any{"name": "Writer", "userIds": []string{}})
	expect(t, rec, 200)
	if got := decode[store.AppRole](t, rec); got.Name != "Writer" || len(got.UserIDs) != 0 || !slices.Equal(got.GroupIDs, []string{staff.ID}) {
		t.Fatalf("updated: %+v", got)
	}
	if e := lastAudit(t, st); e.Action != "application.role.update" || !reflect.DeepEqual(diff(e), map[string]string{"name": `"Editor" → "Writer"`, "userIds": `["` + pat.ID + `"] → []`}) {
		t.Fatalf("audit: %+v %v", e, diff(e))
	}

	app := decode[store.Application](t, call(t, h, userAdmin, "GET", "/api/v1/applications/"+wiki.ID, nil))
	if len(app.Roles) != 1 || app.Roles[0].Name != "Writer" || !slices.Contains(app.Claims, store.Claim{Name: "roles", Source: "user.roles (the application roles the user holds)"}) {
		t.Fatalf("application view: %+v %+v", app.Roles, app.Claims)
	}
	expect(t, call(t, h, appAdmin, "PATCH", "/api/v1/applications/"+wiki.ID, map[string]any{"claimMappings": []store.Claim{{Name: "sub", Source: "user.roles"}}}), 422)
	expect(t, call(t, h, appAdmin, "PATCH", "/api/v1/applications/"+wiki.ID, map[string]any{"claimMappings": []store.Claim{{Name: "app_roles", Source: "user.roles"}}}), 200)

	expect(t, call(t, h, userAdmin, "DELETE", path, nil), 403)
	expect(t, call(t, h, appAdmin, "DELETE", path, nil), 204)
	if e := lastAudit(t, st); e.Action != "application.role.delete" || e.Summary != "Deleted the role Writer (editor)" {
		t.Fatalf("audit: %+v", e)
	}
	expect(t, call(t, h, appAdmin, "DELETE", path, nil), 404)
}
