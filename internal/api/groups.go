package api

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"halo/internal/httpx"
	"halo/internal/store"
)

const ruleHelp = `The membership rule is not valid. Write it as user.<attribute> == "<value>", for example user.department == "Engineering". Supported attributes: status, department, location, title and source.`

func (h *handlers) group(r *http.Request) (store.Group, error) {
	g, err := h.st.GetGroup(r.Context(), r.PathValue("id"))
	return g, found(err, "This group")
}

func (h *handlers) assignedGroup(r *http.Request) (store.Group, error) {
	g, err := h.group(r)
	if err != nil {
		return g, err
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
		if in.Rule == nil || !store.ValidRule(*in.Rule) {
			return httpx.Invalid(ruleHelp)
		}
		rule := strings.TrimSpace(*in.Rule)
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
	g, err := h.assignedGroup(r)
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
	g, err := h.assignedGroup(r)
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
	rule := ""
	if g.Rule != nil {
		rule = *g.Rule
	}
	if r.URL.Query().Has("rule") {
		if g.Kind != "dynamic" {
			return httpx.Invalid(g.Name + " is an assigned group, so it has no rule to preview.")
		}
		if rule = r.URL.Query().Get("rule"); !store.ValidRule(rule) {
			return httpx.Invalid(ruleHelp)
		}
	}
	users, err := h.st.ListUsers(r.Context())
	if err != nil {
		return err
	}
	members := []store.User{}
	for _, u := range users {
		if u.Status != "deprovisioned" && ((g.Kind == "dynamic" && store.RuleMatches(rule, u)) || (g.Kind == "assigned" && slices.Contains(u.GroupIDs, g.ID))) {
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
		if !store.ValidRule(*in.Rule) {
			return httpx.Invalid(ruleHelp)
		}
		if rule := strings.TrimSpace(*in.Rule); rule != *g.Rule {
			next.Rule = &rule
			changed = append(changed, "rule to "+rule)
		}
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
		return record(r, tx, actor, store.AuditEvent{Action: "group.update", Summary: "Updated " + strings.Join(changed, ", "), TargetType: "group", TargetID: g.ID, TargetLabel: next.Name})
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
