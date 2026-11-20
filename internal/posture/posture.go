package posture

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"halo/internal/store"
)

type Check struct {
	ID        string           `json:"id"`
	Title     string           `json:"title"`
	Status    string           `json:"status"`
	Detail    string           `json:"detail"`
	Href      string           `json:"href"`
	Action    string           `json:"action"`
	Dismissal *store.Dismissal `json:"dismissal"`
}

type state struct {
	settings   store.Settings
	passwordOn bool
	users      []store.User
	keys       []store.APIKey
	policies   []store.AccessPolicy
	apps       []store.Application
	signing    []store.SigningKeyInfo
}

const day = 24 * time.Hour

func List(ctx context.Context, st *store.Store, now time.Time) ([]Check, error) {
	var s state
	var err error
	if s.settings, err = st.CurrentSettings(ctx); err != nil {
		return nil, err
	}
	if s.passwordOn, err = st.MethodEnabled(ctx, "password"); err != nil {
		return nil, err
	}
	if s.users, err = st.ListUsers(ctx); err != nil {
		return nil, err
	}
	if s.keys, err = st.ListAPIKeys(ctx, ""); err != nil {
		return nil, err
	}
	if s.policies, err = st.ListAccessPolicies(ctx); err != nil {
		return nil, err
	}
	if s.apps, err = st.ListApplications(ctx); err != nil {
		return nil, err
	}
	if s.signing, err = st.ListSigningKeyInfo(ctx); err != nil {
		return nil, err
	}
	dismissals, err := st.Dismissals(ctx)
	if err != nil {
		return nil, err
	}
	checks := evaluate(s, now)
	for i := range checks {
		if d, ok := dismissals[checks[i].ID]; ok {
			checks[i].Dismissal = &d
		}
	}
	return checks, nil
}

func Open(checks []Check) []Check {
	return slices.DeleteFunc(slices.Clone(checks), func(c Check) bool { return c.Status == "pass" || c.Dismissal != nil })
}

func names(list []string) string {
	if len(list) > 5 {
		return fmt.Sprintf("%s and %d more", strings.Join(list[:5], ", "), len(list)-5)
	}
	return strings.Join(list, ", ")
}

func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func date(t time.Time) string {
	return t.UTC().Format("2 January 2006")
}

func (c Check) when(failing bool, status, problem, fine string) Check {
	c.Status, c.Detail = "pass", fine
	if failing {
		c.Status, c.Detail = status, problem
	}
	return c
}

func evaluate(s state, now time.Time) []Check {
	var weak, global, emergency []string
	for _, u := range s.users {
		if u.Status != "active" {
			continue
		}
		if len(u.RoleKeys) > 0 && u.Strength != "phishing-resistant" {
			weak = append(weak, u.Name)
		}
		if u.HasRole("global_admin") {
			global = append(global, u.Name)
		}
		if slices.Contains(s.settings.EmergencyAccessIDs, u.ID) {
			emergency = append(emergency, u.Name)
		}
	}

	var neverExpire, unused []string
	for _, k := range s.keys {
		if k.RevokedAt != nil || (k.ExpiresAt != nil && !k.ExpiresAt.After(now)) {
			continue
		}
		label := k.Label + " (" + k.ServiceAccountName + ")"
		if k.ExpiresAt == nil {
			neverExpire = append(neverExpire, label)
		}
		if cutoff := now.Add(-90 * day); k.CreatedAt.Before(cutoff) && (k.LastUsedAt == nil || k.LastUsedAt.Before(cutoff)) {
			unused = append(unused, label)
		}
	}

	var reporting []string
	policyHref := "/admin/policies"
	for _, p := range s.policies {
		if p.Enabled && p.Mode == "report" && p.UpdatedAt.Before(now.Add(-30*day)) {
			reporting = append(reporting, "“"+p.Name+"”")
			policyHref = "/admin/policies/" + p.ID
		}
	}
	if len(reporting) > 1 {
		policyHref = "/admin/policies"
	}

	var unassigned []string
	appHref := "/admin/applications"
	for _, a := range s.apps {
		if a.Status == "active" && a.Type != "service" && a.Protocol != "bookmark" && a.ClientID != store.CLIClientID && len(a.GroupIDs) == 0 {
			unassigned = append(unassigned, a.Name)
			appHref = "/admin/applications/" + a.ID
		}
	}
	pronoun := "it"
	if len(unassigned) > 1 {
		appHref, pronoun = "/admin/applications", "them"
	}

	var oldKeys []string
	for _, k := range s.signing {
		if k.Active && k.CreatedAt.Before(now.Add(-365*day)) {
			oldKeys = append(oldKeys, "the "+k.Algorithm+" key created on "+date(k.CreatedAt))
		}
	}

	var shortLogs []string
	if d := s.settings.AuditLogDays; d > 0 && d < 90 {
		shortLogs = append(shortLogs, fmt.Sprintf("audit log entries after %d days", d))
	}
	if d := s.settings.SignInLogDays; d > 0 && d < 90 {
		shortLogs = append(shortLogs, fmt.Sprintf("sign-in log entries after %d days", d))
	}

	signUpOn := s.settings.SignUp != "off"
	var exposed, abuse []string
	if signUpOn {
		exposed, abuse = append(exposed, "sign-up is open"), append(abuse, "submit the sign-up form")
	}
	if s.passwordOn {
		exposed, abuse = append(exposed, "password sign-in is on"), append(abuse, "keep failing sign-ins from one address")
	}
	botFine := "Halo asks for a bot check on sign-up, sign-in emails and repeated failed sign-ins."
	if s.settings.BotProtection == "" {
		botFine = "Bot protection is off, but sign-up and password sign-in are off too."
	}

	passwordFine := "Password sign-in is off."
	if s.passwordOn {
		passwordFine = "Only members of " + count(len(s.settings.PasswordGroupIDs), "group", "groups") + " can sign in with a password."
	}

	approvalsOn := len(s.settings.ApprovalActions) > 0
	emergencyFine := "No change needs approval, so there is nothing for an emergency access account to skip."
	if len(emergency) > 0 {
		emergencyFine = names(emergency) + " can skip approval in an emergency."
	}

	approvalProblem := "One global administrator can make anyone a global administrator, including an account they control, without a second person agreeing."
	if len(global) < 2 {
		approvalProblem += " Approval needs a second global administrator, so grant the role to another person first."
	}

	return []Check{
		Check{ID: "admin-sign-in", Title: "Give every administrator a passkey or security key", Href: "/admin/users?view=weak-admins", Action: "Review administrators"}.when(len(weak) > 0, "fail",
			"No passkey or security key yet: "+names(weak)+". A phishing page that captures a password, code or sign-in email can take over their administrator access.",
			"Every active administrator has a passkey or security key."),
		Check{ID: "global-admins", Title: "Keep global administrators to four or fewer", Href: "/admin/roles", Action: "Review roles"}.when(len(global) > 4, "attention",
			fmt.Sprintf("%d people are global administrators: %s. Each can change every setting and grant any role. Give the others a narrower role, such as security or user administrator.", len(global), names(global)),
			count(len(global), "person is a global administrator.", "people are global administrators.")),
		Check{ID: "global-admin-approval", Title: "Require approval to grant global administrator", Href: "/admin/approvals", Action: "Open approvals"}.when(!slices.Contains(s.settings.ApprovalActions, "global-admin"), "attention",
			approvalProblem,
			"Each new global administrator waits for a second global administrator to approve."),
		Check{ID: "emergency-access", Title: "Name an emergency access account", Href: "/admin/approvals", Action: "Open approvals"}.when(approvalsOn && len(emergency) == 0, "attention",
			"Some changes wait for a second global administrator, and nobody can skip that. If no one else is available, an urgent change waits until its request expires after 72 hours.",
			emergencyFine),
		Check{ID: "password-everyone", Title: "Limit password sign-in to the groups that need it", Href: "/admin/methods", Action: "Open sign-in methods"}.when(s.passwordOn && len(s.settings.PasswordGroupIDs) == 0, "attention",
			"Everyone in the organization can sign in with a password, which can be guessed, reused on other sites or phished. Limit passwords to the groups that can't use passkeys yet.",
			passwordFine),
		Check{ID: "lockout", Title: "Lock accounts after 10 failed attempts or fewer", Href: "/admin/settings/security", Action: "Open security defaults"}.when(s.settings.LockoutThreshold > 10, "attention",
			fmt.Sprintf("Halo allows %d incorrect passwords or codes before it locks an account for %d minutes. Lower it to 10 or fewer so a script gets fewer guesses at each account.", s.settings.LockoutThreshold, s.settings.LockoutMinutes),
			fmt.Sprintf("Halo locks an account for %d minutes after %d incorrect passwords or codes.", s.settings.LockoutMinutes, s.settings.LockoutThreshold)),
		Check{ID: "session-lifetime", Title: "Keep sessions to 24 hours or less", Href: "/admin/settings/security", Action: "Open security defaults"}.when(s.settings.SessionHours > 24, "attention",
			fmt.Sprintf("Sessions last %d hours, so a stolen session cookie keeps working for up to %d hours unless someone ends it.", s.settings.SessionHours, s.settings.SessionHours),
			fmt.Sprintf("Sessions last %d hours.", s.settings.SessionHours)),
		Check{ID: "bot-protection", Title: "Turn on bot protection", Href: "/admin/settings/security", Action: "Open security defaults"}.when(s.settings.BotProtection == "" && len(exposed) > 0, "attention",
			"Bot protection is off while "+strings.Join(exposed, " and ")+", so scripts can "+strings.Join(abuse, " and ")+" without a challenge.",
			botFine),
		Check{ID: "api-key-expiry", Title: "Give every API key an expiry date", Href: "/admin/api-keys", Action: "Review API keys"}.when(len(neverExpire) > 0, "attention",
			"Never expire: "+names(neverExpire)+". A leaked key works until someone revokes it, so replace each one with a key that expires.",
			"Every active API key has an expiry date."),
		Check{ID: "api-key-unused", Title: "Revoke API keys unused for 90 days", Href: "/admin/api-keys", Action: "Review API keys"}.when(len(unused) > 0, "attention",
			"Not used in the last 90 days: "+names(unused)+". Revoke any that nothing depends on.",
			"No active API key has gone 90 days without use."),
		Check{ID: "report-only-policies", Title: "Decide on policies left in report-only mode", Href: policyHref, Action: "Review policies"}.when(len(reporting) > 0, "attention",
			"In report-only mode and unchanged for more than 30 days: "+names(reporting)+". Report-only policies record what they would do without doing it, so turn each one on or delete it.",
			"No policy has been left in report-only mode for more than 30 days."),
		Check{ID: "signing-key-age", Title: "Rotate signing keys every year", Href: "/admin/settings/security", Action: "Open signing keys"}.when(len(oldKeys) > 0, "attention",
			"In use for more than a year: "+names(oldKeys)+". Rotating replaces every active key and keeps the previous ones published for 7 days.",
			"Every signing key in use is less than a year old."),
		Check{ID: "unassigned-apps", Title: "Assign or disable applications nobody can use", Href: appHref, Action: "Review applications"}.when(len(unassigned) > 0, "attention",
			"No groups assigned: "+names(unassigned)+". Nobody can sign in to "+pronoun+", so assign the groups that use "+pronoun+" or disable "+pronoun+".",
			"Every active application is assigned to at least one group."),
		Check{ID: "log-retention", Title: "Keep audit and sign-in logs for at least 90 days", Href: "/admin/settings/organization", Action: "Open log retention"}.when(len(shortLogs) > 0, "attention",
			"Halo deletes "+strings.Join(shortLogs, " and ")+", so an incident found later has no trail. Keep at least 90 days.",
			"Halo keeps audit and sign-in log entries for at least 90 days."),
	}
}
