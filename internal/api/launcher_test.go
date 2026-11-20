package api_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"halo/internal/store"
)

func TestBookmarkApplication(t *testing.T) {
	st, h := setup(t)
	_, admin := person(t, st, "abe@example.com", "app_admin")

	for _, body := range []map[string]any{
		{"name": "Status", "protocol": "bookmark"},
		{"name": "Status", "protocol": "bookmark", "homepage": "javascript:alert(1)"},
		{"name": "Status", "protocol": "bookmark", "homepage": "https://status.example.com", "redirectUris": []string{"https://status.example.com/cb"}},
		{"name": "Status", "protocol": "bookmark", "homepage": "https://status.example.com", "scopes": []string{"openid"}},
	} {
		expect(t, call(t, h, admin, "POST", "/api/v1/applications", body), 422)
	}

	rec := call(t, h, admin, "POST", "/api/v1/applications", map[string]any{"name": "Status", "protocol": "bookmark", "homepage": "https://status.example.com"})
	expect(t, rec, 201)
	out := decode[created](t, rec)
	if out.ClientSecret != nil || len(out.Application.Credentials) != 0 || len(out.Application.Claims) != 0 || out.Application.Protocol != "bookmark" || out.Application.Homepage != "https://status.example.com" {
		t.Fatalf("bookmark: %+v", out)
	}
	if e := lastAudit(t, st); e.Action != "application.create" || e.Summary != "Added a bookmark to https://status.example.com" {
		t.Fatalf("audit: %+v", e)
	}

	path := "/api/v1/applications/" + out.Application.ID
	expect(t, call(t, h, admin, "POST", path+"/secrets", nil), 422)
	for _, body := range []map[string]any{
		{"homepage": ""},
		{"homepage": "ftp://status.example.com"},
		{"scopes": []string{"openid"}},
		{"redirectUris": []string{"https://status.example.com/cb"}},
	} {
		expect(t, call(t, h, admin, "PATCH", path, body), 422)
	}
	rec = call(t, h, admin, "PATCH", path, map[string]any{"homepage": "https://status.example.org"})
	expect(t, rec, 200)
	if got := decode[store.Application](t, rec); got.Homepage != "https://status.example.org" {
		t.Fatalf("bookmark url not updated: %+v", got)
	}
}

func TestCollections(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "abe@example.com", "app_admin")
	_, userAdmin := person(t, st, "uma@example.com", "user_admin")
	wiki, err := st.CreateApplication(ctx, store.NewApplication{Name: "Wiki", Protocol: "bookmark", Type: "web", Homepage: "https://wiki.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	grafana, err := st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://grafana.example.com/login"}})
	if err != nil {
		t.Fatal(err)
	}
	billing, err := st.CreateApplication(ctx, store.NewApplication{Name: "Billing sync", Protocol: "oauth", Type: "service"})
	if err != nil {
		t.Fatal(err)
	}
	engineering, err := st.CreateGroup(ctx, store.NewGroup{Name: "Engineering", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}

	body := map[string]any{"name": " Engineering ", "applicationIds": []string{wiki.ID, grafana.ID, wiki.ID}, "groupIds": []string{engineering.ID}}
	expect(t, call(t, h, userAdmin, "POST", "/api/v1/collections", body), 403)
	for _, bad := range []map[string]any{
		{"name": "", "applicationIds": []string{wiki.ID}},
		{"name": "Engineering", "applicationIds": []string{"app_missing"}},
		{"name": "Engineering", "applicationIds": []string{billing.ID}},
		{"name": "Engineering", "groupIds": []string{"grp_missing"}},
	} {
		expect(t, call(t, h, admin, "POST", "/api/v1/collections", bad), 422)
	}

	rec := call(t, h, admin, "POST", "/api/v1/collections", body)
	expect(t, rec, 201)
	c := decode[store.Collection](t, rec)
	if c.Name != "Engineering" || !slices.Equal(c.ApplicationIDs, slices.Sorted(slices.Values([]string{wiki.ID, grafana.ID}))) || !slices.Equal(c.GroupIDs, []string{engineering.ID}) {
		t.Fatalf("created: %+v", c)
	}
	if e := lastAudit(t, st); e.Action != "collection.create" || e.TargetID != c.ID || e.Summary != "Created the collection with 2 applications for 1 group" {
		t.Fatalf("audit: %+v", e)
	}
	expect(t, call(t, h, admin, "POST", "/api/v1/collections", map[string]any{"name": "engineering"}), 409)

	rec = call(t, h, userAdmin, "GET", "/api/v1/collections", nil)
	expect(t, rec, 200)
	if list := decode[[]store.Collection](t, rec); len(list) != 1 || list[0].ID != c.ID {
		t.Fatalf("list: %+v", list)
	}

	rec = call(t, h, admin, "PUT", "/api/v1/collections/"+c.ID, map[string]any{"name": "Tools", "applicationIds": []string{wiki.ID}, "groupIds": []string{}})
	expect(t, rec, 200)
	if !strings.Contains(rec.Body.String(), `"groupIds":[]`) {
		t.Fatalf("empty groups must be an empty list: %s", rec.Body)
	}
	if got := decode[store.Collection](t, rec); got.Name != "Tools" || !slices.Equal(got.ApplicationIDs, []string{wiki.ID}) || len(got.GroupIDs) != 0 {
		t.Fatalf("updated: %+v", got)
	}
	if e := lastAudit(t, st); e.Action != "collection.update" || e.TargetLabel != "Tools" {
		t.Fatalf("audit: %+v", e)
	}
	expect(t, call(t, h, admin, "PUT", "/api/v1/collections/col_missing", map[string]any{"name": "Tools"}), 404)

	expect(t, call(t, h, admin, "DELETE", "/api/v1/collections/"+c.ID, nil), 204)
	if e := lastAudit(t, st); e.Action != "collection.delete" || e.TargetLabel != "Tools" {
		t.Fatalf("audit: %+v", e)
	}
	expect(t, call(t, h, admin, "DELETE", "/api/v1/collections/"+c.ID, nil), 404)
}
