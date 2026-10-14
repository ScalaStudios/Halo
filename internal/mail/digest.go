package mail

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"strings"
	"time"

	"halo/internal/config"
	"halo/internal/store"
)

//go:embed digest.html
var digestHTML string

var digestTemplate = template.Must(template.New("digest").Parse(digestHTML))

type tone struct{ Background, Text template.CSS }

var (
	calm    = tone{"#e6f5ed", "#1d7a4d"}
	caution = tone{"#fcf3dc", "#8a6208"}
	alarm   = tone{"#fce8ea", "#b01e30"}
	neutral = tone{"#f5f4f2", "#57534d"}
)

type tile struct {
	Label string
	Value int
	Note  string
	Tone  tone
}

type roleRow struct {
	Label   string
	Members int
	Width   int
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func WeeklyDigest(to, name, organization string, d store.Digest, base string) (Message, error) {
	var text strings.Builder
	row := func(label string, value int) { fmt.Fprintf(&text, "  %-28s %d\n", label, value) }
	period := d.From.UTC().Format("2 January") + " – " + d.To.UTC().Format("2 January 2006")
	text.WriteString(greeting(name) + "Here is what happened in Halo from " + period + ".\n\nIdentity protection\n")
	row("New risk events", d.RiskEvents)
	row("High-risk events", d.HighRiskEvents)
	row("People with risk events", d.RiskyUsers)
	row("Failed sign-ins", d.FailedSignIns)
	row("Risk events still open", d.OpenRiskEvents)
	text.WriteString("\nPrivileged roles\n")
	row("Roles assigned this week", d.RolesAssigned)
	for _, r := range d.Roles {
		row(store.RoleLabels[r.Role], r.Members)
	}
	text.WriteString("\nReview risk events: " + base + "/admin/risk\nReview role holders: " + base + "/admin/roles")

	riskNote, riskTone := "None this week", calm
	if d.HighRiskEvents > 0 {
		riskNote, riskTone = fmt.Sprintf("%d high risk", d.HighRiskEvents), alarm
	} else if d.RiskEvents > 0 {
		riskNote, riskTone = "Medium risk only", caution
	}
	people := tile{"People with risk events", d.RiskyUsers, "Nobody flagged", calm}
	if d.RiskyUsers > 0 {
		people.Note, people.Tone = "Check their sign-ins", caution
	}
	open := tile{"Open risk events", d.OpenRiskEvents, "Nothing open", calm}
	if d.OpenRiskEvents > 0 {
		open.Note, open.Tone = "Waiting for review", alarm
	}
	assigned := "roles assigned"
	if d.RolesAssigned == 1 {
		assigned = "role assigned"
	}
	headline := "All clear this week"
	if d.OpenRiskEvents > 0 {
		headline = plural(d.OpenRiskEvents, "risk event needs", "risk events need") + " review"
	}
	roles := make([]roleRow, len(d.Roles))
	for i, r := range d.Roles {
		roles[i] = roleRow{store.RoleLabels[r.Role], r.Members, max(4, r.Members*100/d.Roles[0].Members)}
	}
	first := strings.Fields(name)
	if len(first) == 0 {
		first = []string{"there"}
	}
	var html strings.Builder
	err := digestTemplate.Execute(&html, map[string]any{
		"Preheader":    plural(d.RiskEvents, "new risk event", "new risk events") + " and " + plural(d.RolesAssigned, "role assignment", "role assignments") + " this week.",
		"Range":        period,
		"Headline":     headline,
		"Name":         first[0],
		"Organization": organization,
		"Tiles": [][]tile{
			{{"New risk events", d.RiskEvents, riskNote, riskTone}, people},
			{{"Failed sign-ins", d.FailedSignIns, "Last 7 days", neutral}, open},
		},
		"RolesAssigned":      d.RolesAssigned,
		"RolesAssignedLabel": assigned,
		"Roles":              roles,
		"RisksURL":           base + "/admin/risk",
		"RolesURL":           base + "/admin/roles",
		"ConsoleURL":         base + "/admin",
	})
	return Message{To: to, Subject: "Your weekly Halo security digest", Text: text.String(), HTML: html.String()}, err
}

func SendWeeklyDigest(ctx context.Context, m Mailer, cfg config.Config, st *store.Store) error {
	claimed, err := st.ClaimWeeklyDigest(ctx)
	if err != nil || !claimed {
		return err
	}
	d, err := st.WeeklyDigest(ctx, time.Now())
	if err != nil {
		return err
	}
	org := organization(ctx, cfg, st)
	var failed []error
	for _, u := range d.Recipients {
		msg, err := WeeklyDigest(u.Email, u.Name, org, d, cfg.Issuer())
		if err == nil {
			err = m.Send(ctx, msg)
		}
		failed = append(failed, err)
	}
	return errors.Join(failed...)
}
