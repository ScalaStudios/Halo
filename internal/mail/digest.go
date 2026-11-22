package mail

import (
	"cmp"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"halo/internal/config"
	"halo/internal/posture"
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

type digestCopy struct {
	Lang, Title, Badge, Protection, RisksLink, Privileged, RolesLink, Nobody, Fresh, FreshLink, Console, Footer string

	subject, hello, helloName, summary, intro, introName           string
	riskEvents, highRisk, riskyPeople, failed, stillOpen, openTile string
	assignedRow, reviewRisks, reviewRoles, reviewFresh             string
	noRisk, highNote, mediumOnly, nobodyFlagged, checkSignIns      string
	nothingOpen, waiting, lastWeek, allClear, preheader            string
	needReview, freshCount, newRisks, assignments, assigned        [2]string
	roles                                                          map[string]string
	day                                                            func(t time.Time, year bool) string
}

var digestCopies = map[string]digestCopy{
	"en": {
		Lang: "en", Title: "Weekly security digest", Badge: "Weekly digest", Protection: "Identity protection", RisksLink: "Risk events", Privileged: "Privileged roles", RolesLink: "Roles",
		Nobody: "Nobody holds an administrative role.", Fresh: "New security recommendations", FreshLink: "Recommendations", Console: "Open the console",
		Footer:  "You get this email because you are a Global or Security administrator.",
		subject: "Your weekly Halo security digest", hello: "Hi there, here is what happened in %s this week.", helloName: "Hi %s, here is what happened in %s this week.",
		summary: "Here is what happened in Halo from %s.", intro: "Hi,\n\n", introName: "Hi %s,\n\n",
		riskEvents: "New risk events", highRisk: "High-risk events", riskyPeople: "People with risk events", failed: "Failed sign-ins", stillOpen: "Risk events still open", openTile: "Open risk events",
		assignedRow: "Roles assigned this week", reviewRisks: "Review risk events: ", reviewRoles: "Review role holders: ", reviewFresh: "Review them: ",
		noRisk: "None this week", highNote: "%d high risk", mediumOnly: "Medium risk only", nobodyFlagged: "Nobody flagged", checkSignIns: "Check their sign-ins",
		nothingOpen: "Nothing open", waiting: "Waiting for review", lastWeek: "Last 7 days", allClear: "All clear this week", preheader: "%s and %s this week.",
		needReview: [2]string{"risk event needs review", "risk events need review"}, freshCount: [2]string{"new security recommendation", "new security recommendations"},
		newRisks: [2]string{"new risk event", "new risk events"}, assignments: [2]string{"role assignment", "role assignments"}, assigned: [2]string{"role assigned this week", "roles assigned this week"},
		roles: store.RoleLabels,
		day: func(t time.Time, year bool) string {
			if year {
				return t.Format("2 January 2006")
			}
			return t.Format("2 January")
		},
	},
	"es": {
		Lang: "es", Title: "Resumen semanal de seguridad", Badge: "Resumen semanal", Protection: "Protección de la identidad", RisksLink: "Eventos de riesgo", Privileged: "Roles con privilegios", RolesLink: "Roles",
		Nobody: "Nadie tiene un rol de administración.", Fresh: "Nuevas recomendaciones de seguridad", FreshLink: "Recomendaciones", Console: "Abrir la consola",
		Footer:  "Recibes este correo porque eres administrador global o de seguridad.",
		subject: "Tu resumen semanal de seguridad de Halo", hello: "Hola, esto es lo que pasó en %s esta semana.", helloName: "Hola, %s: esto es lo que pasó en %s esta semana.",
		summary: "Esto es lo que pasó en Halo (%s).", intro: "Hola:\n\n", introName: "Hola, %s:\n\n",
		riskEvents: "Nuevos eventos de riesgo", highRisk: "Eventos de riesgo alto", riskyPeople: "Personas con eventos de riesgo", failed: "Inicios de sesión fallidos", stillOpen: "Eventos de riesgo aún abiertos", openTile: "Eventos de riesgo abiertos",
		assignedRow: "Roles asignados esta semana", reviewRisks: "Revisa los eventos de riesgo: ", reviewRoles: "Revisa quién tiene cada rol: ", reviewFresh: "Revísalas: ",
		noRisk: "Ninguno esta semana", highNote: "%d de riesgo alto", mediumOnly: "Solo riesgo medio", nobodyFlagged: "Nadie señalado", checkSignIns: "Revisa sus inicios de sesión",
		nothingOpen: "Nada abierto", waiting: "Pendientes de revisión", lastWeek: "Últimos 7 días", allClear: "Todo en orden esta semana", preheader: "%s y %s esta semana.",
		needReview: [2]string{"evento de riesgo pendiente de revisión", "eventos de riesgo pendientes de revisión"}, freshCount: [2]string{"nueva recomendación de seguridad", "nuevas recomendaciones de seguridad"},
		newRisks: [2]string{"nuevo evento de riesgo", "nuevos eventos de riesgo"}, assignments: [2]string{"asignación de rol", "asignaciones de rol"}, assigned: [2]string{"rol asignado esta semana", "roles asignados esta semana"},
		roles: map[string]string{
			"global_admin":   "Administrador global",
			"security_admin": "Administrador de seguridad",
			"user_admin":     "Administrador de usuarios",
			"helpdesk_admin": "Administrador del servicio de asistencia",
			"app_admin":      "Administrador de aplicaciones",
			"auditor":        "Auditor",
		},
		day: func(t time.Time, year bool) string {
			if year {
				return fmt.Sprintf("%d de %s de %d", t.Day(), months[t.Month()-1], t.Year())
			}
			return fmt.Sprintf("%d de %s", t.Day(), months[t.Month()-1])
		},
	},
}

func WeeklyDigest(to, name, organization, locale string, d store.Digest, fresh []posture.Check, base string) (Message, error) {
	c := digestCopies[locale]
	count := func(n int, words [2]string) string { return plural(n, words[0], words[1]) }
	var text strings.Builder
	width := 28
	for _, label := range []string{c.riskEvents, c.highRisk, c.riskyPeople, c.failed, c.stillOpen, c.assignedRow} {
		width = max(width, utf8.RuneCountInString(label))
	}
	for _, r := range d.Roles {
		width = max(width, utf8.RuneCountInString(c.roles[r.Role]))
	}
	row := func(label string, value int) { fmt.Fprintf(&text, "  %-*s %d\n", width, label, value) }
	period := c.day(d.From.UTC(), false) + " – " + c.day(d.To.UTC(), true)
	first := firstName(name)
	intro, hello := c.intro, fmt.Sprintf(c.hello, organization)
	if first != "" {
		intro, hello = fmt.Sprintf(c.introName, first), fmt.Sprintf(c.helloName, first, organization)
	}
	text.WriteString(intro + fmt.Sprintf(c.summary, period) + "\n\n" + c.Protection + "\n")
	row(c.riskEvents, d.RiskEvents)
	row(c.highRisk, d.HighRiskEvents)
	row(c.riskyPeople, d.RiskyUsers)
	row(c.failed, d.FailedSignIns)
	row(c.stillOpen, d.OpenRiskEvents)
	text.WriteString("\n" + c.Privileged + "\n")
	row(c.assignedRow, d.RolesAssigned)
	for _, r := range d.Roles {
		row(c.roles[r.Role], r.Members)
	}
	text.WriteString("\n" + c.reviewRisks + base + "/admin/risk\n" + c.reviewRoles + base + "/admin/roles")
	if len(fresh) > 0 {
		text.WriteString("\n\n" + c.Fresh + "\n")
		for _, check := range fresh {
			text.WriteString("  " + check.Title + ". " + check.Detail + "\n")
		}
		text.WriteString(c.reviewFresh + base + "/admin/recommendations")
	}

	riskNote, riskTone := c.noRisk, calm
	if d.HighRiskEvents > 0 {
		riskNote, riskTone = fmt.Sprintf(c.highNote, d.HighRiskEvents), alarm
	} else if d.RiskEvents > 0 {
		riskNote, riskTone = c.mediumOnly, caution
	}
	people := tile{c.riskyPeople, d.RiskyUsers, c.nobodyFlagged, calm}
	if d.RiskyUsers > 0 {
		people.Note, people.Tone = c.checkSignIns, caution
	}
	open := tile{c.openTile, d.OpenRiskEvents, c.nothingOpen, calm}
	if d.OpenRiskEvents > 0 {
		open.Note, open.Tone = c.waiting, alarm
	}
	assigned := c.assigned[1]
	if d.RolesAssigned == 1 {
		assigned = c.assigned[0]
	}
	headline := c.allClear
	if d.OpenRiskEvents > 0 {
		headline = count(d.OpenRiskEvents, c.needReview)
	} else if len(fresh) > 0 {
		headline = count(len(fresh), c.freshCount)
	}
	roles := make([]roleRow, len(d.Roles))
	for i, r := range d.Roles {
		roles[i] = roleRow{c.roles[r.Role], r.Members, max(4, r.Members*100/d.Roles[0].Members)}
	}
	var html strings.Builder
	err := digestTemplate.Execute(&html, map[string]any{
		"T":            c,
		"Preheader":    fmt.Sprintf(c.preheader, count(d.RiskEvents, c.newRisks), count(d.RolesAssigned, c.assignments)),
		"Range":        period,
		"Headline":     headline,
		"Hello":        hello,
		"Organization": organization,
		"Tiles": [][]tile{
			{{c.riskEvents, d.RiskEvents, riskNote, riskTone}, people},
			{{c.failed, d.FailedSignIns, c.lastWeek, neutral}, open},
		},
		"RolesAssigned":      d.RolesAssigned,
		"RolesAssignedLabel": assigned,
		"Roles":              roles,
		"RisksURL":           base + "/admin/risk",
		"RolesURL":           base + "/admin/roles",
		"Recommendations":    fresh,
		"RecommendationsURL": base + "/admin/recommendations",
		"ConsoleURL":         base + "/admin",
	})
	return Message{To: to, Subject: c.subject, Text: text.String(), HTML: html.String()}, err
}

func SendWeeklyDigest(ctx context.Context, m Mailer, cfg config.Config, st *store.Store) error {
	claimed, err := st.Claim(ctx, "weeklyDigestSentAt", 7*24*time.Hour)
	if err != nil || !claimed {
		return err
	}
	d, err := st.WeeklyDigest(ctx, time.Now())
	if err != nil {
		return err
	}
	checks, err := posture.List(ctx, st, d.To)
	if err != nil {
		return err
	}
	open := posture.Open(checks)
	ids := make([]string, len(open))
	for i, c := range open {
		ids[i] = c.ID
	}
	reported, err := st.SwapDigestRecommendations(ctx, ids)
	if err != nil {
		return err
	}
	fresh := slices.DeleteFunc(open, func(c posture.Check) bool { return slices.Contains(reported, c.ID) })
	org := organization(ctx, cfg, st)
	settings, err := st.Settings(ctx)
	if err != nil {
		return err
	}
	var failed []error
	for _, u := range d.Recipients {
		locale := cmp.Or(settings.Locale, "en")
		if u.Locale != nil {
			locale = *u.Locale
		}
		msg, err := WeeklyDigest(u.Email, u.Name, org, locale, d, fresh, cfg.Issuer())
		if err == nil {
			err = m.Send(ctx, msg)
		}
		failed = append(failed, err)
	}
	return errors.Join(failed...)
}
