package settings

import (
	"net/http"
	"strings"

	"halo/internal/httpx"
	"halo/internal/mail"
	"halo/internal/store"
)

type emailTemplate struct {
	mail.Template
	Custom *store.EmailTemplate `json:"custom"`
}

func describe(s store.Settings, t mail.Template) emailTemplate {
	out := emailTemplate{Template: t}
	if custom, ok := s.EmailTemplates[t.Kind]; ok {
		out.Custom = &custom
	}
	return out
}

func (h *handler) respondEmailTemplate(w http.ResponseWriter, r *http.Request, t mail.Template) error {
	s, err := h.st.Settings(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, describe(s, t))
}

func (h *handler) listEmailTemplates(w http.ResponseWriter, r *http.Request, _ store.User) error {
	s, err := h.st.Settings(r.Context())
	if err != nil {
		return err
	}
	list := make([]emailTemplate, len(mail.Templates))
	for i, t := range mail.Templates {
		list[i] = describe(s, t)
	}
	return httpx.JSON(w, http.StatusOK, list)
}

func decodeEmailTemplate(r *http.Request) (mail.Template, store.EmailTemplate, error) {
	t, ok := mail.Lookup(r.PathValue("kind"))
	if !ok {
		return t, store.EmailTemplate{}, httpx.NotFound("The email template")
	}
	var in store.EmailTemplate
	if err := httpx.Decode(r, &in); err != nil {
		return t, in, err
	}
	in.Subject, in.Body = strings.TrimSpace(in.Subject), strings.TrimSpace(in.Body)
	return t, in, nil
}

func (h *handler) previewEmailTemplate(w http.ResponseWriter, r *http.Request, _ store.User) error {
	t, in, err := decodeEmailTemplate(r)
	if err != nil {
		return err
	}
	m, err := mail.Preview(r.Context(), h.cfg, h.st, t, in)
	if err != nil {
		return httpx.Invalid(err.Error())
	}
	return httpx.JSON(w, http.StatusOK, map[string]string{"subject": m.Subject, "text": m.Text})
}

func (h *handler) saveEmailTemplate(w http.ResponseWriter, r *http.Request, actor store.User) error {
	t, in, err := decodeEmailTemplate(r)
	if err != nil {
		return err
	}
	if err := t.Validate(in); err != nil {
		return httpx.Invalid(err.Error())
	}
	ctx := r.Context()
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.SaveEmailTemplate(ctx, t.Kind, in); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "email_template.update", Summary: "Changed the subject and body of the " + strings.ToLower(t.Label) + " email", TargetType: "settings", TargetLabel: "Email templates"})
	})
	h.st.ForgetSettings()
	if err != nil {
		return err
	}
	return h.respondEmailTemplate(w, r, t)
}

func (h *handler) resetEmailTemplate(w http.ResponseWriter, r *http.Request, actor store.User) error {
	t, ok := mail.Lookup(r.PathValue("kind"))
	if !ok {
		return httpx.NotFound("The email template")
	}
	ctx := r.Context()
	err := h.st.Tx(ctx, func(tx *store.Store) error {
		removed, err := tx.ResetEmailTemplate(ctx, t.Kind)
		if err != nil || !removed {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "email_template.reset", Summary: "Reset the " + strings.ToLower(t.Label) + " email to Halo's default", TargetType: "settings", TargetLabel: "Email templates"})
	})
	h.st.ForgetSettings()
	if err != nil {
		return err
	}
	return h.respondEmailTemplate(w, r, t)
}
