package authz

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"halo/internal/httpx"
	"halo/internal/store"
)

const (
	maxTypes     = 50
	maxRelations = 500
)

var (
	namePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	idPattern   = regexp.MustCompile(`^[A-Za-z0-9_.:@|/+=-]{1,256}$`)
	reserved    = []string{"user", "group"}
)

type object struct{ typ, id string }

func (o object) String() string { return o.typ + ":" + o.id }

type subject struct {
	object
	rel string
}

func (s subject) String() string {
	if s.rel == "" {
		return s.object.String()
	}
	return s.object.String() + "#" + s.rel
}

func (s subject) kind() string {
	if s.rel == "" {
		return s.typ
	}
	return s.typ + "#" + s.rel
}

func parseObject(raw string) (object, bool) {
	typ, id, ok := strings.Cut(raw, ":")
	return object{typ, id}, ok && namePattern.MatchString(typ) && idPattern.MatchString(id)
}

func parseSubject(raw string) (subject, bool) {
	head, rel, hasRel := strings.Cut(raw, "#")
	o, ok := parseObject(head)
	return subject{o, rel}, ok && (!hasRel || namePattern.MatchString(rel) && !slices.Contains(reserved, o.typ))
}

func subjectOf(t store.Tuple) subject {
	return subject{object{t.SubjectType, t.SubjectID}, t.SubjectRelation}
}

func hasRelation(types store.AuthzTypes, typ, relation string) bool {
	_, ok := types[typ][relation]
	return ok
}

func validate(types store.AuthzTypes, app store.Application) error {
	if len(types) > maxTypes {
		return httpx.Invalid(fmt.Sprintf("A model holds up to %d types.", maxTypes))
	}
	total := 0
	for _, typ := range slices.Sorted(maps.Keys(types)) {
		if !namePattern.MatchString(typ) || slices.Contains(reserved, typ) {
			return httpx.Invalid(fmt.Sprintf("%q can't be a type. Start with a lowercase letter and use up to 64 lowercase letters, digits, dashes and underscores. user and group are Halo's people and groups.", typ))
		}
		relations := types[typ]
		total += len(relations)
		for _, name := range slices.Sorted(maps.Keys(relations)) {
			rel, where := relations[name], typ+"#"+name
			if !namePattern.MatchString(name) {
				return httpx.Invalid(fmt.Sprintf("%q can't be a relation. Start with a lowercase letter and use up to 64 lowercase letters, digits, dashes and underscores.", where))
			}
			if len(rel.Subjects)+len(rel.Union)+len(rel.Roles)+len(rel.From) == 0 {
				return httpx.Invalid(where + " grants nothing. Give it subjects, a union of other relations, roles or a from.")
			}
			for _, s := range rel.Subjects {
				t, r, hasRel := strings.Cut(s, "#")
				ok := hasRel && !slices.Contains(reserved, t) && hasRelation(types, t, r) || !hasRel && (slices.Contains(reserved, t) || types[t] != nil)
				if !ok {
					return httpx.Invalid(fmt.Sprintf("%s takes the subject %q, which is not user, group, a type or a type#relation in this model.", where, s))
				}
			}
			for _, other := range rel.Union {
				if other == name || !hasRelation(types, typ, other) {
					return httpx.Invalid(fmt.Sprintf("%s includes %q, which is not another relation on %s.", where, other, typ))
				}
			}
			for _, key := range rel.Roles {
				if !slices.ContainsFunc(app.Roles, func(r store.AppRole) bool { return r.Key == key }) {
					return httpx.Invalid(fmt.Sprintf("%s grants to the role %q, which %s doesn't have. Add it on the Roles tab first.", where, key, app.Name))
				}
			}
			for _, f := range rel.From {
				via, ok := relations[f.Via]
				if !ok || !slices.ContainsFunc(via.Subjects, func(s string) bool { return !slices.Contains(reserved, s) && hasRelation(types, s, f.Relation) }) {
					return httpx.Invalid(fmt.Sprintf("%s reads %s from %s, but %s#%s takes no type with that relation.", where, f.Relation, f.Via, typ, f.Via))
				}
			}
		}
	}
	if total > maxRelations {
		return httpx.Invalid(fmt.Sprintf("A model holds up to %d relations.", maxRelations))
	}
	return nil
}
