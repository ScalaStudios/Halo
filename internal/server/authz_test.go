package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"halo/internal/store"
)

type authzApp struct {
	e        *env
	clientID string
	secret   string
}

func (a authzApp) call(method, path string, body any) *httptest.ResponseRecorder {
	a.e.t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	if a.clientID != "" {
		req.SetBasicAuth(a.clientID, a.secret)
	}
	rec := httptest.NewRecorder()
	a.e.h.ServeHTTP(rec, req)
	return rec
}

func (a authzApp) expect(method, path string, body any, status int, code string) *httptest.ResponseRecorder {
	a.e.t.Helper()
	rec := a.call(method, path, body)
	if rec.Code != status || code != "" && !strings.Contains(rec.Body.String(), `"`+code+`"`) {
		a.e.t.Fatalf("%s %s %v: want %d %s, got %d: %s", method, path, body, status, code, rec.Code, rec.Body)
	}
	return rec
}

func (a authzApp) write(tuples ...[3]string) {
	a.e.t.Helper()
	writes := []map[string]string{}
	for _, t := range tuples {
		writes = append(writes, map[string]string{"object": t[0], "relation": t[1], "subject": t[2]})
	}
	a.expect("POST", "/api/v1/authz/tuples", map[string]any{"writes": writes}, http.StatusNoContent, "")
}

func (a authzApp) check(object, relation, userID string) (bool, []string) {
	a.e.t.Helper()
	rec := a.expect("POST", "/api/v1/authz/check", map[string]string{"object": object, "relation": relation, "subject": "user:" + userID}, http.StatusOK, "")
	var out struct {
		Allowed bool
		Path    []string
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out.Allowed, out.Path
}

func list(t *testing.T, rec *httptest.ResponseRecorder, field string) []string {
	t.Helper()
	var out map[string]json.RawMessage
	var items []string
	if json.Unmarshal(rec.Body.Bytes(), &out) != nil || json.Unmarshal(out[field], &items) != nil {
		t.Fatalf("%s: %s", field, rec.Body)
	}
	slices.Sort(items)
	return items
}

var documentModel = map[string]any{"types": map[string]any{
	"team":   map[string]any{"member": map[string]any{"subjects": []string{"user", "group"}}},
	"folder": map[string]any{"viewer": map[string]any{"subjects": []string{"user", "team#member"}}},
	"chain":  map[string]any{"next": map[string]any{"subjects": []string{"user", "chain#next"}}},
	"document": map[string]any{
		"parent": map[string]any{"subjects": []string{"folder"}},
		"owner":  map[string]any{"subjects": []string{"user"}},
		"editor": map[string]any{"subjects": []string{"user", "group"}, "union": []string{"owner"}, "roles": []string{"editor"}},
		"viewer": map[string]any{"subjects": []string{"user", "group", "document#viewer"}, "union": []string{"editor"}, "from": []map[string]string{{"via": "parent", "relation": "viewer"}}},
	},
}}

func TestApplicationAuthorization(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	_, admin, _ := e.person("aya@example.com", "passkey", false, "app_admin")
	_, auditor, _ := e.person("audrey@example.com", "passkey", false, "auditor")
	ada, _, _ := e.person("ada@example.com", "passkey", false)
	bob, _, _ := e.person("bob@example.com", "passkey", false)
	cy, _, _ := e.person("cy@example.com", "passkey", false)
	dee, _, _ := e.person("dee@example.com", "passkey", false)
	sam, _, _ := e.person("sam@example.com", "passkey", false)
	eve, _, _ := e.person("eve@example.com", "passkey", false)
	staff, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Staff", Kind: "assigned"})
	must(err)
	must(e.st.AddGroupMember(ctx, staff.ID, ada.ID))
	must(e.st.AddGroupMember(ctx, staff.ID, sam.ID))
	readers, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Readers", Kind: "assigned"})
	must(err)
	for _, u := range []store.User{ada, bob, cy, dee} {
		must(e.st.AddGroupMember(ctx, readers.ID, u.ID))
	}
	finance, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Finance", Kind: "assigned"})
	must(err)
	must(e.st.AddGroupMember(ctx, finance.ID, eve.ID))
	must(e.st.AddGroupMember(ctx, finance.ID, ada.ID))

	wiki, err := e.st.CreateApplication(ctx, store.NewApplication{Name: "Wiki", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://wiki.example.com/cb"}, GroupIDs: []string{staff.ID, readers.ID}})
	must(err)
	_, err = e.st.CreateAppRole(ctx, wiki.ID, store.AppRole{Key: "editor", Name: "Editor", UserIDs: []string{cy.ID}})
	must(err)
	_, wikiSecret, err := e.st.AddClientSecret(ctx, wiki.ID, "Test", time.Hour)
	must(err)
	billing, err := e.st.CreateApplication(ctx, store.NewApplication{Name: "Billing", Protocol: "oauth", Type: "service"})
	must(err)
	_, billingSecret, err := e.st.AddClientSecret(ctx, billing.ID, "Test", time.Hour)
	must(err)
	bookmark, err := e.st.CreateApplication(ctx, store.NewApplication{Name: "Status", Protocol: "bookmark", Type: "web", Homepage: "https://status.example.com"})
	must(err)
	app := authzApp{e, wiki.ClientID, wikiSecret}
	other := authzApp{e, billing.ClientID, billingSecret}
	model := "/api/v1/applications/" + wiki.ID + "/authorization"

	if rec := e.expect(auditor, "GET", model, nil, http.StatusOK, ""); !strings.Contains(rec.Body.String(), `"types":{}`) || !strings.Contains(rec.Body.String(), `"updatedAt":null`) {
		t.Fatalf("no model yet: %s", rec.Body)
	}
	app.expect("POST", "/api/v1/authz/check", nil, http.StatusForbidden, "ERR_NO_MODEL")
	e.expect(auditor, "PUT", model, documentModel, http.StatusForbidden, "")
	e.expect(admin, "PUT", "/api/v1/applications/"+bookmark.ID+"/authorization", documentModel, http.StatusUnprocessableEntity, "")
	for _, bad := range []map[string]any{
		{"user": map[string]any{"owner": map[string]any{"subjects": []string{"user"}}}},
		{"Doc": map[string]any{"owner": map[string]any{"subjects": []string{"user"}}}},
		{"doc": map[string]any{"owner": map[string]any{}}},
		{"doc": map[string]any{"owner": map[string]any{"subjects": []string{"robot"}}}},
		{"doc": map[string]any{"owner": map[string]any{"subjects": []string{"group#member"}}}},
		{"doc": map[string]any{"owner": map[string]any{"union": []string{"owner"}}}},
		{"doc": map[string]any{"owner": map[string]any{"roles": []string{"approver"}}}},
		{"doc": map[string]any{"owner": map[string]any{"from": []map[string]string{{"via": "parent", "relation": "viewer"}}}}},
	} {
		e.expect(admin, "PUT", model, map[string]any{"types": bad}, http.StatusUnprocessableEntity, "ERR_INVALID")
	}
	e.expect(admin, "PUT", model, documentModel, http.StatusOK, "")
	audit, err := e.st.ListAudit(ctx, 1)
	if err != nil || audit[0].Action != "application.authorization.update" || audit[0].TargetID != wiki.ID || audit[0].Summary != "Saved the authorization model with 4 types and 7 relations" || len(audit[0].Changes) == 0 {
		t.Fatalf("audit: %+v %v", audit, err)
	}

	authzApp{e, "", ""}.expect("POST", "/api/v1/authz/check", nil, http.StatusUnauthorized, "ERR_UNAUTHENTICATED")
	authzApp{e, wiki.ClientID, billingSecret}.expect("POST", "/api/v1/authz/check", nil, http.StatusUnauthorized, "ERR_UNAUTHENTICATED")
	other.expect("POST", "/api/v1/authz/check", nil, http.StatusForbidden, "ERR_NO_MODEL")

	app.write(
		[3]string{"document:readme", "owner", "user:" + bob.ID},
		[3]string{"document:readme", "parent", "folder:eng"},
		[3]string{"folder:eng", "viewer", "team:core#member"},
		[3]string{"team:core", "member", "group:" + staff.ID},
		[3]string{"document:plan", "viewer", "document:readme#viewer"},
		[3]string{"document:loop", "viewer", "document:loop2#viewer"},
		[3]string{"document:loop2", "viewer", "document:loop#viewer"},
		[3]string{"document:readme", "owner", "user:" + bob.ID},
	)
	for _, bad := range []map[string]string{
		{"object": "document:readme", "relation": "owner", "subject": "group:" + staff.ID},
		{"object": "document:readme", "relation": "owner", "subject": "user:usr_missing"},
		{"object": "invoice:1", "relation": "owner", "subject": "user:" + bob.ID},
		{"object": "document:readme", "relation": "owner", "subject": "user:" + bob.ID + "#member"},
		{"object": "readme", "relation": "owner", "subject": "user:" + bob.ID},
	} {
		app.expect("POST", "/api/v1/authz/tuples", map[string]any{"writes": []map[string]string{bad}}, http.StatusUnprocessableEntity, "ERR_INVALID")
	}
	for _, outsider := range []string{"group:" + finance.ID, "user:" + eve.ID, "user:usr_missing"} {
		rec := app.expect("POST", "/api/v1/authz/tuples", map[string]any{"writes": []map[string]string{{"object": "team:core", "relation": "member", "subject": outsider}}}, http.StatusUnprocessableEntity, "ERR_INVALID")
		if !strings.Contains(rec.Body.String(), outsider+" isn't assigned to Wiki") {
			t.Fatalf("%s: %s", outsider, rec.Body)
		}
	}
	if ok, _ := app.check("team:core", "member", eve.ID); ok {
		t.Fatal("a person outside the application must be denied")
	}
	many := []map[string]string{}
	for i := range 101 {
		many = append(many, map[string]string{"object": fmt.Sprintf("document:d%d", i), "relation": "owner", "subject": "user:" + bob.ID})
	}
	app.expect("POST", "/api/v1/authz/tuples", map[string]any{"writes": many}, http.StatusUnprocessableEntity, "ERR_INVALID")

	if ok, path := app.check("document:readme", "viewer", bob.ID); !ok || !slices.Equal(path, []string{"document:readme#viewer", "document:readme#editor", "document:readme#owner", "user:" + bob.ID}) {
		t.Fatalf("bob through owner: %v %v", ok, path)
	}
	if ok, path := app.check("document:readme", "viewer", ada.ID); !ok || !slices.Equal(path, []string{"document:readme#viewer", "folder:eng#viewer", "team:core#member", "group:" + staff.ID}) {
		t.Fatalf("ada through her group: %v %v", ok, path)
	}
	if ok, path := app.check("document:readme", "editor", cy.ID); !ok || !slices.Equal(path, []string{"document:readme#editor", "role:editor"}) {
		t.Fatalf("cy through the editor role: %v %v", ok, path)
	}
	if ok, _ := app.check("document:plan", "viewer", ada.ID); !ok {
		t.Fatal("ada through document:readme#viewer")
	}
	for _, denied := range [][2]string{{"document:readme", dee.ID}, {"document:loop", ada.ID}, {"document:readme", "usr_missing"}} {
		if ok, path := app.check(denied[0], "viewer", denied[1]); ok || len(path) != 0 {
			t.Fatalf("%s on %s must be denied: %v", denied[1], denied[0], path)
		}
	}
	if ok, _ := app.check("document:readme", "editor", ada.ID); ok {
		t.Fatal("viewing does not make ada an editor")
	}
	app.expect("POST", "/api/v1/authz/check", map[string]string{"object": "document:readme", "relation": "approver", "subject": "user:" + ada.ID}, http.StatusUnprocessableEntity, "ERR_INVALID")
	app.expect("POST", "/api/v1/authz/check", map[string]string{"object": "document:readme", "relation": "viewer", "subject": "group:" + staff.ID}, http.StatusUnprocessableEntity, "ERR_INVALID")

	must(e.st.SetUserStatus(ctx, sam.ID, "suspended"))
	if ok, _ := app.check("document:readme", "viewer", sam.ID); ok {
		t.Fatal("a suspended person must be denied")
	}

	chain := [][3]string{}
	for i := range 30 {
		chain = append(chain, [3]string{fmt.Sprintf("chain:c%d", i), "next", fmt.Sprintf("chain:c%d#next", i+1)})
	}
	app.write(append(chain, [3]string{"chain:c30", "next", "user:" + dee.ID})...)
	if ok, _ := app.check("chain:c10", "next", dee.ID); !ok {
		t.Fatal("a chain of 20 must resolve")
	}
	app.expect("POST", "/api/v1/authz/check", map[string]string{"object": "chain:c0", "relation": "next", "subject": "user:" + dee.ID}, http.StatusUnprocessableEntity, "ERR_AUTHZ_DEPTH")

	if got := list(t, app.expect("POST", "/api/v1/authz/list-objects", map[string]string{"type": "document", "relation": "viewer", "subject": "user:" + ada.ID}, http.StatusOK, ""), "objects"); !slices.Equal(got, []string{"document:plan", "document:readme"}) {
		t.Fatalf("ada's documents: %v", got)
	}
	if got := list(t, app.expect("POST", "/api/v1/authz/list-objects", map[string]string{"type": "document", "relation": "editor", "subject": "user:" + cy.ID}, http.StatusOK, ""), "objects"); len(got) != 4 {
		t.Fatalf("the editor role covers every document: %v", got)
	}
	want := []string{"user:" + ada.ID, "user:" + bob.ID, "user:" + cy.ID}
	slices.Sort(want)
	if got := list(t, app.expect("POST", "/api/v1/authz/list-subjects", map[string]string{"object": "document:plan", "relation": "viewer"}, http.StatusOK, ""), "subjects"); !slices.Equal(got, want) {
		t.Fatalf("plan viewers: %v, want %v", got, want)
	}
	expanded := app.expect("POST", "/api/v1/authz/expand", map[string]string{"object": "document:readme", "relation": "viewer"}, http.StatusOK, "").Body.String()
	for _, part := range []string{`"object":"folder:eng","relation":"viewer"`, `"groups":["` + staff.ID + `"]`, `"users":["` + bob.ID + `"]`, `"roles":["editor"]`} {
		if !strings.Contains(expanded, part) {
			t.Fatalf("expand lacks %s: %s", part, expanded)
		}
	}

	var read struct{ Tuples []map[string]any }
	_ = json.Unmarshal(app.expect("GET", "/api/v1/authz/tuples?object=document:readme", nil, http.StatusOK, "").Body.Bytes(), &read)
	if len(read.Tuples) != 2 || read.Tuples[0]["subject"] != "user:"+bob.ID || read.Tuples[1]["subject"] != "folder:eng" {
		t.Fatalf("readme tuples: %+v", read.Tuples)
	}

	e.expect(admin, "PUT", "/api/v1/applications/"+billing.ID+"/authorization", map[string]any{"types": map[string]any{"document": map[string]any{"viewer": map[string]any{"subjects": []string{"user"}}}}}, http.StatusOK, "")
	if ok, _ := other.check("document:readme", "viewer", bob.ID); ok {
		t.Fatal("billing must not see the wiki's tuples")
	}
	_ = json.Unmarshal(other.expect("GET", "/api/v1/authz/tuples", nil, http.StatusOK, "").Body.Bytes(), &read)
	if len(read.Tuples) != 0 {
		t.Fatalf("billing reads the wiki's tuples: %+v", read.Tuples)
	}
	other.expect("POST", "/api/v1/authz/tuples", map[string]any{"deletes": []map[string]string{{"object": "document:readme", "relation": "owner", "subject": "user:" + bob.ID}}}, http.StatusNoContent, "")
	if ok, _ := app.check("document:readme", "owner", bob.ID); !ok {
		t.Fatal("billing deleted the wiki's tuple")
	}

	rec := e.expect(auditor, "POST", model+"/check", map[string]string{"object": "document:readme", "relation": "viewer", "subject": "user:" + ada.ID}, http.StatusOK, "")
	if !strings.Contains(rec.Body.String(), `"allowed":true`) {
		t.Fatalf("console check: %s", rec.Body)
	}
	var relationships []store.PersonTuple
	_ = json.Unmarshal(e.expect(auditor, "GET", "/api/v1/users/"+ada.ID+"/relationships", nil, http.StatusOK, "").Body.Bytes(), &relationships)
	if len(relationships) != 1 || relationships[0].Object != "team:core" || relationships[0].GroupID != staff.ID || relationships[0].AppName != "Wiki" {
		t.Fatalf("ada's relationships: %+v", relationships)
	}

	must(e.st.SetApplicationGroups(ctx, wiki.ID, []string{readers.ID}))
	if ok, _ := app.check("document:readme", "viewer", ada.ID); ok {
		t.Fatal("an unassigned group must not grant access")
	}
	if got := list(t, app.expect("POST", "/api/v1/authz/list-subjects", map[string]string{"object": "document:plan", "relation": "viewer"}, http.StatusOK, ""), "subjects"); slices.Contains(got, "user:"+ada.ID) || !slices.Contains(got, "user:"+bob.ID) {
		t.Fatalf("plan viewers after unassigning staff: %v", got)
	}
	if expanded := app.expect("POST", "/api/v1/authz/expand", map[string]string{"object": "document:readme", "relation": "viewer"}, http.StatusOK, "").Body.String(); strings.Contains(expanded, staff.ID) {
		t.Fatalf("expand names an unassigned group: %s", expanded)
	}

	app.expect("POST", "/api/v1/authz/tuples", map[string]any{"deletes": []map[string]string{{"object": "document:readme", "relation": "owner", "subject": "user:" + bob.ID}}}, http.StatusNoContent, "")
	if ok, _ := app.check("document:readme", "viewer", bob.ID); ok {
		t.Fatal("bob keeps access after the tuple is deleted")
	}
	must(e.st.DeleteGroup(ctx, staff.ID))
	if ok, _ := app.check("document:readme", "viewer", ada.ID); ok {
		t.Fatal("deleting the group must remove its tuples")
	}
	_ = json.Unmarshal(app.expect("GET", "/api/v1/authz/tuples?object=team:core", nil, http.StatusOK, "").Body.Bytes(), &read)
	if len(read.Tuples) != 0 {
		t.Fatalf("group tuples left: %+v", read.Tuples)
	}

	e.expect(admin, "POST", "/api/v1/applications/"+wiki.ID+"/disable", nil, http.StatusOK, "")
	app.expect("POST", "/api/v1/authz/check", nil, http.StatusUnauthorized, "ERR_UNAUTHENTICATED")
	e.expect(admin, "POST", "/api/v1/applications/"+wiki.ID+"/enable", nil, http.StatusOK, "")

	if left := time.Until(time.Now().Truncate(time.Minute).Add(time.Minute)); left < 10*time.Second {
		time.Sleep(left + 100*time.Millisecond)
	}
	for range 6000 {
		_, _, err := e.st.CountRequest(ctx, wiki.ID, "authz", 6000, time.Minute)
		must(err)
	}
	rec = app.expect("POST", "/api/v1/authz/check", map[string]string{"object": "document:readme", "relation": "viewer", "subject": "user:" + ada.ID}, http.StatusTooManyRequests, "ERR_RATE_LIMITED")
	if rec.Header().Get("Retry-After") == "" || !strings.HasPrefix(rec.Header().Get("RateLimit"), `"authz";r=0;t=`) {
		t.Fatalf("limited: %v", rec.Header())
	}
	other.expect("POST", "/api/v1/authz/check", map[string]string{"object": "document:readme", "relation": "viewer", "subject": "user:" + ada.ID}, http.StatusOK, "")

	e.expect(admin, "DELETE", model, nil, http.StatusNoContent, "")
	if audit, err := e.st.ListAudit(ctx, 1); err != nil || audit[0].Action != "application.authorization.delete" || audit[0].Summary != "Removed the authorization model and 36 tuples" {
		t.Fatalf("delete audit: %+v %v", audit, err)
	}
	e.expect(admin, "DELETE", model, nil, http.StatusNotFound, "")
}
