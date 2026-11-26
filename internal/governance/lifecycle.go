package governance

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

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
		Attribute *string                 `json:"attribute"`
		Days      *int                    `json:"days"`
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
	case !slices.Contains([]string{"joiner", "mover", "leaver", "date", "inactivity"}, in.Trigger):
		return out, httpx.Invalid(`Choose a trigger: "joiner" when someone is created, "mover" when their department, title or location changes, "leaver" when they are suspended or deprovisioned, "date" on, before or after a date attribute, or "inactivity" when they have not signed in for a number of days.`)
	case len(in.Actions) == 0 || len(in.Actions) > 10:
		return out, httpx.Invalid("Add between 1 and 10 actions.")
	}
	switch in.Trigger {
	case "date":
		if in.Days == nil || *in.Days < -3650 || *in.Days > 3650 {
			return out, httpx.Invalid("Enter days between -3650 and 3650: negative runs the rule before the date, positive after it, and 0 on the date.")
		}
		attrs, err := h.st.ListAttributes(r.Context())
		if err != nil {
			return out, err
		}
		name := ""
		if in.Attribute != nil {
			name = strings.TrimSpace(*in.Attribute)
		}
		i := slices.IndexFunc(attrs, func(a store.Attribute) bool { return a.Name == name })
		switch {
		case i < 0 || attrs[i].Type != "date":
			return out, httpx.Invalid("Choose a date attribute, such as an end date. Create one under Settings, User attributes first.")
		case attrs[i].SelfService:
			return out, httpx.Invalid(attrs[i].Label + " can be changed by each person, so people could move their own lifecycle date. Choose a date attribute only administrators can change.")
		}
		out.Attribute, out.Days = &name, in.Days
	case "inactivity":
		if in.Days == nil || *in.Days < 1 || *in.Days > 3650 {
			return out, httpx.Invalid("Enter the number of days without a sign-in, between 1 and 3650, for example 90.")
		}
		out.Days = in.Days
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
	article := "a "
	if in.Trigger == "inactivity" {
		article = "an "
	}
	ruleID, action, summary, status := "", "lifecycle_rule.create", "Created "+article+in.Trigger+" rule with "+plural(len(in.Actions), "action"), http.StatusCreated
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

func groupsByID(ctx context.Context, st *store.Store) (map[string]store.Group, error) {
	all, err := st.ListGroupsRaw(ctx)
	groups := map[string]store.Group{}
	for _, g := range all {
		groups[g.ID] = g
	}
	return groups, err
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
	groups, err := groupsByID(ctx, st)
	if err != nil {
		return err
	}
	settings, err := st.CurrentSettings(ctx)
	if err != nil {
		return err
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
					if err := fire(ctx, tx, rule, c.User, e, groups, settings.EmergencyAccessIDs); err != nil {
						return err
					}
				}
			}
			return nil
		}))
	}
	return errors.Join(errs...)
}

var scheduledTriggers = []string{"date", "inactivity"}

type dueRun struct {
	rule       store.LifecycleRule
	user       store.User
	event      lifecycleEvent
	occurrence string
}

func dateChange(label, value string, days int) string {
	switch {
	case days < 0:
		return plural(-days, "day") + " before " + label + " (" + value + ")"
	case days > 0:
		return plural(days, "day") + " after " + label + " (" + value + ")"
	}
	return label + " reached (" + value + ")"
}

func scheduledEvent(rule store.LifecycleRule, attribute store.Attribute, u store.User, emergency []string, now time.Time) (lifecycleEvent, string, bool) {
	if rule.Days == nil || rule.Condition != nil && !store.RuleMatches(*rule.Condition, u) {
		return lifecycleEvent{}, "", false
	}
	if rule.Trigger == "date" {
		value, _ := u.Attributes[attribute.Name].(string)
		date, err := time.Parse(time.DateOnly, value)
		if err != nil || date.AddDate(0, 0, *rule.Days).After(now) {
			return lifecycleEvent{}, "", false
		}
		return lifecycleEvent{"date", dateChange(attribute.Label, value, *rule.Days)}, value, true
	}
	if u.Guest || u.Status != "active" || slices.Contains(emergency, u.ID) {
		return lifecycleEvent{}, "", false
	}
	since, change := u.CreatedAt, "Never signed in; created %s, %d days ago"
	if u.LastSignInAt != nil {
		since, change = *u.LastSignInAt, "Last signed in %s, %d days ago"
	}
	idle := int(now.Sub(since) / (24 * time.Hour))
	if idle < *rule.Days {
		return lifecycleEvent{}, "", false
	}
	return lifecycleEvent{"inactivity", fmt.Sprintf(change, since.UTC().Format(time.DateOnly), idle)}, since.UTC().Format(time.RFC3339Nano), true
}

func dueRuns(ctx context.Context, st *store.Store, rules []store.LifecycleRule, emergency []string, now time.Time) ([]dueRun, error) {
	users, err := st.LifecycleCandidates(ctx)
	if err != nil {
		return nil, err
	}
	attrs, err := st.ListAttributes(ctx)
	if err != nil {
		return nil, err
	}
	fired, err := st.LifecycleOccurrences(ctx)
	if err != nil {
		return nil, err
	}
	var due []dueRun
	for _, rule := range rules {
		var attribute store.Attribute
		if rule.Trigger == "date" {
			i := slices.IndexFunc(attrs, func(a store.Attribute) bool { return rule.Attribute != nil && a.Name == *rule.Attribute })
			if i < 0 || attrs[i].Type != "date" || attrs[i].SelfService {
				continue
			}
			attribute = attrs[i]
		}
		for _, u := range users {
			e, occurrence, ok := scheduledEvent(rule, attribute, u, emergency, now)
			if ok && !fired[[3]string{rule.ID, u.ID, occurrence}] {
				due = append(due, dueRun{rule, u, e, occurrence})
			}
		}
	}
	return due, nil
}

func runScheduledLifecycle(ctx context.Context, st *store.Store, now time.Time) error {
	rules, err := st.ListLifecycleRules(ctx)
	if err != nil {
		return err
	}
	rules = slices.DeleteFunc(rules, func(r store.LifecycleRule) bool { return !r.Enabled || !slices.Contains(scheduledTriggers, r.Trigger) })
	if len(rules) == 0 {
		return nil
	}
	settings, err := st.CurrentSettings(ctx)
	if err != nil {
		return err
	}
	due, err := dueRuns(ctx, st, rules, settings.EmergencyAccessIDs, now)
	if err != nil || len(due) == 0 {
		return err
	}
	groups, err := groupsByID(ctx, st)
	if err != nil {
		return err
	}
	var errs []error
	for _, d := range due {
		errs = append(errs, st.Tx(ctx, func(tx *store.Store) error {
			claimed, err := tx.ClaimLifecycleOccurrence(ctx, d.rule.ID, d.user.ID, d.occurrence)
			if err != nil || !claimed {
				return err
			}
			return fire(ctx, tx, d.rule, d.user, d.event, groups, settings.EmergencyAccessIDs)
		}))
	}
	return errors.Join(errs...)
}

func (h *handler) previewRule(w http.ResponseWriter, r *http.Request, _ store.User) error {
	in, err := h.ruleInput(r)
	if err != nil {
		return err
	}
	if !slices.Contains(scheduledTriggers, in.Trigger) {
		return httpx.Invalid("Only date and inactivity rules have a preview. Joiner, mover and leaver rules run when Halo detects a change.")
	}
	settings, err := h.st.CurrentSettings(r.Context())
	if err != nil {
		return err
	}
	rule := store.LifecycleRule{ID: r.URL.Query().Get("rule"), Trigger: in.Trigger, Condition: in.Condition, Attribute: in.Attribute, Days: in.Days}
	due, err := dueRuns(r.Context(), h.st, []store.LifecycleRule{rule}, settings.EmergencyAccessIDs, time.Now())
	if err != nil {
		return err
	}
	type affected struct {
		User   store.NamedRef `json:"user"`
		Change string         `json:"change"`
	}
	out := []affected{}
	for _, d := range due {
		out = append(out, affected{store.NamedRef{ID: d.user.ID, Name: d.user.Name}, d.event.change})
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func fire(ctx context.Context, tx *store.Store, rule store.LifecycleRule, u store.User, e lifecycleEvent, groups map[string]store.Group, emergency []string) error {
	steps, ok, err := applyRule(ctx, tx, rule, u, groups, emergency)
	if err != nil {
		return err
	}
	result := "succeeded"
	if !ok {
		result = "failed"
	}
	if err := tx.RecordLifecycleRun(ctx, store.LifecycleRun{Rule: store.NamedRef{ID: rule.ID}, User: store.NamedRef{ID: u.ID}, Trigger: e.trigger, Change: e.change, Steps: steps, Result: result}); err != nil {
		return err
	}
	return tx.RecordAudit(ctx, store.AuditEvent{Action: "lifecycle.run", Summary: rule.Name + ": " + strings.Join(steps, "; "), TargetType: "user", TargetID: u.ID, TargetLabel: u.Name})
}

func applyRule(ctx context.Context, tx *store.Store, rule store.LifecycleRule, u store.User, groups map[string]store.Group, emergency []string) ([]string, bool, error) {
	steps, ok := []string{}, true
	failed := func(step string) {
		steps, ok = append(steps, step), false
	}
	protected := func(action string) (bool, error) {
		full, err := tx.GetUser(ctx, u.ID)
		if err != nil || !full.Privileged() {
			return false, err
		}
		failed("Did not " + action + " " + u.Name + " because they hold or are eligible for an administrator role, so only a global administrator can change their account")
		return true, nil
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
			if a.Type == "add-to-group" && u.Guest {
				steps = append(steps, "Did not add "+u.Name+" to "+g.Name+" because guests join only the groups chosen for them")
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
			skip, err := protected("end the sessions of")
			if err != nil {
				return nil, false, err
			}
			if skip {
				continue
			}
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
			if slices.Contains(emergency, u.ID) {
				failed("Did not suspend " + u.Name + " because they are an emergency access account")
				continue
			}
			skip, err := protected("suspend")
			if err != nil {
				return nil, false, err
			}
			if skip {
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
