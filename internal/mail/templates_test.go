package mail

import (
	"context"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
	"time"

	"halo/internal/mail/mailtest"
	"halo/internal/store"
)

func TestDefaultMessagesUnchanged(t *testing.T) {
	expires := time.Date(2026, 3, 4, 14, 5, 0, 0, time.UTC)
	magic, requests := "https://halo.example.com/sign-in/magic?token=t", "https://halo.example.com/account/access"
	for _, c := range []struct {
		name          string
		got           Message
		subject, text string
	}{
		{"invite", Invite("sam@example.com", "Sam Lee", "Grace Hopper", "https://halo.example.com/enroll/abc", expires), "Set up your Halo account", "Hi Sam,\n\nGrace Hopper invited you to Halo, where you sign in to your work applications with a passkey instead of a password.\n\nSet up your passkey with this link:\n\nhttps://halo.example.com/enroll/abc\n\nThe link works once and expires 4 March 2026, 14:05 UTC. If you weren't expecting an invitation, you can ignore this email."},
		{"invite-noname", Invite("sam@example.com", "", "Grace Hopper", "https://halo.example.com/enroll/abc", expires), "Set up your Halo account", "Hi,\n\nGrace Hopper invited you to Halo, where you sign in to your work applications with a passkey instead of a password.\n\nSet up your passkey with this link:\n\nhttps://halo.example.com/enroll/abc\n\nThe link works once and expires 4 March 2026, 14:05 UTC. If you weren't expecting an invitation, you can ignore this email."},
		{"reset", Reset("sam@example.com", "Sam", "Grace Hopper", "https://halo.example.com/enroll/abc", expires), "Reset how you sign in to Halo", "Hi Sam,\n\nGrace Hopper reset how you sign in to Halo. Your passkeys, security keys, authenticator apps and recovery codes no longer work, and you were signed out everywhere.\n\nSet up a new passkey with this link:\n\nhttps://halo.example.com/enroll/abc\n\nThe link works once and expires 4 March 2026, 14:05 UTC. If you didn't expect this, contact your administrator before you use it."},
		{"reset-noname", Reset("sam@example.com", "  ", "Grace Hopper", "https://halo.example.com/enroll/abc", expires), "Reset how you sign in to Halo", "Hi,\n\nGrace Hopper reset how you sign in to Halo. Your passkeys, security keys, authenticator apps and recovery codes no longer work, and you were signed out everywhere.\n\nSet up a new passkey with this link:\n\nhttps://halo.example.com/enroll/abc\n\nThe link works once and expires 4 March 2026, 14:05 UTC. If you didn't expect this, contact your administrator before you use it."},
		{"magic", MagicLink("ada@example.com", "Ada Lovelace", "", magic, ""), "Your Halo sign-in link", "Hi Ada,\n\nOpen this link to sign in to Halo:\n\nhttps://halo.example.com/sign-in/magic?token=t\n\nIt works once and expires in 10 minutes. If you didn't ask to sign in, you can ignore this email."},
		{"magic-app", MagicLink("ada@example.com", "Ada", "Grafana", magic, ""), "Your Halo sign-in link", "Hi Ada,\n\nOpen this link to sign in to Grafana:\n\nhttps://halo.example.com/sign-in/magic?token=t\n\nIt works once and expires in 10 minutes. Open it in the browser where you started signing in. If you didn't ask to sign in, you can ignore this email."},
		{"magic-noname", MagicLink("ada@example.com", "", "", magic, ""), "Your Halo sign-in link", "Hi,\n\nOpen this link to sign in to Halo:\n\nhttps://halo.example.com/sign-in/magic?token=t\n\nIt works once and expires in 10 minutes. If you didn't ask to sign in, you can ignore this email."},
		{"code", MagicLink("ada@example.com", "Ada", "", "", "123456"), "Your Halo sign-in code", "Hi Ada,\n\nEnter this code on the sign-in page to sign in to Halo:\n\n123456\n\nIt works once and expires in 10 minutes. Don't share the code with anyone. If you didn't ask to sign in, you can ignore this email."},
		{"code-app", MagicLink("ada@example.com", "Ada", "Grafana", "", "123456"), "Your Halo sign-in code", "Hi Ada,\n\nEnter this code on the sign-in page to sign in to Grafana:\n\n123456\n\nIt works once and expires in 10 minutes. Don't share the code with anyone. If you didn't ask to sign in, you can ignore this email."},
		{"code-link", MagicLink("ada@example.com", "Ada", "", magic, "123456"), "Your Halo sign-in code", "Hi Ada,\n\nEnter this code on the sign-in page to sign in to Halo:\n\n123456\n\nOr open this link:\n\nhttps://halo.example.com/sign-in/magic?token=t\n\nEach works once and expires in 10 minutes. Don't share the code with anyone. If you didn't ask to sign in, you can ignore this email."},
		{"code-link-app", MagicLink("ada@example.com", "", "Grafana", magic, "123456"), "Your Halo sign-in code", "Hi,\n\nEnter this code on the sign-in page to sign in to Grafana:\n\n123456\n\nOr open this link in the browser where you started signing in:\n\nhttps://halo.example.com/sign-in/magic?token=t\n\nEach works once and expires in 10 minutes. Don't share the code with anyone. If you didn't ask to sign in, you can ignore this email."},
		{"request", AccessRequested("ap@example.com", "Sam Lee", "sam@example.com", "", "Finance tools", "for 30 days", "Quarter close", "", "https://halo.example.com/admin/requests"), "Sam Lee requests Finance tools", "Sam Lee (sam@example.com) requested the access package Finance tools for 30 days.\n\nReason: Quarter close\n\nApprove or deny it in Halo: https://halo.example.com/admin/requests"},
		{"request-noreason", AccessRequested("ap@example.com", "Sam Lee", "sam@example.com", "", "Finance tools", "until revoked", "", "", "https://halo.example.com/admin/requests"), "Sam Lee requests Finance tools", "Sam Lee (sam@example.com) requested the access package Finance tools until revoked.\n\nApprove or deny it in Halo: https://halo.example.com/admin/requests"},
		{"approved", AccessDecided("sam@example.com", true, "Grace Hopper", "Finance tools", "4 March 2026, 14:05 UTC", "Finance, Payroll and Audit", "Ends with the quarter", requests), "Your request for Finance tools was approved", "Grace Hopper approved your request for Finance tools. You have access until 4 March 2026, 14:05 UTC.\n\nGroups: Finance, Payroll and Audit. Applications pick up the change the next time you sign in to them.\n\nNote from Grace Hopper: Ends with the quarter\n\nSee your requests: https://halo.example.com/account/access"},
		{"approved-open", AccessDecided("sam@example.com", true, "Grace Hopper", "Finance tools", "", "Finance, Payroll and Audit", "", requests), "Your request for Finance tools was approved", "Grace Hopper approved your request for Finance tools. You have access until it is revoked.\n\nGroups: Finance, Payroll and Audit. Applications pick up the change the next time you sign in to them.\n\nSee your requests: https://halo.example.com/account/access"},
		{"denied", AccessDecided("sam@example.com", false, "Grace Hopper", "Finance tools", "", "", "Ask your manager", requests), "Your request for Finance tools was denied", "Grace Hopper denied your request for Finance tools.\n\nNote from Grace Hopper: Ask your manager\n\nSee your requests: https://halo.example.com/account/access"},
		{"denied-nonote", AccessDecided("sam@example.com", false, "Grace Hopper", "Finance tools", "", "", "", requests), "Your request for Finance tools was denied", "Grace Hopper denied your request for Finance tools.\n\nSee your requests: https://halo.example.com/account/access"},
	} {
		if c.got.Subject != c.subject || c.got.Text != c.text {
			t.Errorf("%s:\n got %q\n     %q\nwant %q\n     %q", c.name, c.got.Subject, c.got.Text, c.subject, c.text)
		}
	}
}

func TestTemplateAllowsOnlyListedVariables(t *testing.T) {
	invite, _ := Lookup("invite")
	for body, want := range map[string]string{
		`{{define "x"}}hi{{end}}{{.Link}}`:              "Remove {{define}} and {{block}}",
		`{{block "x" .}}{{.Link}}{{end}}`:               "Remove {{define}} and {{block}}",
		`{{template "x"}}{{.Link}}`:                     `Remove {{template "x"}}`,
		`{{printf "%s" .Link}}`:                         `function "printf" not defined`,
		`{{.Link | html}}`:                              `function "html" not defined`,
		`{{call .Link}}{{.Link}}`:                       `function "call" not defined`,
		`{{index . "Link"}}`:                            `function "index" not defined`,
		`{{range .Name}}{{end}}{{.Link}}`:               "Remove {{range .Name}}",
		`{{with .Name}}{{end}}{{.Link}}`:                "Remove {{with .Name}}",
		`{{$link := .Link}}{{$link}}`:                   "Remove {{$link := .Link}}",
		`{{.}}{{.Link}}`:                                "Remove {{.}}",
		`{{.Link.Host}}`:                                "Remove {{.Link.Host}}",
		`{{"text"}}{{.Link}}`:                           `Remove {{"text"}}`,
		`{{.Password}}{{.Link}}`:                        "{{.Password}} is not a variable of the invitation email. Use {{.Name}}, {{.Administrator}}, {{.Link}}, {{.Expires}}, {{.Organization}}.",
		`{{if .Secret}}{{.Link}}{{end}}`:                "{{.Secret}} is not a variable",
		`{{if .Name}}{{else}}{{.Code}}{{end}}{{.Link}}`: "{{.Code}} is not a variable",
		`Set up your passkey.`:                          "Include {{.Link}} in the invitation email",
		`{{if .Link}}Set up your passkey.{{end}}`:       "Include {{.Link}} in the invitation email",
		`{{if .Name}}{{.Link}}{{end}}`:                  "Include {{.Link}} in the invitation email",
		`{{.Link`:                                       "The body can't be read",
	} {
		err := invite.Validate(store.EmailTemplate{Subject: "Welcome", Body: body})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want %q", body, err, want)
		}
	}
	for subject, want := range map[string]string{
		"Welcome {{.Code}}":      "{{.Code}} is not a variable",
		"Welcome\nBcc: x@y":      "Write the subject on one line.",
		"":                       "Enter a subject and a body.",
		strings.Repeat("a", 201): "longer than 200 characters",
	} {
		err := invite.Validate(store.EmailTemplate{Subject: subject, Body: "{{.Link}}"})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("subject %q: got %v, want %q", subject, err, want)
		}
	}
	if err := invite.Validate(store.EmailTemplate{Subject: "Open {{.Link}}", Body: "Hello"}); err == nil || !strings.Contains(err.Error(), "Include {{.Link}}") {
		t.Errorf("a link only in the subject: %v", err)
	}
	if err := invite.Validate(store.EmailTemplate{Subject: "{{.Organization}} account for {{.Name}}", Body: "{{if .Name}}Dear {{.Name}}{{else}}Hello{{end}}, open {{.Link}} by {{.Expires}}."}); err != nil {
		t.Fatal(err)
	}
	for _, tpl := range Templates {
		if err := tpl.Validate(tpl.Default); err != nil {
			t.Errorf("default %s: %v", tpl.Kind, err)
		}
	}
}

func TestPersonValuesAreNotEvaluated(t *testing.T) {
	invite, _ := Lookup("invite")
	name := `{{.Link}} <script>alert(1)</script> & "Bob"`
	subject, body, err := invite.Render(store.EmailTemplate{Subject: "For {{.Name}}", Body: "Hello {{.Name}}, open {{.Link}}"}, map[string]string{"Name": name, "Link": "https://halo.example.com/enroll/abc"})
	if err != nil || subject != "For "+name || body != "Hello "+name+", open https://halo.example.com/enroll/abc" {
		t.Fatalf("got %q %q %v", subject, body, err)
	}
}

func TestOutboxSendsCustomTemplate(t *testing.T) {
	server := mailtest.Start(t, 0)
	o, st := outbox(t, server)
	ctx := context.Background()
	expires := time.Date(2026, 10, 11, 14, 2, 0, 0, time.UTC)
	if err := st.SaveEmailTemplate(ctx, "invite", store.EmailTemplate{Subject: "Welcome to {{.Organization}} from {{.Administrator}}", Body: "Bonjour {{.Name}}, {{.Administrator}} vous invite : {{.Link}}"}); err != nil {
		t.Fatal(err)
	}
	st.ForgetSettings()
	if err := o.Send(ctx, Invite("sam@example.com", "Sam Lee", "Uma Patel\r\nBcc: eve@evil.example", "https://auth.example.com/enroll?token=abc", expires)); err != nil {
		t.Fatal(err)
	}
	if err := o.Send(ctx, MagicLink("sam@example.com", "Sam", "", "https://auth.example.com/sign-in/magic?token=xyz", "")); err != nil {
		t.Fatal(err)
	}
	got := server.Deliveries()
	if head, _, _ := strings.Cut(got[0].Data, "\n\n"); len(got) != 2 || strings.Contains(head, "\nBcc:") {
		t.Fatalf("deliveries: %+v", got)
	}
	msg, err := mail.ReadMessage(strings.NewReader(got[0].Data))
	if err != nil {
		t.Fatal(err)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil || subject != "Welcome to Fernway Systems from Uma Patel  Bcc: eve@evil.example" || msg.Header.Get("Bcc") != "" {
		t.Fatalf("subject %q %v, headers %v", subject, err, msg.Header)
	}
	body, _ := io.ReadAll(quotedprintable.NewReader(msg.Body))
	if text := strings.ReplaceAll(string(body), "\r\n", "\n"); text != "Bonjour Sam, Uma Patel\nBcc: eve@evil.example vous invite : https://auth.example.com/enroll?token=abc\n\nHalo · Fernway Systems\n" {
		t.Fatalf("body %q", text)
	}
	if !strings.Contains(got[1].Data, "Subject: Your Halo sign-in link") {
		t.Fatalf("magic link should keep the default: %s", got[1].Data)
	}

	if err := st.SaveEmailTemplate(ctx, "invite", store.EmailTemplate{Subject: "Broken", Body: "{{.Nope}} {{.Link}}"}); err != nil {
		t.Fatal(err)
	}
	st.ForgetSettings()
	if err := o.Send(ctx, Invite("sam@example.com", "Sam", "Uma Patel", "https://auth.example.com/enroll?token=abc", expires)); err != nil {
		t.Fatal(err)
	}
	if queued := latest(t, st); queued.Subject != "Set up your Halo account" {
		t.Fatalf("a stored template that no longer renders should fall back to the default: %+v", queued)
	}
}
