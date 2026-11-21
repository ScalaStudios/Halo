package webhooks

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"halo/internal/httpx"
	"halo/internal/id"
	"halo/internal/store"
)

var (
	riskLevels   = []string{"none", "low", "medium", "high"}
	destinations = map[string]string{"webhook": "a webhook", "slack": "Slack", "discord": "Discord"}
	slackEscape  = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
)

type ruleInput struct {
	Name          string   `json:"name"`
	Destination   string   `json:"destination"`
	URL           string   `json:"url"`
	Events        []string `json:"events"`
	MinRisk       string   `json:"minRisk"`
	Role          string   `json:"role"`
	Threshold     int      `json:"threshold"`
	WindowMinutes int      `json:"windowMinutes"`
	QuietMinutes  int      `json:"quietMinutes"`
	Enabled       *bool    `json:"enabled"`
}

func (in ruleInput) validate(creating bool) (store.NewNotificationRule, error) {
	r := store.NewNotificationRule{Name: strings.TrimSpace(in.Name), Destination: in.Destination, URL: strings.TrimSpace(in.URL), MinRisk: in.MinRisk, Role: in.Role,
		Threshold: cmp.Or(in.Threshold, 1), WindowMinutes: cmp.Or(in.WindowMinutes, 10), QuietMinutes: cmp.Or(in.QuietMinutes, 15), Enabled: in.Enabled == nil || *in.Enabled}
	if r.Name == "" || len(r.Name) > 100 {
		return r, httpx.Invalid("Name the rule in up to 100 characters, for example New global administrator.")
	}
	if _, ok := destinations[r.Destination]; !ok {
		return r, httpx.Invalid("Choose webhook, slack or discord as the destination.")
	}
	if creating || r.URL != "" {
		u, err := PublicURL(r.URL, "incoming webhook", "Notification rules only send to")
		if err != nil {
			return r, err
		}
		r.Host = u.Host
	}
	if r.MinRisk != "" && !slices.Contains(riskLevels[1:], r.MinRisk) {
		return r, httpx.Invalid("Set the minimum risk to low, medium or high, or leave it empty for any risk.")
	}
	if _, ok := store.RoleLabels[r.Role]; r.Role != "" && !ok {
		return r, httpx.Invalid(fmt.Sprintf("%q is not a role. Use global_admin, security_admin, user_admin, helpdesk_admin, app_admin or auditor.", r.Role))
	}
	if r.Threshold < 1 || r.Threshold > 100 || r.WindowMinutes < 1 || r.WindowMinutes > 1440 || r.QuietMinutes < 1 || r.QuietMinutes > 1440 {
		return r, httpx.Invalid("Send after 1 to 100 matching events within 1 to 1,440 minutes, and stay quiet for 1 to 1,440 minutes after each message.")
	}
	var err error
	r.Events, err = eventList(in.Events)
	return r, err
}

func matches(rule store.NotificationRule, e store.WebhookEvent) bool {
	if e.Time.Before(rule.CreatedAt) || !subscribed(rule.Events, e.Type) {
		return false
	}
	var data struct {
		Risk    string
		Changes []store.Change
	}
	_ = json.Unmarshal(e.Data, &data)
	if rule.MinRisk != "" && slices.Index(riskLevels, data.Risk) < slices.Index(riskLevels, rule.MinRisk) {
		return false
	}
	return rule.Role == "" || slices.ContainsFunc(data.Changes, func(c store.Change) bool {
		var before, after []string
		_ = json.Unmarshal(c.Before, &before)
		_ = json.Unmarshal(c.After, &after)
		return c.Field == "roles" && slices.Contains(after, rule.Role) && !slices.Contains(before, rule.Role)
	})
}

func observe(rule *store.NotificationRule, at time.Time) bool {
	since := at.Add(-time.Duration(rule.WindowMinutes) * time.Minute)
	rule.Hits = append(slices.DeleteFunc(rule.Hits, func(t time.Time) bool { return !t.After(since) }), at)
	rule.Hits = rule.Hits[max(0, len(rule.Hits)-rule.Threshold):]
	if len(rule.Hits) < rule.Threshold || rule.LastSentAt != nil && at.Before(rule.LastSentAt.Add(time.Duration(rule.QuietMinutes)*time.Minute)) {
		return false
	}
	rule.Hits, rule.LastSentAt = []time.Time{}, &at
	return true
}

func notify(ctx context.Context, tx *store.Store, events []store.WebhookEvent, now time.Time) error {
	rules, err := tx.ActiveNotificationRules(ctx)
	if err != nil || len(rules) == 0 {
		return err
	}
	slices.SortStableFunc(events, func(a, b store.WebhookEvent) int { return a.Time.Compare(b.Time) })
	for _, rule := range rules {
		matched := false
		for _, e := range events {
			if !matches(rule, e) {
				continue
			}
			matched = true
			if !observe(&rule, e.Time) {
				continue
			}
			body, err := render(rule, e, rule.Threshold)
			if err != nil {
				return err
			}
			if _, err := tx.QueueNotification(ctx, rule.ID, e, body, now); err != nil {
				return err
			}
		}
		if matched {
			if err := tx.SaveNotificationRuleState(ctx, rule.ID, rule.Hits, rule.LastSentAt); err != nil {
				return err
			}
		}
	}
	return nil
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func render(rule store.NotificationRule, e store.WebhookEvent, count int) ([]byte, error) {
	e.Time = e.Time.UTC()
	if rule.Destination == "webhook" {
		return json.Marshal(map[string]any{"id": e.ID, "type": e.Type, "time": e.Time, "data": e.Data, "count": count, "rule": map[string]string{"id": rule.ID, "name": rule.Name}})
	}
	var d struct{ Summary, Message, Email, Target, Method, Risk, IP, Location, Reason string }
	_ = json.Unmarshal(e.Data, &d)
	var text []string
	if i := slices.IndexFunc(catalog, func(t eventType) bool { return t.Type == e.Type }); i >= 0 {
		text = append(text, catalog[i].Description)
	}
	if detail := cmp.Or(d.Summary, d.Message); detail != "" {
		text = append(text, clip(detail, 1500))
	}
	if count > 1 {
		unit := "minutes"
		if rule.WindowMinutes == 1 {
			unit = "minute"
		}
		text = append(text, fmt.Sprintf("%d matching events within %d %s.", count, rule.WindowMinutes, unit))
	}
	if d.Risk == "none" {
		d.Risk = ""
	}
	fields := slices.DeleteFunc([][2]string{{"Account", d.Email}, {"Target", d.Target}, {"Method", d.Method}, {"Risk", d.Risk}, {"IP address", d.IP}, {"Location", d.Location}, {"Reason", d.Reason}},
		func(f [2]string) bool { return f[1] == "" })
	if rule.Destination == "slack" {
		lines := []string{"*" + slackEscape.Replace(rule.Name) + "*"}
		for _, t := range text {
			lines = append(lines, slackEscape.Replace(t))
		}
		for _, f := range fields {
			lines = append(lines, "*"+f[0]+":* "+slackEscape.Replace(clip(f[1], 256)))
		}
		lines = append(lines, "`"+e.Type+"` · "+e.Time.Format("2 Jan 2006 15:04:05 UTC"))
		return json.Marshal(map[string]string{"text": strings.Join(lines, "\n")})
	}
	embedFields := []map[string]any{}
	for _, f := range fields {
		embedFields = append(embedFields, map[string]any{"name": f[0], "value": clip(f[1], 256), "inline": true})
	}
	return json.Marshal(map[string]any{
		"allowed_mentions": map[string][]string{"parse": {}},
		"embeds": []map[string]any{{"title": clip(rule.Name, 256), "description": clip(strings.Join(text, "\n"), 2048), "color": 0xE5484D,
			"timestamp": e.Time.Format(time.RFC3339), "footer": map[string]string{"text": e.Type}, "fields": embedFields}},
	})
}

func recordRule(r *http.Request, st *store.Store, actor store.User, action, summary string, rule store.NotificationRule) error {
	return st.RecordAudit(r.Context(), store.AuditEvent{ActorID: &actor.ID, IP: httpx.ClientIP(r), Action: action, Summary: summary, TargetType: "notification_rule", TargetID: rule.ID, TargetLabel: rule.Name})
}

func missingRule(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("The notification rule")
	}
	return err
}

func (h *handler) listRules(w http.ResponseWriter, r *http.Request, _ store.User) error {
	rules, err := h.st.ListNotificationRules(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, rules)
}

func (h *handler) getRule(w http.ResponseWriter, r *http.Request, _ store.User) error {
	ctx := r.Context()
	rule, err := h.st.GetNotificationRule(ctx, r.PathValue("id"))
	if err != nil {
		return missingRule(err)
	}
	deliveries, err := h.st.ListWebhookDeliveries(ctx, rule.ID, 50)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"rule": rule, "deliveries": deliveries})
}

func (h *handler) createRule(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in ruleInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	fields, err := in.validate(true)
	if err != nil {
		return err
	}
	ctx := r.Context()
	var rule store.NotificationRule
	var secret string
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		ruleID, s, err := tx.CreateNotificationRule(ctx, fields)
		if err != nil {
			return err
		}
		if rule, err = tx.GetNotificationRule(ctx, ruleID); err != nil {
			return err
		}
		secret = s
		return recordRule(r, tx, actor, "notification_rule.create", "Added a rule that sends "+strings.Join(fields.Events, ", ")+" to "+destinations[fields.Destination], rule)
	})
	if err != nil {
		return err
	}
	out := map[string]any{"rule": rule}
	if rule.Destination == "webhook" {
		out["secret"] = secret
	}
	return httpx.JSON(w, http.StatusCreated, out)
}

func (h *handler) updateRule(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in ruleInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	fields, err := in.validate(false)
	if err != nil {
		return err
	}
	ctx := r.Context()
	var rule store.NotificationRule
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		current, err := tx.GetNotificationRule(ctx, r.PathValue("id"))
		if err != nil {
			return err
		}
		if fields.URL == "" && fields.Destination != current.Destination {
			return httpx.Invalid("Enter the webhook URL again when you change where the rule sends messages.")
		}
		if err := tx.UpdateNotificationRule(ctx, current.ID, fields); err != nil {
			return err
		}
		if rule, err = tx.GetNotificationRule(ctx, current.ID); err != nil {
			return err
		}
		state := "Updated"
		if !fields.Enabled {
			state = "Updated and paused"
		}
		if fields.URL != "" {
			state += " the rule and replaced its destination URL"
		} else {
			state += " the rule"
		}
		return recordRule(r, tx, actor, "notification_rule.update", state, rule)
	})
	if err != nil {
		return missingRule(err)
	}
	return httpx.JSON(w, http.StatusOK, rule)
}

func (h *handler) deleteRule(w http.ResponseWriter, r *http.Request, actor store.User) error {
	ctx := r.Context()
	err := h.st.Tx(ctx, func(tx *store.Store) error {
		rule, err := tx.GetNotificationRule(ctx, r.PathValue("id"))
		if err != nil {
			return err
		}
		if err := tx.DeleteNotificationRule(ctx, rule.ID); err != nil {
			return err
		}
		return recordRule(r, tx, actor, "notification_rule.delete", "Deleted the rule and its delivery log", rule)
	})
	if err != nil {
		return missingRule(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *handler) testRule(w http.ResponseWriter, r *http.Request, actor store.User) error {
	ctx := r.Context()
	rule, err := h.st.GetNotificationRule(ctx, r.PathValue("id"))
	if err != nil {
		return missingRule(err)
	}
	now := time.Now()
	data, _ := json.Marshal(map[string]string{"message": "This is a test message sent from the Halo console.", "ruleId": rule.ID, "sentBy": actor.Email})
	e := store.WebhookEvent{ID: id.New("evt"), Type: "webhook.test", Time: now.UTC(), Data: data}
	body, err := render(rule, e, 1)
	if err != nil {
		return err
	}
	deliveryID, err := h.st.QueueNotification(ctx, rule.ID, e, body, now.Add(lease))
	if err != nil {
		return err
	}
	return h.deliverNow(w, r, deliveryID, now)
}
