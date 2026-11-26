package api_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"halo/internal/store"
)

func rowsOf(t *testing.T, st *store.Store, query string, args ...any) []map[string]any {
	t.Helper()
	var rows []map[string]any
	_, err := st.ExportRows(context.Background(), store.ExportSection{Name: "rows", Table: "users", Query: query}, func(row json.RawMessage) error {
		var v map[string]any
		rows = append(rows, v)
		return json.Unmarshal(row, &rows[len(rows)-1])
	}, args...)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func mentions(t *testing.T, st *store.Store, values ...string) []string {
	t.Helper()
	columns := rowsOf(t, st, `select c.table_name, c.column_name from information_schema.columns c join information_schema.tables t on t.table_name = c.table_name and t.table_schema = c.table_schema
		where c.table_schema = 'public' and t.table_type = 'BASE TABLE' and (c.data_type in ('text', 'character varying', 'jsonb', 'json') or c.udt_name = '_text') order by 1, 2`)
	var found []string
	for _, c := range columns {
		table, column := c["tableName"].(string), c["columnName"].(string)
		hits := rowsOf(t, st, `select count(*) as n from `+table+` where strpos(lower(`+column+`::text), lower($1)) > 0 or strpos(lower(`+column+`::text), lower($2)) > 0 or strpos(lower(`+column+`::text), lower($3)) > 0`, values[0], values[1], values[2])
		if hits[0]["n"].(float64) > 0 {
			found = append(found, table+"."+column)
		}
	}
	return found
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestPersonalDataExportAndPermanentDeletion(t *testing.T) {
	e := impersonationSetup(t)
	ctx := context.Background()
	st := e.st
	ada, adaToken := e.person("ada@example.com", false, "global_admin")
	_, graceToken := e.person("grace@example.com", false, "global_admin")
	hana, _ := e.person("hana@example.com", false, "helpdesk_admin")
	_, userAdmin := e.person("uma@example.com", false, "user_admin")
	erin, err := st.CreateUser(ctx, store.NewUser{Email: "erin.erasure@example.com", Name: "Erin Erasure", Title: "Analyst", Status: "active", GroupIDs: []string{e.group.ID}})
	must(t, err)
	erinToken := session(t, st, erin.ID)
	_, err = st.CreateUser(ctx, store.NewUser{Email: "rory@example.com", Name: "Rory", Status: "active", ManagerID: &erin.ID})
	must(t, err)

	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Payroll", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://payroll.example.com/cb"}})
	must(t, err)
	must(t, st.SetPassword(ctx, erin.ID, "$argon2id$v=19$m=65536,t=3,p=4$c2FsdHNhbHQ$aGFzaGhhc2hoYXNo", false))
	_, err = st.CreateTOTPSecret(ctx, erin.ID, "Phone", st.Sealer.Seal([]byte("JBSWY3DPEHPK3PXP")))
	must(t, err)
	must(t, st.ReplaceRecoveryCodes(ctx, erin.ID, [][]byte{[]byte("recovery-hash-one")}))
	pass, err := st.CreateAccessPass(ctx, erin.ID, ada.ID, 1, time.Now().Add(time.Hour))
	must(t, err)
	_, err = st.AddWebAuthnCredential(ctx, store.WebAuthnCredential{UserID: erin.ID, Kind: "passkey", Label: "Laptop", CredentialID: []byte("credential"), PublicKey: []byte("public-key"), Transports: []string{}})
	must(t, err)
	must(t, st.SetGroupOwners(ctx, e.group.ID, []string{erin.ID}))
	must(t, st.GrantConsent(ctx, erin.ID, app.ID, []string{"openid", "email"}))
	must(t, st.SetAppPreferences(ctx, erin.ID, []string{app.ID}, nil))
	_, err = st.CreateAppRole(ctx, app.ID, store.AppRole{Key: "approver", Name: "Approver", UserIDs: []string{erin.ID}})
	must(t, err)
	must(t, st.WriteTuples(ctx, app.ID, []store.Tuple{{ObjectType: "invoice", ObjectID: "inv-7", Relation: "approver", SubjectType: "user", SubjectID: erin.ID}}, nil))
	must(t, st.SetSecurityEmailOptOuts(ctx, erin.ID, []string{"new_sign_in"}))
	must(t, st.LinkKerberosPrincipal(ctx, "erin@CORP.EXAMPLE.COM", erin.ID))
	_, err = st.CreateIdentityProof(ctx, store.NewIdentityProof{UserID: erin.ID, Method: "in-person", Status: "verified", Document: "passport", Note: "Met at the Lisbon office", CheckedBy: &ada.ID})
	must(t, err)
	_, err = st.CreateIdentityProof(ctx, store.NewIdentityProof{UserID: ada.ID, Method: "in-person", Status: "verified", Document: "national-id", Note: "Compared the photo at the front desk", CheckedBy: &erin.ID})
	must(t, err)
	_, err = st.ClaimSecurityEmail(ctx, erin.ID, "email_changed", "erin.erasure@example.com", time.Now(), time.Minute)
	must(t, err)
	must(t, st.CreateOIDCTokens(ctx, store.OIDCToken{ID: "tok_erin", Kind: "access", ClientID: app.ClientID, UserID: &erin.ID, Scopes: []string{"openid"}, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}))
	terms, err := st.SaveTerms(ctx, store.Terms{Name: "Acceptable use", Text: "Be kind."})
	must(t, err)
	must(t, st.AcceptTerms(ctx, erin.ID, []store.Terms{terms}))
	deviceID, err := st.TouchDevice(ctx, erin.ID, []byte("device-token-hash"), "Firefox on Linux", "203.0.113.9", true)
	must(t, err)
	must(t, st.RecordRiskEvent(ctx, store.RiskEvent{Type: "new-device", Level: "medium", UserID: &erin.ID, IP: "203.0.113.9", DeviceID: &deviceID}))
	must(t, st.RecordSSHCertificate(ctx, store.SSHCertificate{Serial: 7, KeyID: "halo:" + erin.ID + ":7", UserID: &erin.ID, Principals: []string{"ops"}, Fingerprint: "SHA256:abc", KeyType: "ssh-ed25519", ValidAfter: time.Now(), ValidBefore: time.Now().Add(time.Hour)}))
	_, err = st.CreateImpersonation(ctx, hana.ID, erin.ID, "Ticket 7", time.Now().Add(time.Hour))
	must(t, err)
	must(t, st.RecordSignIn(ctx, store.SignInEvent{UserID: &erin.ID, Email: erin.Email, Result: "success", Method: "passkey", IP: "203.0.113.9"}))
	must(t, st.RecordSignIn(ctx, store.SignInEvent{Email: "Erin.Erasure@Example.com", Result: "failure", Method: "passkey", Reason: "No account uses this email address."}))
	must(t, st.RecordSignIn(ctx, store.SignInEvent{UserID: &ada.ID, Email: ada.Email, Result: "success", Method: "passkey", ImpersonatorID: &erin.ID, Reason: "Impersonated by Erin Erasure"}))
	must(t, st.RecordAudit(ctx, store.AuditEvent{ActorID: &erin.ID, Action: "group.update", Summary: "Renamed the group", TargetType: "group", TargetID: e.group.ID, TargetLabel: e.group.Name}))
	must(t, st.RecordAudit(ctx, store.AuditEvent{ActorID: &ada.ID, Action: "group.member.add", Summary: "Added Erin Erasure", TargetType: "group", TargetID: e.group.ID, TargetLabel: e.group.Name}))
	must(t, st.RecordAudit(ctx, store.AuditEvent{ActorID: &ada.ID, Action: "policy.update", Summary: "Updated the policy", TargetType: "policy", Changes: store.Diff(map[string][]string{"userIds": {}}, map[string][]string{"userIds": {erin.ID}})}))
	only, err := st.CreateAccessPolicy(ctx, store.AccessPolicy{Name: "Erin only", Mode: "enforce", Effect: "block", Conditions: store.PolicyConditions{UserIDs: []string{erin.ID}, AllApps: true}})
	must(t, err)
	_, err = st.CreateAccessPolicy(ctx, store.AccessPolicy{Name: "Everyone but Erin", Mode: "enforce", Effect: "block", Conditions: store.PolicyConditions{AllUsers: true, ExcludeUserIDs: []string{erin.ID, ada.ID}, AllApps: true}})
	must(t, err)
	expect(t, e.call(adaToken, "PATCH", "/api/v1/users/"+erin.ID, map[string]any{"title": "Senior analyst"}), 200)
	must(t, st.RecordAudit(ctx, store.AuditEvent{ActorID: &ada.ID, Action: "user.sign_up.reject", Summary: "Rejected the sign-up of Erin Erasure <erin.erasure@example.com>", TargetType: "sign_up", TargetID: "sgn_old", TargetLabel: "Erin Erasure"}))
	_, err = st.CreateApprovalRequest(ctx, store.ApprovalRequest{Action: "approval-settings", Summary: "Make Erin Erasure an emergency access account", TargetType: "settings", TargetLabel: "Approval settings",
		Requester: store.NamedRef{ID: ada.ID}, ExpiresAt: time.Now().Add(time.Hour), Method: "PATCH", Path: "/api/v1/settings", Body: []byte("{}"), State: `{"emergencyAccessIds":["` + erin.ID + `"]}`})
	must(t, err)
	_, err = st.QueueMail(ctx, erin.Email, "Welcome to Halo", "Hello Erin", "", false)
	must(t, err)
	_, err = st.QueueMail(ctx, ada.Email, "Erin Erasure asked for access", "Review the request", "", false)
	must(t, err)
	_, _, err = st.RequestSignUp(ctx, "Erin.Erasure@example.com", "Erin Erasure")
	must(t, err)
	_, _, err = st.CountRequest(ctx, "erin.erasure@example.com", "sign-up-email", 3, time.Hour)
	must(t, err)
	hook, _, err := st.CreateWebhook(ctx, store.NewWebhook{URL: "https://hooks.example.com/halo", Events: []string{"user.update", "sign_in.success"}, Enabled: true})
	must(t, err)
	events, err := st.PollWebhookEvents(ctx, time.Now().Add(time.Second), 100)
	must(t, err)
	for _, event := range events {
		_, err = st.QueueWebhookDelivery(ctx, hook, event, time.Now().Add(time.Hour))
		must(t, err)
	}

	expect(t, e.call(userAdmin, "GET", "/api/v1/users/"+erin.ID+"/export", nil), 403)
	rec := expectStatus(t, e.call(adaToken, "GET", "/api/v1/users/"+erin.ID+"/export", nil), 200)
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("export must download as a file: %v", rec.Header())
	}
	body := rec.Body.String()
	for _, secret := range []string{erinToken, pass, "argon2id", "JBSWY3DPEHPK3PXP", "recovery-hash-one", "device-token-hash", "\\\\x", "Hash", "sealed"} {
		if strings.Contains(body, secret) {
			t.Errorf("export leaks %q", secret)
		}
	}
	var export map[string]json.RawMessage
	must(t, json.Unmarshal(rec.Body.Bytes(), &export))
	var person store.User
	must(t, json.Unmarshal(export["person"], &person))
	if person.ID != erin.ID || person.Title != "Senior analyst" || len(person.Methods) < 4 {
		t.Fatalf("person: %+v", person)
	}
	for _, section := range store.PersonalDataSections {
		var rows []map[string]any
		must(t, json.Unmarshal(export[section.Name], &rows))
		empty := slices.Contains([]string{"groupJoinRequests", "linkedAccounts", "accessRequests", "accessReviews", "provisionedAccounts", "lifecycleRuns"}, section.Name)
		if empty != (len(rows) == 0) {
			t.Errorf("%s: %d rows", section.Name, len(rows))
		}
	}
	var signIns []map[string]any
	must(t, json.Unmarshal(export["signIns"], &signIns))
	if len(signIns) != 2 {
		t.Errorf("sign-ins: %v", signIns)
	}

	mine := decode[map[string]json.RawMessage](t, expectStatus(t, e.call(erinToken, "GET", "/api/v1/me/export", nil), 200))
	must(t, json.Unmarshal(mine["person"], &person))
	if person.ID != erin.ID {
		t.Fatalf("own export is for %s", person.ID)
	}
	if len(e.audit("user.export")) != 2 {
		t.Fatalf("exports must be audited: %+v", e.audit("user.export"))
	}

	before := mentions(t, st, erin.ID, erin.Email, erin.Name)
	for _, want := range []string{"users.email", "sessions.user_id", "sign_in_events.email", "sign_in_events.impersonator_id", "audit_events.actor_id", "audit_events.summary", "audit_events.changes", "ssh_certificates.key_id", "access_policies.conditions", "consent_grants.user_id",
		"approval_requests.state", "approval_requests.summary", "authz_tuples.subject_id", "authz_tuples.user_id", "mail_outbox.recipient", "mail_outbox.subject", "sign_ups.email", "rate_limit_windows.subject", "webhook_deliveries.body", "identity_proofs.user_id", "identity_proofs.checked_by"} {
		if !slices.Contains(before, want) {
			t.Fatalf("expected %s to mention Erin before deletion, got %v", want, before)
		}
	}

	e.configure(map[string]any{"approvalActions": []string{"user-deletion"}})
	pending := decode[store.ApprovalRequest](t, expectStatus(t, e.call(adaToken, "DELETE", "/api/v1/users/"+erin.ID, nil), 202))
	expect(t, e.call(adaToken, "POST", "/api/v1/approvals/"+pending.ID+"/approve", nil), 403)
	expect(t, e.call(graceToken, "POST", "/api/v1/approvals/"+pending.ID+"/approve", nil), 200)

	expect(t, e.call(adaToken, "GET", "/api/v1/users/"+erin.ID, nil), 404)
	expect(t, e.call(erinToken, "GET", "/api/v1/me", nil), 401)
	if left := mentions(t, st, erin.ID, erin.Email, erin.Name); len(left) > 0 {
		t.Fatalf("still mentioned after deletion: %v", left)
	}
	deleted := e.audit("user.delete")
	if len(deleted) != 1 || deleted[0].ActorID == nil || *deleted[0].ActorID != ada.ID || !strings.HasPrefix(deleted[0].TargetID, "deleted_") || deleted[0].TargetLabel != deleted[0].TargetID {
		t.Fatalf("deletion audit: %+v", deleted)
	}
	pseudonym := deleted[0].TargetID
	if n := rowsOf(t, st, `select count(*) as n from audit_events where actor_id = $1`, pseudonym)[0]["n"]; n != 2.0 {
		t.Errorf("Erin's own changes must keep a pseudonymous actor, got %v", n)
	}
	if n := rowsOf(t, st, `select count(*) as n from sign_in_events where email = $1`, pseudonym)[0]["n"]; n != 2.0 {
		t.Errorf("Erin's sign-ins must keep a pseudonymous email, got %v", n)
	}
	if n := rowsOf(t, st, `select count(*) as n from audit_events where summary = 'Added ' || $1`, pseudonym)[0]["n"]; n != 1.0 {
		t.Errorf("mentions of Erin in other entries must be replaced, got %v", n)
	}
	policies := rowsOf(t, st, `select conditions from access_policies where id = $1`, only.ID)
	if users := policies[0]["conditions"].(map[string]any)["userIds"].([]any); len(users) != 0 {
		t.Errorf("policy still lists %v", users)
	}
	if manager := rowsOf(t, st, `select manager_id from users where email = 'rory@example.com'`)[0]["managerId"]; manager != nil {
		t.Errorf("Rory's manager is still %v", manager)
	}
}

func TestPermanentDeletionGuards(t *testing.T) {
	e := impersonationSetup(t)
	ctx := context.Background()
	st := e.st
	ada, adaToken := e.person("ada@example.com", false, "global_admin")
	_, userAdmin := e.person("uma@example.com", false, "user_admin")
	pat, _ := e.person("pat@example.com", true)
	_, helpdesk := e.person("hana@example.com", false, "helpdesk_admin")

	expect(t, e.call(adaToken, "DELETE", "/api/v1/users/"+ada.ID, nil), 403)
	expect(t, e.call(userAdmin, "DELETE", "/api/v1/users/"+pat.ID, nil), 403)

	bot, err := st.CreateServiceAccount(ctx, store.NewServiceAccount{Name: "Directory bot", Roles: []string{"global_admin"}})
	must(t, err)
	_, key, err := st.CreateAPIKey(ctx, bot.ID, "automation", []string{"api"}, nil, nil)
	must(t, err)
	expect(t, e.call(adaToken, "DELETE", "/api/v1/users/"+bot.ID, nil), 404)
	expect(t, e.call(key, "DELETE", "/api/v1/users/"+ada.ID, nil), 422)

	e.configure(map[string]any{"emergencyAccessIds": []string{pat.ID}})
	expect(t, e.call(adaToken, "DELETE", "/api/v1/users/"+pat.ID, nil), 422)
	e.configure(map[string]any{"emergencyAccessIds": []string{}})

	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Wiki", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://wiki.example.com/cb"}})
	must(t, err)
	must(t, st.SaveAppProvisioning(ctx, app.ID, "https://wiki.example.com/scim", st.Sealer.Seal([]byte("token")), true))
	must(t, st.SaveProvisionedUser(ctx, app.ID, pat.ID, store.ProvisionedUser{RemoteID: "remote-pat", Hash: "h", Active: true}))
	expect(t, e.call(adaToken, "DELETE", "/api/v1/users/"+pat.ID, nil), 409)
	must(t, st.SaveProvisionedUser(ctx, app.ID, pat.ID, store.ProvisionedUser{RemoteID: "remote-pat", Hash: "h", Active: false}))

	grant, impersonating := e.start(helpdesk, pat)
	expect(t, e.call(impersonating, "GET", "/api/v1/me/export", nil), 403)
	expect(t, e.call(key, "GET", "/api/v1/me/export", nil), 401)
	expect(t, e.call(impersonating, "POST", "/api/v1/impersonations/"+grant.ID+"/end", nil), 204)

	must(t, st.RecordAudit(ctx, store.AuditEvent{ActorID: &ada.ID, Action: "policy.update", Summary: "Rolled out the patch to pat", TargetType: "policy"}))
	result := decode[map[string]string](t, expectStatus(t, e.call(adaToken, "DELETE", "/api/v1/users/"+pat.ID, nil), 200))
	if !strings.HasPrefix(result["pseudonym"], "deleted_") {
		t.Fatalf("result: %v", result)
	}
	expect(t, e.call(adaToken, "DELETE", "/api/v1/users/"+pat.ID, nil), 404)
	if n := rowsOf(t, st, `select count(*) as n from audit_events where summary = 'Rolled out the patch to ' || $1`, result["pseudonym"])[0]["n"]; n != 1.0 {
		t.Errorf("a name must only be replaced where it is a whole word, got %v", n)
	}
}

func TestPermanentDeletionKeepsUnrelatedAudit(t *testing.T) {
	e := impersonationSetup(t)
	ctx := context.Background()
	st := e.st
	ada, adaToken := e.person("ada@example.com", false, "global_admin")
	group, err := st.CreateGroup(ctx, store.NewGroup{Name: "Support", Kind: "assigned"})
	must(t, err)
	must(t, st.RecordAudit(ctx, store.AuditEvent{ActorID: &ada.ID, Action: "group.update", Summary: "Updated the group Support", TargetType: "group", TargetID: group.ID, TargetLabel: group.Name,
		Changes: store.Diff(map[string]any{"name": "Helpdesk"}, map[string]any{"name": "Support"})}))
	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Wiki", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://wiki.example.com/cb"}})
	must(t, err)
	must(t, st.RecordAudit(ctx, store.AuditEvent{ActorID: &ada.ID, Action: "application.update", Summary: "Updated the application", TargetType: "application", TargetID: app.ID, TargetLabel: app.Name,
		Changes: store.Diff(map[string]any{"enabled": false, "field": "old"}, map[string]any{"enabled": true, "field": "new"})}))
	for _, name := range []string{"Support", "True", "field"} {
		u, err := st.CreateUser(ctx, store.NewUser{Email: strings.ToLower(name) + "@example.com", Name: name, Status: "active"})
		must(t, err)
		expect(t, e.call(adaToken, "DELETE", "/api/v1/users/"+u.ID, nil), 200)
	}
	if row := rowsOf(t, st, `select summary, target_label, changes::text as diff from audit_events where target_id = $1`, group.ID)[0]; row["summary"] != "Updated the group Support" || row["targetLabel"] != "Support" || !strings.Contains(row["diff"].(string), `"Support"`) {
		t.Errorf("the group's audit entry changed: %v", row)
	}
	var changes []store.Change
	raw, err := json.Marshal(rowsOf(t, st, `select changes from audit_events where target_id = $1 and action = 'application.update'`, app.ID)[0]["changes"])
	must(t, err)
	must(t, json.Unmarshal(raw, &changes))
	got := map[string]string{}
	for _, c := range changes {
		got[c.Field] = string(c.Before) + ">" + string(c.After)
	}
	if got["enabled"] != "false>true" || got["field"] != `"old">"new"` {
		t.Errorf("the application diff changed: %v", got)
	}
}
