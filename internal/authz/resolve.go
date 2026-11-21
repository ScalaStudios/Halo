package authz

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"halo/internal/httpx"
	"halo/internal/store"
)

const (
	maxDepth   = 25
	maxFanOut  = 1000
	maxResults = 1000
	checkReads = 1000
	listReads  = 10000
)

var (
	errDepth = httpx.Fail(http.StatusUnprocessableEntity, "ERR_AUTHZ_DEPTH", fmt.Sprintf("Answering this followed more than %d relations in a row. Shorten the chains in the model or in the tuples.", maxDepth))
	errWidth = httpx.Fail(http.StatusUnprocessableEntity, "ERR_AUTHZ_WIDTH", "Answering this needed more tuples than Halo reads for one request. Ask about fewer objects or flatten the tuples.")
)

type resolver struct {
	ctx    context.Context
	st     *store.Store
	app    store.Application
	types  store.AuthzTypes
	user   store.User
	held   []string
	budget int
	cache  map[string][]store.Tuple
	seen   map[string]bool
}

type node struct {
	Object   string   `json:"object"`
	Relation string   `json:"relation"`
	Users    []string `json:"users"`
	Groups   []string `json:"groups"`
	Roles    []string `json:"roles"`
	Children []*node  `json:"children"`
}

func newResolver(ctx context.Context, st *store.Store, c caller, user store.User, budget int) *resolver {
	return &resolver{ctx: ctx, st: st, app: c.app, types: c.types, user: user, held: c.app.RoleKeys(user), budget: budget, cache: map[string][]store.Tuple{}, seen: map[string]bool{}}
}

func active(u store.User) bool {
	return u.Status != "suspended" && u.Status != "deprovisioned"
}

func (r *resolver) related(o object, relation string) ([]store.Tuple, error) {
	key := o.String() + "#" + relation
	if tuples, ok := r.cache[key]; ok {
		return tuples, nil
	}
	if r.budget--; r.budget < 0 {
		return nil, errWidth
	}
	tuples, err := r.st.RelatedTuples(r.ctx, r.app.ID, o.typ, o.id, relation, r.user.ID, r.user.GroupIDs, maxFanOut+1)
	if err != nil {
		return nil, err
	}
	if len(tuples) > maxFanOut {
		return nil, errWidth
	}
	drop, err := r.unassigned(tuples)
	if err != nil {
		return nil, err
	}
	tuples = slices.DeleteFunc(tuples, drop)
	r.cache[key] = tuples
	return tuples, nil
}

func (r *resolver) unassigned(tuples []store.Tuple) (func(store.Tuple) bool, error) {
	people := map[string]bool{r.user.ID: r.user.ID != ""}
	var ids []string
	for _, t := range tuples {
		if _, known := people[t.SubjectID]; t.SubjectType == "user" && !known {
			ids = append(ids, t.SubjectID)
		}
	}
	if len(ids) > 0 {
		users, err := r.st.UsersByID(r.ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, u := range users {
			people[u.ID] = slices.Contains(u.AppIDs, r.app.ID)
		}
	}
	return func(t store.Tuple) bool {
		return t.SubjectType == "user" && !people[t.SubjectID] || t.SubjectType == "group" && !slices.Contains(r.app.GroupIDs, t.SubjectID)
	}, nil
}

func (r *resolver) parents(o object, f store.AuthzFrom) ([]object, error) {
	via, ok := r.types[o.typ][f.Via]
	if !ok {
		return nil, nil
	}
	tuples, err := r.related(o, f.Via)
	var found []object
	for _, t := range tuples {
		if s := subjectOf(t); s.rel == "" && !slices.Contains(reserved, s.typ) && slices.Contains(via.Subjects, s.typ) {
			found = append(found, s.object)
		}
	}
	return found, err
}

func (r *resolver) check(o object, relation string, depth int) ([]string, error) {
	key := o.String() + "#" + relation
	if r.seen[key] {
		return nil, nil
	}
	r.seen[key] = true
	if depth > maxDepth {
		return nil, errDepth
	}
	rel, ok := r.types[o.typ][relation]
	if !ok {
		return nil, nil
	}
	step := func(path []string) []string { return append([]string{key}, path...) }
	for _, role := range rel.Roles {
		if slices.Contains(r.held, role) {
			return step([]string{"role:" + role}), nil
		}
	}
	if len(rel.Subjects) > 0 {
		tuples, err := r.related(o, relation)
		if err != nil {
			return nil, err
		}
		for _, t := range tuples {
			s := subjectOf(t)
			switch {
			case !slices.Contains(rel.Subjects, s.kind()):
			case s.typ == "user" || s.typ == "group":
				return step([]string{s.String()}), nil
			case s.rel != "":
				if path, err := r.check(s.object, s.rel, depth+1); err != nil || path != nil {
					return step(path), err
				}
			}
		}
	}
	for _, other := range rel.Union {
		if path, err := r.check(o, other, depth+1); err != nil || path != nil {
			return step(path), err
		}
	}
	for _, f := range rel.From {
		parents, err := r.parents(o, f)
		if err != nil {
			return nil, err
		}
		for _, p := range parents {
			if path, err := r.check(p, f.Relation, depth+1); err != nil || path != nil {
				return step(path), err
			}
		}
	}
	return nil, nil
}

func (r *resolver) expand(o object, relation string, depth int) (*node, error) {
	n := &node{Object: o.String(), Relation: relation, Users: []string{}, Groups: []string{}, Roles: []string{}, Children: []*node{}}
	key := o.String() + "#" + relation
	if r.seen[key] {
		return n, nil
	}
	r.seen[key] = true
	if depth > maxDepth {
		return nil, errDepth
	}
	rel, ok := r.types[o.typ][relation]
	if !ok {
		return n, nil
	}
	n.Roles = append(n.Roles, rel.Roles...)
	add := func(o object, relation string) error {
		child, err := r.expand(o, relation, depth+1)
		if err == nil {
			n.Children = append(n.Children, child)
		}
		return err
	}
	if len(rel.Subjects) > 0 {
		tuples, err := r.related(o, relation)
		if err != nil {
			return nil, err
		}
		for _, t := range tuples {
			s := subjectOf(t)
			switch {
			case !slices.Contains(rel.Subjects, s.kind()):
			case s.typ == "user":
				n.Users = append(n.Users, s.id)
			case s.typ == "group":
				n.Groups = append(n.Groups, s.id)
			case s.rel != "":
				if err := add(s.object, s.rel); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, other := range rel.Union {
		if err := add(o, other); err != nil {
			return nil, err
		}
	}
	for _, f := range rel.From {
		parents, err := r.parents(o, f)
		if err != nil {
			return nil, err
		}
		for _, p := range parents {
			if err := add(p, f.Relation); err != nil {
				return nil, err
			}
		}
	}
	return n, nil
}

func (r *resolver) collect(n *node, users, groups map[string]bool) {
	for _, id := range n.Users {
		users[id] = true
	}
	for _, id := range n.Groups {
		groups[id] = true
	}
	for _, role := range r.app.Roles {
		if slices.Contains(n.Roles, role.Key) {
			for _, id := range role.UserIDs {
				users[id] = true
			}
			for _, id := range role.GroupIDs {
				groups[id] = true
			}
		}
	}
	for _, child := range n.Children {
		r.collect(child, users, groups)
	}
}

func (r *resolver) people(n *node) ([]string, bool, error) {
	users, groups := map[string]bool{}, map[string]bool{}
	r.collect(n, users, groups)
	if len(users)+len(groups) == 0 {
		return []string{}, false, nil
	}
	all, err := r.st.ListUsers(r.ctx)
	if err != nil {
		return nil, false, err
	}
	subjects := []string{}
	for _, u := range all {
		if active(u) && slices.Contains(u.AppIDs, r.app.ID) && (users[u.ID] || slices.ContainsFunc(u.GroupIDs, func(g string) bool { return groups[g] })) {
			if len(subjects) == maxResults {
				return subjects, true, nil
			}
			subjects = append(subjects, "user:"+u.ID)
		}
	}
	return subjects, false, nil
}

func (r *resolver) objects(typ, relation string) ([]string, bool, error) {
	ids, err := r.st.AuthzObjects(r.ctx, r.app.ID, typ, maxResults+1)
	if err != nil {
		return nil, false, err
	}
	truncated := len(ids) > maxResults
	found := []string{}
	for _, id := range ids[:min(len(ids), maxResults)] {
		r.seen = map[string]bool{}
		path, err := r.check(object{typ, id}, relation, 0)
		if err != nil {
			return nil, false, err
		}
		if path != nil {
			found = append(found, typ+":"+id)
		}
	}
	return found, truncated, nil
}
