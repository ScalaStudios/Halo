package scim

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/mail"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"halo/internal/httpx"
	"halo/internal/store"
)

const (
	coreUserPrefix   = "urn:ietf:params:scim:schemas:core:2.0:user:"
	coreGroupPrefix  = "urn:ietf:params:scim:schemas:core:2.0:group:"
	enterpriseKey    = "urn:ietf:params:scim:schemas:extension:enterprise:2.0:user"
	enterprisePrefix = enterpriseKey + ":"
)

var memberPath = regexp.MustCompile(`(?i)^members\[\s*value\s+eq\s+"([^"]*)"\s*\]$`)

type patchOps struct {
	Operations []struct {
		Op    string          `json:"op"`
		Path  string          `json:"path"`
		Value json.RawMessage `json:"value"`
	} `json:"Operations"`
}

func (p patchOps) each(apply func(op, path string, value json.RawMessage) error) error {
	if len(p.Operations) == 0 {
		return fail(http.StatusBadRequest, "invalidSyntax", "Send a PatchOp document with at least one entry in Operations.")
	}
	for _, o := range p.Operations {
		op := strings.ToLower(o.Op)
		if op != "add" && op != "replace" && op != "remove" {
			return fail(http.StatusBadRequest, "invalidSyntax", fmt.Sprintf("%q is not a PATCH operation. Use add, replace or remove.", o.Op))
		}
		if op == "remove" && o.Path == "" {
			return fail(http.StatusBadRequest, "noTarget", "A remove operation needs a path, such as title.")
		}
		if err := apply(op, o.Path, o.Value); err != nil {
			return err
		}
	}
	return nil
}

func record(r *http.Request, tx *store.Store, actor store.User, e store.AuditEvent) error {
	e.ActorID, e.IP = &actor.ID, httpx.ClientIP(r)
	return tx.RecordAudit(r.Context(), e)
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func text(value json.RawMessage) (string, error) {
	var v any
	if len(value) > 0 {
		if err := json.Unmarshal(value, &v); err != nil {
			return "", fail(http.StatusBadRequest, "invalidValue", "A value is not valid JSON.")
		}
	}
	switch t := v.(type) {
	case nil:
		return "", nil
	case string:
		return strings.TrimSpace(t), nil
	}
	return "", fail(http.StatusBadRequest, "invalidValue", "Expected a string, got "+string(value)+".")
}

func optional(value json.RawMessage) (*string, error) {
	v, err := text(value)
	if v == "" {
		return nil, err
	}
	return &v, err
}

type userState struct {
	current, userName, email string
	display, formatted       *string
	given, family            string
	partsChanged, active     bool
	title, department        string
	managerID, externalID    *string
}

func newUserState(u store.SCIMUser) *userState {
	given, family, _ := strings.Cut(u.Name, " ")
	return &userState{current: u.Name, userName: u.Email, given: given, family: family, title: u.Title, department: u.Department, managerID: u.ManagerID, externalID: u.ExternalID, active: u.Status != "suspended"}
}

func (st *userState) object(op, prefix string, value json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(value, &fields); err != nil {
		return fail(http.StatusBadRequest, "invalidValue", `Expected an object, for example {"active": false}.`)
	}
	for key, v := range fields {
		if err := st.apply(op, prefix+key, v); err != nil {
			return err
		}
	}
	return nil
}

func (st *userState) apply(op, path string, value json.RawMessage) error {
	path = strings.TrimPrefix(strings.ToLower(path), coreUserPrefix)
	if op == "remove" {
		switch path {
		case "title":
			st.title = ""
		case "externalid":
			st.externalID = nil
		case enterprisePrefix + "department":
			st.department = ""
		case enterprisePrefix + "manager", enterprisePrefix + "manager.value":
			st.managerID = nil
		}
		return nil
	}
	var err error
	switch {
	case path == "":
		return st.object(op, "", value)
	case path == "name":
		return st.object(op, "name.", value)
	case path == enterpriseKey:
		return st.object(op, enterprisePrefix, value)
	case path == "username":
		st.userName, err = text(value)
	case path == "displayname":
		st.display = new(string)
		*st.display, err = text(value)
	case path == "name.formatted":
		st.formatted = new(string)
		*st.formatted, err = text(value)
	case path == "name.givenname":
		st.given, err = text(value)
		st.partsChanged = true
	case path == "name.familyname":
		st.family, err = text(value)
		st.partsChanged = true
	case path == "title":
		st.title, err = text(value)
	case path == "active":
		var v any
		_ = json.Unmarshal(value, &v)
		switch t := v.(type) {
		case bool:
			st.active = t
		case string:
			if st.active, err = strconv.ParseBool(t); err != nil {
				return fail(http.StatusBadRequest, "invalidValue", "active must be true or false.")
			}
		default:
			return fail(http.StatusBadRequest, "invalidValue", "active must be true or false.")
		}
	case path == "externalid":
		st.externalID, err = optional(value)
	case path == "emails":
		var emails []struct {
			Value   string `json:"value"`
			Primary any    `json:"primary"`
		}
		if json.Unmarshal(value, &emails) != nil {
			return fail(http.StatusBadRequest, "invalidValue", `emails must be a list such as [{"value": "sam@example.com", "primary": true}].`)
		}
		for i, e := range emails {
			if i == 0 || strings.EqualFold(fmt.Sprint(e.Primary), "true") {
				st.email = strings.TrimSpace(e.Value)
			}
		}
	case strings.HasPrefix(path, "emails[") && strings.HasSuffix(path, "].value"):
		st.email, err = text(value)
	case path == enterprisePrefix+"department":
		st.department, err = text(value)
	case path == enterprisePrefix+"manager" || path == enterprisePrefix+"manager.value":
		var ref struct {
			Value string `json:"value"`
		}
		if json.Unmarshal(value, &ref) != nil {
			ref.Value, err = text(value)
		}
		st.managerID = nil
		if v := strings.TrimSpace(ref.Value); v != "" {
			st.managerID = &v
		}
	}
	return err
}

func validEmail(email string) bool {
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email && len(email) <= 254
}

func (st *userState) final() (string, string, error) {
	email := st.userName
	if !validEmail(email) {
		email = st.email
	}
	if !validEmail(email) {
		return "", "", fail(http.StatusBadRequest, "invalidValue", "userName must be an email address, such as sam@example.com. Halo identifies people by email.")
	}
	name := st.current
	switch {
	case st.display != nil && *st.display != "":
		name = *st.display
	case st.formatted != nil && *st.formatted != "":
		name = *st.formatted
	case st.partsChanged && strings.TrimSpace(st.given+" "+st.family) != "":
		name = strings.TrimSpace(st.given + " " + st.family)
	}
	if name == "" {
		name = email
	}
	if len(name) > 200 || len(st.title) > 200 || len(st.department) > 200 {
		return "", "", fail(http.StatusBadRequest, "invalidValue", "Keep the name, title and department under 200 characters each.")
	}
	return email, name, nil
}

func (s *server) user(u store.SCIMUser) User {
	res := NewUser(u.Email, u.Name, u.Title, u.Department, u.Status != "suspended")
	res.ID, res.ExternalID = u.ID, deref(u.ExternalID)
	if u.ManagerID != nil {
		if res.Enterprise == nil {
			res.Schemas = append(res.Schemas, EnterpriseSchema)
			res.Enterprise = &Enterprise{}
		}
		res.Enterprise.Manager = &Manager{Value: *u.ManagerID, Ref: s.base + "/Users/" + *u.ManagerID, DisplayName: deref(u.ManagerName)}
	}
	res.Meta = &Meta{ResourceType: "User", Created: u.CreatedAt, LastModified: u.UpdatedAt, Location: s.base + "/Users/" + u.ID}
	return res
}

func (s *server) respondUser(w http.ResponseWriter, r *http.Request, userID string, status int) error {
	u, err := s.st.GetSCIMUser(r.Context(), userID)
	if err != nil {
		return err
	}
	if status == http.StatusCreated {
		w.Header().Set("Location", s.base+"/Users/"+userID)
	}
	return write(w, status, s.user(u))
}

func (s *server) checkManager(r *http.Request, managerID *string, self string) error {
	if managerID == nil {
		return nil
	}
	if *managerID == self {
		return fail(http.StatusBadRequest, "invalidValue", "A person cannot be their own manager.")
	}
	_, err := s.st.GetSCIMUser(r.Context(), *managerID)
	if errors.Is(err, store.ErrNotFound) {
		return fail(http.StatusBadRequest, "invalidValue", "The manager "+*managerID+" does not exist in Halo. Provision the manager first, then set the reference.")
	}
	return err
}

func (s *server) listUsers(w http.ResponseWriter, r *http.Request, _ store.User) error {
	attr, value, err := parseFilter(r.URL.Query().Get("filter"), `userName eq "sam@example.com"`, "username", "emails.value", "emails", "externalid")
	if err != nil {
		return err
	}
	start, count := paging(r)
	resources := []User{}
	if attr != "" && value == "" {
		return list(w, 0, start, resources)
	}
	email, externalID := value, ""
	if attr == "externalid" {
		email, externalID = "", value
	}
	users, total, err := s.st.SCIMUsers(r.Context(), email, externalID, start-1, count)
	if err != nil {
		return err
	}
	for _, u := range users {
		resources = append(resources, s.user(u))
	}
	return list(w, total, start, resources)
}

func (s *server) getUser(w http.ResponseWriter, r *http.Request, _ store.User) error {
	return s.respondUser(w, r, r.PathValue("id"), http.StatusOK)
}

func (s *server) createUser(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var body json.RawMessage
	if err := decode(r, &body); err != nil {
		return err
	}
	st := &userState{active: true}
	if err := st.apply("add", "", body); err != nil {
		return err
	}
	email, name, err := st.final()
	if err != nil {
		return err
	}
	if err := s.checkManager(r, st.managerID, ""); err != nil {
		return err
	}
	status := "invited"
	if !st.active {
		status = "suspended"
	}
	var userID string
	err = s.st.Tx(r.Context(), func(tx *store.Store) error {
		u, err := tx.CreateUser(r.Context(), store.NewUser{Email: email, Name: name, Title: st.title, Department: st.department, Status: status, ManagerID: st.managerID, Source: "SCIM · " + actor.Name})
		if err != nil {
			return err
		}
		userID = u.ID
		if err := tx.UpdateSCIMUser(r.Context(), store.SCIMUser{ID: u.ID, Email: email, Name: name, Title: st.title, Department: st.department, ManagerID: st.managerID, ExternalID: st.externalID}); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "scim.user.create", Summary: "Provisioned through SCIM by " + actor.Name, TargetType: "user", TargetID: u.ID, TargetLabel: name})
	})
	if errors.Is(err, store.ErrConflict) {
		return fail(http.StatusConflict, "uniqueness", "A user with userName "+email+" or the same externalId already exists in Halo. Find them with filter=userName eq \""+email+"\" and update them instead.")
	}
	if err != nil {
		return err
	}
	return s.respondUser(w, r, userID, http.StatusCreated)
}

func (s *server) replaceUser(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var body json.RawMessage
	if err := decode(r, &body); err != nil {
		return err
	}
	return s.changeUser(w, r, actor, func(st *userState) error {
		st.userName, st.email, st.title, st.department, st.managerID = "", "", "", "", nil
		return st.apply("replace", "", body)
	})
}

func (s *server) patchUser(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var body patchOps
	if err := decode(r, &body); err != nil {
		return err
	}
	return s.changeUser(w, r, actor, func(st *userState) error { return body.each(st.apply) })
}

func (s *server) changeUser(w http.ResponseWriter, r *http.Request, actor store.User, apply func(*userState) error) error {
	cur, err := s.st.GetSCIMUser(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	st := newUserState(cur)
	if err := apply(st); err != nil {
		return err
	}
	email, name, err := st.final()
	if err != nil {
		return err
	}
	var changed []string
	for _, c := range []struct {
		label   string
		differs bool
	}{
		{"userName", cur.Email != email},
		{"name", cur.Name != name},
		{"title", cur.Title != st.title},
		{"department", cur.Department != st.department},
		{"manager", deref(cur.ManagerID) != deref(st.managerID)},
		{"externalId", deref(cur.ExternalID) != deref(st.externalID)},
	} {
		if c.differs {
			changed = append(changed, c.label)
		}
	}
	wasActive := cur.Status != "suspended"
	if len(changed) == 0 && wasActive == st.active {
		return write(w, http.StatusOK, s.user(cur))
	}
	if err := s.guard(actor, cur); err != nil {
		return err
	}
	if slices.Contains(changed, "manager") {
		if err := s.checkManager(r, st.managerID, cur.ID); err != nil {
			return err
		}
	}
	err = s.st.Tx(r.Context(), func(tx *store.Store) error {
		target := store.AuditEvent{TargetType: "user", TargetID: cur.ID, TargetLabel: name}
		if len(changed) > 0 {
			next := store.SCIMUser{ID: cur.ID, Email: email, Name: name, Title: st.title, Department: st.department, ManagerID: st.managerID, ExternalID: st.externalID}
			if err := tx.UpdateSCIMUser(r.Context(), next); err != nil {
				return err
			}
			target.Action, target.Summary = "scim.user.update", "Updated "+strings.Join(changed, ", ")+" through SCIM"
			if err := record(r, tx, actor, target); err != nil {
				return err
			}
		}
		if wasActive == st.active {
			return nil
		}
		if !st.active {
			if err := tx.SetUserStatus(r.Context(), cur.ID, "suspended"); err != nil {
				return err
			}
			revoked, err := tx.RevokeUserSessions(r.Context(), cur.ID, "")
			if err != nil {
				return err
			}
			target.Action, target.Summary = "scim.user.suspend", "Suspended through SCIM and ended "+plural(revoked, "session")
			return record(r, tx, actor, target)
		}
		full, err := tx.GetUser(r.Context(), cur.ID)
		if err != nil {
			return err
		}
		status := "active"
		if full.LastSignInAt == nil && len(full.Methods) == 0 {
			status = "invited"
		}
		if err := tx.SetUserStatus(r.Context(), cur.ID, status); err != nil {
			return err
		}
		target.Action, target.Summary = "scim.user.restore", "Restored through SCIM"
		return record(r, tx, actor, target)
	})
	if errors.Is(err, store.ErrConflict) {
		return fail(http.StatusConflict, "uniqueness", "Another user in Halo already has the userName "+email+" or the same externalId.")
	}
	if err != nil {
		return err
	}
	return s.respondUser(w, r, cur.ID, http.StatusOK)
}

func (s *server) guard(actor store.User, u store.SCIMUser) error {
	if u.Privileged && !actor.HasRole("global_admin") {
		return fail(http.StatusForbidden, "", u.Name+" holds an administrator role in Halo, so only a service account with the Global administrator role can change their account.")
	}
	return nil
}

func (s *server) deleteUser(w http.ResponseWriter, r *http.Request, actor store.User) error {
	u, err := s.st.GetSCIMUser(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	if err := s.guard(actor, u); err != nil {
		return err
	}
	err = s.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.SetUserStatus(r.Context(), u.ID, "deprovisioned"); err != nil {
			return err
		}
		revoked, err := tx.RevokeUserSessions(r.Context(), u.ID, "")
		if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "scim.user.deprovision", Summary: "Deprovisioned through SCIM and ended " + plural(revoked, "session"), TargetType: "user", TargetID: u.ID, TargetLabel: u.Name})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type member struct {
	Value   string `json:"value"`
	Display string `json:"display,omitempty"`
	Ref     string `json:"$ref,omitempty"`
	Type    string `json:"type,omitempty"`
}

type group struct {
	Schemas     []string `json:"schemas"`
	ID          string   `json:"id"`
	ExternalID  string   `json:"externalId,omitempty"`
	DisplayName string   `json:"displayName"`
	Members     []member `json:"members,omitempty"`
	Meta        Meta     `json:"meta"`
}

func (s *server) group(g store.SCIMGroup, withMembers bool) group {
	res := group{Schemas: []string{GroupSchema}, ID: g.ID, ExternalID: deref(g.ExternalID), DisplayName: g.Name,
		Meta: Meta{ResourceType: "Group", Created: g.CreatedAt, LastModified: g.CreatedAt, Location: s.base + "/Groups/" + g.ID}}
	if !withMembers {
		return res
	}
	for _, m := range g.Members {
		res.Members = append(res.Members, member{Value: m.ID, Display: m.Name, Ref: s.base + "/Users/" + m.ID, Type: "User"})
	}
	return res
}

func (s *server) respondGroup(w http.ResponseWriter, r *http.Request, groupID string, status int) error {
	g, err := s.st.GetSCIMGroup(r.Context(), groupID)
	if err != nil {
		return err
	}
	if status == http.StatusCreated {
		w.Header().Set("Location", s.base+"/Groups/"+groupID)
	}
	return write(w, status, s.group(g, true))
}

type groupState struct {
	name       string
	externalID *string
	members    map[string]bool
}

func (st *groupState) apply(op, path string, value json.RawMessage) error {
	if m := memberPath.FindStringSubmatch(path); m != nil {
		if op == "remove" {
			delete(st.members, m[1])
		}
		return nil
	}
	var err error
	switch strings.TrimPrefix(strings.ToLower(path), coreGroupPrefix) {
	case "":
		var fields map[string]json.RawMessage
		if json.Unmarshal(value, &fields) != nil {
			return fail(http.StatusBadRequest, "invalidValue", `Expected an object, for example {"displayName": "Engineering"}.`)
		}
		for key, v := range fields {
			if err := st.apply(op, key, v); err != nil {
				return err
			}
		}
	case "displayname":
		if op != "remove" {
			st.name, err = text(value)
		}
	case "externalid":
		st.externalID = nil
		if op != "remove" {
			st.externalID, err = optional(value)
		}
	case "members":
		var refs []struct {
			Value string `json:"value"`
		}
		present := len(value) > 0 && string(value) != "null"
		if present && json.Unmarshal(value, &refs) != nil {
			return fail(http.StatusBadRequest, "invalidValue", `members must be a list such as [{"value": "usr_…"}].`)
		}
		if op == "replace" || (op == "remove" && !present) {
			clear(st.members)
		}
		for _, ref := range refs {
			if op == "remove" {
				delete(st.members, ref.Value)
			} else if ref.Value != "" {
				st.members[ref.Value] = true
			}
		}
	}
	return err
}

func (s *server) people(r *http.Request, ids []string) (map[string]string, error) {
	people, err := s.st.SCIMPeople(r.Context(), ids)
	if err != nil {
		return nil, err
	}
	for _, userID := range ids {
		if _, ok := people[userID]; !ok {
			return nil, fail(http.StatusBadRequest, "invalidValue", "User "+userID+" does not exist in Halo, so it cannot be a group member. Provision the user first.")
		}
	}
	return people, nil
}

func names(list []string) string {
	if len(list) > 3 {
		return plural(len(list), "member")
	}
	return strings.Join(list, ", ")
}

func (s *server) listGroups(w http.ResponseWriter, r *http.Request, _ store.User) error {
	attr, value, err := parseFilter(r.URL.Query().Get("filter"), `displayName eq "Engineering"`, "displayname", "externalid")
	if err != nil {
		return err
	}
	start, count := paging(r)
	resources := []group{}
	if attr != "" && value == "" {
		return list(w, 0, start, resources)
	}
	name, externalID := value, ""
	if attr == "externalid" {
		name, externalID = "", value
	}
	groups, total, err := s.st.SCIMGroups(r.Context(), name, externalID, start-1, count)
	if err != nil {
		return err
	}
	withMembers := !strings.Contains(strings.ToLower(r.URL.Query().Get("excludedAttributes")), "members")
	for _, g := range groups {
		resources = append(resources, s.group(g, withMembers))
	}
	return list(w, total, start, resources)
}

func (s *server) getGroup(w http.ResponseWriter, r *http.Request, _ store.User) error {
	return s.respondGroup(w, r, r.PathValue("id"), http.StatusOK)
}

func (s *server) createGroup(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var body json.RawMessage
	if err := decode(r, &body); err != nil {
		return err
	}
	st := &groupState{members: map[string]bool{}}
	if err := st.apply("add", "", body); err != nil {
		return err
	}
	if st.name == "" || len(st.name) > 200 {
		return fail(http.StatusBadRequest, "invalidValue", "displayName is required and must be under 200 characters.")
	}
	ids := slices.Sorted(maps.Keys(st.members))
	if _, err := s.people(r, ids); err != nil {
		return err
	}
	var groupID string
	err := s.st.Tx(r.Context(), func(tx *store.Store) error {
		g, err := tx.CreateGroup(r.Context(), store.NewGroup{Name: st.name, Kind: "assigned", Source: "SCIM · " + actor.Name})
		if err != nil {
			return err
		}
		groupID = g.ID
		if st.externalID != nil {
			if err := tx.UpdateSCIMGroup(r.Context(), g.ID, g.Name, st.externalID); err != nil {
				return err
			}
		}
		for _, userID := range ids {
			if err := tx.AddGroupMember(r.Context(), g.ID, userID); err != nil {
				return err
			}
		}
		return record(r, tx, actor, store.AuditEvent{Action: "scim.group.create", Summary: "Created through SCIM with " + plural(len(ids), "member"), TargetType: "group", TargetID: g.ID, TargetLabel: g.Name})
	})
	if errors.Is(err, store.ErrConflict) {
		return fail(http.StatusConflict, "uniqueness", "A group named "+st.name+" or with the same externalId already exists in Halo. Find it with filter=displayName eq \""+st.name+"\".")
	}
	if err != nil {
		return err
	}
	return s.respondGroup(w, r, groupID, http.StatusCreated)
}

func (s *server) replaceGroup(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var body json.RawMessage
	if err := decode(r, &body); err != nil {
		return err
	}
	return s.changeGroup(w, r, actor, func(st *groupState) error {
		clear(st.members)
		return st.apply("replace", "", body)
	})
}

func (s *server) patchGroup(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var body patchOps
	if err := decode(r, &body); err != nil {
		return err
	}
	return s.changeGroup(w, r, actor, func(st *groupState) error { return body.each(st.apply) })
}

func (s *server) changeGroup(w http.ResponseWriter, r *http.Request, actor store.User, apply func(*groupState) error) error {
	cur, err := s.st.GetSCIMGroup(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	st := &groupState{name: cur.Name, externalID: cur.ExternalID, members: map[string]bool{}}
	for _, m := range cur.Members {
		st.members[m.ID] = true
	}
	if err := apply(st); err != nil {
		return err
	}
	if st.name == "" || len(st.name) > 200 {
		return fail(http.StatusBadRequest, "invalidValue", "displayName is required and must be under 200 characters.")
	}
	current := map[string]bool{}
	var addIDs, added, removeIDs, removed []string
	for _, m := range cur.Members {
		current[m.ID] = true
		if !st.members[m.ID] {
			removeIDs, removed = append(removeIDs, m.ID), append(removed, m.Name)
		}
	}
	for _, userID := range slices.Sorted(maps.Keys(st.members)) {
		if !current[userID] {
			addIDs = append(addIDs, userID)
		}
	}
	people, err := s.people(r, addIDs)
	if err != nil {
		return err
	}
	for _, userID := range addIDs {
		added = append(added, people[userID])
	}
	var parts []string
	if st.name != cur.Name {
		parts = append(parts, "renamed to "+st.name)
	}
	if deref(st.externalID) != deref(cur.ExternalID) {
		parts = append(parts, "updated externalId")
	}
	if len(added) > 0 {
		parts = append(parts, "added "+names(added))
	}
	if len(removed) > 0 {
		parts = append(parts, "removed "+names(removed))
	}
	if len(parts) == 0 {
		return write(w, http.StatusOK, s.group(cur, true))
	}
	summary := strings.Join(parts, "; ") + " through SCIM"
	err = s.st.Tx(r.Context(), func(tx *store.Store) error {
		if st.name != cur.Name || deref(st.externalID) != deref(cur.ExternalID) {
			if err := tx.UpdateSCIMGroup(r.Context(), cur.ID, st.name, st.externalID); err != nil {
				return err
			}
		}
		for _, userID := range addIDs {
			if err := tx.AddGroupMember(r.Context(), cur.ID, userID); err != nil {
				return err
			}
		}
		for _, userID := range removeIDs {
			if err := tx.RemoveGroupMember(r.Context(), cur.ID, userID); err != nil {
				return err
			}
		}
		return record(r, tx, actor, store.AuditEvent{Action: "scim.group.update", Summary: strings.ToUpper(summary[:1]) + summary[1:], TargetType: "group", TargetID: cur.ID, TargetLabel: st.name})
	})
	if errors.Is(err, store.ErrConflict) {
		return fail(http.StatusConflict, "uniqueness", "Another group in Halo is already named "+st.name+" or has the same externalId.")
	}
	if err != nil {
		return err
	}
	return s.respondGroup(w, r, cur.ID, http.StatusOK)
}

func (s *server) deleteGroup(w http.ResponseWriter, r *http.Request, actor store.User) error {
	g, err := s.st.GetSCIMGroup(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	err = s.st.Tx(r.Context(), func(tx *store.Store) error {
		var inUse *store.GroupInUseError
		if err := tx.DeleteGroup(r.Context(), g.ID); errors.As(err, &inUse) {
			return fail(http.StatusConflict, "mutability", inUse.Error())
		} else if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "scim.group.delete", Summary: "Deleted through SCIM", TargetType: "group", TargetID: g.ID, TargetLabel: g.Name})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
