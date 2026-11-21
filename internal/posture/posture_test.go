package posture

import (
	"slices"
	"strings"
	"testing"
	"time"

	"halo/internal/store"
)

var now = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

func ago(days int) *time.Time {
	t := now.Add(-time.Duration(days) * day)
	return &t
}

func admin(id, name string) store.User {
	return store.User{ID: id, Name: name, Status: "active", RoleKeys: []string{"global_admin"}, Strength: "phishing-resistant"}
}

func healthy() state {
	settings := store.DefaultSettings
	settings.ApprovalActions = []string{"global-admin"}
	settings.EmergencyAccessIDs = []string{"u1"}
	settings.BotProtection = "turnstile"
	settings.PasswordGroupIDs = []string{"g1"}
	return state{
		settings:   settings,
		passwordOn: true,
		users:      []store.User{admin("u1", "Uma Patel"), admin("u2", "Sam Lee"), {ID: "u3", Name: "Ola Berg", Status: "active", Strength: "single-factor"}},
		keys: []store.APIKey{
			{Label: "Terraform", ServiceAccountName: "CI", CreatedAt: *ago(200), ExpiresAt: ago(-30), LastUsedAt: ago(1)},
			{Label: "Old", ServiceAccountName: "CI", CreatedAt: *ago(400), RevokedAt: ago(100)},
			{Label: "Expired", ServiceAccountName: "CI", CreatedAt: *ago(400), ExpiresAt: ago(5)},
			{Label: "New", ServiceAccountName: "CI", CreatedAt: *ago(10), ExpiresAt: ago(-80)},
		},
		policies: []store.AccessPolicy{
			{ID: "p1", Name: "Block risky networks", Enabled: true, Mode: "enforce", UpdatedAt: *ago(300)},
			{ID: "p2", Name: "Require security keys", Enabled: true, Mode: "report", UpdatedAt: *ago(10)},
			{ID: "p3", Name: "Old trial", Enabled: false, Mode: "report", UpdatedAt: *ago(300)},
		},
		apps: []store.Application{
			{ID: "a1", Name: "Grafana", Status: "active", Protocol: "oidc", Type: "web", GroupIDs: []string{"g1"}},
			{ID: "a2", Name: "Backups", Status: "active", Protocol: "oauth", Type: "service"},
			{ID: "a3", Name: "Wiki", Status: "active", Protocol: "bookmark", Type: "web"},
			{ID: "a4", Name: "Retired", Status: "disabled", Protocol: "saml", Type: "web"},
			{ID: "a5", Name: "Halo CLI", Status: "active", Protocol: "oidc", Type: "native", ClientID: store.CLIClientID},
		},
		signing: []store.SigningKeyInfo{
			{Algorithm: "RS256", Active: true, CreatedAt: *ago(100)},
			{Algorithm: "RS256", Active: false, CreatedAt: *ago(500)},
		},
	}
}

func find(t *testing.T, checks []Check, id string) Check {
	t.Helper()
	i := slices.IndexFunc(checks, func(c Check) bool { return c.ID == id })
	if i < 0 {
		t.Fatalf("no check %q", id)
	}
	return checks[i]
}

func TestHealthyOrganizationPassesEveryCheck(t *testing.T) {
	checks := evaluate(healthy(), now)
	if len(checks) != 14 {
		t.Fatalf("got %d checks", len(checks))
	}
	for _, c := range checks {
		if c.Status != "pass" || c.Detail == "" || c.Title == "" || !strings.HasPrefix(c.Href, "/admin") || c.Action == "" {
			t.Errorf("%s: %+v", c.ID, c)
		}
	}
}

func TestEachCheckFiresAndClears(t *testing.T) {
	for _, tc := range []struct {
		id, status, mentions, href string
		breakIt, fix               func(*state)
	}{
		{"admin-sign-in", "fail", "Sam Lee", "/admin/users?view=weak-admins",
			func(s *state) { s.users[1].Strength = "multi-factor" },
			func(s *state) { s.users[1].Strength = "phishing-resistant" }},
		{"global-admins", "attention", "5 people", "/admin/roles",
			func(s *state) { s.users = append(s.users, admin("u4", "A"), admin("u5", "B"), admin("u6", "C")) },
			func(s *state) { s.users[3].RoleKeys = []string{"security_admin"} }},
		{"global-admin-approval", "attention", "without a second person", "/admin/approvals",
			func(s *state) { s.settings.ApprovalActions = []string{"policy-disable"} },
			func(s *state) { s.settings.ApprovalActions = append(s.settings.ApprovalActions, "global-admin") }},
		{"emergency-access", "attention", "72 hours", "/admin/approvals",
			func(s *state) { s.settings.EmergencyAccessIDs = []string{"deleted-user"} },
			func(s *state) { s.settings.EmergencyAccessIDs = []string{"u2"} }},
		{"password-everyone", "attention", "Everyone in the organization", "/admin/methods",
			func(s *state) { s.settings.PasswordGroupIDs = []string{} },
			func(s *state) { s.passwordOn = false }},
		{"lockout", "attention", "11 incorrect", "/admin/settings/security",
			func(s *state) { s.settings.LockoutThreshold = 11 },
			func(s *state) { s.settings.LockoutThreshold = 10 }},
		{"session-lifetime", "attention", "25 hours", "/admin/settings/security",
			func(s *state) { s.settings.SessionHours = 25 },
			func(s *state) { s.settings.SessionHours = 24 }},
		{"bot-protection", "attention", "password sign-in is on", "/admin/settings/security",
			func(s *state) { s.settings.BotProtection = "" },
			func(s *state) { s.settings.BotProtection = "hcaptcha" }},
		{"api-key-expiry", "attention", "Terraform (CI)", "/admin/api-keys",
			func(s *state) { s.keys[0].ExpiresAt = nil },
			func(s *state) { s.keys[0].RevokedAt = ago(0) }},
		{"api-key-unused", "attention", "Terraform (CI)", "/admin/api-keys",
			func(s *state) { s.keys[0].LastUsedAt = ago(91) },
			func(s *state) { s.keys[0].LastUsedAt = ago(0) }},
		{"report-only-policies", "attention", "“Require security keys”", "/admin/policies/p2",
			func(s *state) { s.policies[1].UpdatedAt = *ago(31) },
			func(s *state) { s.policies[1].Mode = "enforce" }},
		{"signing-key-age", "attention", "the RS256 key created on", "/admin/settings/security",
			func(s *state) { s.signing[0].CreatedAt = *ago(366) },
			func(s *state) {
				s.signing[0].Active = false
				s.signing = append(s.signing, store.SigningKeyInfo{Algorithm: "RS256", Active: true, CreatedAt: now})
			}},
		{"unassigned-apps", "attention", "Grafana. Nobody can sign in to it,", "/admin/applications/a1",
			func(s *state) { s.apps[0].GroupIDs = nil },
			func(s *state) { s.apps[0].Status = "disabled" }},
		{"log-retention", "attention", "audit log entries after 30 days", "/admin/settings/organization",
			func(s *state) { s.settings.AuditLogDays = 30 },
			func(s *state) { s.settings.AuditLogDays = 90 }},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s := healthy()
			tc.breakIt(&s)
			checks := evaluate(s, now)
			got := find(t, checks, tc.id)
			if got.Status != tc.status || !strings.Contains(got.Detail, tc.mentions) || got.Href != tc.href {
				t.Fatalf("fired: %+v", got)
			}
			for _, c := range checks {
				if c.ID != tc.id && c.Status != "pass" {
					t.Errorf("%s also fired: %s", c.ID, c.Detail)
				}
			}
			tc.fix(&s)
			if fixed := find(t, evaluate(s, now), tc.id); fixed.Status != "pass" {
				t.Fatalf("fixed: %+v", fixed)
			}
		})
	}
}

func TestChecksPassForTheRightReasons(t *testing.T) {
	s := healthy()
	s.passwordOn = false
	s.settings.PasswordGroupIDs = []string{}
	s.settings.BotProtection = ""
	s.settings.ApprovalActions = []string{}
	s.keys[0].LastUsedAt = nil
	s.keys[0].CreatedAt = *ago(89)
	checks := evaluate(s, now)
	for id, detail := range map[string]string{
		"password-everyone": "Password sign-in is off.",
		"bot-protection":    "Bot protection is off, but sign-up and password sign-in are off too.",
		"emergency-access":  "Blocking policies never apply to Uma Patel",
		"api-key-unused":    "No active API key",
	} {
		if c := find(t, checks, id); c.Status != "pass" || !strings.HasPrefix(c.Detail, detail) {
			t.Errorf("%s: %+v", id, c)
		}
	}
	if c := find(t, checks, "global-admin-approval"); c.Status != "attention" || strings.Contains(c.Detail, "grant the role to another person") {
		t.Errorf("two global administrators can turn approval on: %+v", c)
	}
	s.settings.EmergencyAccessIDs = []string{}
	if c := find(t, evaluate(s, now), "emergency-access"); c.Status != "attention" || !strings.Contains(c.Detail, "blocks every administrator") {
		t.Errorf("no emergency access account is a lockout risk even without approval: %+v", c)
	}

	s = healthy()
	s.users = s.users[:1]
	s.settings.SignUp = "open"
	s.settings.BotProtection = ""
	s.settings.ApprovalActions = []string{}
	s.policies[0].Mode, s.policies[1].UpdatedAt = "report", *ago(40)
	checks = evaluate(s, now)
	if c := find(t, checks, "global-admin-approval"); !strings.Contains(c.Detail, "grant the role to another person first") {
		t.Errorf("one global administrator: %+v", c)
	}
	if c := find(t, checks, "bot-protection"); !strings.Contains(c.Detail, "sign-up is open and password sign-in is on") || !strings.Contains(c.Detail, "submit the sign-up form and keep failing") {
		t.Errorf("bot protection: %+v", c)
	}
	if c := find(t, checks, "report-only-policies"); c.Href != "/admin/policies" || !strings.Contains(c.Detail, "“Block risky networks”, “Require security keys”") {
		t.Errorf("two report-only policies: %+v", c)
	}
}

func TestOpenLeavesOutPassingAndDismissedChecks(t *testing.T) {
	checks := []Check{{ID: "a", Status: "pass"}, {ID: "b", Status: "fail"}, {ID: "c", Status: "attention", Dismissal: &store.Dismissal{Reason: "Handled elsewhere"}}, {ID: "d", Status: "attention"}}
	open := Open(checks)
	if len(open) != 2 || open[0].ID != "b" || open[1].ID != "d" || len(checks) != 4 {
		t.Fatalf("open: %+v", open)
	}
}
