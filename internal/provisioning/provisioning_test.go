package provisioning_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/server"
	"halo/internal/store"
	"halo/internal/testdb"
)

type fake struct {
	mu         sync.Mutex
	users      map[string]map[string]any
	groups     map[string]*remoteGroup
	groupCalls []string
	next       int
	reject     string
}

type remoteGroup struct {
	displayName, externalID string
	members                 []string
}

func (f *fake) group(displayName string) *remoteGroup {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, g := range f.groups {
		if g.displayName == displayName {
			copied := *g
			slices.Sort(copied.members)
			return &copied
		}
	}
	return nil
}

func (f *fake) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.groupCalls)
}

func (f *fake) find(userName string) (string, map[string]any) {
	for id, u := range f.users {
		if strings.EqualFold(u["userName"].(string), userName) {
			return id, u
		}
	}
	return "", nil
}

func (f *fake) user(userName string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, u := f.find(userName)
	return u
}

func (f *fake) handler(token string) http.Handler {
	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, status int, body any) {
		w.Header().Set("Content-Type", "application/scim+json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}
	read := func(r *http.Request) map[string]any {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		return body
	}
	mux.HandleFunc("GET /scim/v2/ServiceProviderConfig", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"}, "patch": map[string]bool{"supported": true}})
	})
	mux.HandleFunc("GET /scim/v2/Users", func(w http.ResponseWriter, r *http.Request) {
		userName := strings.TrimSuffix(strings.TrimPrefix(r.URL.Query().Get("filter"), `userName eq "`), `"`)
		resources := []map[string]any{}
		if id, _ := f.find(userName); id != "" {
			resources = append(resources, map[string]any{"id": id})
		}
		reply(w, 200, map[string]any{"totalResults": len(resources), "Resources": resources})
	})
	mux.HandleFunc("POST /scim/v2/Users", func(w http.ResponseWriter, r *http.Request) {
		body := read(r)
		userName, _ := body["userName"].(string)
		if userName == f.reject {
			reply(w, 400, map[string]any{"detail": "userName is reserved"})
			return
		}
		if id, _ := f.find(userName); id != "" {
			reply(w, 409, map[string]any{"detail": "userName taken"})
			return
		}
		f.next++
		id := fmt.Sprintf("remote-%d", f.next)
		f.users[id] = body
		reply(w, 201, map[string]any{"id": id})
	})
	mux.HandleFunc("PUT /scim/v2/Users/{id}", func(w http.ResponseWriter, r *http.Request) {
		if f.users[r.PathValue("id")] == nil {
			reply(w, 404, map[string]any{"detail": "no such user"})
			return
		}
		f.users[r.PathValue("id")] = read(r)
		reply(w, 200, map[string]any{"id": r.PathValue("id")})
	})
	mux.HandleFunc("PATCH /scim/v2/Users/{id}", func(w http.ResponseWriter, r *http.Request) {
		u := f.users[r.PathValue("id")]
		if u == nil {
			reply(w, 404, map[string]any{"detail": "no such user"})
			return
		}
		for _, op := range read(r)["Operations"].([]any) {
			if op := op.(map[string]any); op["path"] == "active" {
				u["active"] = op["value"]
			}
		}
		reply(w, 200, map[string]any{"id": r.PathValue("id")})
	})
	values := func(raw any) []string {
		var ids []string
		for _, m := range raw.([]any) {
			ids = append(ids, m.(map[string]any)["value"].(string))
		}
		return ids
	}
	findGroup := func(displayName string) string {
		for id, g := range f.groups {
			if g.displayName == displayName {
				return id
			}
		}
		return ""
	}
	mux.HandleFunc("GET /scim/v2/Groups", func(w http.ResponseWriter, r *http.Request) {
		displayName := strings.TrimSuffix(strings.TrimPrefix(r.URL.Query().Get("filter"), `displayName eq "`), `"`)
		resources := []map[string]any{}
		if id := findGroup(displayName); id != "" {
			resources = append(resources, map[string]any{"id": id})
		}
		reply(w, 200, map[string]any{"totalResults": len(resources), "Resources": resources})
	})
	mux.HandleFunc("POST /scim/v2/Groups", func(w http.ResponseWriter, r *http.Request) {
		body := read(r)
		if body["displayName"] == f.reject {
			reply(w, 400, map[string]any{"detail": "displayName is reserved"})
			return
		}
		if findGroup(body["displayName"].(string)) != "" {
			reply(w, 409, map[string]any{"detail": "displayName taken"})
			return
		}
		f.next++
		id := fmt.Sprintf("remote-group-%d", f.next)
		f.groups[id] = &remoteGroup{displayName: body["displayName"].(string), externalID: body["externalId"].(string), members: values(body["members"])}
		reply(w, 201, map[string]any{"id": id})
	})
	mux.HandleFunc("PUT /scim/v2/Groups/{id}", func(w http.ResponseWriter, r *http.Request) {
		if f.groups[r.PathValue("id")] == nil {
			reply(w, 404, map[string]any{"detail": "no such group"})
			return
		}
		body := read(r)
		f.groups[r.PathValue("id")] = &remoteGroup{displayName: body["displayName"].(string), externalID: body["externalId"].(string), members: values(body["members"])}
		reply(w, 200, map[string]any{"id": r.PathValue("id")})
	})
	mux.HandleFunc("PATCH /scim/v2/Groups/{id}", func(w http.ResponseWriter, r *http.Request) {
		g := f.groups[r.PathValue("id")]
		if g == nil {
			reply(w, 404, map[string]any{"detail": "no such group"})
			return
		}
		for _, op := range read(r)["Operations"].([]any) {
			op := op.(map[string]any)
			path := op["path"].(string)
			switch {
			case op["op"] == "replace" && path == "displayName":
				g.displayName = op["value"].(string)
			case op["op"] == "add" && path == "members":
				g.members = append(g.members, values(op["value"])...)
			case op["op"] == "remove" && strings.HasPrefix(path, `members[value eq "`):
				g.members = slices.DeleteFunc(g.members, func(m string) bool { return path == `members[value eq "`+m+`"]` })
			default:
				reply(w, 400, map[string]any{"detail": "unsupported operation"})
				return
			}
		}
		reply(w, 200, map[string]any{"id": r.PathValue("id")})
	})
	mux.HandleFunc("DELETE /scim/v2/Groups/{id}", func(w http.ResponseWriter, r *http.Request) {
		if f.groups[r.PathValue("id")] == nil {
			reply(w, 404, map[string]any{"detail": "no such group"})
			return
		}
		delete(f.groups, r.PathValue("id"))
		w.WriteHeader(204)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if strings.HasPrefix(r.URL.Path, "/scim/v2/Groups") {
			f.groupCalls = append(f.groupCalls, r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			reply(w, 401, map[string]any{"detail": "invalid token"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func call(t *testing.T, h http.Handler, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: token})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func expect(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("want %d, got %d: %s", status, rec.Code, rec.Body)
	}
}

func TestOutboundProvisioning(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	public, _ := url.Parse("http://localhost:3200")
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	app, err := server.New(config.Config{Organization: "Test", PublicURL: public, SecretKey: secret, Dev: true}, st)
	if err != nil {
		t.Fatal(err)
	}
	h := app.Handler

	remote := &fake{users: map[string]map[string]any{"remote-eve": {"userName": "eve@example.com", "active": true}}}
	srv := httptest.NewServer(remote.handler("secret-token"))
	defer srv.Close()

	group, err := st.CreateGroup(ctx, store.NewGroup{Name: "Slack users", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	slack, err := st.CreateApplication(ctx, store.NewApplication{Name: "Slack", Protocol: "saml", Type: "web", GroupIDs: []string{group.ID}})
	if err != nil {
		t.Fatal(err)
	}
	billing, err := st.CreateApplication(ctx, store.NewApplication{Name: "Billing API", Protocol: "oauth", Type: "service"})
	if err != nil {
		t.Fatal(err)
	}
	people := map[string]store.User{}
	for _, p := range []struct {
		name, status string
		assigned     bool
	}{{"ada", "active", true}, {"bob", "active", true}, {"cy", "invited", true}, {"dee", "active", false}, {"bad", "active", true}, {"eve", "active", true}} {
		in := store.NewUser{Email: p.name + "@example.com", Name: strings.ToUpper(p.name[:1]) + p.name[1:] + " Example", Title: "Engineer", Department: "Engineering", Status: p.status}
		if p.assigned {
			in.GroupIDs = []string{group.ID}
		}
		if people[p.name], err = st.CreateUser(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	remote.reject = "bad@example.com"
	session := func(email, role string) string {
		u, err := st.CreateUser(ctx, store.NewUser{Email: email, Name: email, Status: "active", Roles: []string{role}})
		if err != nil {
			t.Fatal(err)
		}
		_, token, err := st.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: "passkey"})
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	admin, auditor := session("abe@example.com", "app_admin"), session("audrey@example.com", "auditor")

	base := "/api/v1/provisioning/applications/" + slack.ID
	endpoint := srv.URL + "/scim/v2"
	expect(t, call(t, h, auditor, "PUT", base, map[string]any{"baseUrl": endpoint, "token": "secret-token", "enabled": true}), 403)
	expect(t, call(t, h, admin, "PUT", base, map[string]any{"baseUrl": "ftp://scim.example.com", "token": "x", "enabled": true}), 422)
	expect(t, call(t, h, admin, "PUT", base, map[string]any{"baseUrl": "http://scim.example.com/scim/v2", "token": "x", "enabled": true}), 422)
	expect(t, call(t, h, admin, "PUT", base, map[string]any{"baseUrl": endpoint, "enabled": true}), 422)
	expect(t, call(t, h, admin, "PUT", "/api/v1/provisioning/applications/"+billing.ID, map[string]any{"baseUrl": endpoint, "token": "x", "enabled": true}), 422)
	expect(t, call(t, h, admin, "POST", base+"/sync", nil), 404)
	expect(t, call(t, h, admin, "PUT", base, map[string]any{"baseUrl": endpoint + "/", "token": "wrong-token", "enabled": true}), 200)
	rec := call(t, h, admin, "POST", base+"/test", nil)
	expect(t, rec, 502)
	if !strings.Contains(rec.Body.String(), "401") || !strings.Contains(rec.Body.String(), "bearer token") {
		t.Fatalf("test failure should explain the 401: %s", rec.Body)
	}
	expect(t, call(t, h, admin, "PUT", base, map[string]any{"baseUrl": endpoint, "token": "secret-token", "enabled": true}), 200)
	expect(t, call(t, h, admin, "PUT", base, map[string]any{"baseUrl": endpoint, "enabled": true}), 200)
	expect(t, call(t, h, admin, "POST", base+"/test", nil), 200)

	sync := func() store.AppProvisioning {
		t.Helper()
		rec := call(t, h, admin, "POST", base+"/sync", nil)
		expect(t, rec, 200)
		var c store.AppProvisioning
		_ = json.Unmarshal(rec.Body.Bytes(), &c)
		return c
	}
	c := sync()
	if c.Created != 2 || c.Updated != 1 || c.Deactivated != 0 || c.Provisioned != 3 || c.LastSyncAt == nil || !strings.Contains(c.LastError, "bad@example.com") || !strings.Contains(c.LastError, "userName is reserved") {
		t.Fatalf("first sync: %+v", c)
	}
	ada := remote.user("ada@example.com")
	if ada == nil || ada["active"] != true || ada["externalId"] != people["ada"].ID || ada["title"] != "Engineer" || ada["displayName"] != "Ada Example" {
		t.Fatalf("ada was not provisioned: %v", ada)
	}
	if remote.user("cy@example.com") != nil || remote.user("dee@example.com") != nil {
		t.Fatal("invited and unassigned people must not be pushed")
	}
	if eve := remote.user("eve@example.com"); eve["externalId"] != people["eve"].ID {
		t.Fatalf("existing remote user was not adopted: %v", eve)
	}

	if c = sync(); c.Created != 0 || c.Updated != 0 || c.Deactivated != 0 {
		t.Fatalf("unchanged people were pushed again: %+v", c)
	}

	current, err := st.GetSCIMUser(ctx, people["ada"].ID)
	if err != nil {
		t.Fatal(err)
	}
	current.Title = "Staff engineer"
	if err := st.UpdateSCIMUser(ctx, current); err != nil {
		t.Fatal(err)
	}
	if c = sync(); c.Updated != 1 || remote.user("ada@example.com")["title"] != "Staff engineer" {
		t.Fatalf("attribute change: %+v %v", c, remote.user("ada@example.com"))
	}

	if err := st.RemoveGroupMember(ctx, group.ID, people["bob"].ID); err != nil {
		t.Fatal(err)
	}
	if err := st.SetUserStatus(ctx, people["eve"].ID, "suspended"); err != nil {
		t.Fatal(err)
	}
	if c = sync(); c.Deactivated != 2 || c.Provisioned != 1 || remote.user("bob@example.com")["active"] != false || remote.user("eve@example.com")["active"] != false {
		t.Fatalf("deactivation: %+v", c)
	}

	if err := st.AddGroupMember(ctx, group.ID, people["dee"].ID); err != nil {
		t.Fatal(err)
	}
	jobs, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		app.Jobs.Run(jobs)
		close(done)
	}()
	for deadline := time.Now().Add(10 * time.Second); remote.user("dee@example.com") == nil; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the background job did not push the newly assigned person")
		}
	}
	stop()
	<-done

	rec = call(t, h, auditor, "GET", "/api/v1/provisioning/applications", nil)
	expect(t, rec, 200)
	if strings.Contains(rec.Body.String(), "secret-token") || strings.Contains(strings.ToLower(rec.Body.String()), "token") || !strings.Contains(rec.Body.String(), slack.ID) {
		t.Fatalf("listing leaks the token or misses the app: %s", rec.Body)
	}

	srv.Close()
	if c = sync(); c.Created != 0 || !strings.Contains(c.LastError, "could not connect") {
		t.Fatalf("unreachable endpoint: %+v", c)
	}
	audit, err := st.ListAudit(ctx, 1)
	if err != nil || audit[0].Action != "provisioning.sync" || audit[0].TargetID != slack.ID {
		t.Fatalf("audit: %+v %v", audit, err)
	}
}

func TestOutboundGroupProvisioning(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	public, _ := url.Parse("http://localhost:3200")
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	app, err := server.New(config.Config{Organization: "Test", PublicURL: public, SecretKey: secret, Dev: true}, st)
	if err != nil {
		t.Fatal(err)
	}
	h := app.Handler
	remote := &fake{users: map[string]map[string]any{}, groups: map[string]*remoteGroup{}}
	srv := httptest.NewServer(remote.handler("secret-token"))
	defer srv.Close()

	newGroup := func(name string, rule *string) store.Group {
		kind := "assigned"
		if rule != nil {
			kind = "dynamic"
		}
		g, err := st.CreateGroup(ctx, store.NewGroup{Name: name, Kind: kind, Rule: rule})
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	design := `user.department == "Design"`
	staff, sre, designers, platform := newGroup("Staff", nil), newGroup("SRE", nil), newGroup("Designers", &design), newGroup("Platform", nil)
	grafana, err := st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", GroupIDs: []string{staff.ID}})
	if err != nil {
		t.Fatal(err)
	}
	people := map[string]store.User{}
	for _, p := range []struct {
		name, department string
		groups           []string
	}{{"ada", "Engineering", []string{staff.ID, sre.ID}}, {"bob", "Engineering", []string{staff.ID}}, {"cy", "Engineering", []string{sre.ID}}, {"dee", "Design", []string{staff.ID}}, {"eli", "Design", nil}} {
		if people[p.name], err = st.CreateUser(ctx, store.NewUser{Email: p.name + "@example.com", Name: p.name, Department: p.department, Status: "active", GroupIDs: p.groups}); err != nil {
			t.Fatal(err)
		}
	}
	u, err := st.CreateUser(ctx, store.NewUser{Email: "abe@example.com", Name: "Abe", Status: "active", Roles: []string{"app_admin"}})
	if err != nil {
		t.Fatal(err)
	}
	_, admin, err := st.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: "passkey"})
	if err != nil {
		t.Fatal(err)
	}

	base := "/api/v1/provisioning/applications/" + grafana.ID
	configure := func(body map[string]any) store.AppProvisioning {
		t.Helper()
		body["baseUrl"], body["enabled"] = srv.URL+"/scim/v2", true
		rec := call(t, h, admin, "PUT", base, body)
		expect(t, rec, 200)
		var c store.AppProvisioning
		_ = json.Unmarshal(rec.Body.Bytes(), &c)
		return c
	}
	sync := func() store.AppProvisioning {
		t.Helper()
		rec := call(t, h, admin, "POST", base+"/sync", nil)
		expect(t, rec, 200)
		var c store.AppProvisioning
		_ = json.Unmarshal(rec.Body.Bytes(), &c)
		return c
	}
	if c := configure(map[string]any{"token": "secret-token"}); c.GroupIDs == nil || len(c.GroupIDs) != 0 {
		t.Fatalf("a new configuration pushes no groups: %+v", c)
	}
	if c := sync(); c.Created != 3 || len(remote.calls()) != 0 {
		t.Fatalf("groups were pushed without being selected: %+v %v", c, remote.calls())
	}
	expect(t, call(t, h, admin, "PUT", base, map[string]any{"baseUrl": srv.URL + "/scim/v2", "enabled": true, "groupIds": []string{sre.ID, "grp_missing"}}), 422)
	if c := configure(map[string]any{"groupIds": []string{sre.ID, designers.ID, sre.ID}}); !slices.Equal(c.GroupIDs, []string{designers.ID, sre.ID}) {
		t.Fatalf("selection: %+v", c.GroupIDs)
	}
	if c := configure(map[string]any{}); !slices.Equal(c.GroupIDs, []string{designers.ID, sre.ID}) {
		t.Fatalf("saving without groupIds must keep the selection: %+v", c.GroupIDs)
	}

	provisioned, err := st.ListProvisionedUsers(ctx, grafana.ID)
	if err != nil {
		t.Fatal(err)
	}
	member := func(name string) string { return provisioned[people[name].ID].RemoteID }

	c := sync()
	if c.GroupsCreated != 2 || c.GroupsUpdated != 0 || c.GroupsDeleted != 0 || c.LastError != "" {
		t.Fatalf("first group push: %+v", c)
	}
	if g := remote.group("SRE"); g == nil || g.externalID != sre.ID || !slices.Equal(g.members, []string{member("ada")}) {
		t.Fatalf("SRE must hold only the provisioned member: %+v", g)
	}
	if g := remote.group("Designers"); g == nil || !slices.Equal(g.members, []string{member("dee")}) {
		t.Fatalf("the rule-based group must hold only the provisioned member: %+v", g)
	}

	before := len(remote.calls())
	if c = sync(); c.GroupsCreated != 0 || c.GroupsUpdated != 0 || len(remote.calls()) != before {
		t.Fatalf("unchanged groups were pushed again: %+v %v", c, remote.calls()[before:])
	}

	sre.Name = "Site reliability"
	if err := st.UpdateGroup(ctx, sre); err != nil {
		t.Fatal(err)
	}
	if err := st.AddGroupMember(ctx, sre.ID, people["bob"].ID); err != nil {
		t.Fatal(err)
	}
	if err := st.RemoveGroupMember(ctx, sre.ID, people["ada"].ID); err != nil {
		t.Fatal(err)
	}
	if c = sync(); c.GroupsUpdated != 1 || !slices.Equal(remote.calls()[before:], []string{"PATCH"}) {
		t.Fatalf("rename and membership change must be one PATCH: %+v %v", c, remote.calls()[before:])
	}
	if g := remote.group("Site reliability"); g == nil || !slices.Equal(g.members, []string{member("bob")}) || remote.group("SRE") != nil {
		t.Fatalf("rename and membership: %+v", g)
	}

	remote.reject = "Platform"
	configure(map[string]any{"groupIds": []string{sre.ID, designers.ID, platform.ID}})
	if c = sync(); c.GroupsCreated != 0 || !strings.Contains(c.LastError, "Platform") || !strings.Contains(c.LastError, "displayName is reserved") {
		t.Fatalf("a rejected group must be reported: %+v", c)
	}
	remote.reject = ""
	if c = sync(); c.GroupsCreated != 1 || c.LastError != "" || remote.group("Platform") == nil {
		t.Fatalf("the rejected group must be retried at the next sync: %+v", c)
	}

	configure(map[string]any{"groupIds": []string{sre.ID, platform.ID}})
	if c = sync(); c.GroupsDeleted != 1 || remote.group("Designers") != nil {
		t.Fatalf("a deselected group must be deleted: %+v", c)
	}
	if err := st.DeleteGroup(ctx, platform.ID); err != nil {
		t.Fatal(err)
	}
	if c = sync(); c.GroupsDeleted != 1 || remote.group("Platform") != nil || !slices.Equal(c.GroupIDs, []string{sre.ID}) {
		t.Fatalf("a group deleted in Halo must be deleted: %+v", c)
	}
	audit, err := st.ListAudit(ctx, 1)
	if err != nil || audit[0].Action != "provisioning.sync" || !strings.Contains(audit[0].Summary, "groups: 0 created, 0 updated, 1 deleted") {
		t.Fatalf("audit: %+v %v", audit, err)
	}
}
