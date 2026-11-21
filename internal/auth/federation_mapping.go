package auth

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"halo/internal/httpx"
	"halo/internal/store"
)

var profileFields = map[string]func(*store.User) *string{
	"name":       func(u *store.User) *string { return &u.Name },
	"title":      func(u *store.User) *string { return &u.Title },
	"department": func(u *store.User) *string { return &u.Department },
	"location":   func(u *store.User) *string { return &u.Location },
}

func validMapping(kind string, in store.Mapping, assigned func(string) bool) (store.Mapping, error) {
	m := store.Mapping{Profile: []store.ProfileMapping{}, GroupClaim: strings.TrimSpace(in.GroupClaim), Groups: []store.GroupMapping{}}
	for _, f := range in.Profile {
		f.Claim = strings.TrimSpace(f.Claim)
		switch {
		case f.Claim == "":
			return m, httpx.Invalid("Enter the claim to copy into each profile field, such as department.")
		case profileFields[f.Field] == nil:
			return m, httpx.Invalid("Claims can fill only a person's name, title, department and location.")
		case slices.ContainsFunc(m.Profile, func(o store.ProfileMapping) bool { return o.Field == f.Field }):
			return m, httpx.Invalid("Fill each profile field from one claim. The " + f.Field + " field is mapped twice.")
		}
		m.Profile = append(m.Profile, f)
	}
	for _, g := range in.Groups {
		g.Value = strings.TrimSpace(g.Value)
		switch {
		case g.Value == "":
			return m, httpx.Invalid("Enter the value the provider sends for each mapped group, such as the group's name or ID.")
		case !assigned(g.GroupID):
			return m, httpx.Invalid("Provider groups can only map to assigned groups that exist. Refresh the page and pick the groups again.")
		}
		if !slices.Contains(m.Groups, g) {
			m.Groups = append(m.Groups, g)
		}
	}
	switch {
	case len(m.Groups) > 100:
		return m, httpx.Invalid("Map up to 100 group values per identity provider.")
	case len(m.Groups) > 0 && m.GroupClaim == "":
		return m, httpx.Invalid("Enter the claim that lists the person's groups, such as groups.")
	case kind == "github" && len(m.Profile)+len(m.Groups) > 0:
		return m, httpx.Invalid("GitHub doesn't send claims Halo can map. Remove the mapping rules.")
	}
	return m, nil
}

func claimValues(claims map[string]any, name string) ([]string, bool) {
	switch v := claims[name].(type) {
	case string:
		return []string{v}, true
	case []string:
		return v, true
	case []any:
		values := make([]string, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			values[i] = s
		}
		return values, true
	}
	return nil, false
}

func (h *handler) applyMapping(r *http.Request, p store.IdentityProvider, u *store.User, claims map[string]any) error {
	ctx := r.Context()
	next, changed := *u, []string{}
	for _, f := range p.Mapping.Profile {
		values, ok := claimValues(claims, f.Claim)
		value := ""
		if len(values) > 0 {
			value = strings.TrimSpace(values[0])
		}
		field := profileFields[f.Field]
		if !ok || field == nil || len(value) > 200 || f.Field == "name" && value == "" || *field(&next) == value {
			continue
		}
		*field(&next) = value
		changed = append(changed, f.Field)
	}
	type move struct {
		group store.Group
		join  bool
	}
	moves := []move{}
	if values, ok := claimValues(claims, p.Mapping.GroupClaim); ok && len(p.Mapping.Groups) > 0 {
		groups, err := h.st.ListGroupsRaw(ctx)
		if err != nil {
			return err
		}
		for _, g := range groups {
			mapped := slices.ContainsFunc(p.Mapping.Groups, func(m store.GroupMapping) bool { return m.GroupID == g.ID })
			wanted := slices.ContainsFunc(p.Mapping.Groups, func(m store.GroupMapping) bool { return m.GroupID == g.ID && slices.Contains(values, m.Value) })
			if g.Kind == "assigned" && mapped && wanted != slices.Contains(u.GroupIDs, g.ID) {
				moves = append(moves, move{g, wanted})
			}
		}
	}
	if len(changed) == 0 && len(moves) == 0 {
		return nil
	}
	ip, via := httpx.ClientIP(r), " from "+p.Name+" at sign-in"
	err := h.st.Tx(ctx, func(tx *store.Store) error {
		if len(changed) > 0 {
			if err := tx.UpdateUserProfile(ctx, next); err != nil {
				return err
			}
			if err := tx.RecordAudit(ctx, store.AuditEvent{Action: "user.update", Summary: "Updated " + strings.Join(changed, ", ") + via, TargetType: "user", TargetID: u.ID, TargetLabel: next.Name, IP: ip, Changes: store.Diff(*u, next)}); err != nil {
				return err
			}
		}
		for _, m := range moves {
			apply, action, verb := tx.RemoveGroupMember, "group.member.remove", "Removed "
			if m.join {
				apply, action, verb = tx.AddGroupMember, "group.member.add", "Added "
			}
			if err := apply(ctx, m.group.ID, u.ID); errors.Is(err, store.ErrNotFound) {
				continue
			} else if err != nil {
				return err
			}
			changes := store.Diff(map[string]bool{"member": !m.join}, map[string]bool{"member": m.join})
			if err := tx.RecordAudit(ctx, store.AuditEvent{Action: action, Summary: verb + next.Name + via, TargetType: "group", TargetID: m.group.ID, TargetLabel: m.group.Name, IP: ip, Changes: changes}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	*u, err = h.st.GetUser(ctx, u.ID)
	return err
}
