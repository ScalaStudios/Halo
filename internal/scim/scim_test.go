package scim_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/server"
	"halo/internal/store"
	"halo/internal/testdb"
)

const enterprise = "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"

type env struct {
	t   *testing.T
	st  *store.Store
	h   http.Handler
	key string
	bot store.ServiceAccount
}

func setup(t *testing.T, roles ...string) *env {
	t.Helper()
	st := testdb.New(t)
	public, _ := url.Parse("http://localhost:3200")
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	app, err := server.New(config.Config{Organization: "Test", PublicURL: public, SecretKey: secret, Dev: true}, st)
	if err != nil {
		t.Fatal(err)
	}
	if roles == nil {
		roles = []string{"user_admin"}
	}
	bot, err := st.CreateServiceAccount(context.Background(), store.NewServiceAccount{Name: "Workday connector", Roles: roles})
	if err != nil {
		t.Fatal(err)
	}
	_, key, err := st.CreateAPIKey(context.Background(), bot.ID, "Workday", []string{"scim"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &env{t: t, st: st, h: app.Handler, key: key, bot: bot}
}

func (e *env) call(method, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	return e.as("Bearer "+e.key, method, path, body)
}

func (e *env) as(authorization, method, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	e.t.Helper()
	var reader io.Reader
	if raw, ok := body.(string); ok {
		reader = strings.NewReader(raw)
	} else if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/scim+json")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func (e *env) expect(rec *httptest.ResponseRecorder, status int) {
	e.t.Helper()
	if rec.Code != status {
		e.t.Fatalf("want %d, got %d: %s", status, rec.Code, rec.Body)
	}
	if rec.Code != http.StatusNoContent && rec.Header().Get("Content-Type") != "application/scim+json" {
		e.t.Fatalf("content type %q", rec.Header().Get("Content-Type"))
	}
}

func (e *env) person(email string, roles ...string) store.User {
	e.t.Helper()
	u, err := e.st.CreateUser(context.Background(), store.NewUser{Email: email, Name: strings.Split(email, "@")[0], Status: "active", Roles: roles})
	if err != nil {
		e.t.Fatal(err)
	}
	return u
}

func (e *env) session(userID string) {
	e.t.Helper()
	if _, _, err := e.st.CreateSession(context.Background(), store.NewSession{UserID: userID, Method: "passkey"}); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) expectError(rec *httptest.ResponseRecorder, body map[string]any, status int, scimType string) {
	e.t.Helper()
	if rec.Code != status {
		e.t.Fatalf("want %d, got %d: %s", status, rec.Code, rec.Body)
	}
	schemas, _ := body["schemas"].([]any)
	if len(schemas) != 1 || schemas[0] != "urn:ietf:params:scim:api:messages:2.0:Error" || body["status"] != strconv.Itoa(status) || body["detail"] == "" {
		e.t.Fatalf("not a SCIM error: %s", rec.Body)
	}
	if scimType != "" && body["scimType"] != scimType {
		e.t.Fatalf("scimType: want %s, got %v", scimType, body["scimType"])
	}
}

func TestUserLifecycle(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	manager := e.person("grace@example.com")

	okta := map[string]any{
		"schemas":    []string{"urn:ietf:params:scim:schemas:core:2.0:User", enterprise},
		"userName":   "ada@example.com",
		"name":       map[string]any{"givenName": "Ada", "familyName": "Lovelace"},
		"emails":     []map[string]any{{"primary": true, "value": "ada@example.com", "type": "work"}},
		"externalId": "00u1ada",
		"title":      "Engineer",
		"active":     true,
		"password":   "ignored",
		enterprise:   map[string]any{"department": "Engineering", "manager": map[string]any{"value": manager.ID}},
	}
	rec, body := e.call("POST", "/scim/v2/Users", okta)
	e.expect(rec, 201)
	id, _ := body["id"].(string)
	if rec.Header().Get("Location") != "http://localhost:3200/scim/v2/Users/"+id || body["userName"] != "ada@example.com" || body["active"] != true || body["displayName"] != "Ada Lovelace" {
		t.Fatalf("created: %s %s", rec.Header().Get("Location"), rec.Body)
	}
	ext, _ := body[enterprise].(map[string]any)
	if ext["department"] != "Engineering" || ext["manager"].(map[string]any)["value"] != manager.ID {
		t.Fatalf("enterprise extension: %v", ext)
	}
	u, err := e.st.GetUser(ctx, id)
	if err != nil || u.Status != "invited" || u.Source != "SCIM · Workday connector" || u.Title != "Engineer" || u.ManagerID == nil || *u.ManagerID != manager.ID {
		t.Fatalf("stored user: %+v %v", u, err)
	}

	rec, body = e.call("POST", "/scim/v2/Users", okta)
	e.expectError(rec, body, 409, "uniqueness")

	for _, filter := range []string{`userName eq "ADA@example.com"`, `externalId eq "00u1ada"`, `emails.value eq "ada@example.com"`} {
		rec, body = e.call("GET", "/scim/v2/Users?filter="+url.QueryEscape(filter), nil)
		e.expect(rec, 200)
		resources, _ := body["Resources"].([]any)
		if body["totalResults"] != float64(1) || len(resources) != 1 || resources[0].(map[string]any)["id"] != id {
			t.Fatalf("filter %s: %s", filter, rec.Body)
		}
	}
	rec, body = e.call("GET", "/scim/v2/Users?filter="+url.QueryEscape(`userName eq "nobody@example.com"`), nil)
	if resources, _ := body["Resources"].([]any); rec.Code != 200 || body["totalResults"] != float64(0) || resources == nil || len(resources) != 0 {
		t.Fatalf("empty filter result: %s", rec.Body)
	}
	rec, body = e.call("GET", "/scim/v2/Users?filter="+url.QueryEscape(`title eq "Engineer"`), nil)
	e.expectError(rec, body, 400, "invalidFilter")
	rec, body = e.call("GET", "/scim/v2/Users?startIndex=2&count=1", nil)
	if resources, _ := body["Resources"].([]any); rec.Code != 200 || body["totalResults"] != float64(2) || body["startIndex"] != float64(2) || len(resources) != 1 {
		t.Fatalf("paging: %s", rec.Body)
	}

	e.session(id)
	entra := map[string]any{
		"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		"Operations": []map[string]any{
			{"op": "Replace", "path": "active", "value": "False"},
			{"op": "Add", "path": enterprise + ":department", "value": "Research"},
			{"op": "Replace", "path": "name.familyName", "value": "King"},
		},
	}
	rec, body = e.call("PATCH", "/scim/v2/Users/"+id, entra)
	e.expect(rec, 200)
	if body["active"] != false || body["displayName"] != "Ada King" {
		t.Fatalf("entra patch: %s", rec.Body)
	}
	u, _ = e.st.GetUser(ctx, id)
	sessions, _ := e.st.ListSessions(ctx, id)
	if u.Status != "suspended" || u.Department != "Research" || len(sessions) != 0 {
		t.Fatalf("after deactivation: %s %s %d sessions", u.Status, u.Department, len(sessions))
	}

	rec, body = e.call("PATCH", "/scim/v2/Users/"+id, map[string]any{
		"schemas":    []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		"Operations": []map[string]any{{"op": "replace", "value": map[string]any{"active": true, "displayName": "Augusta Ada King", "name": map[string]any{"givenName": "Ignored"}}}},
	})
	e.expect(rec, 200)
	if u, _ = e.st.GetUser(ctx, id); u.Status != "invited" || u.Name != "Augusta Ada King" || body["active"] != true {
		t.Fatalf("okta reactivation: %+v %s", u, rec.Body)
	}

	rec, body = e.call("PATCH", "/scim/v2/Users/"+id, map[string]any{"Operations": []map[string]any{
		{"op": "remove", "path": enterprise + ":manager"},
		{"op": "add", "path": `emails[type eq "work"].value`, "value": "other@example.com"},
	}})
	e.expect(rec, 200)
	if _, has := body[enterprise].(map[string]any)["manager"]; has || body["userName"] != "ada@example.com" {
		t.Fatalf("manager removal: %s", rec.Body)
	}
	rec, body = e.call("PATCH", "/scim/v2/Users/"+id, map[string]any{"Operations": []map[string]any{{"op": "move", "path": "title"}}})
	e.expectError(rec, body, 400, "invalidSyntax")
	rec, body = e.call("PATCH", "/scim/v2/Users/"+id, map[string]any{"Operations": []map[string]any{{"op": "replace", "path": enterprise + ":manager", "value": "usr_missing"}}})
	e.expectError(rec, body, 400, "invalidValue")

	rec, body = e.call("PUT", "/scim/v2/Users/"+id, map[string]any{"userName": "ada.lovelace@example.com", "displayName": "Ada Lovelace", "externalId": "00u1ada", "active": true})
	e.expect(rec, 200)
	if u, _ = e.st.GetUser(ctx, id); u.Email != "ada.lovelace@example.com" || u.Name != "Ada Lovelace" || u.Title != "" || u.Department != "" {
		t.Fatalf("put: %+v", u)
	}
	rec, body = e.call("PUT", "/scim/v2/Users/"+id, map[string]any{"displayName": "No username"})
	e.expectError(rec, body, 400, "invalidValue")

	e.session(id)
	rec, _ = e.call("DELETE", "/scim/v2/Users/"+id, nil)
	e.expect(rec, 204)
	u, _ = e.st.GetUser(ctx, id)
	sessions, _ = e.st.ListSessions(ctx, id)
	if u.Status != "deprovisioned" || len(sessions) != 0 {
		t.Fatalf("after delete: %s, %d sessions", u.Status, len(sessions))
	}
	rec, body = e.call("GET", "/scim/v2/Users/"+id, nil)
	e.expectError(rec, body, 404, "")

	audit, err := e.st.ListAudit(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]bool{}
	for _, a := range audit {
		if a.ActorID == nil || *a.ActorID != e.bot.ID || a.TargetID != id {
			t.Fatalf("audit event without the service account as actor: %+v", a)
		}
		actions[a.Action] = true
	}
	for _, action := range []string{"scim.user.create", "scim.user.update", "scim.user.suspend", "scim.user.restore", "scim.user.deprovision"} {
		if !actions[action] {
			t.Fatalf("missing audit action %s in %v", action, actions)
		}
	}
}

func TestGroups(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	ada, bob := e.person("ada@example.com"), e.person("bob@example.com")
	rule := `user.department == "Engineering"`
	dynamic, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Engineering", Kind: "dynamic", Rule: &rule})
	if err != nil {
		t.Fatal(err)
	}

	rec, body := e.call("POST", "/scim/v2/Groups", map[string]any{"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:Group"}, "displayName": "Okta engineers", "externalId": "00g1", "members": []map[string]any{{"value": ada.ID}}})
	e.expect(rec, 201)
	id, _ := body["id"].(string)
	members := func(body map[string]any) []string {
		list, _ := body["members"].([]any)
		ids := []string{}
		for _, m := range list {
			ids = append(ids, m.(map[string]any)["value"].(string))
		}
		return ids
	}
	if got := members(body); len(got) != 1 || got[0] != ada.ID || body["externalId"] != "00g1" {
		t.Fatalf("created group: %s", rec.Body)
	}
	if g, err := e.st.GetGroup(ctx, id); err != nil || g.Source != "SCIM · Workday connector" || g.Kind != "assigned" {
		t.Fatalf("stored group: %+v %v", g, err)
	}

	rec, body = e.call("GET", "/scim/v2/Groups?filter="+url.QueryEscape(`displayName eq "okta ENGINEERS"`)+"&excludedAttributes=members", nil)
	resources, _ := body["Resources"].([]any)
	if rec.Code != 200 || len(resources) != 1 || resources[0].(map[string]any)["members"] != nil {
		t.Fatalf("group filter: %s", rec.Body)
	}
	rec, body = e.call("GET", "/scim/v2/Groups", nil)
	if resources, _ = body["Resources"].([]any); rec.Code != 200 || len(resources) != 1 {
		t.Fatalf("dynamic groups must stay hidden: %s", rec.Body)
	}
	rec, body = e.call("GET", "/scim/v2/Groups/"+dynamic.ID, nil)
	e.expectError(rec, body, 404, "")

	patch := func(ops ...map[string]any) []string {
		t.Helper()
		rec, body := e.call("PATCH", "/scim/v2/Groups/"+id, map[string]any{"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"}, "Operations": ops})
		e.expect(rec, 200)
		return members(body)
	}
	if got := patch(map[string]any{"op": "Add", "path": "members", "value": []map[string]any{{"value": bob.ID}}}); len(got) != 2 {
		t.Fatalf("entra add: %v", got)
	}
	if got := patch(map[string]any{"op": "Remove", "path": `members[value eq "` + ada.ID + `"]`}); len(got) != 1 || got[0] != bob.ID {
		t.Fatalf("remove by filter: %v", got)
	}
	patch(map[string]any{"op": "replace", "value": map[string]any{"id": id, "displayName": "Engineers"}})
	if g, _ := e.st.GetSCIMGroup(ctx, id); g.Name != "Engineers" {
		t.Fatalf("rename: %+v", g)
	}
	if got := patch(map[string]any{"op": "replace", "path": "members", "value": []map[string]any{{"value": ada.ID}}}); len(got) != 1 || got[0] != ada.ID {
		t.Fatalf("replace members: %v", got)
	}
	if got := patch(map[string]any{"op": "remove", "path": "members", "value": []map[string]any{{"value": ada.ID}}}); len(got) != 0 {
		t.Fatalf("remove listed members: %v", got)
	}

	rec, body = e.call("PATCH", "/scim/v2/Groups/"+id, map[string]any{"Operations": []map[string]any{{"op": "add", "path": "members", "value": []map[string]any{{"value": "usr_missing"}}}}})
	e.expectError(rec, body, 400, "invalidValue")

	rec, body = e.call("PUT", "/scim/v2/Groups/"+id, map[string]any{"displayName": "Engineers", "members": []map[string]any{{"value": ada.ID}, {"value": bob.ID}}})
	e.expect(rec, 200)
	if got := members(body); len(got) != 2 {
		t.Fatalf("put members: %s", rec.Body)
	}
	rec, body = e.call("POST", "/scim/v2/Groups", map[string]any{"displayName": "engineers"})
	e.expectError(rec, body, 409, "uniqueness")

	app, err := e.st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", GroupIDs: []string{id}})
	if err != nil {
		t.Fatal(err)
	}
	ruleID, err := e.st.SaveLifecycleRule(ctx, "", store.LifecycleRuleInput{Name: "Engineering joiners", Trigger: "joiner", Actions: []store.LifecycleAction{{Type: "add-to-group", GroupID: id}}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	rec, body = e.call("DELETE", "/scim/v2/Groups/"+id, nil)
	e.expectError(rec, body, 409, "mutability")
	if detail, _ := body["detail"].(string); !strings.Contains(detail, "the application Grafana") || !strings.Contains(detail, "the lifecycle rule Engineering joiners") {
		t.Fatalf("conflict must name what uses the group: %s", rec.Body)
	}
	if err := errors.Join(e.st.SetApplicationGroups(ctx, app.ID, nil), e.st.DeleteLifecycleRule(ctx, ruleID)); err != nil {
		t.Fatal(err)
	}
	rec, _ = e.call("DELETE", "/scim/v2/Groups/"+id, nil)
	e.expect(rec, 204)
	rec, body = e.call("GET", "/scim/v2/Groups/"+id, nil)
	e.expectError(rec, body, 404, "")

	audit, _ := e.st.ListAudit(ctx, 1)
	if audit[0].Action != "scim.group.delete" || *audit[0].ActorID != e.bot.ID {
		t.Fatalf("audit: %+v", audit[0])
	}
}

func TestAuthenticationDiscoveryAndGuards(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	rec, body := e.as("", "GET", "/scim/v2/Users", nil)
	e.expectError(rec, body, 401, "")
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("401 without WWW-Authenticate")
	}
	admin := e.person("gia@example.com", "global_admin")
	_, cookie, err := e.st.CreateSession(ctx, store.NewSession{UserID: admin.ID, Method: "passkey"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/scim/v2/Users", nil)
	req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: cookie})
	cookieRec := httptest.NewRecorder()
	e.h.ServeHTTP(cookieRec, req)
	if cookieRec.Code != 401 {
		t.Fatalf("SCIM must not accept browser sessions: %d", cookieRec.Code)
	}
	_, apiKey, _ := e.st.CreateAPIKey(ctx, e.bot.ID, "API only", []string{"api"}, nil, nil)
	rec, body = e.as("Bearer "+apiKey, "GET", "/scim/v2/Users", nil)
	e.expectError(rec, body, 401, "")

	rec, body = e.call("GET", "/scim/v2/ServiceProviderConfig", nil)
	e.expect(rec, 200)
	if body["patch"].(map[string]any)["supported"] != true || body["bulk"].(map[string]any)["supported"] != false {
		t.Fatalf("service provider config: %s", rec.Body)
	}
	rec, body = e.call("GET", "/scim/v2/ResourceTypes", nil)
	e.expect(rec, 200)
	if body["totalResults"] != float64(2) {
		t.Fatalf("resource types: %s", rec.Body)
	}
	rec, body = e.call("GET", "/scim/v2/Schemas", nil)
	e.expect(rec, 200)
	if body["totalResults"] != float64(3) {
		t.Fatalf("schemas: %s", rec.Body)
	}
	rec, _ = e.call("GET", "/scim/v2/Schemas/"+enterprise, nil)
	e.expect(rec, 200)
	rec, body = e.call("GET", "/scim/v2/Bulk", nil)
	e.expectError(rec, body, 404, "")
	rec, body = e.call("POST", "/scim/v2/Users", "{not json")
	e.expectError(rec, body, 400, "invalidSyntax")
	rec, body = e.call("POST", "/scim/v2/Users", map[string]any{"userName": "not-an-email"})
	e.expectError(rec, body, 400, "invalidValue")

	rec, body = e.call("PATCH", "/scim/v2/Users/"+admin.ID, map[string]any{"Operations": []map[string]any{{"op": "replace", "path": "active", "value": false}}})
	e.expectError(rec, body, 403, "")
	rec, body = e.call("GET", "/scim/v2/Users/"+e.bot.ID, nil)
	e.expectError(rec, body, 404, "")

	noRole, err := e.st.CreateServiceAccount(ctx, store.NewServiceAccount{Name: "Reporting", Roles: []string{"auditor"}})
	if err != nil {
		t.Fatal(err)
	}
	_, weak, _ := e.st.CreateAPIKey(ctx, noRole.ID, "Reporting", []string{"scim"}, nil, nil)
	rec, body = e.as("Bearer "+weak, "GET", "/scim/v2/Users", nil)
	e.expectError(rec, body, 403, "")
}
