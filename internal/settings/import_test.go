package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"halo/internal/api"
	"halo/internal/approval"
	"halo/internal/config"
	"halo/internal/governance"
	"halo/internal/policy"
	"halo/internal/store"
)

func importEnv(t *testing.T) *env {
	return setup(t, api.Register, policy.Register, governance.Register, approval.Register)
}

func (e *env) exportFile(token string) map[string]any {
	e.t.Helper()
	var file map[string]any
	if err := json.Unmarshal(e.expect(e.call(token, "GET", "/api/v1/export", nil), http.StatusOK).Body.Bytes(), &file); err != nil {
		e.t.Fatal(err)
	}
	return file
}

func (e *env) importFile(token string, file map[string]any, remove, dryRun bool, status int) importReport {
	e.t.Helper()
	return decode[importReport](e.t, e.expect(e.call(token, "POST", "/api/v1/import", map[string]any{"configuration": file, "remove": remove, "dryRun": dryRun}), status))
}

func (e *env) userID(email string) string {
	e.t.Helper()
	u, err := e.st.GetUserByEmail(context.Background(), email)
	if err != nil {
		e.t.Fatal(err)
	}
	return u.ID
}

func (e *env) snapshot() string {
	e.t.Helper()
	var b strings.Builder
	for _, section := range store.ExportSections {
		if section.Name == "auditEvents" {
			continue
		}
		if _, err := e.st.ExportRows(context.Background(), section, func(row json.RawMessage) error {
			b.Write(row)
			return nil
		}); err != nil {
			e.t.Fatal(err)
		}
	}
	return b.String()
}

func rows(file map[string]any, name string) []any {
	list, _ := file[name].([]any)
	return list
}

func named(file map[string]any, name, label string) map[string]any {
	for _, row := range rows(file, name) {
		if row.(map[string]any)["name"] == label {
			return row.(map[string]any)
		}
	}
	return nil
}

func lines(items []importItem) []string {
	out := []string{}
	for _, c := range items {
		line := c.Section + " " + c.Action + " " + c.Label
		if c.Error != nil {
			line += ": " + c.Error.Code
		}
		out = append(out, line)
	}
	return out
}

func TestImportRoundTripIsNoOp(t *testing.T) {
	e := importEnv(t)
	ctx := context.Background()
	if err := e.st.SeedDemo(ctx); err != nil {
		t.Fatal(err)
	}
	admin := e.person("luna@example.com")
	file := e.exportFile(admin)
	before, state := e.audits(), e.snapshot()
	for _, remove := range []bool{false, true} {
		if plan := e.importFile(admin, file, remove, true, http.StatusOK); len(plan.Changes) != 0 || len(plan.Secrets) != 0 {
			t.Fatalf("round trip with remove %v: %v %v", remove, lines(plan.Changes), lines(plan.Secrets))
		}
	}
	pretty, _ := json.MarshalIndent(map[string]any{"configuration": file, "remove": true}, "", "  ")
	rep := decode[importReport](t, e.expect(e.call(admin, "POST", "/api/v1/import", pretty), http.StatusOK))
	if rep.Applied || len(rep.Changes) != 0 || !slices.Equal(rep.Skipped, skippedSections) {
		t.Fatalf("pretty-printed round trip: %+v", rep)
	}
	if e.audits() != before || e.snapshot() != state {
		t.Fatal("a round trip import changed the database")
	}
}

func (e *env) audits() int {
	e.t.Helper()
	events, err := e.st.ListAudit(context.Background(), 1000)
	if err != nil {
		e.t.Fatal(err)
	}
	return len(events)
}

func TestImportRestoresDeletedObjectsAndDryRunWritesNothing(t *testing.T) {
	e := importEnv(t)
	admin := e.person("ada@example.com", "global_admin")
	e.person("sam@example.com")
	ada, sam := e.userID("ada@example.com"), e.userID("sam@example.com")
	group := decode[store.Group](t, e.expect(e.call(admin, "POST", "/api/v1/groups", map[string]any{"name": "Finance", "kind": "assigned"}), http.StatusCreated))
	e.expect(e.call(admin, "PUT", "/api/v1/groups/"+group.ID+"/owners", map[string]any{"userIds": []string{ada}}), http.StatusOK)
	e.expect(e.call(admin, "POST", "/api/v1/groups/"+group.ID+"/members", map[string]any{"userId": sam}), http.StatusNoContent)
	pol := decode[store.AccessPolicy](t, e.expect(e.call(admin, "POST", "/api/v1/policies", map[string]any{"name": "Finance passkeys", "enabled": true, "mode": "report", "effect": "require-phishing-resistant", "conditions": map[string]any{"groupIds": []string{group.ID}, "allApps": true}}), http.StatusCreated))
	terms := decode[store.Terms](t, e.expect(e.call(admin, "POST", "/api/v1/terms", map[string]any{"name": "Acceptable use", "text": "Use company systems for work."}), http.StatusCreated))
	pkg := decode[store.AccessPackage](t, e.expect(e.call(admin, "POST", "/api/v1/access-packages", map[string]any{"name": "Finance access", "groupIds": []string{group.ID}, "approverIds": []string{ada}}), http.StatusCreated))
	file := e.exportFile(admin)

	e.expect(e.call(admin, "DELETE", "/api/v1/policies/"+pol.ID, nil), http.StatusNoContent)
	e.expect(e.call(admin, "DELETE", "/api/v1/access-packages/"+pkg.ID, nil), http.StatusNoContent)
	e.expect(e.call(admin, "DELETE", "/api/v1/groups/"+group.ID, nil), http.StatusNoContent)
	e.expect(e.call(admin, "PUT", "/api/v1/terms/"+terms.ID, map[string]any{"name": "Acceptable use policy", "text": "Use company systems for work."}), http.StatusOK)
	e.expect(e.call(admin, "POST", "/api/v1/applications", map[string]any{"name": "Scratch", "protocol": "bookmark", "homepage": "https://scratch.example.com"}), http.StatusCreated)

	audits, state := e.audits(), e.snapshot()
	plan := e.importFile(admin, file, true, true, http.StatusOK)
	want := []string{"groups create Finance", "terms change Acceptable use", "policies create Finance passkeys", "accessPackages create Finance access", "memberships create sam in Finance", "applications remove Scratch"}
	if !slices.Equal(lines(plan.Changes), want) || plan.Applied {
		t.Fatalf("dry run: %v", lines(plan.Changes))
	}
	if e.audits() != audits || e.snapshot() != state {
		t.Fatal("the dry run wrote to the database")
	}

	rep := e.importFile(admin, file, true, false, http.StatusOK)
	if !rep.Applied || !slices.Equal(lines(rep.Changes), want) {
		t.Fatalf("import: %+v", rep)
	}
	now := e.exportFile(admin)
	restored := named(now, "groups", "Finance")
	if restored == nil || restored["id"] == group.ID || fmt.Sprint(restored["ownerIds"]) != fmt.Sprint([]any{ada}) {
		t.Fatalf("restored group: %v", restored)
	}
	if !slices.ContainsFunc(rows(now, "memberships"), func(m any) bool {
		return m.(map[string]any)["groupId"] == restored["id"] && m.(map[string]any)["userId"] == sam
	}) {
		t.Fatal("the restored group has no members")
	}
	conditions := named(now, "policies", "Finance passkeys")["conditions"].(map[string]any)
	if fmt.Sprint(conditions["groupIds"]) != fmt.Sprint([]any{restored["id"]}) {
		t.Fatalf("the restored policy points at %v, not the restored group", conditions["groupIds"])
	}
	if p := named(now, "accessPackages", "Finance access"); fmt.Sprint(p["groupIds"]) != fmt.Sprint([]any{restored["id"]}) {
		t.Fatalf("restored package: %v", p)
	}
	if named(now, "terms", "Acceptable use") == nil || named(now, "applications", "Scratch") != nil {
		t.Fatal("terms not renamed back, or the application not in the file was kept")
	}
	if !slices.ContainsFunc(audit(t, e.st), func(ev store.AuditEvent) bool { return ev.Action == "organization.import" }) {
		t.Fatal("no organization.import audit event")
	}
	if again := e.importFile(admin, file, true, true, http.StatusOK); len(again.Changes) != 0 {
		t.Fatalf("importing the same file twice: %v", lines(again.Changes))
	}
}

func audit(t *testing.T, st *store.Store) []store.AuditEvent {
	t.Helper()
	events, err := st.ListAudit(context.Background(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	return events
}

func TestImportRejectsInvalidObjects(t *testing.T) {
	e := importEnv(t)
	admin := e.person("ada@example.com", "global_admin")
	e.expect(e.call(admin, "POST", "/api/v1/policies", map[string]any{"name": "Everyone", "enabled": true, "mode": "report", "effect": "allow", "conditions": map[string]any{"allUsers": true, "allApps": true}}), http.StatusCreated)
	file := e.exportFile(admin)
	named(file, "policies", "Everyone")["mode"] = "sometimes"
	file["groups"] = append(rows(file, "groups"), map[string]any{"id": "grp_new", "name": "Operations", "kind": "assigned"}, map[string]any{"id": "grp_blank", "name": "", "kind": "assigned"})
	state := e.snapshot()

	rep := e.importFile(admin, file, false, false, http.StatusUnprocessableEntity)
	if rep.Applied || rep.Error == nil || !slices.Equal(lines(rep.Changes), []string{"groups create Operations", "groups create grp_blank: ERR_INVALID", "policies change Everyone: ERR_INVALID"}) {
		t.Fatalf("invalid import: %+v", lines(rep.Changes))
	}
	if !strings.Contains(rep.Changes[2].Error.Message, "Choose a mode") || rep.Changes[1].Error.Message != "Enter a group name." {
		t.Fatalf("errors: %+v %+v", rep.Changes[1].Error, rep.Changes[2].Error)
	}
	if e.snapshot() != state {
		t.Fatal("a refused import changed the database")
	}
	e.expect(e.call(admin, "POST", "/api/v1/import", map[string]any{"configuration": map[string]any{"groups": "Finance"}}), http.StatusUnprocessableEntity)
	e.expect(e.call(e.person("sam@example.com", "security_admin"), "POST", "/api/v1/import", map[string]any{"configuration": file}), http.StatusForbidden)
}

func TestImportKeepsSecretsAndReportsNewOnes(t *testing.T) {
	e := importEnv(t)
	ctx := context.Background()
	admin := e.person("ada@example.com", "global_admin")
	created := decode[struct {
		Application  store.Application `json:"application"`
		ClientSecret string            `json:"clientSecret"`
	}](t, e.expect(e.call(admin, "POST", "/api/v1/applications", map[string]any{"name": "Payroll", "protocol": "oidc", "type": "web", "redirectUris": []string{"https://payroll.example.com/callback"}, "scopes": []string{"openid"}}), http.StatusCreated))
	file := e.exportFile(admin)
	payroll := named(file, "applications", "Payroll")
	payroll["description"] = "Pays everyone"
	payroll["accessTokenTtl"] = 1800.0
	copied := map[string]any{}
	for k, v := range payroll {
		copied[k] = v
	}
	copied["id"], copied["name"] = "app_copy", "Payroll staging"
	file["applications"] = append(rows(file, "applications"), copied)

	rep := e.importFile(admin, file, false, false, http.StatusOK)
	if !slices.Equal(lines(rep.Changes), []string{"applications change Payroll", "applications create Payroll staging"}) || !slices.Equal(rep.Changes[0].Fields, []string{"accessTokenTtl", "description"}) {
		t.Fatalf("import: %v %v", lines(rep.Changes), rep.Changes[0].Fields)
	}
	if len(rep.Secrets) != 1 || rep.Secrets[0].Label != "Payroll staging" || !slices.Equal(rep.Secrets[0].Fields, []string{"client ID", "client secret"}) {
		t.Fatalf("secrets: %+v", rep.Secrets)
	}
	app, err := e.st.VerifyClientSecret(ctx, created.Application.ClientID, created.ClientSecret)
	if err != nil || app.Description != "Pays everyone" || app.TokenPolicy.AccessTokenTTL != 1800 {
		t.Fatalf("existing secret after import: %v %+v", err, app)
	}
	staging := named(e.exportFile(admin), "applications", "Payroll staging")
	if staging == nil || staging["clientId"] == created.Application.ClientID || staging["accessTokenTtl"] != 1800.0 || fmt.Sprint(staging["redirectUris"]) != fmt.Sprint(payroll["redirectUris"]) {
		t.Fatalf("created application: %v", staging)
	}
}

func TestImportRefusesLockoutAndRoleGrants(t *testing.T) {
	e := importEnv(t)
	admin := e.person("ada@example.com", "global_admin")
	e.person("sam@example.com")
	file := e.exportFile(admin)
	file["policies"] = append(rows(file, "policies"), map[string]any{"id": "pol_lock", "name": "Block everyone", "enabled": true, "mode": "enforce", "effect": "block", "conditions": map[string]any{"allUsers": true, "allApps": true}})
	file["roles"] = append(rows(file, "roles"), map[string]any{"userId": e.userID("sam@example.com"), "role": "global_admin", "scope": nil})
	state := e.snapshot()

	plan := e.importFile(admin, file, false, true, http.StatusOK)
	if !slices.Equal(lines(plan.Changes), []string{"roles create sam: global_admin: ERR_ROLE_GRANT", "policies create Block everyone: ERR_LOCKOUT"}) {
		t.Fatalf("dry run: %v", lines(plan.Changes))
	}
	e.importFile(admin, file, false, false, http.StatusUnprocessableEntity)
	delete(file, "roles")
	file["policies"] = rows(file, "policies")[:len(rows(file, "policies"))-1]
	if rep := e.importFile(admin, file, false, false, http.StatusOK); len(rep.Changes) != 0 || e.snapshot() != state {
		t.Fatalf("without the refused objects: %v", lines(rep.Changes))
	}
}

func TestImportRefusesLockoutFromOtherChanges(t *testing.T) {
	e := importEnv(t)
	ctx := context.Background()
	admin := e.person("ada@example.com", "global_admin")
	if err := e.st.SaveSettingValues(ctx, map[string]json.RawMessage{"emergencyAccessIds": json.RawMessage(`["` + e.userID("ada@example.com") + `"]`)}); err != nil {
		t.Fatal(err)
	}
	e.st.ForgetSettings()
	e.expect(e.call(admin, "POST", "/api/v1/policies", map[string]any{"name": "Block everyone", "enabled": true, "mode": "enforce", "effect": "block", "conditions": map[string]any{"allUsers": true, "allApps": true}}), http.StatusCreated)
	file := e.exportFile(admin)
	file["settings"].(map[string]any)["emergencyAccessIds"] = []any{}

	plan := e.importFile(admin, file, false, true, http.StatusOK)
	if !slices.Equal(lines(plan.Changes), []string{"settings change Organization settings", "policies change Access policies: ERR_LOCKOUT"}) {
		t.Fatalf("dry run: %v", lines(plan.Changes))
	}
	e.importFile(admin, file, false, false, http.StatusUnprocessableEntity)
	if s, _ := e.st.CurrentSettings(ctx); len(s.EmergencyAccessIDs) != 1 {
		t.Fatal("the refused import removed the emergency access account")
	}
}

func TestImportWaitsForApproval(t *testing.T) {
	e := importEnv(t)
	ctx := context.Background()
	admin := e.person("ada@example.com", "global_admin")
	second := e.person("grace@example.com", "global_admin")
	pol := decode[store.AccessPolicy](t, e.expect(e.call(admin, "POST", "/api/v1/policies", map[string]any{"name": "Passkeys for contractors", "enabled": true, "mode": "enforce", "effect": "require-phishing-resistant", "conditions": map[string]any{"userIds": []string{e.userID("grace@example.com")}, "allApps": true}}), http.StatusCreated))
	if err := e.st.SaveSettingValues(ctx, map[string]json.RawMessage{"approvalActions": json.RawMessage(`["policy-disable"]`)}); err != nil {
		t.Fatal(err)
	}
	e.st.ForgetSettings()
	file := e.exportFile(admin)
	delete(file, "settings")
	named(file, "policies", pol.Name)["enabled"] = false

	held := decode[store.ApprovalRequest](t, e.expect(e.call(admin, "POST", "/api/v1/import", map[string]any{"configuration": file}), http.StatusAccepted))
	if held.Action != "configuration-import" || !strings.HasSuffix(held.Summary, "change 1 and remove 0 objects in policies. Includes: change Passkeys for contractors (enabled)") {
		t.Fatalf("approval request: %+v", held)
	}
	if p, _ := e.st.GetAccessPolicy(ctx, pol.ID); !p.Enabled {
		t.Fatal("the import changed the policy before approval")
	}
	e.expect(e.call(second, "POST", "/api/v1/approvals/"+held.ID+"/approve", nil), http.StatusOK)
	if p, _ := e.st.GetAccessPolicy(ctx, pol.ID); p.Enabled {
		t.Fatal("the approved import did not turn the policy off")
	}
}

type fakeBucket struct {
	mu      sync.Mutex
	objects map[string][]byte
	puts    int
}

func (b *fakeBucket) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := strings.TrimPrefix(r.URL.Path, "/halo/")
	switch {
	case r.Method == http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		if strings.HasPrefix(r.Header.Get("X-Amz-Content-Sha256"), "STREAMING") {
			var plain []byte
			for {
				line, rest, _ := bytes.Cut(body, []byte("\r\n"))
				size, _ := strconv.ParseInt(string(bytes.Split(line, []byte(";"))[0]), 16, 64)
				if size == 0 {
					break
				}
				plain, body = append(plain, rest[:size]...), rest[size+2:]
			}
			body = plain
		}
		b.objects[key] = body
		b.puts++
		w.Header().Set("ETag", `"etag"`)
	case r.Method == http.MethodDelete:
		delete(b.objects, key)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2":
		prefix := r.URL.Query().Get("prefix")
		var keys []string
		for k := range b.objects {
			if strings.HasPrefix(k, prefix) {
				keys = append(keys, k)
			}
		}
		slices.Sort(keys)
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>halo</Name><Prefix>%s</Prefix><KeyCount>%d</KeyCount><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>`, prefix, len(keys))
		for _, k := range keys {
			fmt.Fprintf(w, `<Contents><Key>%s</Key><LastModified>2026-01-01T00:00:00.000Z</LastModified><ETag>"etag"</ETag><Size>%d</Size><StorageClass>STANDARD</StorageClass></Contents>`, k, len(b.objects[k]))
		}
		fmt.Fprint(w, `</ListBucketResult>`)
	default:
		w.WriteHeader(http.StatusNotImplemented)
	}
}

func TestScheduledExport(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	admin := e.person("ada@example.com", "global_admin")
	e.expect(e.call(admin, "PATCH", "/api/v1/settings", map[string]any{"exportSchedule": "daily"}), http.StatusUnprocessableEntity)

	b := &fakeBucket{objects: map[string][]byte{
		"avatars/usr_1": []byte("picture"),
		exportPrefix + "2026-01-01T00-00-00Z.json": []byte("{}"),
		exportPrefix + "2026-01-02T00-00-00Z.json": []byte("{}"),
		exportPrefix + "2026-01-03T00-00-00Z.json": []byte("{}"),
	}}
	srv := httptest.NewServer(b)
	defer srv.Close()
	endpoint, _ := url.Parse(srv.URL)
	public, _ := url.Parse("http://localhost:3200")
	h := &handler{st: e.st, cfg: config.Config{Organization: "Fernway Systems", PublicURL: public, S3: config.S3{Endpoint: endpoint, Region: "us-east-1", Bucket: "halo", AccessKeyID: "key", SecretAccessKey: "secret"}}}
	if err := h.scheduledExport(ctx); err != nil || b.puts != 0 {
		t.Fatalf("export with the schedule off: %v, %d uploads", err, b.puts)
	}
	if err := e.st.SaveSettingValues(ctx, map[string]json.RawMessage{"exportSchedule": json.RawMessage(`"daily"`), "exportKeep": json.RawMessage(`2`)}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := h.scheduledExport(ctx); err != nil {
			t.Fatal(err)
		}
	}
	keys := slices.Sorted(maps.Keys(b.objects))
	if b.puts != 1 || len(keys) != 3 || keys[0] != "avatars/usr_1" || keys[1] != exportPrefix+"2026-01-03T00-00-00Z.json" {
		t.Fatalf("bucket after two runs a moment apart: %d uploads, %v", b.puts, keys)
	}
	var export map[string]any
	if err := json.Unmarshal(b.objects[keys[2]], &export); err != nil || export["settings"] == nil || export["groups"] == nil {
		t.Fatalf("scheduled export is not a Halo export: %v %s", err, b.objects[keys[2]])
	}
	events := audit(t, e.st)
	if events[0].Action != "organization.export" || !strings.Contains(events[0].Summary, keys[2]) {
		t.Fatalf("audit: %+v", events[0])
	}
}
