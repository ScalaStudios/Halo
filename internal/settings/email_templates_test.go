package settings

import (
	"context"
	"strings"
	"testing"

	"halo/internal/store"
)

type emailTemplateResponse struct {
	Kind    string               `json:"kind"`
	Default store.EmailTemplate  `json:"default"`
	Custom  *store.EmailTemplate `json:"custom"`
}

func TestEmailTemplates(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	admin := e.person("gia@example.com", "global_admin")
	security := e.person("sam@example.com", "security_admin")
	auditor := e.person("aud@example.com", "auditor")

	if err := e.st.SaveSettings(ctx, store.DefaultSettings); err != nil {
		t.Fatal(err)
	}
	list := decode[[]emailTemplateResponse](t, e.expect(e.call(auditor, "GET", "/api/v1/email-templates", nil), 200))
	if len(list) != 7 || list[0].Kind != "invite" || list[0].Default.Subject != "Set up your Halo account" || list[0].Custom != nil {
		t.Fatalf("list: %+v", list)
	}

	custom := map[string]string{"subject": " Welcome to {{.Organization}} ", "body": "Hello {{if .Name}}{{.Name}}{{else}}there{{end}}, {{.Administrator}} invited you: {{.Link}}\n"}
	e.expect(e.call(security, "PUT", "/api/v1/email-templates/invite", custom), 403)
	e.expect(e.call(admin, "PUT", "/api/v1/email-templates/welcome", custom), 404)
	for body, want := range map[string]string{
		"{{.Link}} {{.Password}}":        "{{.Password}} is not a variable of the invitation email",
		`{{template "secret"}}{{.Link}}`: "Templates can only use",
		"Hello {{.Name}}":                "Include {{.Link}}",
	} {
		rec := e.expect(e.call(admin, "PUT", "/api/v1/email-templates/invite", map[string]string{"subject": "Hi", "body": body}), 422)
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("%s: %s", body, rec.Body.String())
		}
	}

	preview := decode[map[string]string](t, e.expect(e.call(auditor, "POST", "/api/v1/email-templates/invite/preview", custom), 200))
	if preview["subject"] != "Welcome to Fernway Systems" || preview["text"] != "Hello Ada, Grace Hopper invited you: https://halo.example.com/2f9c4e\n\nHalo · Fernway Systems\n" {
		t.Fatalf("preview: %+v", preview)
	}
	e.expect(e.call(auditor, "POST", "/api/v1/email-templates/invite/preview", map[string]string{"subject": "Hi", "body": "{{.Code}}"}), 422)

	saved := decode[emailTemplateResponse](t, e.expect(e.call(admin, "PUT", "/api/v1/email-templates/invite", custom), 200))
	if saved.Custom == nil || saved.Custom.Subject != "Welcome to {{.Organization}}" || !strings.HasSuffix(saved.Custom.Body, "{{.Link}}") {
		t.Fatalf("saved: %+v", saved)
	}
	e.expect(e.call(admin, "PUT", "/api/v1/email-templates/magic_code", map[string]string{"subject": "Your code", "body": "{{.Code}}"}), 200)
	settings := decode[settingsResponse](t, e.expect(e.call(auditor, "GET", "/api/v1/settings", nil), 200)).Settings
	if len(settings.EmailTemplates) != 2 || settings.EmailTemplates["magic_code"].Body != "{{.Code}}" {
		t.Fatalf("settings: %+v", settings.EmailTemplates)
	}

	e.expect(e.call(security, "DELETE", "/api/v1/email-templates/invite", nil), 403)
	reset := decode[emailTemplateResponse](t, e.expect(e.call(admin, "DELETE", "/api/v1/email-templates/invite", nil), 200))
	if reset.Custom != nil {
		t.Fatalf("reset: %+v", reset)
	}
	e.expect(e.call(admin, "DELETE", "/api/v1/email-templates/invite", nil), 200)
	list = decode[[]emailTemplateResponse](t, e.expect(e.call(auditor, "GET", "/api/v1/email-templates", nil), 200))
	if list[0].Custom != nil || list[3].Kind != "magic_code" || list[3].Custom == nil {
		t.Fatalf("after reset: %+v", list)
	}

	audit, _ := e.st.ListAudit(ctx, 10)
	if len(audit) != 3 || audit[0].Action != "email_template.reset" || audit[0].Summary != "Reset the invitation email to Halo's default" ||
		audit[1].Summary != "Changed the subject and body of the sign-in code email" || audit[2].Action != "email_template.update" {
		t.Fatalf("audit: %+v", audit)
	}

	if export := e.expect(e.call(admin, "GET", "/api/v1/export", nil), 200).Body.String(); !strings.Contains(export, `"magic_code":{"subject":"Your code","body":"{{.Code}}"}`) {
		t.Fatalf("export: %.400s", export)
	}
}
