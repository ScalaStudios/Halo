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
	Default     store.EmailTemplate `json:"default"`
}

var variables = map[string]Variable{
	"Name":           {"Name", "The person's first name. Empty when Halo doesn't know it.", "Ada"},
	"Organization":   {"Organization", "Your organization's name.", "Fernway Systems"},
	"Administrator":  {"Administrator", "The administrator who sent the email.", "Grace Hopper"},
	"Link":           {"Link", "The link the person opens.", "https://halo.example.com/2f9c4e"},
	"Expires":        {"Expires", "When the link or pass stops working.", "4 March 2026, 14:00 UTC"},
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
		Kind: "access_pass", Label: "Temporary access pass", Description: "Sent when an administrator creates a temporary access pass for someone. The pass itself is never emailed.",
		Variables: uses("Name", "Administrator", "Expires", "Organization"),
		Default: store.EmailTemplate{Subject: "A temporary access pass was created for you", Body: greetingLine +
			"{{.Administrator}} created a temporary access pass for your Halo account. It lets you set up a new passkey and stops working {{.Expires}}.\n\n" +
			"Halo never sends the pass by email, so expect it from {{.Administrator}} in person or by phone. If you didn't ask for one, contact your administrator."},
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
