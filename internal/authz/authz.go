package authz

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"halo/internal/httpx"
	"halo/internal/store"
)

const (
	rateLimit = 6000
	maxWrites = 100
)

var errClient = httpx.Fail(http.StatusUnauthorized, "ERR_UNAUTHENTICATED", "Authenticate as the application with HTTP Basic: its client ID as the user name and one of its client secrets as the password.")

type handlers struct {
	st   *store.Store
	auth *httpx.Auth
}

type caller struct {
	app   store.Application
	types store.AuthzTypes
}

type tupleKey struct {
	Object   string `json:"object"`
	Relation string `json:"relation"`
	Subject  string `json:"subject"`
}

type tupleView struct {
	tupleKey
	CreatedAt time.Time `json:"createdAt"`
}

type question struct {
	Object   string `json:"object"`
	Relation string `json:"relation"`
	Subject  string `json:"subject"`
	Type     string `json:"type"`
}

type answer struct {
	Allowed bool     `json:"allowed"`
	Path    []string `json:"path"`
}

func Register(mux *http.ServeMux, d httpx.Deps) error {
	h := &handlers{st: d.Store, auth: d.Auth}
	readers, writers := []string{"security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"}, []string{"app_admin"}
	route := func(pattern string, roles []string, fn func(http.ResponseWriter, *http.Request, store.User) error) {
		mux.Handle(pattern, httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
			actor, err := httpx.RequireRole(r, roles...)
			if err != nil {
				return err
			}
			return fn(w, r, actor)
		}))
	}
	machine := func(pattern string, fn func(http.ResponseWriter, *http.Request, caller) error) {
		mux.Handle(pattern, httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
			c, err := h.caller(w, r)
			if err != nil {
				return err
			}
			return fn(w, r, c)
		}))
	}
	route("GET /api/v1/applications/{id}/authorization", readers, h.getModel)
	route("PUT /api/v1/applications/{id}/authorization", writers, h.putModel)
	route("DELETE /api/v1/applications/{id}/authorization", writers, h.deleteModel)
	route("POST /api/v1/applications/{id}/authorization/check", readers, h.inspect)
	machine("POST /api/v1/authz/check", h.check)
	machine("POST /api/v1/authz/expand", h.expand)
	machine("POST /api/v1/authz/list-objects", h.listObjects)
	machine("POST /api/v1/authz/list-subjects", h.listSubjects)
	machine("GET /api/v1/authz/tuples", h.readTuples)
	machine("POST /api/v1/authz/tuples", h.writeTuples)
	return nil
}

func (h *handlers) caller(w http.ResponseWriter, r *http.Request) (caller, error) {
	clientID, secret, ok := r.BasicAuth()
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="halo"`)
		return caller{}, errClient
	}
	app, err := h.st.VerifyClientSecret(r.Context(), clientID, secret)
	if errors.Is(err, store.ErrNotFound) || err == nil && (app.Status != "active" || !modelled(app)) {
		return caller{}, errClient
	}
	if err != nil {
		return caller{}, err
	}
	if e := h.auth.Limit(w, r, app.ID, "authz", rateLimit, "application"); e != nil {
		return caller{}, e
	}
	m, err := h.st.GetAuthzModel(r.Context(), app.ID)
	if errors.Is(err, store.ErrNotFound) {
		return caller{}, httpx.Fail(http.StatusForbidden, "ERR_NO_MODEL", app.Name+" has no authorization model. An application administrator adds one on its Authorization tab in the Halo console.")
	}
	return caller{app, m.Types}, err
}

func modelled(app store.Application) bool {
	return (app.Protocol == "oidc" || app.Protocol == "oauth") && (app.Type == "web" || app.Type == "service") && app.ClientID != store.CLIClientID
}

func (h *handlers) application(r *http.Request) (store.Application, error) {
	app, err := h.st.GetApplication(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		return app, httpx.NotFound("This application")
	}
	return app, err
}

func record(r *http.Request, tx *store.Store, actor store.User, e store.AuditEvent) error {
	e.ActorID, e.IP = &actor.ID, httpx.ClientIP(r)
	return tx.RecordAudit(r.Context(), e)
}

func (h *handlers) getModel(w http.ResponseWriter, r *http.Request, _ store.User) error {
	app, err := h.application(r)
	if err != nil {
		return err
	}
	m, err := h.st.GetAuthzModel(r.Context(), app.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	return httpx.JSON(w, http.StatusOK, m)
}

func (h *handlers) putModel(w http.ResponseWriter, r *http.Request, actor store.User) error {
	app, err := h.application(r)
	if err != nil {
		return err
	}
	if !modelled(app) {
		return httpx.Invalid(app.Name + " can't call the authorization API. Only web and service applications with a client secret can.")
	}
	var in struct {
		Types store.AuthzTypes `json:"types"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if in.Types == nil {
		in.Types = store.AuthzTypes{}
	}
	if err := validate(in.Types, app); err != nil {
		return err
	}
	current, err := h.st.GetAuthzModel(r.Context(), app.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	changes := store.Diff(current.Types, in.Types)
	if current.UpdatedAt != nil && len(changes) == 0 {
		return httpx.JSON(w, http.StatusOK, current)
	}
	relations := 0
	for _, rels := range in.Types {
		relations += len(rels)
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.SaveAuthzModel(r.Context(), app.ID, in.Types); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "application.authorization.update", Summary: fmt.Sprintf("Saved the authorization model with %s and %s", plural(len(in.Types), "type"), plural(relations, "relation")),
			TargetType: "application", TargetID: app.ID, TargetLabel: app.Name, Changes: changes})
	})
	if err != nil {
		return err
	}
	saved, err := h.st.GetAuthzModel(r.Context(), app.ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, saved)
}

func (h *handlers) deleteModel(w http.ResponseWriter, r *http.Request, actor store.User) error {
	app, err := h.application(r)
	if err != nil {
		return err
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		removed, err := tx.DeleteAuthzModel(r.Context(), app.ID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.NotFound("This authorization model")
		}
		if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "application.authorization.delete", Summary: "Removed the authorization model and " + plural(int(removed), "tuple"),
			TargetType: "application", TargetID: app.ID, TargetLabel: app.Name})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *handlers) inspect(w http.ResponseWriter, r *http.Request, _ store.User) error {
	app, err := h.application(r)
	if err != nil {
		return err
	}
	m, err := h.st.GetAuthzModel(r.Context(), app.ID)
	if errors.Is(err, store.ErrNotFound) {
		return httpx.Invalid(app.Name + " has no authorization model yet. Save one first.")
	}
	if err != nil {
		return err
	}
	return h.check(w, r, caller{app, m.Types})
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func (c caller) relation(o object, relation string) error {
	if !hasRelation(c.types, o.typ, relation) {
		return httpx.Invalid(fmt.Sprintf("The model has no relation %q on %q.", relation, o.typ))
	}
	return nil
}

func (h *handlers) ask(r *http.Request, c caller, byType bool) (question, object, error) {
	var q question
	if err := httpx.Decode(r, &q); err != nil {
		return q, object{}, err
	}
	o, ok := parseObject(q.Object)
	if byType {
		o, ok = object{typ: q.Type}, namePattern.MatchString(q.Type)
	}
	if !ok {
		return q, o, httpx.Invalid("Name the object as type:id, such as document:readme, or the type alone when listing objects.")
	}
	return q, o, c.relation(o, q.Relation)
}

func (h *handlers) person(r *http.Request, raw string) (store.User, error) {
	s, ok := parseSubject(raw)
	if !ok || s.typ != "user" || s.rel != "" {
		return store.User{}, httpx.Invalid("Ask about a person as user:id, with the Halo user ID that is the sub claim of their tokens.")
	}
	u, err := h.st.GetUser(r.Context(), s.id)
	if errors.Is(err, store.ErrNotFound) || err == nil && !active(u) {
		return store.User{}, nil
	}
	return u, err
}

func (h *handlers) check(w http.ResponseWriter, r *http.Request, c caller) error {
	q, o, err := h.ask(r, c, false)
	if err != nil {
		return err
	}
	u, err := h.person(r, q.Subject)
	if err != nil {
		return err
	}
	a := answer{Path: []string{}}
	if u.ID != "" {
		path, err := newResolver(r.Context(), h.st, c, u, checkReads).check(o, q.Relation, 0)
		if err != nil {
			return err
		}
		a.Allowed, a.Path = path != nil, append(a.Path, path...)
	}
	return httpx.JSON(w, http.StatusOK, a)
}

func (h *handlers) expand(w http.ResponseWriter, r *http.Request, c caller) error {
	q, o, err := h.ask(r, c, false)
	if err != nil {
		return err
	}
	n, err := newResolver(r.Context(), h.st, c, store.User{}, checkReads).expand(o, q.Relation, 0)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, n)
}

func (h *handlers) listSubjects(w http.ResponseWriter, r *http.Request, c caller) error {
	q, o, err := h.ask(r, c, false)
	if err != nil {
		return err
	}
	res := newResolver(r.Context(), h.st, c, store.User{}, checkReads)
	n, err := res.expand(o, q.Relation, 0)
	if err != nil {
		return err
	}
	subjects, truncated, err := res.people(n)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"subjects": subjects, "truncated": truncated})
}

func (h *handlers) listObjects(w http.ResponseWriter, r *http.Request, c caller) error {
	q, o, err := h.ask(r, c, true)
	if err != nil {
		return err
	}
	u, err := h.person(r, q.Subject)
	if err != nil {
		return err
	}
	objects, truncated := []string{}, false
	if u.ID != "" {
		if objects, truncated, err = newResolver(r.Context(), h.st, c, u, listReads).objects(o.typ, q.Relation); err != nil {
			return err
		}
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"objects": objects, "truncated": truncated})
}

func (h *handlers) readTuples(w http.ResponseWriter, r *http.Request, c caller) error {
	var f store.Tuple
	q := r.URL.Query()
	if raw := q.Get("object"); raw != "" {
		o, ok := parseObject(raw)
		if !ok && namePattern.MatchString(raw) {
			o, ok = object{typ: raw}, true
		}
		if !ok {
			return httpx.Invalid("Filter by object as type or type:id, such as document or document:readme.")
		}
		f.ObjectType, f.ObjectID = o.typ, o.id
	}
	if raw := q.Get("subject"); raw != "" {
		s, ok := parseSubject(raw)
		if !ok {
			return httpx.Invalid("Filter by subject as type:id or type:id#relation, such as user:usr_… or team:design#member.")
		}
		f.SubjectType, f.SubjectID, f.SubjectRelation = s.typ, s.id, s.rel
	}
	f.Relation = q.Get("relation")
	tuples, err := h.st.ReadTuples(r.Context(), c.app.ID, f, maxResults+1)
	if err != nil {
		return err
	}
	views := []tupleView{}
	for _, t := range tuples[:min(len(tuples), maxResults)] {
		views = append(views, tupleView{tupleKey{object{t.ObjectType, t.ObjectID}.String(), t.Relation, subjectOf(t).String()}, t.CreatedAt})
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"tuples": views, "truncated": len(tuples) > maxResults})
}

func (c caller) tuple(k tupleKey, write bool) (store.Tuple, error) {
	o, ok := parseObject(k.Object)
	s, valid := parseSubject(k.Subject)
	if !ok || !valid || slices.Contains(reserved, o.typ) || !namePattern.MatchString(k.Relation) {
		return store.Tuple{}, httpx.Invalid(`Write each tuple as {"object": "document:readme", "relation": "viewer", "subject": "user:usr_…"}. Subjects are user:id, group:id, type:id or type:id#relation.`)
	}
	if rel, ok := c.types[o.typ][k.Relation]; write && (!ok || !slices.Contains(rel.Subjects, s.kind())) {
		return store.Tuple{}, httpx.Invalid(fmt.Sprintf("%s#%s doesn't take %s subjects in the model, so %s can't be written.", o.typ, k.Relation, s.kind(), k.Subject))
	}
	return store.Tuple{ObjectType: o.typ, ObjectID: o.id, Relation: k.Relation, SubjectType: s.typ, SubjectID: s.id, SubjectRelation: s.rel}, nil
}

func (h *handlers) writeTuples(w http.ResponseWriter, r *http.Request, c caller) error {
	var in struct {
		Writes  []tupleKey `json:"writes"`
		Deletes []tupleKey `json:"deletes"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if len(in.Writes)+len(in.Deletes) > maxWrites {
		return httpx.Invalid(fmt.Sprintf("Send up to %d writes and deletes in one request.", maxWrites))
	}
	var writes, deletes []store.Tuple
	for _, k := range in.Writes {
		t, err := c.tuple(k, true)
		if err != nil {
			return err
		}
		writes = append(writes, t)
	}
	for _, k := range in.Deletes {
		t, err := c.tuple(k, false)
		if err != nil {
			return err
		}
		deletes = append(deletes, t)
	}
	if err := h.st.WriteTuples(r.Context(), c.app.ID, writes, deletes); errors.Is(err, store.ErrNotFound) {
		return httpx.Invalid("A person or group in these tuples is not in Halo. Use their Halo IDs, such as user:usr_… and group:grp_….")
	} else if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
