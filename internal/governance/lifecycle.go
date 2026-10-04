package governance

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"halo/internal/httpx"
	"halo/internal/store"
)

func (h *handler) listRules(w http.ResponseWriter, r *http.Request, _ store.User) error {
	rules, err := h.st.ListLifecycleRules(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, nonNil(rules))
}

func (h *handler) listRuns(w http.ResponseWriter, r *http.Request, _ store.User) error {
	runs, err := h.st.ListLifecycleRuns(r.Context(), 500)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, nonNil(runs))
}

func (h *handler) ruleInput(r *http.Request) (store.LifecycleRuleInput, error) {
	var in struct {
		Name      string                  `json:"name"`
		Trigger   string                  `json:"trigger"`
		Condition *string                 `json:"condition"`
		Actions   []store.LifecycleAction `json:"actions"`
		Enabled   bool                    `json:"enabled"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return store.LifecycleRuleInput{}, err
	}
	out := store.LifecycleRuleInput{Name: strings.TrimSpace(in.Name), Trigger: in.Trigger, Enabled: in.Enabled}
	switch {
	case out.Name == "":
		return out, httpx.Invalid("Enter a rule name, for example Engineering joiners.")
	case len(out.Name) > 200:
		return out, httpx.Invalid("Keep the rule name under 200 characters.")
	case !slices.Contains([]string{"joiner", "mover", "leaver"}, in.Trigger):
		return out, httpx.Invalid(`Choose a trigger: "joiner" when someone is created, "mover" when their department, title or location changes, or "leaver" when they are suspended or deprovisioned.`)
	case len(in.Actions) == 0 || len(in.Actions) > 10:
		return out, httpx.Invalid("Add between 1 and 10 actions.")
	}
	if in.Condition != nil && strings.TrimSpace(*in.Condition) != "" {
		condition := strings.TrimSpace(*in.Condition)
		if !store.ValidRule(condition) {
			return out, httpx.Invalid(`The condition is not valid. Write it as user.<attribute> == "<value>", for example user.department == "Engineering". Supported attributes: status, department, location, title and source. Leave it empty to match everyone.`)
		}
		out.Condition = &condition
	}
	var groupIDs []string
	for _, a := range in.Actions {
		switch a.Type {
		case "add-to-group", "remove-from-group":
			if a.GroupID == "" {
				return out, httpx.Invalid("Choose a group for every add or remove action.")
			}
			groupIDs = append(groupIDs, a.GroupID)
			out.Actions = append(out.Actions, a)
		case "revoke-sessions", "suspend":
			out.Actions = append(out.Actions, store.LifecycleAction{Type: a.Type})
		default:
			return out, httpx.Invalid(`Each action must be "add-to-group", "remove-from-group", "revoke-sessions" or "suspend".`)
		}
	}
	if _, err := h.assignedGroups(r.Context(), groupIDs); err != nil {
		return out, err
	}
	return out, nil
}

func (h *handler) saveRule(w http.ResponseWriter, r *http.Request, actor store.User, current *store.LifecycleRule) error {
	in, err := h.ruleInput(r)
	if err != nil {
		return err
	}
	ruleID, action, summary, status := "", "lifecycle_rule.create", "Created a "+in.Trigger+" rule with "+plural(len(in.Actions), "action"), http.StatusCreated
	if current != nil {
		ruleID, action, summary, status = current.ID, "lifecycle_rule.update", "Updated the rule", http.StatusOK
		if in.Enabled != current.Enabled {
			summary = "Disabled the rule"
			if in.Enabled {
				summary = "Enabled the rule"
			}
		}
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		if ruleID, err = tx.SaveLifecycleRule(r.Context(), ruleID, in); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: action, Summary: summary, TargetType: "lifecycle_rule", TargetID: ruleID, TargetLabel: in.Name})
	})
	if err != nil {
		return err
	}
	rule, err := h.st.GetLifecycleRule(r.Context(), ruleID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, status, rule)
}

func (h *handler) createRule(w http.ResponseWriter, r *http.Request, actor store.User) error {
	return h.saveRule(w, r, actor, nil)
}

func (h *handler) updateRule(w http.ResponseWriter, r *http.Request, actor store.User) error {
	current, err := h.st.GetLifecycleRule(r.Context(), r.PathValue("id"))
	if err != nil {
		return found(err, "This lifecycle rule")
	}
	return h.saveRule(w, r, actor, &current)
}

func (h *handler) deleteRule(w http.ResponseWriter, r *http.Request, actor store.User) error {
	current, err := h.st.GetLifecycleRule(r.Context(), r.PathValue("id"))
	if err != nil {
		return found(err, "This lifecycle rule")
	}
	summary := "Deleted the " + current.Trigger + " rule"
	if current.Runs > 0 {
		summary += " and its " + plural(current.Runs, "run")
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.DeleteLifecycleRule(r.Context(), current.ID); err != nil {
			return found(err, "This lifecycle rule")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "lifecycle_rule.delete", Summary: summary, TargetType: "lifecycle_rule", TargetID: current.ID, TargetLabel: current.Name})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type lifecycleEvent struct{ trigger, change string }

func describeChange(label, before, after string) string {
	switch {
	case before == "":
		return label + " set to " + after
	case after == "":
		return label + " cleared (was " + before + ")"
	}
	return label + " changed from " + before + " to " + after
}

func detect(c store.LifecycleChange) []lifecycleEvent {
	u, p := c.User, c.Previous
	if p == nil {
		change := "Joined"
		if u.Title != "" {
			change += " as " + u.Title
		}
		if u.Department != "" {
			change += " in " + u.Department
		}
		return []lifecycleEvent{{"joiner", change}}
	}
	var events []lifecycleEvent
	var moved []string
	for _, f := range [][3]string{{"Department", p.Department, u.Department}, {"Title", p.Title, u.Title}, {"Location", p.Location, u.Location}} {
		if f[1] != f[2] {
			moved = append(moved, describeChange(f[0], f[1], f[2]))
		}
	}
	if len(moved) > 0 {
		events = append(events, lifecycleEvent{"mover", strings.Join(moved, "; ")})
	}
	if u.Status != p.Status && (u.Status == "suspended" || u.Status == "deprovisioned") {
		events = append(events, lifecycleEvent{"leaver", "Status changed from " + p.Status + " to " + u.Status})
	}
	return events
}

func runLifecycle(ctx context.Context, st *store.Store) error {
	changes, err := st.PendingLifecycleChanges(ctx)
	if err != nil || len(changes) == 0 {
		return err
	}
	rules, err := st.ListLifecycleRules(ctx)
	if err != nil {
		return err
	}
	all, err := st.ListGroupsRaw(ctx)
	if err != nil {
		return err
	}
	groups := map[string]store.Group{}
	for _, g := range all {
		groups[g.ID] = g
	}
	var errs []error
	for _, c := range changes {
		errs = append(errs, st.Tx(ctx, func(tx *store.Store) error {
			claimed, err := tx.ClaimLifecycleChange(ctx, c)
			if err != nil || !claimed {
				return err
			}
			for _, e := range detect(c) {
				for _, rule := range rules {
					if !rule.Enabled || rule.Trigger != e.trigger || rule.Condition != nil && !store.RuleMatches(*rule.Condition, c.User) {
						continue
					}
					steps, ok, err := applyRule(ctx, tx, rule, c.User, groups)
					if err != nil {
						return err
					}
					result := "succeeded"
					if !ok {
						result = "failed"
					}
					if err := tx.RecordLifecycleRun(ctx, store.LifecycleRun{Rule: store.NamedRef{ID: rule.ID}, User: store.NamedRef{ID: c.User.ID}, Trigger: e.trigger, Change: e.change, Steps: steps, Result: result}); err != nil {
						return err
					}
					if err := tx.RecordAudit(ctx, store.AuditEvent{Action: "lifecycle.run", Summary: rule.Name + ": " + strings.Join(steps, "; "), TargetType: "user", TargetID: c.User.ID, TargetLabel: c.User.Name}); err != nil {
						return err
					}
				}
			}
			return nil
		}))
	}
	return errors.Join(errs...)
}

func applyRule(ctx context.Context, tx *store.Store, rule store.LifecycleRule, u store.User, groups map[string]store.Group) ([]string, bool, error) {
	steps, ok := []string{}, true
	failed := func(step string) {
		steps, ok = append(steps, step), false
	}
	for _, a := range rule.Actions {
		switch a.Type {
		case "add-to-group", "remove-from-group":
			g, exists := groups[a.GroupID]
			if !exists {
				failed("Skipped a group action because the group no longer exists")
				continue
			}
			if g.Kind != "assigned" {
				failed("Could not change " + g.Name + " because it is a rule-based group")
				continue
			}
			if a.Type == "add-to-group" {
				added, err := tx.EnsureGroupMember(ctx, g.ID, u.ID)
				if err != nil {
					return nil, false, err
				}
				step := "Already in " + g.Name
				if added {
					step = "Added to " + g.Name
				}
				steps = append(steps, step)
				continue
			}
			err := tx.RemoveGroupMember(ctx, g.ID, u.ID)
			if errors.Is(err, store.ErrNotFound) {
				steps = append(steps, "Was not in "+g.Name)
				continue
			}
			if err != nil {
				return nil, false, err
			}
			steps = append(steps, "Removed from "+g.Name)
		case "revoke-sessions":
			n, err := tx.RevokeUserSessions(ctx, u.ID, "")
			if err != nil {
				return nil, false, err
			}
			steps = append(steps, "Ended "+plural(n, "session"))
		case "suspend":
			if u.Status == "suspended" || u.Status == "deprovisioned" {
				steps = append(steps, "Already "+u.Status)
				continue
			}
			last, err := tx.IsLastActiveGlobalAdmin(ctx, u.ID)
			if err != nil {
				return nil, false, err
			}
			if last {
				failed("Did not suspend " + u.Name + " because they are the last active global administrator")
				continue
			}
			if err := tx.SetUserStatus(ctx, u.ID, "suspended"); err != nil {
				return nil, false, err
			}
			n, err := tx.RevokeUserSessions(ctx, u.ID, "")
			if err != nil {
				return nil, false, err
			}
			steps = append(steps, "Suspended the account and ended "+plural(n, "session"))
		}
	}
	return steps, ok, nil
}
