package api

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"halo/internal/httpx"
	"halo/internal/rule"
	"halo/internal/store"
)

func ruleError(err error) error {
	var invalid *rule.Error
	if errors.As(err, &invalid) {
		return httpx.Fail(http.StatusUnprocessableEntity, "ERR_INVALID_RULE", invalid.Error())
	}
	return err
}

func (h *handlers) group(r *http.Request) (store.Group, error) {
	g, err := h.st.GetGroup(r.Context(), r.PathValue("id"))
	return g, found(err, "This group")
}

func (h *handlers) assignedGroup(r *http.Request, actor store.User) (store.Group, error) {
	g, err := h.group(r)
	if err != nil {
		return g, err
	}
	if !actor.HasRole("global_admin", "user_admin") && !g.OwnedBy(actor.ID) {
		return g, httpx.ErrForbidden
	}
	if g.Kind == "dynamic" && g.Rule != nil {
		return g, conflict(g.Name + " is a dynamic group, so its members come from the rule " + *g.Rule + ". Change the person's profile or the group's rule instead.")
	}
	return g, nil
}

func (h *handlers) listGroups(w http.ResponseWriter, r *http.Request, _ store.User) error {
	groups, err := h.st.ListGroups(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, nonNil(groups))
}

func (h *handlers) createGroup(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Kind        string  `json:"kind"`
		Rule        *string `json:"rule"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return httpx.Invalid("Enter a group name.")
	}
	if tooLong(200, name) || tooLong(2000, in.Description) {
		return httpx.Invalid("Keep the group name under 200 characters and the description under 2,000.")
	}
	summary := "Created assigned group"
	switch in.Kind {
	case "assigned":
		if in.Rule != nil && strings.TrimSpace(*in.Rule) != "" {
			return httpx.Invalid("Assigned groups do not take a rule. Choose a dynamic group if membership should follow a rule.")
		}
		in.Rule = nil
	case "dynamic":
		rule := ""
		if in.Rule != nil {
			rule = strings.TrimSpace(*in.Rule)
		}
		if err := h.st.CheckRule(r.Context(), "", rule); err != nil {
			return ruleError(err)
		}
		in.Rule = &rule
		summary = "Created dynamic group with rule " + rule
	default:
		return httpx.Invalid(`Choose a group kind: "assigned" for hand-picked members or "dynamic" for rule-based membership.`)
	}
	var g store.Group
	err := h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		g, err = tx.CreateGroup(r.Context(), store.NewGroup{Name: name, Description: strings.TrimSpace(in.Description), Kind: in.Kind, Rule: in.Rule})
		if errors.Is(err, store.ErrConflict) {
			return conflict("A group named " + name + " already exists. Pick a different name.")
		}
		if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "group.create", Summary: summary, TargetType: "group", TargetID: g.ID, TargetLabel: g.Name})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, g)
}

func (h *handlers) addMember(w http.ResponseWriter, r *http.Request, actor store.User) error {
	g, err := h.assignedGroup(r, actor)
	if err != nil {
		return err
	}
	var in struct {
		UserID string `json:"userId"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	u, err := h.st.GetUser(r.Context(), in.UserID)
	if errors.Is(err, store.ErrNotFound) {
		return httpx.Invalid("That person does not exist. Refresh the page and pick someone from the directory.")
	}
	if err != nil {
		return err
	}
	if slices.Contains(u.GroupIDs, g.ID) {
		return noContent(w)
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.AddGroupMember(r.Context(), g.ID, u.ID); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "group.member.add", Summary: "Added " + u.Name, TargetType: "group", TargetID: g.ID, TargetLabel: g.Name})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

func (h *handlers) removeMember(w http.ResponseWriter, r *http.Request, actor store.User) error {
	g, err := h.assignedGroup(r, actor)
	if err != nil {
		return err
	}
	u, err := h.st.GetUser(r.Context(), r.PathValue("userId"))
	if err != nil {
		return found(err, "This user")
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		err := tx.RemoveGroupMember(r.Context(), g.ID, u.ID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.Fail(http.StatusNotFound, "ERR_NOT_FOUND", u.Name+" is not a member of "+g.Name+". Refresh the page to see current members.")
		}
		if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "group.member.remove", Summary: "Removed " + u.Name, TargetType: "group", TargetID: g.ID, TargetLabel: g.Name})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

func (h *handlers) getGroup(w http.ResponseWriter, r *http.Request, _ store.User) error {
	g, err := h.group(r)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, g)
}

func (h *handlers) groupMembers(w http.ResponseWriter, r *http.Request, _ store.User) error {
	g, err := h.group(r)
	if err != nil {
		return err
	}
	preview := r.URL.Query().Has("rule")
	matched := map[string]bool{}
	if preview {
		if g.Kind != "dynamic" {
			return httpx.Invalid(g.Name + " is an assigned group, so it has no rule to preview.")
		}
		ids, err := h.st.MatchRule(r.Context(), g.ID, strings.TrimSpace(r.URL.Query().Get("rule")))
		if err != nil {
			return ruleError(err)
		}
		for _, id := range ids {
			matched[id] = true
		}
	}
	users, err := h.st.ListUsers(r.Context())
	if err != nil {
		return err
	}
	members := []store.User{}
	for _, u := range users {
		if u.Status != "deprovisioned" && (matched[u.ID] || !preview && slices.Contains(u.GroupIDs, g.ID)) {
			members = append(members, u)
		}
	}
	return httpx.JSON(w, http.StatusOK, members)
}

func (h *handlers) updateGroup(w http.ResponseWriter, r *http.Request, actor store.User) error {
	g, err := h.group(r)
	if err != nil {
		return err
	}
	var in struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Kind        *string `json:"kind"`
		Rule        *string `json:"rule"`
		JoinPolicy  *string `json:"joinPolicy"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if in.Kind != nil && *in.Kind != g.Kind {
		return httpx.Invalid("A group's membership kind cannot change. Create a new group with the membership you want and assign it to the same applications.")
	}
	next, changed := g, []string{}
	if in.Name != nil && strings.TrimSpace(*in.Name) != g.Name {
		next.Name = strings.TrimSpace(*in.Name)
		changed = append(changed, "name")
	}
	if in.Description != nil && strings.TrimSpace(*in.Description) != g.Description {
		next.Description = strings.TrimSpace(*in.Description)
		changed = append(changed, "description")
	}
	if in.Rule != nil && g.Kind == "assigned" && strings.TrimSpace(*in.Rule) != "" {
		return httpx.Invalid("Assigned groups do not take a rule. Create a dynamic group if membership should follow a rule.")
	}
	if in.Rule != nil && g.Kind == "dynamic" {
		rule := strings.TrimSpace(*in.Rule)
		if err := h.st.CheckRule(r.Context(), g.ID, rule); err != nil {
			return ruleError(err)
		}
		if rule != *g.Rule {
			next.Rule = &rule
			changed = append(changed, "rule to "+rule)
		}
	}
	if in.JoinPolicy != nil && *in.JoinPolicy != g.JoinPolicy {
		switch {
		case !slices.Contains([]string{"closed", "request", "open"}, *in.JoinPolicy):
			return httpx.Invalid(`Choose who can join: "closed" for owners only, "request" for owner approval or "open" for anyone.`)
		case g.Kind == "dynamic":
			return httpx.Invalid(g.Name + " is a rule-based group, so its members come from the rule. Nobody can join it by hand.")
		case *in.JoinPolicy == "request" && len(g.Owners) == 0:
			return httpx.Invalid("Add an owner first. Owners approve requests to join " + g.Name + ".")
		}
		next.JoinPolicy = *in.JoinPolicy
		changed = append(changed, "join setting to "+next.JoinPolicy)
	}
	if next.Name == "" {
		return httpx.Invalid("Enter a group name.")
	}
	if tooLong(200, next.Name) || tooLong(2000, next.Description) {
		return httpx.Invalid("Keep the group name under 200 characters and the description under 2,000.")
	}
	if len(changed) == 0 {
		return httpx.JSON(w, http.StatusOK, g)
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.UpdateGroup(r.Context(), next); errors.Is(err, store.ErrConflict) {
			return conflict("A group named " + next.Name + " already exists. Pick a different name.")
		} else if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "group.update", Summary: "Updated " + strings.Join(changed, ", "), TargetType: "group", TargetID: g.ID, TargetLabel: next.Name, Changes: store.Diff(g, next)})
	})
	if err != nil {
		return err
	}
	updated, err := h.st.GetGroup(r.Context(), g.ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, updated)
}

func (h *handlers) deleteGroup(w http.ResponseWriter, r *http.Request, actor store.User) error {
	g, err := h.group(r)
	if err != nil {
		return err
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var inUse *store.GroupInUseError
		if err := tx.DeleteGroup(r.Context(), g.ID); errors.As(err, &inUse) {
			return conflict(inUse.Error())
		} else if err != nil {
			return found(err, "This group")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "group.delete", Summary: "Deleted the " + g.Kind + " group, which had " + plural(g.MemberCount, "member"), TargetType: "group", TargetID: g.ID, TargetLabel: g.Name})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

func (h *handlers) setGroupOwners(w http.ResponseWriter, r *http.Request, actor store.User) error {
	g, err := h.group(r)
	if err != nil {
		return err
	}
	if g.Kind == "dynamic" {
		return conflict(g.Name + " is a rule-based group, so it can't have owners: its members come from the rule " + *g.Rule + ".")
	}
	var in struct {
		UserIDs []string `json:"userIds"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ids := slices.Compact(slices.Sorted(slices.Values(in.UserIDs)))
	if len(ids) == 0 && g.JoinPolicy == "request" {
		return httpx.Invalid("Owners approve requests to join " + g.Name + ". Keep at least one owner, or change who can join first.")
	}
	owners := []string{}
	for _, userID := range ids {
		u, err := h.st.GetUser(r.Context(), userID)
		if errors.Is(err, store.ErrNotFound) || err == nil && (u.Kind != "person" || u.Status == "deprovisioned") {
			return httpx.Invalid("That person can't own a group. Refresh the page and pick someone from the directory.")
		}
		if err != nil {
			return err
		}
		owners = append(owners, u.Name)
	}
	summary := "Removed every owner"
	if len(owners) > 0 {
		summary = "Set owners to " + strings.Join(owners, ", ")
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.SetGroupOwners(r.Context(), g.ID, ids); err != nil {
			return found(err, "This group")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "group.update", Summary: summary, TargetType: "group", TargetID: g.ID, TargetLabel: g.Name})
	})
	if err != nil {
		return err
	}
	updated, err := h.st.GetGroup(r.Context(), g.ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, updated)
}

func (h *handlers) joinGroup(w http.ResponseWriter, r *http.Request, u store.User) error {
	g, err := h.group(r)
	if err != nil {
		return err
	}
	status := map[string]string{"status": "joined"}
	if slices.Contains(u.GroupIDs, g.ID) {
		return httpx.JSON(w, http.StatusOK, status)
	}
	switch {
	case g.JoinPolicy == "open" && u.Guest:
		return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "Guests can't join groups by themselves. Ask your sponsor or an owner of "+g.Name+" to add you.")
	case g.JoinPolicy == "open":
		err = h.st.Tx(r.Context(), func(tx *store.Store) error {
			if err := tx.AddGroupMember(r.Context(), g.ID, u.ID); err != nil {
				return err
			}
			return record(r, tx, u, store.AuditEvent{Action: "group.member.add", Summary: u.Name + " joined", TargetType: "group", TargetID: g.ID, TargetLabel: g.Name})
		})
	case g.JoinPolicy == "request":
		status["status"] = "requested"
		err = h.st.Tx(r.Context(), func(tx *store.Store) error {
			if added, err := tx.RequestToJoinGroup(r.Context(), g.ID, u.ID); err != nil || !added {
				return err
			}
			return record(r, tx, u, store.AuditEvent{Action: "group.join.request", Summary: u.Name + " asked to join", TargetType: "group", TargetID: g.ID, TargetLabel: g.Name})
		})
	default:
		return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "Nobody can join "+g.Name+" by themselves. Ask one of its owners or a Halo administrator.")
	}
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, status)
}

func (h *handlers) decideJoinRequest(approve bool) handler {
	return func(w http.ResponseWriter, r *http.Request, actor store.User) error {
		g, err := h.assignedGroup(r, actor)
		if err != nil {
			return err
		}
		u, err := h.st.GetUser(r.Context(), r.PathValue("userId"))
		if err != nil {
			return found(err, "This person")
		}
		err = h.st.Tx(r.Context(), func(tx *store.Store) error {
			if err := tx.DeleteJoinRequest(r.Context(), g.ID, u.ID); errors.Is(err, store.ErrNotFound) {
				return httpx.Fail(http.StatusNotFound, "ERR_NOT_FOUND", u.Name+" has no pending request to join "+g.Name+". Refresh the page to see current requests.")
			} else if err != nil {
				return err
			}
			if !approve {
				return record(r, tx, actor, store.AuditEvent{Action: "group.join.deny", Summary: "Declined " + u.Name + "'s request to join", TargetType: "group", TargetID: g.ID, TargetLabel: g.Name})
			}
			if err := tx.AddGroupMember(r.Context(), g.ID, u.ID); err != nil {
				return err
			}
			return record(r, tx, actor, store.AuditEvent{Action: "group.member.add", Summary: "Added " + u.Name + " after their request to join", TargetType: "group", TargetID: g.ID, TargetLabel: g.Name})
		})
		if err != nil {
			return err
		}
		return noContent(w)
	}
}

type ownedGroup struct {
	store.Group
	RequestCount int `json:"requestCount"`
}

func (h *handlers) myOwnedGroups(w http.ResponseWriter, r *http.Request, u store.User) error {
	all, err := h.st.ListGroups(r.Context())
	if err != nil {
		return err
	}
	out := []ownedGroup{}
	for _, g := range all {
		if !g.OwnedBy(u.ID) {
			continue
		}
		requests, err := h.st.GroupJoinRequests(r.Context(), g.ID)
		if err != nil {
			return err
		}
		out = append(out, ownedGroup{g, len(requests)})
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h *handlers) myOwnedGroup(w http.ResponseWriter, r *http.Request, u store.User) error {
	g, err := h.group(r)
	if err != nil {
		return err
	}
	if !g.OwnedBy(u.ID) {
		return httpx.NotFound("This group")
	}
	requests, err := h.st.GroupJoinRequests(r.Context(), g.ID)
	if err != nil {
		return err
	}
	users, err := h.st.ListUsers(r.Context())
	if err != nil {
		return err
	}
	members, people := []store.NamedRef{}, []store.NamedRef{}
	for _, p := range users {
		ref := store.NamedRef{ID: p.ID, Name: p.Name, Email: p.Email}
		switch {
		case p.Status == "deprovisioned":
		case slices.Contains(p.GroupIDs, g.ID):
			members = append(members, ref)
		default:
			people = append(people, ref)
		}
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"group": ownedGroup{g, len(requests)}, "members": members, "requests": requests, "people": people})
}

type joinableGroup struct {
	store.Group
	Requested bool `json:"requested"`
}

func (h *handlers) myJoinableGroups(w http.ResponseWriter, r *http.Request, u store.User) error {
	all, err := h.st.ListGroups(r.Context())
	if err != nil {
		return err
	}
	requested, err := h.st.JoinRequestGroupIDs(r.Context(), u.ID)
	if err != nil {
		return err
	}
	out := []joinableGroup{}
	for _, g := range all {
		if g.JoinPolicy != "closed" && !(u.Guest && g.JoinPolicy == "open") && !slices.Contains(u.GroupIDs, g.ID) {
			out = append(out, joinableGroup{g, slices.Contains(requested, g.ID)})
		}
	}
	return httpx.JSON(w, http.StatusOK, out)
}
