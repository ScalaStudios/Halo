package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	spec "halo/api"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/store"
)

const maxLogoBytes = 256 << 10

var (
	readers       = []string{"security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"}
	domainPattern = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
	logoTypes     = []string{"image/png", "image/jpeg", "image/webp"}
	lookupTXT     = net.DefaultResolver.LookupTXT
)

var fields = []struct {
	key, label string
	global     bool
}{
	{"organizationName", "organization name", true},
	{"contactEmail", "contact email", true},
	{"signInMessage", "sign-in message", true},
	{"sessionHours", "session lifetime", false},
	{"lockoutThreshold", "lockout threshold", false},
	{"lockoutMinutes", "lockout window", false},
	{"inviteDays", "setup link lifetime", false},
	{"accessTokenMinutes", "access token lifetime", false},
	{"idTokenMinutes", "ID token lifetime", false},
	{"refreshTokenHours", "refresh token lifetime", false},
}

type handler struct {
	st  *store.Store
	cfg config.Config
}

type Org struct {
	Name          string  `json:"name"`
	Issuer        string  `json:"issuer"`
	ContactEmail  string  `json:"contactEmail"`
	SignInMessage string  `json:"signInMessage"`
	LogoURL       *string `json:"logoUrl"`
}

func Register(mux *http.ServeMux, d httpx.Deps) error {
	h := &handler{st: d.Store, cfg: d.Config}
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
	route("GET /api/v1/export", nil, h.export)
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
	org := Org{Name: s.OrganizationName, Issuer: h.cfg.Issuer(), ContactEmail: s.ContactEmail, SignInMessage: s.SignInMessage}
	if org.Name == "" {
		org.Name = h.cfg.Organization
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
	return httpx.JSON(w, http.StatusOK, map[string]any{"settings": s, "defaultOrganizationName": h.cfg.Organization, "issuer": h.cfg.Issuer()})
}

func (h *handler) get(w http.ResponseWriter, r *http.Request, _ store.User) error {
	return h.respond(w, r)
}

func (h *handler) update(w http.ResponseWriter, r *http.Request, actor store.User) error {
	ctx := r.Context()
	h.st.ForgetSettings()
	before, err := h.st.Settings(ctx)
	if err != nil {
		return err
	}
	next := before
	if err := httpx.Decode(r, &next); err != nil {
		return err
	}
	next.OrganizationName = strings.TrimSpace(next.OrganizationName)
	next.ContactEmail = strings.TrimSpace(next.ContactEmail)
	next.SignInMessage = strings.TrimSpace(next.SignInMessage)
	if err := validate(next); err != nil {
		return err
	}
	var old, updated map[string]json.RawMessage
	encoded, _ := json.Marshal(before)
	_ = json.Unmarshal(encoded, &old)
	encoded, _ = json.Marshal(next)
	_ = json.Unmarshal(encoded, &updated)
	var changed []string
	for _, f := range fields {
		if string(old[f.key]) == string(updated[f.key]) {
			continue
		}
		if f.global && !actor.HasRole("global_admin") {
			return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "Only a global administrator can change the "+f.label+". Ask one to make this change.")
		}
		changed = append(changed, f.label)
	}
	if len(changed) == 0 {
		return h.respond(w, r)
	}
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.SaveSettings(ctx, next); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "settings.update", Summary: "Changed the " + strings.Join(changed, ", "), TargetType: "settings", TargetLabel: "Organization settings"})
	})
	h.st.ForgetSettings()
	if err != nil {
		return err
	}
	return h.respond(w, r)
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
	case !between(s.SessionHours, 1, 72):
		return httpx.Invalid("Session lifetime must be between 1 and 72 hours.")
	case !between(s.LockoutThreshold, 3, 20):
		return httpx.Invalid("The lockout threshold must be between 3 and 20 failed codes.")
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
	}
	return nil
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
	s, err := h.st.Settings(ctx)
	if err != nil {
		return err
	}
	if err := record(r, h.st, actor, store.AuditEvent{Action: "organization.export", Summary: "Exported the organization's directory, applications and policies as JSON", TargetType: "organization", TargetLabel: org.Name}); err != nil {
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `attachment; filename="halo-export-`+time.Now().UTC().Format("2006-01-02")+`.json"`)
	write := func(b []byte) error {
		_, err := w.Write(b)
		return err
	}
	if err := write(head[:len(head)-1]); err != nil {
		return nil
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
			slog.ErrorContext(ctx, "export stopped", "section", section.Name, "error", err)
			return nil
		}
		closing := "]"
		if count == 0 {
			closing = `,"` + section.Name + `":[]`
		}
		if exists {
			if err := write([]byte(closing)); err != nil {
				return nil
			}
		}
	}
	_ = write([]byte("}\n"))
	return nil
}
