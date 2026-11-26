package webhooks

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"halo/internal/store"
)

func grant(before, after []string) json.RawMessage {
	data, _ := json.Marshal(map[string]any{"changes": store.Diff(map[string][]string{"roles": before}, map[string][]string{"roles": after})})
	return data
}

func TestRuleMatching(t *testing.T) {
	created := time.Now()
	at := created.Add(time.Minute)
	risky := store.NotificationRule{Events: []string{"sign_in.*"}, MinRisk: "medium", CreatedAt: created}
	admin := store.NotificationRule{Events: []string{"role.update"}, Role: "global_admin", CreatedAt: created}
	for _, c := range []struct {
		name string
		rule store.NotificationRule
		e    store.WebhookEvent
		want bool
	}{
		{"high risk", risky, store.WebhookEvent{Type: "sign_in.failure", Time: at, Data: json.RawMessage(`{"risk":"high"}`)}, true},
		{"medium risk", risky, store.WebhookEvent{Type: "sign_in.success", Time: at, Data: json.RawMessage(`{"risk":"medium"}`)}, true},
		{"low risk", risky, store.WebhookEvent{Type: "sign_in.failure", Time: at, Data: json.RawMessage(`{"risk":"low"}`)}, false},
		{"no risk field", risky, store.WebhookEvent{Type: "sign_in.failure", Time: at, Data: json.RawMessage(`{}`)}, false},
		{"other event", risky, store.WebhookEvent{Type: "user.invite", Time: at, Data: json.RawMessage(`{"risk":"high"}`)}, false},
		{"before the rule", risky, store.WebhookEvent{Type: "sign_in.failure", Time: created.Add(-time.Second), Data: json.RawMessage(`{"risk":"high"}`)}, false},
		{"grants global admin", admin, store.WebhookEvent{Type: "role.update", Time: at, Data: grant([]string{"auditor"}, []string{"auditor", "global_admin"})}, true},
		{"invited as global admin", admin, store.WebhookEvent{Type: "role.update", Time: at, Data: grant([]string{}, []string{"global_admin"})}, true},
		{"already global admin", admin, store.WebhookEvent{Type: "role.update", Time: at, Data: grant([]string{"global_admin"}, []string{"global_admin", "auditor"})}, false},
		{"removes global admin", admin, store.WebhookEvent{Type: "role.update", Time: at, Data: grant([]string{"global_admin"}, []string{})}, false},
		{"grants another role", admin, store.WebhookEvent{Type: "role.update", Time: at, Data: grant([]string{}, []string{"auditor"})}, false},
		{"no changes recorded", admin, store.WebhookEvent{Type: "role.update", Time: at, Data: json.RawMessage(`{"summary":"Set roles to Global administrator"}`)}, false},
	} {
		if got := matches(c.rule, c.e); got != c.want {
			t.Errorf("%s: matched %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRuleThresholdAndQuietPeriod(t *testing.T) {
	start := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	rule := store.NotificationRule{Threshold: 3, WindowMinutes: 10, QuietMinutes: 15}
	for _, step := range []struct {
		minute float64
		fires  bool
	}{
		{0, false}, {1, false}, {11.5, false}, {12, false},
		{13, true},
		{14, false}, {15, false}, {16, false}, {20, false},
		{30, false}, {31, false}, {32, true},
	} {
		at := start.Add(time.Duration(step.minute * float64(time.Minute)))
		if got := observe(&rule, at); got != step.fires {
			t.Fatalf("minute %v: fired %v, want %v (hits %v)", step.minute, got, step.fires, rule.Hits)
		}
		if len(rule.Hits) > rule.Threshold {
			t.Fatalf("minute %v: kept %d hits", step.minute, len(rule.Hits))
		}
	}
	if !rule.LastSentAt.Equal(start.Add(32 * time.Minute)) {
		t.Fatalf("last sent %s", rule.LastSentAt)
	}
	single := store.NotificationRule{Threshold: 1, WindowMinutes: 10, QuietMinutes: 5}
	if !observe(&single, start) || observe(&single, start.Add(4*time.Minute)) || !observe(&single, start.Add(5*time.Minute)) {
		t.Fatal("a single-event rule must send once per quiet period")
	}
}

func TestRuleValidationAndSecrets(t *testing.T) {
	st, _, h := setup(t)
	security := person(t, st, "sam@example.com", "security_admin")
	auditor := person(t, st, "aud@example.com", "auditor")
	slackURL := "https://hooks.slack.com/services/T000/B000/s3cr3tT0k3n"
	valid := map[string]any{"name": "New global administrator", "destination": "slack", "url": slackURL, "events": []string{"role.update"}, "role": "global_admin"}

	if rec := call(t, h, auditor, "POST", "/api/v1/notification-rules", valid); rec.Code != 403 {
		t.Fatalf("auditor create: %d", rec.Code)
	}
	with := func(key string, value any) map[string]any {
		body := map[string]any{}
		for k, v := range valid {
			body[k] = v
		}
		body[key] = value
		return body
	}
	for _, body := range []map[string]any{
		with("url", "http://hooks.example.com/halo"),
		with("url", "https://127.0.0.1/halo"),
		with("url", "https://localhost/halo"),
		with("url", "https://10.1.2.3/halo"),
		with("url", "https://192.168.1.10/halo"),
		with("url", "https://169.254.169.254/latest/meta-data"),
		with("url", "https://100.100.100.200/latest"),
		with("url", "https://[::1]/halo"),
		with("url", "https://[fe80::1]/halo"),
		with("url", "https://[::ffff:10.0.0.1]/halo"),
		with("url", "https://user:pass@hooks.example.com/halo"),
		with("name", " "),
		with("destination", "teams"),
		with("role", "owner"),
		with("minRisk", "none"),
		with("threshold", 101),
		with("quietMinutes", 2000),
		with("events", []string{}),
	} {
		if rec := call(t, h, security, "POST", "/api/v1/notification-rules", body); rec.Code != 422 {
			t.Fatalf("invalid %v: %d %s", body, rec.Code, rec.Body.String())
		}
	}

	rec := call(t, h, security, "POST", "/api/v1/notification-rules", valid)
	var created struct {
		Rule   store.NotificationRule `json:"rule"`
		Secret *string                `json:"secret"`
	}
	if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &created) != nil || created.Secret != nil || created.Rule.Host != "hooks.slack.com" ||
		created.Rule.Threshold != 1 || created.Rule.WindowMinutes != 10 || created.Rule.QuietMinutes != 15 || !created.Rule.Enabled {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	leaks := func(where, body string) {
		t.Helper()
		if strings.Contains(body, "s3cr3tT0k3n") || strings.Contains(body, "/services/") {
			t.Fatalf("%s returned the destination URL: %s", where, body)
		}
	}
	leaks("create", rec.Body.String())
	for _, path := range []string{"/api/v1/notification-rules", "/api/v1/notification-rules/" + created.Rule.ID} {
		rec := call(t, h, auditor, "GET", path, nil)
		if rec.Code != 200 {
			t.Fatalf("auditor read %s: %d", path, rec.Code)
		}
		leaks(path, rec.Body.String())
	}

	update := with("url", "")
	update["enabled"] = false
	if rec := call(t, h, security, "PUT", "/api/v1/notification-rules/"+created.Rule.ID, update); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"host":"hooks.slack.com"`) || !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Fatalf("update keeping the URL: %d %s", rec.Code, rec.Body.String())
	}
	rule, _ := st.GetNotificationRule(context.Background(), created.Rule.ID)
	if plain, err := st.Sealer.Open(rule.URLSealed); err != nil || string(plain) != slackURL {
		t.Fatalf("sealed URL after update: %q %v", plain, err)
	}
	moved := with("url", "")
	moved["destination"] = "discord"
	if rec := call(t, h, security, "PUT", "/api/v1/notification-rules/"+created.Rule.ID, moved); rec.Code != 422 {
		t.Fatalf("changing the destination without a URL: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, h, security, "PUT", "/api/v1/notification-rules/"+created.Rule.ID, with("url", "https://chat.example.org/hooks/abc")); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"host":"chat.example.org"`) {
		t.Fatalf("update replacing the URL: %d %s", rec.Code, rec.Body.String())
	}

	rec = call(t, h, security, "POST", "/api/v1/notification-rules", with("destination", "webhook"))
	if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &created) != nil || created.Secret == nil || !strings.HasPrefix(*created.Secret, "whsec_") {
		t.Fatalf("webhook rule: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, h, security, "DELETE", "/api/v1/notification-rules/"+created.Rule.ID, nil); rec.Code != 204 {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec := call(t, h, security, "GET", "/api/v1/notification-rules/"+created.Rule.ID, nil); rec.Code != 404 {
		t.Fatalf("deleted rule: %d", rec.Code)
	}

	events, err := st.ListAudit(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]int{}
	for _, e := range events {
		raw, _ := json.Marshal(e)
		leaks("audit log", string(raw))
		actions[e.Action]++
	}
	if actions["notification_rule.create"] != 2 || actions["notification_rule.update"] != 2 || actions["notification_rule.delete"] != 1 {
		t.Fatalf("audit actions: %v", actions)
	}
}

func TestRuleDeliveries(t *testing.T) {
	st, h, _ := setup(t)
	ctx := context.Background()
	slack, discord, hook := &receiver{}, &receiver{}, &receiver{}
	servers := []*httptest.Server{httptest.NewServer(slack), httptest.NewServer(discord), httptest.NewServer(hook)}
	for _, srv := range servers {
		defer srv.Close()
	}
	newRule := func(in store.NewNotificationRule) (string, string) {
		in.Enabled, in.Host = true, "localhost"
		in.Threshold, in.WindowMinutes, in.QuietMinutes = max(in.Threshold, 1), 10, 15
		ruleID, secret, err := st.CreateNotificationRule(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		return ruleID, secret
	}
	slackID, _ := newRule(store.NewNotificationRule{Name: "Global admin <granted>", Destination: "slack", URL: servers[0].URL, Events: []string{"role.update"}, Role: "global_admin"})
	discordID, _ := newRule(store.NewNotificationRule{Name: "Risky failures", Destination: "discord", URL: servers[1].URL, Events: []string{"sign_in.failure"}, MinRisk: "high", Threshold: 2})
	hookID, secret := newRule(store.NewNotificationRule{Name: "Role changes", Destination: "webhook", URL: servers[2].URL, Events: []string{"role.*"}})
	time.Sleep(10 * time.Millisecond)

	roles := func(target string, before, after []string) {
		t.Helper()
		if err := st.RecordAudit(ctx, store.AuditEvent{Action: "role.update", Summary: "Set roles <!channel> & more", TargetType: "user", TargetLabel: target,
			Changes: store.Diff(map[string][]string{"roles": before}, map[string][]string{"roles": after})}); err != nil {
			t.Fatal(err)
		}
	}
	signIn := func(risk string) {
		t.Helper()
		if err := st.RecordSignIn(ctx, store.SignInEvent{Email: "ada@example.com", Result: "failure", Method: "totp", IP: "203.0.113.9", Risk: risk, Reason: "@everyone wrong code"}); err != nil {
			t.Fatal(err)
		}
	}
	roles("Ada", []string{}, []string{"global_admin"})
	roles("Grace", []string{"auditor"}, []string{"global_admin"})
	roles("Alan", []string{"global_admin"}, []string{})
	signIn("high")
	signIn("low")
	signIn("medium")
	signIn("high")
	signIn("high")

	if err := h.run(ctx, time.Now().Add(settle+time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(slack.bodies) != 1 || len(discord.bodies) != 1 || len(hook.bodies) != 1 {
		t.Fatalf("messages: slack %d, discord %d, webhook %d", len(slack.bodies), len(discord.bodies), len(hook.bodies))
	}

	var slackBody map[string]string
	if err := json.Unmarshal([]byte(slack.bodies[0]), &slackBody); err != nil || len(slackBody) != 1 {
		t.Fatalf("slack body: %s", slack.bodies[0])
	}
	for _, want := range []string{"*Global admin &lt;granted&gt;*", "A person's administrator roles changed", "Set roles &lt;!channel&gt; &amp; more", "*Target:* `Ada`", "`role.update`"} {
		if !strings.Contains(slackBody["text"], want) {
			t.Fatalf("slack text lacks %q: %s", want, slackBody["text"])
		}
	}
	if strings.Contains(slackBody["text"], "<!channel>") {
		t.Fatalf("slack text can mention the channel: %s", slackBody["text"])
	}

	var discordBody struct {
		AllowedMentions struct {
			Parse []string `json:"parse"`
		} `json:"allowed_mentions"`
		Embeds []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Timestamp   string `json:"timestamp"`
			Footer      struct {
				Text string `json:"text"`
			} `json:"footer"`
			Fields []struct {
				Name   string `json:"name"`
				Value  string `json:"value"`
				Inline bool   `json:"inline"`
			} `json:"fields"`
		} `json:"embeds"`
	}
	if err := json.Unmarshal([]byte(discord.bodies[0]), &discordBody); err != nil || !strings.Contains(discord.bodies[0], `"parse":[]`) || len(discordBody.Embeds) != 1 {
		t.Fatalf("discord body: %s", discord.bodies[0])
	}
	embed := discordBody.Embeds[0]
	if embed.Title != "Risky failures" || embed.Footer.Text != "sign_in.failure" || !strings.Contains(embed.Description, "2 matching events within 10 minutes.") || embed.Timestamp == "" {
		t.Fatalf("discord embed: %+v", embed)
	}
	fields := map[string]string{}
	for _, f := range embed.Fields {
		fields[f.Name] = f.Value
	}
	if fields["Account"] != "ada@example.com" || fields["Risk"] != "high" || fields["IP address"] != "203.0.113.9" || fields["Method"] != "totp" {
		t.Fatalf("discord fields: %v", fields)
	}

	verify(t, secret, hook.headers[0], hook.bodies[0])
	var hookBody struct {
		Type  string            `json:"type"`
		Count int               `json:"count"`
		Rule  map[string]string `json:"rule"`
		Data  map[string]any    `json:"data"`
	}
	if err := json.Unmarshal([]byte(hook.bodies[0]), &hookBody); err != nil || hookBody.Type != "role.update" || hookBody.Rule["id"] != hookID || hookBody.Count != 1 || hookBody.Data["target"] != "Ada" {
		t.Fatalf("webhook body: %s", hook.bodies[0])
	}

	deliveries, err := st.ListWebhookDeliveries(ctx, slackID, 10)
	if err != nil || len(deliveries) != 1 || deliveries[0].RuleID != slackID || deliveries[0].EndpointID != "" || deliveries[0].Status != "delivered" || deliveries[0].Body != slack.bodies[0] {
		t.Fatalf("slack deliveries: %+v %v", deliveries, err)
	}
	rule, err := st.GetNotificationRule(ctx, discordID)
	if err != nil || rule.LastSentAt == nil || len(rule.Hits) != 1 || rule.LastStatus == nil || *rule.LastStatus != "delivered" {
		t.Fatalf("discord rule state: %+v %v", rule, err)
	}
}

func TestRuleTestSendRefusesPrivateAddresses(t *testing.T) {
	st, _, h := setup(t)
	security := person(t, st, "sam@example.com", "security_admin")
	rc := &receiver{}
	srv := httptest.NewServer(rc)
	defer srv.Close()
	ruleID, _, err := st.CreateNotificationRule(context.Background(), store.NewNotificationRule{Name: "Rebinding", Destination: "slack", URL: srv.URL + "/services/s3cr3tT0k3n", Host: "hooks.example.com",
		Events: []string{"*"}, Threshold: 1, WindowMinutes: 10, QuietMinutes: 15})
	if err != nil {
		t.Fatal(err)
	}
	rec := call(t, h, security, "POST", "/api/v1/notification-rules/"+ruleID+"/test", nil)
	var delivery store.WebhookDelivery
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &delivery) != nil || delivery.Status != "pending" || delivery.EventType != "webhook.test" || !strings.Contains(delivery.Error, "private") {
		t.Fatalf("test send: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "s3cr3tT0k3n") || strings.Contains(rec.Body.String(), srv.URL) {
		t.Fatalf("the delivery error names the destination URL: %s", rec.Body.String())
	}
	if len(rc.bodies) != 0 {
		t.Fatalf("the guarded client reached a loopback address %d times", len(rc.bodies))
	}
}

func TestChatMessagesShowSignInFieldsLiterally(t *testing.T) {
	email := "[a](https://e.example)\n*Risk:* high"
	data, _ := json.Marshal(map[string]string{"email": email, "reason": "bad password"})
	event := store.WebhookEvent{ID: "evt_1", Type: "sign_in.failure", Time: time.Now(), Data: data}

	body, err := render(store.NotificationRule{Name: "Failures", Destination: "slack"}, event, 1)
	if err != nil {
		t.Fatal(err)
	}
	var slack map[string]string
	if err := json.Unmarshal(body, &slack); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(slack["text"], "\n"); len(lines) != 5 || !strings.Contains(slack["text"], "*Account:* `[a](https://e.example) *Risk:* high`") {
		t.Fatalf("slack text: %q", slack["text"])
	}

	body, err = render(store.NotificationRule{Name: "Failures", Destination: "discord"}, event, 1)
	if err != nil {
		t.Fatal(err)
	}
	var discord struct {
		Embeds []struct {
			Fields []struct{ Name, Value string } `json:"fields"`
		} `json:"embeds"`
	}
	if err := json.Unmarshal(body, &discord); err != nil || len(discord.Embeds) != 1 {
		t.Fatalf("discord body: %s", body)
	}
	fields := map[string]string{}
	for _, f := range discord.Embeds[0].Fields {
		fields[f.Name] = f.Value
	}
	if account := fields["Account"]; strings.Contains(account, "](") || strings.ContainsAny(account, "\r\n") || account != `\[a\]\(https://e.example\) \*Risk:\* high` {
		t.Fatalf("discord account field: %q", account)
	}
}
