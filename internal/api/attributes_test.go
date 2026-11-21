package api_test

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"halo/internal/store"
)

func TestAttributeDefinitions(t *testing.T) {
	st, h := setup(t)
	_, global := person(t, st, "gia@example.com", "global_admin")
	_, userAdmin := person(t, st, "uma@example.com", "user_admin")
	_, plain := person(t, st, "pat@example.com")

	expect(t, call(t, h, userAdmin, "POST", "/api/v1/attributes", map[string]any{"name": "costCentre", "label": "Cost centre", "type": "string"}), 403)
	for _, body := range []map[string]any{
		{"name": "1st", "label": "First", "type": "string"},
		{"name": "cost centre", "label": "Cost centre", "type": "string"},
		{"name": "costCentre", "label": "", "type": "string"},
		{"name": "costCentre", "label": "Cost centre", "type": "money"},
	} {
		expect(t, call(t, h, global, "POST", "/api/v1/attributes", body), 422)
	}
	rec := call(t, h, global, "POST", "/api/v1/attributes", map[string]any{"name": "costCentre", "label": " Cost centre ", "type": "string"})
	expect(t, rec, 201)
	a := decode[store.Attribute](t, rec)
	if a.Name != "costCentre" || a.Label != "Cost centre" || a.Type != "string" || a.SelfService {
		t.Fatalf("created: %+v", a)
	}
	if e := lastAudit(t, st); e.Action != "attribute.create" || e.TargetID != a.ID {
		t.Fatalf("audit: %+v", e)
	}
	expect(t, call(t, h, global, "POST", "/api/v1/attributes", map[string]any{"name": "costCentre", "label": "Again", "type": "number"}), 409)

	expect(t, call(t, h, userAdmin, "PATCH", "/api/v1/attributes/"+a.ID, map[string]any{"selfService": true}), 403)
	expect(t, call(t, h, global, "PATCH", "/api/v1/attributes/"+a.ID, map[string]any{"type": "number"}), 400)
	expect(t, call(t, h, global, "PATCH", "/api/v1/attributes/"+a.ID, map[string]any{"name": "cost"}), 400)
	rec = call(t, h, global, "PATCH", "/api/v1/attributes/"+a.ID, map[string]any{"label": "Cost center", "selfService": true})
	expect(t, rec, 200)
	if got := decode[store.Attribute](t, rec); got.Label != "Cost center" || !got.SelfService || got.Type != "string" {
		t.Fatalf("updated: %+v", got)
	}
	if e := lastAudit(t, st); e.Action != "attribute.update" || !reflect.DeepEqual(diff(e), map[string]string{"label": `"Cost centre" → "Cost center"`, "selfService": `false → true`}) {
		t.Fatalf("audit: %+v %v", e, diff(e))
	}

	list := decode[[]store.Attribute](t, call(t, h, plain, "GET", "/api/v1/attributes", nil))
	if len(list) != 1 || list[0].ID != a.ID {
		t.Fatalf("list: %+v", list)
	}
	expect(t, call(t, h, plain, "DELETE", "/api/v1/attributes/"+a.ID, nil), 403)
	expect(t, call(t, h, global, "DELETE", "/api/v1/attributes/atr_missing", nil), 404)
}

func TestAttributeValues(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, global := person(t, st, "gia@example.com", "global_admin")
	_, helpdesk := person(t, st, "hal@example.com", "helpdesk_admin")
	sam, samToken := person(t, st, "sam@example.com")
	define := func(name, kind string, selfService bool) store.Attribute {
		t.Helper()
		a, err := st.CreateAttribute(ctx, store.Attribute{Name: name, Label: name, Type: kind, SelfService: selfService})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	number := define("employeeNumber", "number", false)
	define("startDate", "date", false)
	define("remote", "boolean", false)
	define("teams", "list", true)
	define("pronouns", "string", true)
	path := "/api/v1/users/" + sam.ID

	for _, values := range []map[string]any{
		{"employeeNumber": "42"},
		{"employeeNumber": true},
		{"employeeNumber": 1 << 54},
		{"startDate": "2026-13-01"},
		{"startDate": 20260101},
		{"remote": "yes"},
		{"teams": "Platform"},
		{"teams": []any{"Platform", 7}},
		{"pronouns": []string{"they"}},
		{"unknown": "x"},
	} {
		expect(t, call(t, h, global, "PATCH", path, map[string]any{"attributes": values}), 422)
	}
	rec := call(t, h, helpdesk, "PATCH", path, map[string]any{"attributes": map[string]any{"employeeNumber": 1042, "startDate": "2026-01-31", "remote": false, "teams": []string{" Platform ", "Security", "Platform", ""}, "pronouns": "  "}})
	expect(t, rec, 200)
	got := decode[store.User](t, rec)
	want := map[string]any{"employeeNumber": float64(1042), "startDate": "2026-01-31", "remote": false, "teams": []any{"Platform", "Security"}}
	if !reflect.DeepEqual(got.Attributes, want) {
		t.Fatalf("attributes: %#v", got.Attributes)
	}
	if e := lastAudit(t, st); e.Action != "user.update" || e.Summary != "Updated employeeNumber, remote, startDate, teams" || diff(e)["attributes.employeeNumber"] != `null → 1042` {
		t.Fatalf("audit: %+v %v", e, diff(e))
	}

	rec = call(t, h, global, "PATCH", path, map[string]any{"attributes": map[string]any{"employeeNumber": nil, "remote": false}})
	expect(t, rec, 200)
	if got := decode[store.User](t, rec); got.Attributes["employeeNumber"] != nil || got.Attributes["remote"] != false {
		t.Fatalf("cleared: %#v", got.Attributes)
	}
	if e := lastAudit(t, st); e.Summary != "Updated employeeNumber" || diff(e)["attributes.employeeNumber"] != `1042 → null` {
		t.Fatalf("audit: %+v %v", e, diff(e))
	}

	expect(t, call(t, h, samToken, "PATCH", "/api/v1/me/attributes", map[string]any{"startDate": "2020-01-01"}), 403)
	rec = call(t, h, samToken, "PATCH", "/api/v1/me/attributes", map[string]any{"pronouns": "they/them", "teams": []string{"Design"}})
	expect(t, rec, 200)
	if got := decode[store.User](t, rec); got.Attributes["pronouns"] != "they/them" || fmt.Sprint(got.Attributes["teams"]) != "[Design]" || got.Attributes["startDate"] != "2026-01-31" {
		t.Fatalf("self-service: %#v", got.Attributes)
	}
	if e := lastAudit(t, st); e.Action != "user.update" || e.Summary != "Updated their pronouns, teams" || e.ActorID == nil || *e.ActorID != sam.ID {
		t.Fatalf("audit: %+v", e)
	}

	expect(t, call(t, h, global, "PATCH", path, map[string]any{"attributes": map[string]any{"employeeNumber": 7}}), 200)
	expect(t, call(t, h, global, "DELETE", "/api/v1/attributes/"+number.ID, nil), 204)
	if e := lastAudit(t, st); e.Action != "attribute.delete" || e.Summary != "Deleted the attribute employeeNumber and its value for 1 person" {
		t.Fatalf("audit: %+v", e)
	}
	u, err := st.GetUser(ctx, sam.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := u.Attributes["employeeNumber"]; ok || len(u.Attributes) != 4 {
		t.Fatalf("after delete: %#v", u.Attributes)
	}
}

func TestClaimMappingValidation(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, appAdmin := person(t, st, "abe@example.com", "app_admin")
	_, global := person(t, st, "gia@example.com", "global_admin")
	attribute, err := st.CreateAttribute(ctx, store.Attribute{Name: "employeeNumber", Label: "Employee number", Type: "number"})
	if err != nil {
		t.Fatal(err)
	}
	oidcApp, err := st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://grafana.example.com/cb"}})
	if err != nil {
		t.Fatal(err)
	}
	samlApp, err := st.CreateApplication(ctx, store.NewApplication{Name: "Helpdesk", Protocol: "saml", Type: "web", ClientID: "https://help.example.com", RedirectURIs: []string{"https://help.example.com/acs"}})
	if err != nil {
		t.Fatal(err)
	}
	service, err := st.CreateApplication(ctx, store.NewApplication{Name: "Robot", Protocol: "oidc", Type: "service"})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/applications/" + oidcApp.ID
	mapping := func(name, source string) map[string]any {
		return map[string]any{"claimMappings": []store.Claim{{Name: name, Source: source}}}
	}

	for _, reserved := range []string{"sub", "iss", "aud", "exp", "iat", "nonce", "azp", "acr", "amr", "sid"} {
		expect(t, call(t, h, appAdmin, "PATCH", path, mapping(reserved, "user.email")), 422)
	}
	expect(t, call(t, h, appAdmin, "PATCH", path, mapping("employee id", "user.id")), 422)
	expect(t, call(t, h, appAdmin, "PATCH", path, mapping("employee_id", "attribute.missing")), 422)
	expect(t, call(t, h, appAdmin, "PATCH", path, mapping("employee_id", "user.password")), 422)
	expect(t, call(t, h, appAdmin, "PATCH", path, map[string]any{"claimMappings": []store.Claim{{Name: "mail", Source: "user.email"}, {Name: "mail", Source: "user.name"}}}), 422)
	expect(t, call(t, h, appAdmin, "PATCH", "/api/v1/applications/"+service.ID, mapping("employee_id", "user.id")), 422)
	if got, _ := st.GetApplication(ctx, oidcApp.ID); len(got.ClaimMappings) != 0 {
		t.Fatalf("refused mappings were saved: %+v", got.ClaimMappings)
	}

	mappings := []store.Claim{{Name: " employee_id ", Source: "attribute.employeeNumber"}, {Name: "email", Source: store.OmitClaim}, {Name: "mail", Source: "user.email"}}
	rec := call(t, h, appAdmin, "PATCH", path, map[string]any{"claimMappings": mappings})
	expect(t, rec, 200)
	app := decode[store.Application](t, rec)
	want := []store.Claim{{Name: "employee_id", Source: "attribute.employeeNumber"}, {Name: "email", Source: "none"}, {Name: "mail", Source: "user.email"}}
	if !slices.Equal(app.ClaimMappings, want) {
		t.Fatalf("mappings: %+v", app.ClaimMappings)
	}
	if slices.ContainsFunc(app.Claims, func(c store.Claim) bool { return c.Name == "email" }) || !slices.Contains(app.Claims, store.Claim{Name: "employee_id", Source: "attribute.employeeNumber"}) {
		t.Fatalf("effective claims: %+v", app.Claims)
	}
	if e := lastAudit(t, st); e.Action != "application.update" || e.Summary != "Updated claim mappings" || diff(e)["claimMappings"] == "" {
		t.Fatalf("audit: %+v %v", e, diff(e))
	}

	expect(t, call(t, h, appAdmin, "PATCH", "/api/v1/applications/"+samlApp.ID, mapping("sub", "attribute.employeeNumber")), 200)
	expect(t, call(t, h, global, "DELETE", "/api/v1/attributes/"+attribute.ID, nil), 409)
	expect(t, call(t, h, appAdmin, "PATCH", path, map[string]any{"claimMappings": []store.Claim{}}), 200)
	expect(t, call(t, h, appAdmin, "PATCH", "/api/v1/applications/"+samlApp.ID, map[string]any{"claimMappings": []store.Claim{}}), 200)
	expect(t, call(t, h, global, "DELETE", "/api/v1/attributes/"+attribute.ID, nil), 204)
}
