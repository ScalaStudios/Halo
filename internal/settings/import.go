package settings

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"

	"halo/internal/approval"
	"halo/internal/httpx"
	"halo/internal/policy"
	"halo/internal/store"
)

const maxImportBytes = 16 << 20

var (
	errRollback      = errors.New("rolled back")
	errGrant         = httpx.Fail(http.StatusForbidden, "ERR_ROLE_GRANT", "An import never grants administrator roles. Grant this role on the Roles page, or leave the roles and eligibleRoles sections out of the file.")
	importedSections = []string{"settings", "attributes", "groups", "memberships", "roles", "eligibleRoles", "terms", "applications", "policies", "accessPackages"}
	skippedSections  = []string{"users", "applicationRoles", "termsAcceptances", "accessReviews", "auditEvents"}
	termsFields      = []string{"name", "text", "url"}
	policyFields     = []string{"name", "description", "enabled", "mode", "effect", "termsId", "conditions"}
	packageFields    = []string{"name", "description", "ownerId", "groupIds", "approverIds", "stages", "questions", "maxDays", "requireJustification", "archived"}
	tokenFields      = map[string]string{"accessTokenTtl": "accessTokenTtl", "refreshTokenTtl": "refreshTokenTtl", "idTokenTtl": "idTokenTtl", "refreshRotation": "rotation", "tokenExchange": "exchange", "tokenExchangeFrom": "exchangeFrom"}
)

type importItem struct {
	Section string       `json:"section"`
	Action  string       `json:"action"`
	ID      string       `json:"id"`
	Label   string       `json:"label"`
	Fields  []string     `json:"fields,omitempty"`
	Error   *httpx.Error `json:"error,omitempty"`
}

type importReport struct {
	Applied bool         `json:"applied"`
	Changes []importItem `json:"changes"`
	Secrets []importItem `json:"secrets"`
	Skipped []string     `json:"skipped"`
	Error   *httpx.Error `json:"error,omitempty"`
}

func (rep importReport) failed() bool {
	return slices.ContainsFunc(rep.Changes, func(c importItem) bool { return c.Error != nil })
}

func (rep importReport) summary(format string) string {
	counts, sections := map[string]int{}, []string{}
	for _, c := range rep.Changes {
		counts[c.Action]++
		if !slices.Contains(sections, c.Section) {
			sections = append(sections, c.Section)
		}
	}
	return fmt.Sprintf(format, counts["create"], counts["change"], counts["remove"], strings.Join(sections, ", "))
}

func (rep importReport) highlights() string {
	parts := []string{}
	for _, c := range rep.Changes {
		if c.Action == "create" {
			continue
		}
		part := c.Action + " " + c.Label
		if len(c.Fields) > 0 {
			part += " (" + strings.Join(c.Fields, ", ") + ")"
		}
		parts = append(parts, part)
	}
	if len(parts) > 5 {
		parts = append(parts[:5], fmt.Sprintf("%d more", len(parts)-5))
	}
	if len(parts) == 0 {
		return ""
	}
	return ". Includes: " + strings.Join(parts, "; ")
}

type call struct {
	method, path string
	body         any
}

type kind struct {
	name, path, nouns string
	fields            []string
	create            func(row map[string]any) []call
	update            func(id string, row map[string]any, changed []string) []call
}

var kinds = []kind{
	{"attributes", "/api/v1/attributes", "attributes", []string{"label", "selfService"}, post("/api/v1/attributes", "name", "label", "type", "selfService"), patch("/api/v1/attributes")},
	{"groups", "/api/v1/groups", "groups", []string{"name", "description", "kind", "rule", "joinPolicy", "ownerIds"}, createGroup, updateGroup},
	{"terms", "/api/v1/terms", "terms of use", termsFields, post("/api/v1/terms", termsFields...), put("/api/v1/terms", termsFields...)},
	{"applications", "/api/v1/applications", "applications", []string{"name", "description", "homepage", "redirectUris", "postLogoutUris", "scopes", "groupIds", "accessTokenTtl", "refreshTokenTtl", "idTokenTtl", "refreshRotation", "tokenExchange", "tokenExchangeFrom", "maxAuthAge", "ownerId", "claimMappings", "status"}, createApplication, updateApplication},
	{"policies", "/api/v1/policies", "policies", policyFields, post("/api/v1/policies", policyFields...), put("/api/v1/policies", policyFields...)},
	{"accessPackages", "/api/v1/access-packages", "access packages", packageFields, post("/api/v1/access-packages", packageFields...), put("/api/v1/access-packages", packageFields...)},
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func pick(row map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, k := range keys {
		if v, ok := row[k]; ok {
			out[k] = v
		}
	}
	return out
}

func post(path string, keys ...string) func(map[string]any) []call {
	return func(row map[string]any) []call { return []call{{"POST", path, pick(row, keys...)}} }
}

func patch(path string) func(string, map[string]any, []string) []call {
	return func(id string, row map[string]any, changed []string) []call {
		return []call{{"PATCH", path + "/" + id, pick(row, changed...)}}
	}
}

func put(path string, keys ...string) func(string, map[string]any, []string) []call {
	return func(id string, row map[string]any, _ []string) []call {
		return []call{{"PUT", path + "/" + id, pick(row, keys...)}}
	}
}

func createGroup(row map[string]any) []call {
	path := "/api/v1/groups/" + str(row["id"])
	calls := []call{{"POST", "/api/v1/groups", pick(row, "name", "description", "kind", "rule")}}
	if owners, _ := row["ownerIds"].([]any); len(owners) > 0 {
		calls = append(calls, call{"PUT", path + "/owners", map[string]any{"userIds": owners}})
	}
	if join := str(row["joinPolicy"]); join != "" && join != "closed" {
		calls = append(calls, call{"PATCH", path, map[string]any{"joinPolicy": join}})
	}
	return calls
}

func updateGroup(id string, row map[string]any, changed []string) []call {
	path := "/api/v1/groups/" + id
	rest := slices.DeleteFunc(slices.Clone(changed), func(f string) bool { return f == "ownerIds" })
	calls := []call{}
	if len(rest) > 0 {
		calls = append(calls, call{"PATCH", path, pick(row, rest...)})
	}
	if len(rest) < len(changed) {
		owners := call{"PUT", path + "/owners", map[string]any{"userIds": row["ownerIds"]}}
		if str(row["joinPolicy"]) == "request" {
			return append([]call{owners}, calls...)
		}
		calls = append(calls, owners)
	}
	return calls
}

func createApplication(row map[string]any) []call {
	body := pick(row, "name", "description", "protocol", "homepage", "redirectUris", "postLogoutUris", "scopes", "groupIds", "setupGuide")
	body["type"] = row["appType"]
	protocol := str(row["protocol"])
	if protocol == "bookmark" || protocol == "forward-auth" || protocol == "radius" {
		delete(body, "scopes")
	}
	if protocol == "saml" {
		body["saml"] = map[string]any{"entityId": row["clientId"], "acsUrls": row["redirectUris"]}
		delete(body, "redirectUris")
		delete(body, "postLogoutUris")
	}
	later := []string{"ownerId"}
	if protocol == "oidc" || protocol == "oauth" {
		later = append(later, "accessTokenTtl", "refreshTokenTtl", "idTokenTtl", "refreshRotation", "tokenExchange", "tokenExchangeFrom")
	}
	if protocol != "bookmark" && row["appType"] != "service" {
		later = append(later, "maxAuthAge", "claimMappings")
	}
	if row["status"] == "disabled" {
		later = append(later, "status")
	}
	return append([]call{{"POST", "/api/v1/applications", body}}, updateApplication(str(row["id"]), row, later)...)
}

func updateApplication(id string, row map[string]any, changed []string) []call {
	path := "/api/v1/applications/" + id
	body, tokens, calls := map[string]any{}, map[string]any{}, []call{}
	for _, f := range changed {
		switch {
		case f == "status" && row[f] == "disabled":
			calls = append(calls, call{"POST", path + "/disable", nil})
		case f == "status":
			calls = append(calls, call{"POST", path + "/enable", nil})
		case f == "ownerId":
			body["owner"] = str(row[f])
		case tokenFields[f] != "":
			tokens[tokenFields[f]] = row[f]
		default:
			body[f] = row[f]
		}
	}
	if len(tokens) > 0 {
		body["tokenPolicy"] = tokens
	}
	if len(body) > 0 {
		calls = append([]call{{"PATCH", path, body}}, calls...)
	}
	return calls
}

func changedFields(current, row map[string]any, fields []string) []string {
	keys := slices.DeleteFunc(slices.Clone(fields), func(f string) bool {
		_, ok := row[f]
		return !ok
	})
	changed := []string{}
	for _, c := range store.Diff(pick(current, keys...), pick(row, keys...)) {
		if f, _, _ := strings.Cut(c.Field, "."); !slices.Contains(changed, f) {
			changed = append(changed, f)
		}
	}
	return changed
}

func roleKey(row map[string]any) string {
	scope, _ := row["scope"].([]any)
	ids := []string{}
	for _, s := range scope {
		ids = append(ids, str(s))
	}
	slices.Sort(ids)
	return str(row["userId"]) + " " + str(row["role"]) + " " + strings.Join(ids, ",")
}

func (h *handler) importConfiguration(w http.ResponseWriter, r *http.Request, actor store.User) error {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxImportBytes))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return httpx.Fail(http.StatusRequestEntityTooLarge, "ERR_TOO_LARGE", "The import is larger than 16 MB. Leave out the users, applicationRoles, auditEvents, termsAcceptances and accessReviews sections, which Halo doesn't import, and send it again.")
	}
	if err != nil {
		return err
	}
	var in struct {
		Configuration map[string]json.RawMessage `json:"configuration"`
		Remove        bool                       `json:"remove"`
		DryRun        bool                       `json:"dryRun"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return httpx.ErrBadJSON
	}
	if in.Configuration == nil {
		return httpx.Invalid(`Send a Halo export as configuration, for example {"configuration": {"groups": [...]}, "dryRun": true}.`)
	}
	plan, err := h.runImport(r, actor, in.Configuration, in.Remove, false)
	if err != nil || in.DryRun || plan.failed() || len(plan.Changes) == 0 {
		return respondImport(w, plan, !in.DryRun, err)
	}
	compact := map[string]any{"remove": in.Remove}
	sections := map[string]json.RawMessage{}
	for _, name := range importedSections {
		if section, ok := in.Configuration[name]; ok {
			sections[name] = section
		}
	}
	compact["configuration"] = sections
	body, err := json.Marshal(compact)
	if err != nil {
		return err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	change := approval.Change{Action: "configuration-import", Summary: plan.summary("Import configuration from a file: create %d, change %d and remove %d objects in %s") + plan.highlights(), TargetType: "configuration", TargetLabel: "Configuration import", State: plan.Changes}
	if held, err := h.gate.Hold(w, r, actor, change); held || err != nil {
		return err
	}
	result, err := h.runImport(r, actor, in.Configuration, in.Remove, true)
	return respondImport(w, result, true, err)
}

func respondImport(w http.ResponseWriter, rep importReport, apply bool, err error) error {
	if err != nil {
		return err
	}
	status := http.StatusOK
	if apply && !rep.Applied && len(rep.Changes) > 0 {
		status = http.StatusUnprocessableEntity
		rep.Error = httpx.Invalid("Nothing was imported, because some objects in the file can't be imported. Fix them in the file and import it again.")
	}
	return httpx.JSON(w, status, rep)
}

func (h *handler) runImport(r *http.Request, actor store.User, file map[string]json.RawMessage, remove, apply bool) (importReport, error) {
	report := importReport{Changes: []importItem{}, Secrets: []importItem{}, Skipped: []string{}}
	for _, name := range skippedSections {
		if _, ok := file[name]; ok {
			report.Skipped = append(report.Skipped, name)
		}
	}
	err := h.st.Savepoint(r.Context(), func(tx *store.Store) error {
		imp := &importer{h: h, r: r, actor: actor, tx: tx, ctx: approval.Approved(tx.Bind(r.Context())), report: &report,
			ids: map[string]string{}, names: map[string]string{}, rows: map[string][]map[string]any{}, claimed: map[string]map[string]bool{}}
		if err := imp.run(file, remove); err != nil {
			return err
		}
		if !apply || report.failed() {
			return errRollback
		}
		report.Applied = true
		return record(r, tx, actor, store.AuditEvent{Action: "organization.import", Summary: report.summary("Imported configuration from a file: created %d, changed %d and removed %d objects in %s"), TargetType: "organization", TargetLabel: "Configuration import"})
	})
	h.st.ForgetSettings()
	if errors.Is(err, errRollback) {
		err = nil
	}
	return report, err
}

type importer struct {
	h       *handler
	r       *http.Request
	actor   store.User
	tx      *store.Store
	ctx     context.Context
	report  *importReport
	ids     map[string]string
	names   map[string]string
	rows    map[string][]map[string]any
	claimed map[string]map[string]bool
}

func (imp *importer) run(file map[string]json.RawMessage, remove bool) error {
	users, err := imp.current("users")
	if err != nil {
		return err
	}
	for _, u := range users {
		imp.names[str(u["id"])] = cmp.Or(str(u["name"]), str(u["email"]))
	}
	if err := imp.checkRoles(file); err != nil {
		return err
	}
	present := map[string]bool{}
	for _, k := range kinds {
		rows, ok, err := section(file, k.name)
		if err != nil {
			return err
		}
		if ok {
			present[k.name] = true
			if err := imp.apply(k, rows); err != nil {
				return err
			}
		}
	}
	if err := imp.memberships(file, remove); err != nil {
		return err
	}
	if err := imp.settings(file); err != nil {
		return err
	}
	for _, k := range slices.Backward(kinds) {
		if remove && present[k.name] {
			if err := imp.remove(k); err != nil {
				return err
			}
		}
	}
	if imp.report.failed() || len(imp.report.Changes) == 0 {
		return nil
	}
	if err := policy.RefuseLockout(imp.r.WithContext(imp.ctx), imp.tx, imp.actor); err != nil {
		var problem *httpx.Error
		if !errors.As(err, &problem) {
			return err
		}
		imp.report.Changes = append(imp.report.Changes, importItem{Section: "policies", Action: "change", ID: "policies", Label: "Access policies", Error: problem})
	}
	return nil
}

func section(file map[string]json.RawMessage, name string) ([]map[string]any, bool, error) {
	raw, ok := file[name]
	if !ok {
		return nil, false, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, true, httpx.Invalid("The " + name + " section of the file is not a list of objects. Choose a Halo export file and try again.")
	}
	return rows, true, nil
}

func (imp *importer) current(name string) ([]map[string]any, error) {
	if rows, ok := imp.rows[name]; ok {
		return rows, nil
	}
	rows := []map[string]any{}
	i := slices.IndexFunc(store.ExportSections, func(s store.ExportSection) bool { return s.Name == name })
	_, err := imp.tx.ExportRows(imp.ctx, store.ExportSections[i], func(raw json.RawMessage) error {
		var row map[string]any
		err := json.Unmarshal(raw, &row)
		rows = append(rows, row)
		return err
	})
	imp.rows[name] = rows
	return rows, err
}

func (imp *importer) mapped(id string) string {
	return cmp.Or(imp.ids[id], id)
}

func (imp *importer) name(id string) string {
	return cmp.Or(imp.names[id], imp.names[imp.mapped(id)], id)
}

func (imp *importer) add(item importItem, problem *httpx.Error, err error) error {
	item.Error = problem
	imp.report.Changes = append(imp.report.Changes, item)
	return err
}

func (imp *importer) checkRoles(file map[string]json.RawMessage) error {
	for _, name := range []string{"roles", "eligibleRoles"} {
		rows, ok, err := section(file, name)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		current, err := imp.current(name)
		if err != nil {
			return err
		}
		held := map[string]bool{}
		for _, c := range current {
			held[roleKey(c)] = true
		}
		for _, row := range rows {
			if !held[roleKey(row)] {
				user, role := str(row["userId"]), str(row["role"])
				imp.report.Changes = append(imp.report.Changes, importItem{Section: name, Action: "create", ID: user + "/" + role, Label: imp.name(user) + ": " + role, Error: errGrant})
			}
		}
	}
	return nil
}

func (imp *importer) apply(k kind, rows []map[string]any) error {
	current, err := imp.current(k.name)
	if err != nil {
		return err
	}
	byID, byName := map[string]map[string]any{}, map[string][]string{}
	for _, c := range current {
		id, label := str(c["id"]), str(c["name"])
		byID[id], imp.names[id] = c, label
		byName[strings.ToLower(label)] = append(byName[strings.ToLower(label)], id)
	}
	claimed := map[string]bool{}
	imp.claimed[k.name] = claimed
	for _, row := range rows {
		fid, label := str(row["id"]), str(row["name"])
		item := importItem{Section: k.name, Action: "create", ID: fid, Label: cmp.Or(label, fid)}
		imp.names[fid] = item.Label
		target, found := byID[fid]
		if !found {
			same := slices.DeleteFunc(slices.Clone(byName[strings.ToLower(label)]), func(id string) bool { return claimed[id] })
			if len(same) > 1 {
				if err := imp.add(item, httpx.Fail(http.StatusConflict, "ERR_CONFLICT", "Several "+k.nouns+" here are named "+label+", so Halo can't tell which one to update. Rename all but one and import again."), nil); err != nil {
					return err
				}
				continue
			}
			if len(same) == 1 {
				target, found = byID[same[0]], true
				imp.ids[fid] = same[0]
			}
		}
		row = imp.rewrite(row).(map[string]any)
		var problem *httpx.Error
		var created json.RawMessage
		switch {
		case fid == "":
			problem = httpx.Invalid("This object has no id. Choose a Halo export file and try again.")
		case found && claimed[str(target["id"])]:
			problem = httpx.Fail(http.StatusConflict, "ERR_CONFLICT", item.Label+" appears twice in the file. Keep one and import again.")
		case found:
			tid := str(target["id"])
			claimed[tid] = true
			if item.Action, item.Fields = "change", changedFields(target, row, k.fields); len(item.Fields) == 0 {
				continue
			}
			_, problem, err = imp.dispatch("", k.update(tid, row, item.Fields))
		default:
			created, problem, err = imp.dispatch(fid, k.create(row))
		}
		if err := imp.add(item, problem, err); err != nil {
			return err
		}
		if problem == nil && item.Action == "create" && k.name == "applications" {
			imp.secrets(item, row, created)
		}
	}
	return nil
}

func (imp *importer) secrets(item importItem, row map[string]any, created json.RawMessage) {
	if p := str(row["protocol"]); p != "oidc" && p != "oauth" {
		return
	}
	var out struct {
		ClientSecret *string `json:"clientSecret"`
	}
	_ = json.Unmarshal(created, &out)
	item.Fields = []string{"client ID"}
	if out.ClientSecret != nil {
		item.Fields = append(item.Fields, "client secret")
	}
	imp.report.Secrets = append(imp.report.Secrets, item)
}

func (imp *importer) memberships(file map[string]json.RawMessage, remove bool) error {
	rows, ok, err := section(file, "memberships")
	if err != nil || !ok {
		return err
	}
	groups, err := imp.current("groups")
	if err != nil {
		return err
	}
	kindOf, scope := map[string]string{}, map[string]bool{}
	for _, g := range groups {
		kindOf[str(g["id"])] = str(g["kind"])
	}
	fileGroups, _, err := section(file, "groups")
	if err != nil {
		return err
	}
	for _, g := range fileGroups {
		id := imp.mapped(str(g["id"]))
		kindOf[id], scope[id] = str(g["kind"]), true
	}
	current, err := imp.current("memberships")
	if err != nil {
		return err
	}
	have, want := map[[2]string]bool{}, map[[2]string]bool{}
	for _, m := range current {
		have[[2]string{str(m["groupId"]), str(m["userId"])}] = true
	}
	for _, m := range rows {
		group, user := str(m["groupId"]), str(m["userId"])
		key := [2]string{imp.mapped(group), imp.mapped(user)}
		scope[key[0]] = true
		if want[key] || kindOf[key[0]] == "dynamic" {
			continue
		}
		want[key] = true
		if have[key] {
			continue
		}
		item := importItem{Section: "memberships", Action: "create", ID: group + "/" + user, Label: imp.name(user) + " in " + imp.name(group)}
		_, problem, err := imp.dispatch("", []call{{"POST", "/api/v1/groups/" + key[0] + "/members", map[string]any{"userId": key[1]}}})
		if err := imp.add(item, problem, err); err != nil {
			return err
		}
	}
	for _, m := range current {
		key := [2]string{str(m["groupId"]), str(m["userId"])}
		if !remove || want[key] || !scope[key[0]] || kindOf[key[0]] != "assigned" {
			continue
		}
		item := importItem{Section: "memberships", Action: "remove", ID: key[0] + "/" + key[1], Label: imp.name(key[1]) + " in " + imp.name(key[0])}
		_, problem, err := imp.dispatch("", []call{{"DELETE", "/api/v1/groups/" + key[0] + "/members/" + key[1], nil}})
		if err := imp.add(item, problem, err); err != nil {
			return err
		}
	}
	return nil
}

func (imp *importer) settings(file map[string]json.RawMessage) error {
	raw, ok := file["settings"]
	if !ok {
		return nil
	}
	var want map[string]any
	if json.Unmarshal(raw, &want) != nil || want == nil {
		return httpx.Invalid("The settings in the file are not an object. Choose a Halo export file and try again.")
	}
	want = imp.rewrite(want).(map[string]any)
	s, err := imp.tx.CurrentSettings(imp.ctx)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(s)
	if err != nil {
		return err
	}
	var current map[string]any
	if err := json.Unmarshal(encoded, &current); err != nil {
		return err
	}
	keys := make([]string, len(fields))
	for i, f := range fields {
		keys[i] = f.key
	}
	item := importItem{Section: "settings", Action: "change", ID: "settings", Label: "Organization settings", Fields: changedFields(current, want, keys)}
	if len(item.Fields) == 0 {
		return nil
	}
	_, problem, err := imp.dispatch("", []call{{"PATCH", "/api/v1/settings", pick(want, item.Fields...)}})
	return imp.add(item, problem, err)
}

func (imp *importer) remove(k kind) error {
	current, err := imp.current(k.name)
	if err != nil {
		return err
	}
	for _, c := range current {
		id := str(c["id"])
		if imp.claimed[k.name][id] {
			continue
		}
		_, problem, err := imp.dispatch("", []call{{"DELETE", k.path + "/" + id, nil}})
		if err := imp.add(importItem{Section: k.name, Action: "remove", ID: id, Label: cmp.Or(str(c["name"]), id)}, problem, err); err != nil {
			return err
		}
	}
	return nil
}

func (imp *importer) rewrite(v any) any {
	switch v := v.(type) {
	case string:
		return imp.mapped(v)
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = imp.rewrite(x)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, x := range v {
			out[k] = imp.rewrite(x)
		}
		return out
	}
	return v
}

func (imp *importer) dispatch(fileID string, calls []call) (json.RawMessage, *httpx.Error, error) {
	var first json.RawMessage
	for i, c := range calls {
		out, problem, err := imp.send(c)
		if err != nil || problem != nil {
			return first, problem, err
		}
		if i == 0 {
			first = out
		}
		if i == 0 && fileID != "" {
			var created struct {
				ID          string `json:"id"`
				Application struct {
					ID string `json:"id"`
				} `json:"application"`
			}
			_ = json.Unmarshal(out, &created)
			imp.ids[fileID] = cmp.Or(created.ID, created.Application.ID)
		}
	}
	return first, nil, nil
}

func (imp *importer) send(c call) (json.RawMessage, *httpx.Error, error) {
	var payload []byte
	if c.body != nil {
		var err error
		if payload, err = json.Marshal(imp.rewrite(c.body)); err != nil {
			return nil, nil, err
		}
	}
	parts := strings.Split(c.path, "/")
	for i, p := range parts {
		parts[i] = imp.mapped(p)
	}
	var out json.RawMessage
	var problem *httpx.Error
	err := imp.tx.Savepoint(imp.ctx, func(sp *store.Store) error {
		req, err := http.NewRequestWithContext(sp.Bind(imp.ctx), c.method, strings.Join(parts, "/"), bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header = imp.r.Header.Clone()
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = imp.r.RemoteAddr
		rec := httptest.NewRecorder()
		imp.h.mux.ServeHTTP(rec, req)
		if rec.Code >= http.StatusInternalServerError {
			return fmt.Errorf("import %s %s: status %d", c.method, req.URL.Path, rec.Code)
		}
		if rec.Code >= http.StatusMultipleChoices {
			var failed struct {
				Error httpx.Error `json:"error"`
			}
			if json.Unmarshal(rec.Body.Bytes(), &failed) != nil || failed.Error.Message == "" {
				failed.Error = httpx.Error{Code: "ERR_IMPORT", Message: fmt.Sprintf("Halo refused this change with status %d.", rec.Code)}
			}
			failed.Error.Status = rec.Code
			problem = &failed.Error
			return errRollback
		}
		out = rec.Body.Bytes()
		return nil
	})
	if errors.Is(err, errRollback) {
		err = nil
	}
	return out, problem, err
}
