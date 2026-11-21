package mail

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"text/template"
	"text/template/parse"
	"unicode/utf8"

	"halo/internal/config"
	"halo/internal/store"
)

type Variable struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Example     string `json:"example"`
}

type Template struct {
	Kind        string              `json:"kind"`
	Label       string              `json:"label"`
	Description string              `json:"description"`
	Variables   []Variable          `json:"variables"`
	Required    string              `json:"required,omitempty"`
	Security    bool                `json:"security,omitempty"`
	Default     store.EmailTemplate `json:"default"`
}

var variables = map[string]Variable{
	"Name":           {"Name", "The person's first name. Empty when Halo doesn't know it.", "Ada"},
	"Organization":   {"Organization", "Your organization's name.", "Fernway Systems"},
	"Administrator":  {"Administrator", "The administrator who sent the email.", "Grace Hopper"},
	"Link":           {"Link", "The link the person opens.", "https://halo.example.com/2f9c4e"},
	"Expires":        {"Expires", "When the link, pass or guest access stops working.", "4 March 2026, 14:00 UTC"},
	"Application":    {"Application", "The application the person is signing in to. Empty when they sign in to Halo itself.", "Grafana"},
	"Code":           {"Code", "The six-digit sign-in code.", "482913"},
	"Requester":      {"Requester", "The person who asked for access.", "Sam Lee"},
	"RequesterEmail": {"RequesterEmail", "The requester's email address.", "sam@example.com"},
	"Package":        {"Package", "The access package.", "Finance tools"},
	"Duration":       {"Duration", "How long the access was asked for, such as for 30 days or until revoked.", "for 30 days"},
	"Reason":         {"Reason", "The requester's reason. Empty when they gave none.", "Quarter close"},
	"Approver":       {"Approver", "The person who approved or denied the request.", "Grace Hopper"},
	"Groups":         {"Groups", "The groups the approval added the person to.", "Finance and Payroll"},
	"Note":           {"Note", "The approver's note. Empty when they left none.", "Ends with the quarter"},
	"Guest":          {"Guest", "The guest whose access is ending.", "Sam Lee"},
	"GuestEmail":     {"GuestEmail", "The guest's email address.", "sam@partner.example"},
	"Time":           {"Time", "When it happened.", "4 March 2026, 13:45 UTC"},
	"Device":         {"Device", "The browser and operating system that signed in.", "Firefox 143 · Linux"},
	"IP":             {"IP", "The IP address of the sign-in.", "203.0.113.24"},
	"Method":         {"Method", "The passkey, security key or authenticator app by its name in the account portal, or A new password.", "YubiKey 5C"},
	"OldEmail":       {"OldEmail", "The email address the account used before.", "ada@old.example.com"},
	"NewEmail":       {"NewEmail", "The email address the account uses now.", "ada@example.com"},
	"Attempts":       {"Attempts", "How many incorrect passwords or codes locked the account.", "5"},
	"Until":          {"Until", "When the lock or the impersonation ends.", "4 March 2026, 14:00 UTC"},
	"Person":         {"Person", "The person whose account was locked.", "Sam Lee"},
	"PersonEmail":    {"PersonEmail", "The email address of the person whose account was locked.", "sam@example.com"},
	"Purpose":        {"Purpose", "What the administrator is helping with.", "Can't open Grafana"},
	"Risk":           {"Risk", "What made the sign-in risky.", "203.0.113.24 is in the risky network “Tor exit nodes”."},
	"Response":       {"Response", "What Halo did about it. Empty when it only tells the person.", "Halo ended your sessions, so you need to sign in again."},
}

func uses(names ...string) []Variable {
	list := make([]Variable, len(names))
	for i, name := range names {
		list[i] = variables[name]
	}
	return list
}

const greetingLine = "Hi{{if .Name}} {{.Name}}{{end}},\n\n"

var Templates = []Template{
	{
		Kind: "invite", Label: "Invitation", Description: "Sent when an administrator invites someone to set up their account.", Required: "Link",
		Variables: uses("Name", "Administrator", "Link", "Expires", "Organization"),
		Default: store.EmailTemplate{Subject: "Set up your Halo account", Body: greetingLine +
			"{{.Administrator}} invited you to Halo, where you sign in to your work applications with a passkey instead of a password.\n\n" +
			"Set up your passkey with this link:\n\n{{.Link}}\n\n" +
			"The link works once and expires {{.Expires}}. If you weren't expecting an invitation, you can ignore this email."},
	},
	{
		Kind: "reset", Label: "Authentication reset", Description: "Sent when an administrator resets how someone signs in.", Required: "Link",
		Variables: uses("Name", "Administrator", "Link", "Expires", "Organization"),
		Default: store.EmailTemplate{Subject: "Reset how you sign in to Halo", Body: greetingLine +
			"{{.Administrator}} reset how you sign in to Halo. Your passkeys, security keys, authenticator apps and recovery codes no longer work, and you were signed out everywhere.\n\n" +
			"Set up a new passkey with this link:\n\n{{.Link}}\n\n" +
			"The link works once and expires {{.Expires}}. If you didn't expect this, contact your administrator before you use it."},
	},
	{
		Kind: "magic_link", Label: "Sign-in link", Description: "Sent when someone asks for a sign-in email and email sign-in sends a link.", Required: "Link",
		Variables: uses("Name", "Application", "Link", "Organization"),
		Default: store.EmailTemplate{Subject: "Your Halo sign-in link", Body: greetingLine +
			"Open this link to sign in to {{if .Application}}{{.Application}}{{else}}Halo{{end}}:\n\n{{.Link}}\n\n" +
			"It works once and expires in 10 minutes.{{if .Application}} Open it in the browser where you started signing in.{{end}} If you didn't ask to sign in, you can ignore this email."},
	},
	{
		Kind: "magic_code", Label: "Sign-in code", Description: "Sent when someone asks for a sign-in email and email sign-in sends a code, or a code and a link.", Required: "Code",
		Variables: uses("Name", "Application", "Code", "Link", "Organization"),
		Default: store.EmailTemplate{Subject: "Your Halo sign-in code", Body: greetingLine +
			"Enter this code on the sign-in page to sign in to {{if .Application}}{{.Application}}{{else}}Halo{{end}}:\n\n{{.Code}}\n\n" +
			"{{if .Link}}Or open this link{{if .Application}} in the browser where you started signing in{{end}}:\n\n{{.Link}}\n\nEach works{{else}}It works{{end}}" +
			" once and expires in 10 minutes. Don't share the code with anyone. If you didn't ask to sign in, you can ignore this email."},
	},
	{
		Kind: "sign_up", Label: "Sign-up", Description: "Sent when someone signs up, to confirm their email address before they create a passkey.", Required: "Link",
		Variables: uses("Name", "Link", "Expires", "Organization"),
		Default: store.EmailTemplate{Subject: "Finish signing up for Halo", Body: greetingLine +
			"Confirm your email address and create your passkey to finish signing up for Halo:\n\n{{.Link}}\n\n" +
			"The link works once and expires {{.Expires}}. If you didn't sign up, you can ignore this email and no account is created."},
	},
	{
		Kind: "access_request", Label: "Access request", Description: "Sent to approvers when someone asks for an access package.",
		Variables: uses("Requester", "RequesterEmail", "Package", "Duration", "Reason", "Link", "Organization"),
		Default: store.EmailTemplate{Subject: "{{.Requester}} requests {{.Package}}", Body: "{{.Requester}} ({{.RequesterEmail}}) requested the access package {{.Package}} {{.Duration}}." +
			"{{if .Reason}}\n\nReason: {{.Reason}}{{end}}\n\nApprove or deny it in Halo: {{.Link}}"},
	},
	{
		Kind: "access_approved", Label: "Access approved", Description: "Sent to the requester when their access request is approved.",
		Variables: uses("Approver", "Package", "Expires", "Groups", "Note", "Link", "Organization"),
		Default: store.EmailTemplate{Subject: "Your request for {{.Package}} was approved", Body: "{{.Approver}} approved your request for {{.Package}}. You have access until {{if .Expires}}{{.Expires}}{{else}}it is revoked{{end}}.\n\n" +
			"Groups: {{.Groups}}. Applications pick up the change the next time you sign in to them.{{if .Note}}\n\nNote from {{.Approver}}: {{.Note}}{{end}}\n\nSee your requests: {{.Link}}"},
	},
	{
		Kind: "access_denied", Label: "Access denied", Description: "Sent to the requester when their access request is denied.",
		Variables: uses("Approver", "Package", "Note", "Link", "Organization"),
		Default: store.EmailTemplate{Subject: "Your request for {{.Package}} was denied", Body: "{{.Approver}} denied your request for {{.Package}}." +
			"{{if .Note}}\n\nNote from {{.Approver}}: {{.Note}}{{end}}\n\nSee your requests: {{.Link}}"},
	},
	{
		Kind: "access_pass", Label: "Temporary access pass", Description: "Sent when an administrator creates a temporary access pass for someone. The pass itself is never emailed.", Security: true,
		Variables: uses("Name", "Administrator", "Expires", "Organization"),
		Default: store.EmailTemplate{Subject: "A temporary access pass was created for you", Body: greetingLine +
			"{{.Administrator}} created a temporary access pass for your Halo account. It lets you set up a new passkey and stops working {{.Expires}}.\n\n" +
			"Halo never sends the pass by email, so expect it from {{.Administrator}} in person or by phone. If you didn't ask for one, contact your administrator."},
	},
	{
		Kind: "new_sign_in", Label: "New sign-in", Description: "Sent to a person when they sign in from a device or network their account hasn't used before. People can turn it off for themselves in the account portal.", Security: true,
		Variables: uses("Name", "Time", "Device", "IP", "Application", "Link", "Organization"),
		Default: store.EmailTemplate{Subject: "New sign-in to your Halo account", Body: greetingLine +
			"Your Halo account signed in{{if .Application}} to {{.Application}}{{end}} from a device or network it hasn't used before.\n\nWhen: {{.Time}}\nDevice: {{.Device}}\nIP address: {{.IP}}\n\n" +
			"If this was you, you don't need to do anything. If it wasn't, sign out that device in the account portal, replace your passkeys and tell your administrator:\n\n{{.Link}}"},
	},
	{
		Kind: "method_added", Label: "Sign-in method added", Description: "Sent to a person when they add a passkey, security key, authenticator app or password to their account.", Security: true,
		Variables: uses("Name", "Method", "Time", "Link", "Organization"),
		Default: store.EmailTemplate{Subject: "A sign-in method was added to your Halo account", Body: greetingLine +
			"{{.Method}} was added to your Halo account on {{.Time}} and can now sign in as you.\n\n" +
			"If you didn't add it, remove it in the account portal and tell your administrator:\n\n{{.Link}}"},
	},
	{
		Kind: "method_removed", Label: "Sign-in method removed", Description: "Sent to a person when one of their passkeys, security keys, authenticator apps or their password is removed, by them or by an administrator.", Security: true,
		Variables: uses("Name", "Method", "Administrator", "Time", "Link", "Organization"),
		Default: store.EmailTemplate{Subject: "A sign-in method was removed from your Halo account", Body: greetingLine +
			"{{.Method}} was removed from your Halo account{{if .Administrator}} by {{.Administrator}}{{end}} on {{.Time}} and no longer signs you in.\n\n" +
			"If you didn't expect this, check your sign-in methods in the account portal and tell your administrator:\n\n{{.Link}}"},
	},
	{
		Kind: "recovery_codes", Label: "Recovery codes replaced", Description: "Sent to a person when new recovery codes are generated for their account. The codes are never emailed.", Security: true,
		Variables: uses("Name", "Time", "Link", "Organization"),
		Default: store.EmailTemplate{Subject: "New recovery codes for your Halo account", Body: greetingLine +
			"New recovery codes were generated for your Halo account on {{.Time}}, and your previous codes no longer work.\n\n" +
			"If you didn't do this, generate new codes in the account portal and tell your administrator:\n\n{{.Link}}"},
	},
	{
		Kind: "email_changed", Label: "Email address changed", Description: "Sent to both the old and the new address when the email address of a person's account changes.", Security: true,
		Variables: uses("Name", "OldEmail", "NewEmail", "Administrator", "Time", "Link", "Organization"),
		Default: store.EmailTemplate{Subject: "The email address of your Halo account changed", Body: greetingLine +
			"{{.Administrator}} changed the email address of your Halo account from {{.OldEmail}} to {{.NewEmail}} on {{.Time}}. Halo sent this email to both addresses.\n\n" +
			"If you didn't expect this, contact your administrator. Review your account in the account portal:\n\n{{.Link}}"},
	},
	{
		Kind: "account_locked", Label: "Account locked", Description: "Sent to a person when incorrect passwords or codes lock their account.", Security: true,
		Variables: uses("Name", "Attempts", "Until", "Link", "Organization"),
		Default: store.EmailTemplate{Subject: "Your Halo account is locked", Body: greetingLine +
			"Halo locked your account after {{.Attempts}} incorrect passwords or codes. You can try again after {{.Until}}.\n\n" +
			"If these attempts weren't yours, someone may be guessing your password. Review your recent sign-ins in the account portal and tell your administrator:\n\n{{.Link}}"},
	},
	{
		Kind: "account_locked_admin", Label: "Account locked, to administrators", Description: "Sent to global administrators when incorrect passwords or codes lock someone's account, when turned on under Security defaults.",
		Variables: uses("Person", "PersonEmail", "Attempts", "Until", "Link", "Organization"),
		Default: store.EmailTemplate{Subject: "The Halo account of {{.Person}} is locked", Body: "Halo locked the account of {{.Person}} ({{.PersonEmail}}) after {{.Attempts}} incorrect passwords or codes. They can try again after {{.Until}}.\n\n" +
			"See the person and their recent sign-ins in Halo: {{.Link}}"},
	},
	{
		Kind: "impersonation", Label: "Impersonation started", Description: "Sent to a person when a helpdesk administrator starts seeing Halo as them.", Security: true,
		Variables: uses("Name", "Administrator", "Purpose", "Until", "Link", "Organization"),
		Default: store.EmailTemplate{Subject: "{{.Administrator}} is viewing Halo as you", Body: greetingLine +
			"{{.Administrator}} started viewing your Halo account as you, to help with: {{.Purpose}}\n\n" +
			"They can look but not change anything, and their access ends by {{.Until}}. If you did not expect this, tell your IT team, or end it now by signing out of your other sessions:\n\n{{.Link}}"},
	},
	{
		Kind: "risk_response", Label: "Risky sign-in", Description: "Sent to a person when an automatic risk response under Risk events tells them about a risky sign-in to their account.", Security: true,
		Variables: uses("Name", "Time", "Risk", "Response", "Link", "Organization"),
		Default: store.EmailTemplate{Subject: "A risky sign-in to your Halo account", Body: greetingLine +
			"Halo noticed a risky sign-in to your account on {{.Time}}: {{.Risk}}\n\n{{if .Response}}{{.Response}}\n\n{{end}}" +
			"If this was you, you don't need to do anything. If it wasn't, replace your passkeys, review your recent sign-ins and tell your administrator:\n\n{{.Link}}"},
	},
	{
		Kind: "guest_expiring", Label: "Guest access ending", Description: "Sent to a guest's sponsor before the guest's access ends, or to global administrators when the sponsor is no longer active.",
		Variables: uses("Guest", "GuestEmail", "Expires", "Link", "Organization"),
		Default: store.EmailTemplate{Subject: "Guest access for {{.Guest}} ends soon", Body: "The guest account of {{.Guest}} ({{.GuestEmail}}) stops working {{.Expires}}, and Halo suspends it then.\n\n" +
			"If they still need access, extend it in Halo: {{.Link}}\n\nIf they don't, you don't need to do anything."},
	},
}

func Lookup(kind string) (Template, bool) {
	i := slices.IndexFunc(Templates, func(t Template) bool { return t.Kind == kind })
	if i < 0 {
		return Template{}, false
	}
	return Templates[i], true
}

func (t Template) Render(custom store.EmailTemplate, vars map[string]string) (string, string, error) {
	switch {
	case strings.TrimSpace(custom.Subject) == "" || strings.TrimSpace(custom.Body) == "":
		return "", "", errors.New("Enter a subject and a body.")
	case strings.ContainsAny(custom.Subject, "\r\n"):
		return "", "", errors.New("Write the subject on one line.")
	case utf8.RuneCountInString(custom.Subject) > 200:
		return "", "", errors.New("The subject is longer than 200 characters. Shorten it and save again.")
	case utf8.RuneCountInString(custom.Body) > 10000:
		return "", "", errors.New("The body is longer than 10,000 characters. Shorten it and save again.")
	}
	printed := map[string]bool{}
	var out [2]strings.Builder
	for i, part := range [][2]string{{"subject", custom.Subject}, {"body", custom.Body}} {
		trees, err := parse.Parse(part[0], part[1], "", "")
		if err != nil {
			return "", "", fmt.Errorf("The %s can't be read: %s.", part[0], strings.TrimPrefix(err.Error(), "template: "))
		}
		if len(trees) != 1 {
			return "", "", fmt.Errorf("Remove {{define}} and {{block}} from the %s. Templates can only use {{.Variable}} and {{if .Variable}}…{{else}}…{{end}}.", part[0])
		}
		clear(printed)
		if err := t.check(trees[part[0]].Root, printed); err != nil {
			return "", "", err
		}
		tmpl, err := template.New(part[0]).Option("missingkey=zero").AddParseTree(part[0], trees[part[0]])
		if err == nil {
			err = tmpl.Execute(&out[i], vars)
		}
		if err != nil {
			return "", "", err
		}
	}
	if t.Required != "" && !printed[t.Required] {
		return "", "", fmt.Errorf("Include {{.%s}} in the %s email. People need it to sign in.", t.Required, strings.ToLower(t.Label))
	}
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(out[0].String()), out[1].String(), nil
}

func (t Template) check(node parse.Node, printed map[string]bool) error {
	switch n := node.(type) {
	case *parse.TextNode:
		return nil
	case *parse.ListNode:
		for _, child := range n.Nodes {
			if err := t.check(child, printed); err != nil {
				return err
			}
		}
		return nil
	case *parse.ActionNode:
		name, err := t.variable(n.Pipe)
		printed[name] = true
		return err
	case *parse.IfNode:
		if _, err := t.variable(n.Pipe); err != nil {
			return err
		}
		if err := t.check(n.List, map[string]bool{}); err != nil || n.ElseList == nil {
			return err
		}
		return t.check(n.ElseList, map[string]bool{})
	}
	return fmt.Errorf("Remove %s. Templates can only use {{.Variable}} and {{if .Variable}}…{{else}}…{{end}}.", node)
}

func (t Template) variable(pipe *parse.PipeNode) (string, error) {
	if len(pipe.Decl) == 0 && len(pipe.Cmds) == 1 && len(pipe.Cmds[0].Args) == 1 {
		if field, ok := pipe.Cmds[0].Args[0].(*parse.FieldNode); ok && len(field.Ident) == 1 {
			if !slices.ContainsFunc(t.Variables, func(v Variable) bool { return v.Name == field.Ident[0] }) {
				names := make([]string, len(t.Variables))
				for i, v := range t.Variables {
					names[i] = "{{." + v.Name + "}}"
				}
				return "", fmt.Errorf("{{.%s}} is not a variable of the %s email. Use %s.", field.Ident[0], strings.ToLower(t.Label), strings.Join(names, ", "))
			}
			return field.Ident[0], nil
		}
	}
	return "", fmt.Errorf("Remove {{%s}}. Templates can only use {{.Variable}} and {{if .Variable}}…{{else}}…{{end}}.", pipe)
}

func (t Template) examples() map[string]string {
	vars := map[string]string{}
	for _, v := range t.Variables {
		vars[v.Name] = v.Example
	}
	return vars
}

func (t Template) Validate(custom store.EmailTemplate) error {
	_, _, err := t.Render(custom, t.examples())
	return err
}

func Preview(ctx context.Context, cfg config.Config, st *store.Store, t Template, custom store.EmailTemplate) (Message, error) {
	vars := t.examples()
	org := organization(ctx, cfg, st)
	vars["Organization"] = org
	subject, text, err := t.Render(custom, vars)
	return Message{Subject: subject, Text: text + footer(org)}, err
}

func build(kind, to string, vars map[string]string) Message {
	t, _ := Lookup(kind)
	subject, text, _ := t.Render(t.Default, vars)
	return Message{To: to, Subject: subject, Text: text, kind: kind, vars: vars}
}
