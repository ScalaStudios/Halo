package settings

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/mail"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	spec "halo/api"
	"halo/internal/approval"
	"halo/internal/config"
	"halo/internal/features"
	"halo/internal/httpx"
	halomail "halo/internal/mail"
	"halo/internal/store"
)

const (
	maxLogoBytes = 256 << 10
	maxCustomCSS = 32 << 10
)

var (
	readers       = []string{"security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"}
	domainPattern = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
	logoTypes     = []string{"image/png", "image/jpeg", "image/webp"}
	hexColour     = regexp.MustCompile(`^#[0-9a-f]{6}$`)
	aaguidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	siteKey       = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)
	lookupTXT     = net.DefaultResolver.LookupTXT
)

var fields = []struct {
	key, label string
	global     bool
}{
	{"organizationName", "organization name", true},
	{"contactEmail", "contact email", true},
	{"signInMessage", "sign-in message", true},
	{"theme", "colours", true},
	{"customCss", "custom CSS", true},
	{"consoleBranding", "branding in the console", true},
	{"sessionHours", "session lifetime", false},
	{"lockoutThreshold", "lockout threshold", false},
	{"lockoutMinutes", "lockout window", false},
	{"inviteDays", "setup link lifetime", false},
	{"accessTokenMinutes", "access token lifetime", false},
	{"idTokenMinutes", "ID token lifetime", false},
	{"refreshTokenHours", "refresh token lifetime", false},
	{"passwordSecondFactor", "second factor for passwords", false},
	{"passwordGroupIds", "groups that can use passwords", false},
	{"passwordBreachCheck", "breached password check", false},
	{"emailSignIn", "sign-in email", false},
	{"sessionLimit", "session limit", false},
	{"sessionLimitAction", "action at the session limit", false},
	{"sessionLimitCli", "Halo CLI session limit", false},
	{"sessionLimitGroups", "group session limits", false},
	{"authenticatorRules", "passkey rules for groups", false},
	{"approvalActions", "changes that need approval", true},
	{"emergencyAccessIds", "emergency access accounts", true},
	{"impersonationGroupIds", "groups whose members can be impersonated", true},
	{"reauthMinutes", "re-authentication interval", true},
	{"clientRegistration", "client registration", false},
	{"botProtection", "bot protection", false},
	{"botProtectionSiteKey", "bot protection site key", false},
	{"signInLogDays", "sign-in log retention", true},
	{"auditLogDays", "audit log retention", true},
	{"signUp", "sign-up", false},
	{"signUpDomains", "sign-up domains", false},
	{"signUpGroupIds", "groups for people who sign up", false},
	{"securityEmailsOff", "security emails turned off", false},
	{"lockoutEmailAdmins", "lockout emails to administrators", false},
	{"riskResponses", "automatic risk responses", false},
	{"exportSchedule", "scheduled exports", true},
	{"exportKeep", "number of scheduled exports kept", true},
	{"featuresOff", "features turned off", true},
}

type handler struct {
	st   *store.Store
	cfg  config.Config
	gate *approval.Gate
	mux  *http.ServeMux
}

type Org struct {
	Name            string      `json:"name"`
	Issuer          string      `json:"issuer"`
	ContactEmail    string      `json:"contactEmail"`
	SignInMessage   string      `json:"signInMessage"`
	LogoURL         *string     `json:"logoUrl"`
	Theme           store.Theme `json:"theme"`
	CustomCSS       string      `json:"customCss"`
	ConsoleBranding bool        `json:"consoleBranding"`
	FeaturesOff     []string    `json:"featuresOff"`
}

func Register(mux *http.ServeMux, d httpx.Deps) error {
	h := &handler{st: d.Store, cfg: d.Config, gate: approval.New(d), mux: mux}
	route := func(pattern string, roles []string, fn func(http.ResponseWriter, *http.Request, store.User) error) {
		mux.Handle(pattern, httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
			actor, err := httpx.RequireRole(r, roles...)
			if err != nil {
				return err
			}
			return fn(w, r, actor)
		}))
	}
	route("GET /api/v1/settings", readers, h.get)
	route("PATCH /api/v1/settings", []string{"security_admin"}, h.update)
	mux.Handle("GET /api/v1/branding/logo", httpx.Handle(h.logo))
	route("PUT /api/v1/branding/logo", nil, h.putLogo)
	route("DELETE /api/v1/branding/logo", nil, h.deleteLogo)
	route("GET /api/v1/domains", readers, h.listDomains)
	route("POST /api/v1/domains", nil, h.addDomain)
	route("POST /api/v1/domains/{id}/verify", nil, h.verifyDomain)
	route("DELETE /api/v1/domains/{id}", nil, h.deleteDomain)
	route("GET /api/v1/email-templates", readers, h.listEmailTemplates)
	route("PUT /api/v1/email-templates/{kind}", nil, h.saveEmailTemplate)
	route("DELETE /api/v1/email-templates/{kind}", nil, h.resetEmailTemplate)
	route("POST /api/v1/email-templates/{kind}/preview", readers, h.previewEmailTemplate)
	route("GET /api/v1/export", nil, h.export)
	route("POST /api/v1/import", nil, h.importConfiguration)
	if d.Jobs != nil && d.Config.S3.Configured() {
		d.Jobs.Every("scheduled export", time.Hour, h.scheduledExport)
	}
	mux.HandleFunc("GET /api/v1/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.Write(spec.OpenAPI)
	})
	return nil
}

func Organization(d httpx.Deps) http.Handler {
	h := &handler{st: d.Store, cfg: d.Config}
	return httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		org, err := h.organization(r.Context())
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, org)
	})
}

func (h *handler) organization(ctx context.Context) (Org, error) {
	s, err := h.st.Settings(ctx)
	if err != nil {
		return Org{}, err
	}
	org := Org{Name: s.OrganizationName, Issuer: h.cfg.Issuer(), ContactEmail: s.ContactEmail, SignInMessage: s.SignInMessage, Theme: s.Theme, ConsoleBranding: s.ConsoleBranding, FeaturesOff: s.FeaturesOff}
	if org.Name == "" {
		org.Name = h.cfg.Organization
	}
	if !h.cfg.NoCustomCSS {
		org.CustomCSS = s.CustomCSS
	}
	updated, err := h.st.LogoUpdatedAt(ctx)
	if updated != nil {
		url := fmt.Sprintf("/api/v1/branding/logo?v=%d", updated.Unix())
		org.LogoURL = &url
	}
	return org, err
}

func record(r *http.Request, st *store.Store, actor store.User, e store.AuditEvent) error {
	e.ActorID = &actor.ID
	e.IP = httpx.ClientIP(r)
	return st.RecordAudit(r.Context(), e)
}

func (h *handler) respond(w http.ResponseWriter, r *http.Request) error {
	s, err := h.st.Settings(r.Context())
	if err != nil {
		return err
	}
	_, err = h.st.BotProtectionSecret(r.Context())
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"settings": s, "defaultOrganizationName": h.cfg.Organization, "issuer": h.cfg.Issuer(), "customCssDisabled": h.cfg.NoCustomCSS, "botProtectionSecretSet": err == nil, "bucketConfigured": h.cfg.S3.Configured()})
}

func (h *handler) get(w http.ResponseWriter, r *http.Request, _ store.User) error {
	return h.respond(w, r)
}

func (h *handler) update(w http.ResponseWriter, r *http.Request, actor store.User) error {
	ctx := r.Context()
	before, err := h.st.CurrentSettings(ctx)
	if err != nil {
		return err
	}
	next := before
	next.PasswordGroupIDs = slices.Clone(before.PasswordGroupIDs)
	next.SessionLimitGroups = slices.Clone(before.SessionLimitGroups)
	next.ApprovalActions = slices.Clone(before.ApprovalActions)
	next.EmergencyAccessIDs = slices.Clone(before.EmergencyAccessIDs)
	next.ImpersonationGroups = slices.Clone(before.ImpersonationGroups)
	next.SignUpDomains = slices.Clone(before.SignUpDomains)
	next.SignUpGroupIDs = slices.Clone(before.SignUpGroupIDs)
	next.SecurityEmailsOff = slices.Clone(before.SecurityEmailsOff)
	next.RiskResponses = slices.Clone(before.RiskResponses)
	next.FeaturesOff = slices.Clone(before.FeaturesOff)
	var rules []store.AuthenticatorRule
	in := struct {
		store.Settings
		BotProtectionSecret string                     `json:"botProtectionSecret"`
		AuthenticatorRules  *[]store.AuthenticatorRule `json:"authenticatorRules"`
		ConfirmFeatures     bool                       `json:"confirmFeatures"`
	}{Settings: next, AuthenticatorRules: &rules}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	next = in.Settings
	if in.AuthenticatorRules == nil || rules != nil {
		next.AuthenticatorRules = []store.AuthenticatorRule{}
		for _, rule := range rules {
			aaguids := []string{}
			for _, a := range rule.AAGUIDs {
				if a = strings.ToLower(strings.TrimSpace(a)); a != "" && !slices.Contains(aaguids, a) {
					aaguids = append(aaguids, a)
				}
			}
			rule.AAGUIDs = aaguids
			next.AuthenticatorRules = append(next.AuthenticatorRules, rule)
		}
	}
	botSecret := strings.TrimSpace(in.BotProtectionSecret)
	next.BotProtectionSiteKey = strings.TrimSpace(next.BotProtectionSiteKey)
	next.OrganizationName = strings.TrimSpace(next.OrganizationName)
	next.ContactEmail = strings.TrimSpace(next.ContactEmail)
	next.SignInMessage = strings.TrimSpace(next.SignInMessage)
	next.CustomCSS = strings.TrimSpace(next.CustomCSS)
	for _, c := range colours(&next.Theme) {
		*c = strings.ToLower(strings.TrimSpace(*c))
	}
	if next.PasswordGroupIDs == nil {
		next.PasswordGroupIDs = []string{}
	}
	if next.SessionLimitGroups == nil {
		next.SessionLimitGroups = []store.GroupSessionLimit{}
	}
	if next.ApprovalActions == nil {
		next.ApprovalActions = []string{}
	}
	if next.EmergencyAccessIDs == nil {
		next.EmergencyAccessIDs = []string{}
	}
	if next.ImpersonationGroups == nil {
		next.ImpersonationGroups = []string{}
	}
	if next.SignUpGroupIDs == nil {
		next.SignUpGroupIDs = []string{}
	}
	domains := []string{}
	for _, d := range next.SignUpDomains {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" && !slices.Contains(domains, d) {
			domains = append(domains, d)
		}
	}
	next.SignUpDomains = domains
	next.SecurityEmailsOff = slices.Compact(slices.Sorted(slices.Values(next.SecurityEmailsOff)))
	if next.SecurityEmailsOff == nil {
		next.SecurityEmailsOff = []string{}
	}
	if next.RiskResponses == nil {
		next.RiskResponses = []store.RiskResponse{}
	}
	next.FeaturesOff = slices.Compact(slices.Sorted(slices.Values(next.FeaturesOff)))
	if next.FeaturesOff == nil {
		next.FeaturesOff = []string{}
	}
	if err := validate(next); err != nil {
		return err
	}
	turnedOff := slices.DeleteFunc(slices.Clone(next.FeaturesOff), func(id string) bool { return slices.Contains(before.FeaturesOff, id) })
	if len(turnedOff) > 0 && !in.ConfirmFeatures {
		inUse, err := features.InUse(ctx, h.st, turnedOff)
		if err != nil {
			return err
		}
		if inUse != "" {
			return httpx.Fail(http.StatusConflict, "ERR_FEATURE_IN_USE", inUse+". It stops working while the feature is off. To turn it off anyway, confirm under Settings, Features in the console, or send \"confirmFeatures\": true.")
		}
	}
	if next.ExportSchedule != before.ExportSchedule && next.ExportSchedule != "off" && !h.cfg.S3.Configured() {
		return httpx.Invalid("Scheduled exports are saved to the S3-compatible bucket. Set HALO_S3_ENDPOINT, HALO_S3_BUCKET, HALO_S3_ACCESS_KEY_ID and HALO_S3_SECRET_ACCESS_KEY first.")
	}
	if len(botSecret) > 200 {
		return httpx.Invalid("The bot protection secret key is longer than 200 characters. Copy it again from your provider's dashboard.")
	}
	if next.BotProtection != "" && botSecret == "" {
		if _, err := h.st.BotProtectionSecret(ctx); errors.Is(err, store.ErrNotFound) {
			return httpx.Invalid("Enter the secret key from your bot protection provider to turn bot protection on.")
		} else if err != nil {
			return err
		}
	}
	if !slices.Equal(before.PasswordGroupIDs, next.PasswordGroupIDs) {
		existing, err := h.st.ExistingIDs(ctx, "groups", next.PasswordGroupIDs)
		if err != nil {
			return err
		}
		if slices.ContainsFunc(next.PasswordGroupIDs, func(id string) bool { return !existing[id] }) {
			return httpx.Invalid("One of the groups chosen for passwords no longer exists. Refresh the page and choose again.")
		}
	}
	if !slices.Equal(before.ImpersonationGroups, next.ImpersonationGroups) {
		existing, err := h.st.ExistingIDs(ctx, "groups", next.ImpersonationGroups)
		if err != nil {
			return err
		}
		if slices.ContainsFunc(next.ImpersonationGroups, func(id string) bool { return !existing[id] }) {
			return httpx.Invalid("One of the groups chosen for impersonation no longer exists. Refresh the page and choose again.")
		}
	}
	if !slices.Equal(before.SessionLimitGroups, next.SessionLimitGroups) {
		ids := make([]string, len(next.SessionLimitGroups))
		for i, g := range next.SessionLimitGroups {
			ids[i] = g.GroupID
		}
		existing, err := h.st.ExistingIDs(ctx, "groups", ids)
		if err != nil {
			return err
		}
		if slices.ContainsFunc(ids, func(id string) bool { return !existing[id] }) {
			return httpx.Invalid("One of the groups given a session limit no longer exists. Refresh the page and choose again.")
		}
	}
	if !slices.EqualFunc(before.AuthenticatorRules, next.AuthenticatorRules, func(a, b store.AuthenticatorRule) bool { return a.GroupID == b.GroupID }) {
		ids := make([]string, len(next.AuthenticatorRules))
		for i, rule := range next.AuthenticatorRules {
			ids[i] = rule.GroupID
		}
		existing, err := h.st.ExistingIDs(ctx, "groups", ids)
		if err != nil {
			return err
		}
		if slices.ContainsFunc(ids, func(id string) bool { return !existing[id] }) {
			return httpx.Invalid("One of the groups given passkey rules no longer exists. Refresh the page and choose again.")
		}
	}
	if !slices.Equal(before.SignUpGroupIDs, next.SignUpGroupIDs) {
		existing, err := h.st.ExistingIDs(ctx, "groups", next.SignUpGroupIDs)
		if err != nil {
			return err
		}
		if slices.ContainsFunc(next.SignUpGroupIDs, func(id string) bool { return !existing[id] }) {
			return httpx.Invalid("One of the groups chosen for people who sign up no longer exists. Refresh the page and choose again.")
		}
	}
	if next.SignUp != "off" && len(next.SignUpDomains) == 0 && (before.SignUp != next.SignUp || !slices.Equal(before.SignUpDomains, next.SignUpDomains)) {
		verified, err := h.st.ListDomains(ctx)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(verified, func(d store.Domain) bool { return d.VerifiedAt != nil }) {
			return httpx.Invalid("No domain is verified yet, so nobody could sign up. Verify a domain under Domains, or list the email domains people can sign up with.")
		}
	}
	var old, updated map[string]json.RawMessage
	encoded, _ := json.Marshal(before)
	_ = json.Unmarshal(encoded, &old)
	encoded, _ = json.Marshal(next)
	_ = json.Unmarshal(encoded, &updated)
	var changed []string
	state, values := map[string]json.RawMessage{}, map[string]json.RawMessage{}
	for _, f := range fields {
		if string(old[f.key]) == string(updated[f.key]) {
			continue
		}
		if f.global && !actor.HasRole("global_admin") {
			return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "Only a global administrator can change the "+f.label+". Ask one to make this change.")
		}
		changed = append(changed, f.label)
		state[f.key], values[f.key] = old[f.key], updated[f.key]
	}
	if botSecret != "" {
		changed = append(changed, "bot protection secret key")
	}
	if len(changed) == 0 {
		return h.respond(w, r)
	}
	change := approval.Change{TargetType: "settings", TargetLabel: "Approval settings", State: state}
	if !slices.Equal(before.ApprovalActions, next.ApprovalActions) || !slices.Equal(before.EmergencyAccessIDs, next.EmergencyAccessIDs) {
		if change.Summary, err = h.approvalSummary(ctx, actor, before, next); err != nil {
			return err
		}
		change.Action = "approval-settings"
	} else if !slices.Equal(before.FeaturesOff, next.FeaturesOff) {
		names := []string{}
		for _, id := range next.FeaturesOff {
			names = append(names, features.Name(id))
		}
		change.Action, change.TargetLabel, change.Summary = "features", "Features", "Turn every feature on"
		if len(names) > 0 {
			change.Summary = "Features off after this change: " + strings.Join(names, ", ")
		}
	}
	if held, err := h.gate.Hold(w, r, actor, change); held || err != nil {
		return err
	}
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if change.Action != "" {
			if err := tx.LockApprovalTarget(ctx, "settings", ""); err != nil {
				return err
			}
			current, err := tx.CurrentSettings(ctx)
			if err != nil {
				return err
			}
			if !slices.Equal(current.ApprovalActions, before.ApprovalActions) || !slices.Equal(current.EmergencyAccessIDs, before.EmergencyAccessIDs) {
				return httpx.Fail(http.StatusConflict, "ERR_CONFLICT", "Someone changed the approval settings a moment ago. Refresh the page and make your change again.")
			}
		}
		if err := tx.SaveSettingValues(ctx, values); err != nil {
			return err
		}
		if botSecret != "" {
			if err := tx.SetBotProtectionSecret(ctx, botSecret); err != nil {
				return err
			}
		}
		diff := store.Diff(state, values)
		if botSecret != "" {
			diff = append(diff, store.Change{Field: "botProtectionSecretKey", Redacted: true})
		}
		return record(r, tx, actor, store.AuditEvent{Action: "settings.update", Summary: "Changed the " + strings.Join(changed, ", "), TargetType: "settings", TargetLabel: "Organization settings", Changes: diff})
	})
	h.st.ForgetSettings()
	if err != nil {
		return err
	}
	return h.respond(w, r)
}

func (h *handler) approvalSummary(ctx context.Context, actor store.User, before, next store.Settings) (string, error) {
	labels, names := []string{}, []string{}
	for i, action := range next.ApprovalActions {
		label, ok := store.ApprovalActions[action]
		if !ok || slices.Index(next.ApprovalActions, action) != i {
			return "", httpx.Invalid("Choose changes that need approval from " + strings.Join(slices.Sorted(maps.Keys(store.ApprovalActions)), ", ") + ", each once.")
		}
		labels = append(labels, label)
	}
	if len(next.EmergencyAccessIDs) > 5 {
		return "", httpx.Invalid("Choose at most five emergency access accounts. Keep the list short so every use stands out.")
	}
	for _, userID := range next.EmergencyAccessIDs {
		u, err := h.st.GetUser(ctx, userID)
		if errors.Is(err, store.ErrNotFound) || (err == nil && u.Kind != "person") {
			return "", httpx.Invalid("One of the emergency access accounts is not a person in the directory. Refresh the page and choose again.")
		}
		if err != nil {
			return "", err
		}
		names = append(names, u.Name)
	}
	if len(before.ApprovalActions) == 0 && len(next.ApprovalActions) > 0 {
		others, err := h.st.GlobalAdminEmails(ctx, actor.ID)
		if err != nil {
			return "", err
		}
		if len(others) == 0 {
			return "", httpx.Invalid("Approval needs a second active global administrator to approve your changes. Grant the role to someone else first.")
		}
	}
	summary := "Turn off approval"
	if len(labels) > 0 {
		summary = "Require approval for " + strings.Join(labels, ", ")
	}
	return summary + ". Emergency access accounts: " + cmp.Or(strings.Join(names, ", "), "none"), nil
}

func validate(s store.Settings) error {
	between := func(v, low, high int) bool { return v >= low && v <= high }
	switch {
	case utf8.RuneCountInString(s.OrganizationName) > 100:
		return httpx.Invalid("The organization name is longer than 100 characters. Shorten it and save again.")
	case s.ContactEmail != "" && !plainAddress(s.ContactEmail):
		return httpx.Invalid("Enter the contact email as a plain address, such as it@example.com.")
	case utf8.RuneCountInString(s.SignInMessage) > 500:
		return httpx.Invalid("The sign-in message is longer than 500 characters. Shorten it and save again.")
	case slices.ContainsFunc(colours(&s.Theme), func(c *string) bool { return *c != "" && !hexColour.MatchString(*c) }):
		return httpx.Invalid("Enter each colour as a six-digit hex value such as #fa7e26, or leave it empty to use Halo's.")
	case len(s.CustomCSS) > maxCustomCSS:
		return httpx.Invalid("The custom CSS is larger than 32 KB. Remove rules you don't need and save again.")
	case strings.Contains(s.CustomCSS, "</"):
		return httpx.Invalid(`Custom CSS can't contain "</", because it would end the style element. In a CSS string, write \3c/ instead.`)
	case !between(s.SessionHours, 1, 72):
		return httpx.Invalid("Session lifetime must be between 1 and 72 hours.")
	case !between(s.LockoutThreshold, 3, 20):
		return httpx.Invalid("The lockout threshold must be between 3 and 20 failed passwords or codes.")
	case !between(s.LockoutMinutes, 5, 1440):
		return httpx.Invalid("The lockout window must be between 5 and 1,440 minutes (24 hours).")
	case !between(s.InviteDays, 1, 30):
		return httpx.Invalid("Setup links must last between 1 and 30 days.")
	case !between(s.AccessTokenMinutes, 5, 1440):
		return httpx.Invalid("Access tokens must last between 5 and 1,440 minutes (24 hours).")
	case !between(s.IDTokenMinutes, 5, 1440):
		return httpx.Invalid("ID tokens must last between 5 and 1,440 minutes (24 hours).")
	case !between(s.RefreshTokenHours, 1, 2160):
		return httpx.Invalid("Refresh tokens must last between 1 and 2,160 hours (90 days).")
	case !between(s.ReauthMinutes, 1, 1440):
		return httpx.Invalid("Identity confirmations must last between 1 and 1,440 minutes (24 hours).")
	case !between(s.SessionLimit, 0, 100):
		return httpx.Invalid("The session limit must be between 1 and 100 sessions, or 0 for no limit.")
	case s.SessionLimitAction != "refuse" && s.SessionLimitAction != "end-oldest":
		return httpx.Invalid(`Set the action at the session limit to "refuse" or "end-oldest".`)
	case s.EmailSignIn != "link" && s.EmailSignIn != "code" && s.EmailSignIn != "both":
		return httpx.Invalid(`Set the sign-in email to "link", "code" or "both".`)
	case !slices.Contains([]string{"off", "open", "token", "approval"}, s.ClientRegistration):
		return httpx.Invalid(`Set client registration to "off", "open", "token" or "approval".`)
	case s.BotProtection != "" && s.BotProtection != "turnstile" && s.BotProtection != "hcaptcha":
		return httpx.Invalid(`Set bot protection to "turnstile" or "hcaptcha", or leave it empty to turn it off.`)
	case s.BotProtection != "" && !siteKey.MatchString(s.BotProtectionSiteKey):
		return httpx.Invalid("Enter the site key from your bot protection provider. It has only letters, digits, dashes and underscores.")
	case s.SignInLogDays != 0 && !between(s.SignInLogDays, 7, 3650):
		return httpx.Invalid("Keep the sign-in log for between 7 and 3,650 days (10 years), or 0 to keep it forever.")
	case s.AuditLogDays != 0 && !between(s.AuditLogDays, 30, 3650):
		return httpx.Invalid("Keep the audit log for between 30 and 3,650 days (10 years), or 0 to keep it forever.")
	case !slices.Contains([]string{"off", "open", "approval"}, s.SignUp):
		return httpx.Invalid(`Set sign-up to "off", "open" or "approval".`)
	case len(s.SignUpDomains) > 100:
		return httpx.Invalid("List at most 100 email domains for sign-up.")
	case len(s.RiskResponses) > 20:
		return httpx.Invalid("Add at most 20 automatic risk responses.")
	case !slices.Contains([]string{"off", "daily", "weekly"}, s.ExportSchedule):
		return httpx.Invalid(`Set scheduled exports to "off", "daily" or "weekly".`)
	case !between(s.ExportKeep, 1, 365):
		return httpx.Invalid("Keep between 1 and 365 scheduled exports.")
	case slices.ContainsFunc(s.FeaturesOff, func(id string) bool { return !features.Known(id) }):
		return httpx.Invalid("One of the features to turn off does not exist. Choose features from GET /api/v1/features.")
	}
	for i, r := range s.RiskResponses {
		switch {
		case !slices.Contains([]string{"", "risky-network", "failed-sign-ins", "new-device"}, r.Signal):
			return httpx.Invalid(`Choose the signal of each risk response from "risky-network", "failed-sign-ins" or "new-device", or leave it empty for any signal.`)
		case r.Level != "medium" && r.Level != "high":
			return httpx.Invalid(`Set the level of each risk response to "medium" or "high".`)
		case !slices.Contains([]string{"revoke-sessions", "require-passkey", "suspend", "notify"}, r.Action):
			return httpx.Invalid(`Choose the action of each risk response from "revoke-sessions", "require-passkey", "suspend" or "notify".`)
		case slices.Index(s.RiskResponses, r) != i:
			return httpx.Invalid("Each risk response must be different. Remove the repeated one and save again.")
		}
	}
	for _, kind := range s.SecurityEmailsOff {
		if t, ok := halomail.Lookup(kind); !ok || !t.Security {
			return httpx.Invalid("“" + kind + "” is not a security email. Use new_sign_in, method_added, method_removed, recovery_codes, email_changed, account_locked, access_pass, impersonation or risk_response.")
		}
	}
	for _, d := range s.SignUpDomains {
		if len(d) > 253 || !domainPattern.MatchString(d) {
			return httpx.Invalid("“" + d + "” is not a domain. Enter domains like example.com, without the @ or a path.")
		}
	}
	seen := map[string]bool{}
	for _, g := range s.SessionLimitGroups {
		if !between(g.Limit, 1, 100) {
			return httpx.Invalid("Each group's session limit must be between 1 and 100 sessions.")
		}
		if seen[g.GroupID] {
			return httpx.Invalid("A group can have only one session limit. Remove the repeated group and save again.")
		}
		seen[g.GroupID] = true
	}
	ruled := map[string]bool{}
	for _, rule := range s.AuthenticatorRules {
		switch {
		case ruled[rule.GroupID]:
			return httpx.Invalid("A group can have only one set of passkey rules. Remove the repeated group and save again.")
		case !between(rule.Minimum, 0, 10):
			return httpx.Invalid("The number of passkeys and security keys a group needs must be between 0 and 10.")
		case len(rule.AAGUIDs) > 0 && !rule.Attestation:
			return httpx.Invalid("Approved models are checked through attestation. Turn on attestation for the group, or clear its approved models.")
		case len(rule.AAGUIDs) > 100:
			return httpx.Invalid("List at most 100 approved models for a group.")
		}
		ruled[rule.GroupID] = true
		for _, a := range rule.AAGUIDs {
			if !aaguidPattern.MatchString(a) {
				return httpx.Invalid("“" + a + "” is not an AAGUID. Enter each model's AAGUID like cb69481e-8ff7-4039-93ec-0a2729a154a8.")
			}
		}
	}
	return nil
}

func colours(t *store.Theme) []*string {
	return []*string{&t.Accent, &t.Background, &t.Surface, &t.Border, &t.Text, &t.Link}
}

func plainAddress(v string) bool {
	addr, err := mail.ParseAddress(v)
	return err == nil && addr.Address == v && len(v) <= 254
}

func (h *handler) logo(w http.ResponseWriter, r *http.Request) error {
	l, err := h.st.Logo(r.Context())
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("The logo")
	}
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", l.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, err = w.Write(l.Data)
	return err
}

func (h *handler) putLogo(w http.ResponseWriter, r *http.Request, actor store.User) error {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxLogoBytes))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return httpx.Fail(http.StatusRequestEntityTooLarge, "ERR_TOO_LARGE", "The logo is larger than 256 KB. Export a smaller PNG, JPEG or WebP image and upload it again.")
	}
	if err != nil {
		return err
	}
	contentType := http.DetectContentType(data)
	if len(data) == 0 || !slices.Contains(logoTypes, contentType) {
		return httpx.Invalid("The logo must be a PNG, JPEG or WebP image. SVG and other formats are not accepted.")
	}
	ctx := r.Context()
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.SetLogo(ctx, contentType, data); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "branding.logo.update", Summary: fmt.Sprintf("Uploaded a new sign-in logo (%s, %d KB)", strings.TrimPrefix(contentType, "image/"), (len(data)+1023)/1024), TargetType: "settings", TargetLabel: "Branding"})
	})
	if err != nil {
		return err
	}
	org, err := h.organization(ctx)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, org)
}

func (h *handler) deleteLogo(w http.ResponseWriter, r *http.Request, actor store.User) error {
	ctx := r.Context()
	err := h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.DeleteLogo(ctx); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "branding.logo.remove", Summary: "Removed the sign-in logo", TargetType: "settings", TargetLabel: "Branding"})
	})
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("The logo")
	}
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *handler) listDomains(w http.ResponseWriter, r *http.Request, _ store.User) error {
	domains, err := h.st.ListDomains(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, domains)
}

func (h *handler) addDomain(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in struct {
		Name string `json:"name"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	name := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(in.Name)), ".")
	if len(name) > 253 || !domainPattern.MatchString(name) {
		return httpx.Invalid("Enter a domain name such as example.com, without https:// or a path.")
	}
	ctx := r.Context()
	var d store.Domain
	err := h.st.Tx(ctx, func(tx *store.Store) error {
		var err error
		if d, err = tx.AddDomain(ctx, name); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "domain.add", Summary: "Added " + name + " for verification", TargetType: "domain", TargetID: d.ID, TargetLabel: name})
	})
	if errors.Is(err, store.ErrConflict) {
		return httpx.Fail(http.StatusConflict, "ERR_CONFLICT", name+" is already in the list. Verify it there instead of adding it again.")
	}
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, d)
}

func (h *handler) verifyDomain(w http.ResponseWriter, r *http.Request, actor store.User) error {
	ctx := r.Context()
	d, err := h.st.GetDomain(ctx, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("The domain")
	}
	if err != nil {
		return err
	}
	if d.VerifiedAt != nil {
		return httpx.JSON(w, http.StatusOK, d)
	}
	lookup, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	records, err := lookupTXT(lookup, d.Name)
	want := "halo-verification=" + d.Token
	if !slices.Contains(records, want) {
		reason := "no TXT records"
		if len(records) > 0 {
			reason = fmt.Sprintf("%d TXT records, none of them %s", len(records), want)
		}
		if err != nil {
			slog.InfoContext(ctx, "domain verification lookup failed", "domain", d.Name, "error", err)
		}
		return httpx.Fail(http.StatusUnprocessableEntity, "ERR_NOT_VERIFIED", fmt.Sprintf("Halo found %s at %s. Add the TXT record at your DNS provider, wait for it to publish (often a few minutes, up to an hour), then verify again.", reason, d.Name))
	}
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if d, err = tx.VerifyDomain(ctx, d.ID); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "domain.verify", Summary: "Verified " + d.Name + " with a DNS TXT record", TargetType: "domain", TargetID: d.ID, TargetLabel: d.Name})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, d)
}

func (h *handler) deleteDomain(w http.ResponseWriter, r *http.Request, actor store.User) error {
	ctx := r.Context()
	d, err := h.st.GetDomain(ctx, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("The domain")
	}
	if err != nil {
		return err
	}
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.DeleteDomain(ctx, d.ID); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "domain.remove", Summary: "Removed " + d.Name, TargetType: "domain", TargetID: d.ID, TargetLabel: d.Name})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *handler) export(w http.ResponseWriter, r *http.Request, actor store.User) error {
	ctx := r.Context()
	org, err := h.organization(ctx)
	if err != nil {
		return err
	}
	if err := record(r, h.st, actor, store.AuditEvent{Action: "organization.export", Summary: "Exported the organization's directory, applications and policies as JSON", TargetType: "organization", TargetLabel: org.Name}); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `attachment; filename="halo-export-`+time.Now().UTC().Format("2006-01-02")+`.json"`)
	if err := h.writeExport(ctx, w); err != nil {
		slog.ErrorContext(ctx, "export stopped", "error", err)
	}
	return nil
}

func (h *handler) writeExport(ctx context.Context, w io.Writer) error {
	org, err := h.organization(ctx)
	if err != nil {
		return err
	}
	s, err := h.st.Settings(ctx)
	if err != nil {
		return err
	}
	head, err := json.Marshal(struct {
		ExportedAt   time.Time      `json:"exportedAt"`
		Organization Org            `json:"organization"`
		Settings     store.Settings `json:"settings"`
	}{time.Now().UTC(), org, s})
	if err != nil {
		return err
	}
	write := func(b []byte) error {
		_, err := w.Write(b)
		return err
	}
	if err := write(head[:len(head)-1]); err != nil {
		return err
	}
	for _, section := range store.ExportSections {
		count := 0
		exists, err := h.st.ExportRows(ctx, section, func(row json.RawMessage) error {
			prefix := ","
			if count == 0 {
				prefix = `,"` + section.Name + `":[`
			}
			count++
			return write(append([]byte(prefix), row...))
		})
		if err != nil {
			return fmt.Errorf("%s: %w", section.Name, err)
		}
		closing := "]"
		if count == 0 {
			closing = `,"` + section.Name + `":[]`
		}
		if exists {
			if err := write([]byte(closing)); err != nil {
				return err
			}
		}
	}
	return write([]byte("}\n"))
}
