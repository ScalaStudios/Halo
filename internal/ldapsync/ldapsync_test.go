package ldapsync

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"halo/internal/api"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/store"
	"halo/internal/testdb"
)

func setup(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	st := testdb.New(t)
	public, _ := url.Parse("http://localhost:3200")
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	mux := http.NewServeMux()
	deps := httpx.Deps{Config: config.Config{PublicURL: public, Dev: true}, Store: st, Auth: authn}
	if err := Register(mux, deps); err != nil {
		t.Fatal(err)
	}
	if err := api.Register(mux, deps); err != nil {
		t.Fatal(err)
	}
	return st, authn.Resolve(authn.SameOrigin(mux))
}

func person(t *testing.T, st *store.Store, email string, roles ...string) (store.User, string) {
	t.Helper()
	u, err := st.CreateUser(context.Background(), store.NewUser{Email: email, Name: strings.Split(email, "@")[0], Status: "active", Roles: roles})
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := st.CreateSession(context.Background(), store.NewSession{UserID: u.ID, Method: "passkey"})
	if err != nil {
		t.Fatal(err)
	}
	return u, token
}

func call(t *testing.T, h http.Handler, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: token})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder, status int) T {
	t.Helper()
	var v T
	if rec.Code != status || json.Unmarshal(rec.Body.Bytes(), &v) != nil {
		t.Fatalf("want %d, got %d: %s", status, rec.Code, rec.Body)
	}
	return v
}

func directoryInput(f *fakeLDAP, security string) map[string]any {
	return map[string]any{"name": "Corp", "url": f.url, "security": security, "caCertificate": f.ca, "bindDn": f.bindDN, "password": f.password,
		"userBaseDn": "ou=people,dc=example,dc=com", "groupBaseDn": "ou=groups,dc=example,dc=com",
		"mapping": map[string]any{"id": "objectGUID", "email": "mail", "name": "displayName", "title": "title", "department": "department", "manager": "manager", "attributes": map[string]string{"employeeNumber": "employeeNumber"}}}
}

func personEntry(uid, guid, email, name, title string, uac string, extra ...string) fakeEntry {
	attrs := map[string][]string{"objectGUID": {guid}, "mail": {email}, "displayName": {name}, "title": {title}, "department": {"Engineering"}, "userAccountControl": {uac}, "employeeNumber": {"E-" + uid}}
	for i := 0; i+1 < len(extra); i += 2 {
		attrs[extra[i]] = []string{extra[i+1]}
	}
	return fakeEntry{dn: "uid=" + uid + ",ou=people,dc=example,dc=com", attrs: attrs}
}

func groupEntry(cn, guid string, members ...string) fakeEntry {
	dns := make([]string, len(members))
	for i, m := range members {
		dns[i] = "UID=" + m + ", OU=People, DC=example, DC=com"
	}
	return fakeEntry{dn: "cn=" + cn + ",ou=groups,dc=example,dc=com", attrs: map[string][]string{"objectGUID": {guid}, "cn": {cn}, "member": dns}}
}

func syncNow(t *testing.T, h http.Handler, token, directoryID string, dryRun bool) report {
	t.Helper()
	path := "/api/v1/ldap-directories/" + directoryID + "/sync"
	if dryRun {
		path += "?dryRun=true"
	}
	return decode[report](t, call(t, h, token, "POST", path, nil), 200)
}

func TestDirectorySync(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "uma@example.com", "user_admin")
	_, auditor := person(t, st, "aud@example.com", "auditor")
	local, _ := person(t, st, "lin@example.com")
	if _, err := st.CreateGroup(ctx, store.NewGroup{Name: "Admins", Kind: "assigned"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAttribute(ctx, store.Attribute{Name: "employeeNumber", Label: "Employee number", Type: "string"}); err != nil {
		t.Fatal(err)
	}
	f := serveLDAP(t, "ldaps")
	pageSize = 2
	t.Cleanup(func() { pageSize = 500 })
	binary := "\x00\x9a\xfe\x10"
	f.set(
		personEntry("ada", "guid-ada", "ada@example.com", "Ada Lovelace", "Engineer", "512", "manager", "uid=grace,ou=people,dc=example,dc=com"),
		personEntry("grace", "guid-grace", "grace@example.com", "Grace Hopper", "Director", "512"),
		personEntry("alan", binary, "alan@example.com", "Alan Turing", "Researcher", "512"),
		personEntry("bob", "guid-bob", "bob@example.com", "Bob", "Intern", "514"),
		personEntry("lin", "guid-lin", "lin@example.com", "Lin", "Local", "512"),
		groupEntry("Engineering", "guid-eng", "ada", "grace"),
		groupEntry("admins", "guid-admins", "ada"),
	)

	valid := directoryInput(f, "ldaps")
	if rec := call(t, h, auditor, "POST", "/api/v1/ldap-directories", valid); rec.Code != 403 {
		t.Fatalf("auditor create: %d", rec.Code)
	}
	for _, change := range []map[string]any{
		{"security": "starttls"},
		{"url": "ldaps://dc1.example.com/dc=example"},
		{"security": "plain"},
		{"password": ""},
		{"userFilter": "(objectClass=person"},
		{"caCertificate": "not a certificate"},
		{"intervalMinutes": 5},
		{"mapping": map[string]any{"id": "objectGUID", "email": "mail"}},
		{"mapping": map[string]any{"id": "objectGUID", "email": "mail", "name": "cn", "attributes": map[string]string{"badge": "badgeNumber"}}},
	} {
		bad := map[string]any{}
		for k, v := range valid {
			bad[k] = v
		}
		for k, v := range change {
			bad[k] = v
		}
		if rec := call(t, h, admin, "POST", "/api/v1/ldap-directories", bad); rec.Code != 422 {
			t.Fatalf("%v: want 422, got %d: %s", change, rec.Code, rec.Body)
		}
	}
	d := decode[store.LDAPDirectory](t, call(t, h, admin, "POST", "/api/v1/ldap-directories", valid), 201)
	if d.Enabled || d.Security != "ldaps" || d.UserFilter == "" || d.Mapping.GroupName != "cn" || d.Mapping.Member != "member" {
		t.Fatalf("created: %+v", d)
	}
	if strings.Contains(call(t, h, auditor, "GET", "/api/v1/ldap-directories", nil).Body.String(), f.password) {
		t.Fatal("the bind password must never be returned")
	}

	preview := syncNow(t, h, admin, d.ID, true)
	if !preview.DryRun || preview.Result != "succeeded" || preview.Counts.Created != 3 || preview.Counts.GroupsCreated != 1 || preview.Counts.MembersAdded != 2 {
		t.Fatalf("dry run: %+v", preview)
	}
	if !slices.ContainsFunc(preview.Skipped, func(s string) bool { return strings.Contains(s, "lin@example.com is already in Halo") }) ||
		!slices.ContainsFunc(preview.Skipped, func(s string) bool { return strings.Contains(s, "A group named admins is already in Halo") }) {
		t.Fatalf("dry run skipped: %v", preview.Skipped)
	}
	if _, err := st.GetUserByEmail(ctx, "ada@example.com"); err == nil {
		t.Fatal("a dry run must not create anyone")
	}
	if history := decode[[]store.LDAPSync](t, call(t, h, auditor, "GET", "/api/v1/ldap-directories/"+d.ID+"/syncs", nil), 200); len(history) != 0 {
		t.Fatalf("a dry run must not be recorded: %+v", history)
	}

	first := syncNow(t, h, admin, d.ID, false)
	if first.Result != "succeeded" || first.Counts != preview.Counts || first.ID == "" {
		t.Fatalf("first sync: %+v", first)
	}
	_, pages := f.stats()
	if len(pages) < 3 || pages[0] != 2 {
		t.Fatalf("the people search must be paged 2 at a time: %v", pages)
	}
	ada, err := st.GetUserByEmail(ctx, "ada@example.com")
	if err != nil {
		t.Fatal(err)
	}
	grace, err := st.GetUserByEmail(ctx, "grace@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if ada.Name != "Ada Lovelace" || ada.Title != "Engineer" || ada.Department != "Engineering" || ada.Status != "invited" || ada.Source != "LDAP · Corp" ||
		ada.ManagerID == nil || *ada.ManagerID != grace.ID || ada.Attributes["employeeNumber"] != "E-ada" || ada.Directory == nil || ada.Directory.ID != d.ID {
		t.Fatalf("ada: %+v", ada)
	}
	if _, err := st.GetUserByEmail(ctx, "alan@example.com"); err != nil {
		t.Fatalf("an entry with a binary objectGUID must sync: %v", err)
	}
	if _, err := st.GetUserByEmail(ctx, "bob@example.com"); err == nil {
		t.Fatal("a disabled entry must not be created")
	}
	groups, err := st.ListGroupsRaw(ctx)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(groups, func(g store.Group) bool { return g.Name == "Engineering" })
	if i < 0 || groups[i].Kind != "assigned" || groups[i].Source != "LDAP · Corp" || !slices.Contains(ada.GroupIDs, groups[i].ID) || !slices.Contains(grace.GroupIDs, groups[i].ID) {
		t.Fatalf("engineering: %+v %v %v", groups, ada.GroupIDs, grace.GroupIDs)
	}
	engineering := groups[i]
	if err := st.AddGroupMember(ctx, engineering.ID, local.ID); err != nil {
		t.Fatal(err)
	}
	if rec := call(t, h, admin, "PATCH", "/api/v1/users/"+ada.ID, map[string]any{"title": "Changed in Halo"}); rec.Code != 409 || !strings.Contains(rec.Body.String(), "synced from the LDAP directory Corp") {
		t.Fatalf("editing a synced person: %d %s", rec.Code, rec.Body)
	}

	again := syncNow(t, h, admin, d.ID, false)
	if again.Counts != (store.LDAPCounts{}) || len(again.Changes) != 0 {
		t.Fatalf("an unchanged directory must change nothing: %+v", again)
	}

	_, graceToken, err := st.CreateSession(ctx, store.NewSession{UserID: grace.ID, Method: "passkey"})
	if err != nil {
		t.Fatal(err)
	}
	f.set(
		personEntry("ada", "guid-ada", "ada@example.com", "Ada King", "Principal engineer", "512", "manager", "uid=grace,ou=people,dc=example,dc=com"),
		personEntry("grace", "guid-grace", "grace@example.com", "Grace Hopper", "Director", "514"),
		personEntry("alan", binary, "alan@example.com", "Alan Turing", "Researcher", "512"),
		groupEntry("Engineering", "guid-eng", "ada", "alan"),
	)
	changed := syncNow(t, h, admin, d.ID, false)
	if changed.Counts.Updated != 1 || changed.Counts.Suspended != 1 || changed.Counts.MembersAdded != 1 || changed.Counts.MembersRemoved != 1 {
		t.Fatalf("changed: %+v", changed)
	}
	ada, _ = st.GetUserByEmail(ctx, "ada@example.com")
	grace, _ = st.GetUserByEmail(ctx, "grace@example.com")
	alan, _ := st.GetUserByEmail(ctx, "alan@example.com")
	if ada.Name != "Ada King" || ada.Title != "Principal engineer" || grace.Status != "suspended" || slices.Contains(grace.GroupIDs, engineering.ID) || !slices.Contains(alan.GroupIDs, engineering.ID) {
		t.Fatalf("after changes: ada %+v grace %+v alan %v", ada, grace, alan.GroupIDs)
	}
	if lin, _ := st.GetUser(ctx, local.ID); !slices.Contains(lin.GroupIDs, engineering.ID) {
		t.Fatal("members added in Halo who are not synced from the directory must stay")
	}
	if rec := call(t, h, graceToken, "GET", "/api/v1/ldap-directories", nil); rec.Code != 401 {
		t.Fatalf("a suspended person's sessions must end: %d", rec.Code)
	}

	f.set(
		personEntry("grace", "guid-grace", "grace@example.com", "Grace Hopper", "Director", "512"),
		personEntry("alan", binary, "alan@example.com", "Alan Turing", "Researcher", "512"),
		groupEntry("Engineering", "guid-eng", "alan"),
	)
	back := syncNow(t, h, admin, d.ID, false)
	ada, _ = st.GetUserByEmail(ctx, "ada@example.com")
	grace, _ = st.GetUserByEmail(ctx, "grace@example.com")
	if back.Counts.Restored != 1 || back.Counts.Suspended != 1 || grace.Status != "invited" || ada.Status != "suspended" {
		t.Fatalf("restore and removal: %+v grace %s ada %s", back.Counts, grace.Status, ada.Status)
	}

	if _, err := st.CreateUser(ctx, store.NewUser{Email: "boss@example.com", Name: "Boss", Status: "active", Roles: []string{"global_admin"}}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetUserRoles(ctx, alan.ID, []string{"global_admin"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.SetUserStatus(ctx, alan.ID, "active"); err != nil {
		t.Fatal(err)
	}
	boss, _ := st.GetUserByEmail(ctx, "boss@example.com")
	if err := st.SetUserStatus(ctx, boss.ID, "suspended"); err != nil {
		t.Fatal(err)
	}
	settings, err := st.CurrentSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.EmergencyAccessIDs = []string{grace.ID}
	if err := st.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	f.set(personEntry("alan", binary, "alan.t@example.com", "Alan Turing", "Researcher", "512"), groupEntry("Engineering", "guid-eng", "alan"))
	admins := syncNow(t, h, admin, d.ID, false)
	if alan, _ = st.GetUser(ctx, alan.ID); alan.Email != "alan@example.com" || !slices.ContainsFunc(admins.Skipped, func(s string) bool { return strings.Contains(s, "eligible for an administrator role") }) {
		t.Fatalf("the directory must not change an administrator's email address: %s %v", alan.Email, admins.Skipped)
	}
	f.set(personEntry("nobody", "guid-nobody", "nobody@example.com", "Nobody", "Temp", "514"), groupEntry("Engineering", "guid-eng"))
	guarded := syncNow(t, h, admin, d.ID, false)
	alan, _ = st.GetUser(ctx, alan.ID)
	grace, _ = st.GetUser(ctx, grace.ID)
	if guarded.Counts.Suspended != 0 || alan.Status != "active" || grace.Status != "invited" ||
		!slices.ContainsFunc(guarded.Skipped, func(s string) bool { return strings.Contains(s, "last active global administrator") }) ||
		!slices.ContainsFunc(guarded.Skipped, func(s string) bool { return strings.Contains(s, "emergency access account") }) {
		t.Fatalf("guards: %+v alan %s grace %s", guarded, alan.Status, grace.Status)
	}

	for _, entries := range [][]fakeEntry{{personEntry("nobody", "guid-nobody", "nobody@example.com", "Nobody", "Temp", "514")}, nil} {
		f.set(entries...)
		empty := syncNow(t, h, admin, d.ID, false)
		if empty.Result != "failed" || !strings.Contains(empty.Error, "returned nothing") {
			t.Fatalf("an empty search must fail without changing anything: %+v", empty)
		}
	}
	history := decode[[]store.LDAPSync](t, call(t, h, auditor, "GET", "/api/v1/ldap-directories/"+d.ID+"/syncs", nil), 200)
	if len(history) != 8 || history[0].Result != "failed" || history[len(history)-1].Counts.Created != 3 {
		t.Fatalf("history: %+v", history)
	}
	events, err := st.ListAudit(ctx, 200)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"ldap_directory.create", "ldap.user.create", "ldap.user.update", "ldap.user.suspend", "ldap.user.restore", "ldap.group.create", "ldap.group.update"} {
		if !slices.ContainsFunc(events, func(e store.AuditEvent) bool { return e.Action == action }) {
			t.Errorf("no %s audit event", action)
		}
	}

	if rec := call(t, h, admin, "DELETE", "/api/v1/ldap-directories/"+d.ID, nil); rec.Code != 204 {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec := call(t, h, admin, "PATCH", "/api/v1/users/"+ada.ID, map[string]any{"title": "Editable again"}); rec.Code != 200 {
		t.Fatalf("people stay and become editable after the directory is deleted: %d %s", rec.Code, rec.Body)
	}
}

func TestDirectoryConnection(t *testing.T) {
	st, h := setup(t)
	_, admin := person(t, st, "uma@example.com", "user_admin")
	if _, err := st.CreateAttribute(context.Background(), store.Attribute{Name: "employeeNumber", Label: "Employee number", Type: "string"}); err != nil {
		t.Fatal(err)
	}
	entries := []fakeEntry{personEntry("ada", "guid-ada", "ada@example.com", "Ada", "Engineer", "512")}

	plain := serveLDAP(t, "plain")
	plain.set(entries...)
	in := directoryInput(plain, "starttls")
	in["name"] = "No StartTLS"
	d := decode[store.LDAPDirectory](t, call(t, h, admin, "POST", "/api/v1/ldap-directories", in), 201)
	if rep := syncNow(t, h, admin, d.ID, false); rep.Result != "failed" || !strings.Contains(rep.Error, "StartTLS") {
		t.Fatalf("StartTLS refused: %+v", rep)
	}
	if binds, _ := plain.stats(); binds != 0 {
		t.Fatal("the bind password must not be sent without TLS")
	}
	if got, _ := st.GetLDAPDirectory(context.Background(), d.ID); got.SyncError == "" || got.SyncedAt == nil {
		t.Fatalf("a failed sync must be recorded: %+v", got)
	}

	tlsUpgrade := serveLDAP(t, "starttls")
	tlsUpgrade.set(entries...)
	in = directoryInput(tlsUpgrade, "starttls")
	in["name"] = "StartTLS"
	d = decode[store.LDAPDirectory](t, call(t, h, admin, "POST", "/api/v1/ldap-directories", in), 201)
	if rep := syncNow(t, h, admin, d.ID, true); rep.Result != "succeeded" || rep.Counts.Created != 1 {
		t.Fatalf("StartTLS: %+v", rep)
	}

	ldaps := serveLDAP(t, "ldaps")
	ldaps.set(entries...)
	in = directoryInput(ldaps, "ldaps")
	in["name"], in["caCertificate"] = "Untrusted", ""
	d = decode[store.LDAPDirectory](t, call(t, h, admin, "POST", "/api/v1/ldap-directories", in), 201)
	if rep := syncNow(t, h, admin, d.ID, false); rep.Result != "failed" || !strings.Contains(rep.Error, "certificate") {
		t.Fatalf("untrusted certificate: %+v", rep)
	}

	in = directoryInput(ldaps, "ldaps")
	in["name"], in["password"] = "Wrong password", "wrong"
	d = decode[store.LDAPDirectory](t, call(t, h, admin, "POST", "/api/v1/ldap-directories", in), 201)
	if rep := syncNow(t, h, admin, d.ID, false); rep.Result != "failed" || !strings.Contains(rep.Error, "refused the bind DN and password") {
		t.Fatalf("bad bind: %+v", rep)
	}
	if _, err := st.GetUserByEmail(context.Background(), "ada@example.com"); err == nil {
		t.Fatal("a refused bind must change nothing")
	}

	in["password"], in["enabled"], in["intervalMinutes"] = "s3cret", true, 15
	decode[store.LDAPDirectory](t, call(t, h, admin, "PUT", "/api/v1/ldap-directories/"+d.ID, in), 200)
	hs := &handlers{st: st}
	if err := hs.syncDue(context.Background(), time.Now().Add(16*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetUserByEmail(context.Background(), "ada@example.com"); err != nil {
		t.Fatalf("the scheduled sync must run for enabled directories: %v", err)
	}
	others, _ := st.ListLDAPSyncs(context.Background(), d.ID, 10)
	if err := hs.syncDue(context.Background(), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if after, _ := st.ListLDAPSyncs(context.Background(), d.ID, 10); len(after) != len(others) {
		t.Fatal("a directory must not sync again before its interval")
	}
}

func TestDirectoryRetriesRestoreAfterIdentityCheck(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "uma@example.com", "user_admin")
	if _, err := st.CreateAttribute(ctx, store.Attribute{Name: "employeeNumber", Label: "Employee number", Type: "string"}); err != nil {
		t.Fatal(err)
	}
	settings, err := st.CurrentSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.IdentityProofingInvites = true
	if err := st.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	f := serveLDAP(t, "ldaps")
	dir := decode[store.LDAPDirectory](t, call(t, h, admin, "POST", "/api/v1/ldap-directories", directoryInput(f, "ldaps")), 201)
	people := func(state string) {
		f.set(personEntry("ada", "guid-ada", "ada@example.com", "Ada Lovelace", "Engineer", state))
	}
	people("512")
	first := syncNow(t, h, admin, dir.ID, false)
	if first.Counts.Created != 1 {
		t.Fatalf("first sync: %+v", first)
	}
	u, err := st.GetUserByEmail(ctx, "ada@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetPassword(ctx, u.ID, "test-password-hash", false); err != nil {
		t.Fatal(err)
	}
	people("514")
	if out := syncNow(t, h, admin, dir.ID, false); out.Counts.Suspended != 1 {
		t.Fatalf("suspension: %+v", out)
	}
	people("512")
	blocked := syncNow(t, h, admin, dir.ID, false)
	if blocked.Counts.Restored != 0 || !slices.ContainsFunc(blocked.Skipped, func(s string) bool { return strings.Contains(s, "identity check is still required") }) {
		t.Fatalf("restore before identity check: %+v", blocked)
	}
	if _, err := st.CreateIdentityProof(ctx, store.NewIdentityProof{UserID: u.ID, Method: "in-person", Status: "verified"}); err != nil {
		t.Fatal(err)
	}
	if out := syncNow(t, h, admin, dir.ID, false); out.Counts.Restored != 1 {
		t.Fatalf("restore after identity check: %+v", out)
	}
}
