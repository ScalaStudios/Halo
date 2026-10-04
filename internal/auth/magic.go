package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"halo/internal/httpx"
	"halo/internal/mail"
	"halo/internal/store"
)

const (
	magicLinkLifetime = 10 * time.Minute
	magicLinkWindow   = 15 * time.Minute
	magicLinkLimit    = 5
)

var errMagicLinkExpired = httpx.Fail(http.StatusNotFound, "ERR_LINK_EXPIRED", "This sign-in link has expired or was already used. Request a new one from the sign-in page.")

func (h *handler) magicLink(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Email       string `json:"email"`
		AuthRequest string `json:"authRequest"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	app, err := h.target(r.Context(), in.AuthRequest)
	if err != nil {
		return err
	}
	ctx := context.WithoutCancel(r.Context())
	go func() {
		if err := h.mailMagicLink(ctx, strings.TrimSpace(in.Email), in.AuthRequest, app); err != nil {
			slog.ErrorContext(ctx, "magic link not sent", "error", err)
		}
	}()
	return httpx.JSON(w, http.StatusAccepted, map[string]string{})
}

func (h *handler) mailMagicLink(ctx context.Context, email, authRequest string, app *store.Application) error {
	settings, err := h.st.ListMethodSettings(ctx)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(settings, func(m store.MethodSetting) bool { return m.Method == "magic-link" && !m.Enabled }) {
		return nil
	}
	u, err := h.st.GetUserByEmail(ctx, email)
	if errors.Is(err, store.ErrNotFound) || err == nil && u.Status != "active" {
		return nil
	}
	if err != nil {
		return err
	}
	recent, err := h.st.CountMagicLinks(ctx, u.ID, time.Now().Add(-magicLinkWindow))
	if err != nil {
		return err
	}
	if recent >= magicLinkLimit {
		slog.WarnContext(ctx, "magic link not sent: too many requests in 15 minutes", "user", u.ID)
		return nil
	}
	token, err := h.st.CreateMagicLink(ctx, u.ID, authRequest, time.Now().Add(magicLinkLifetime))
	if err != nil {
		return err
	}
	appName := ""
	if app != nil {
		appName = app.Name
	}
	return h.mailer.Send(ctx, mail.MagicLink(u.Email, u.Name, appName, h.cfg.Issuer()+store.MagicLinkPath(token)))
}

func (h *handler) redeemMagicLink(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Token string `json:"token"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	link, err := h.st.GetMagicLink(ctx, in.Token)
	if errors.Is(err, store.ErrNotFound) {
		return errMagicLinkExpired
	}
	if err != nil {
		return err
	}
	u, err := h.st.GetUser(ctx, link.UserID)
	if err != nil {
		return err
	}
	a := attempt{method: "magic-link", user: &u, request: link.AuthRequest}
	switch {
	case link.UsedAt != nil:
		return h.reject(r, a, "The sign-in link was already used.", errMagicLinkExpired)
	case time.Now().After(link.ExpiresAt):
		return h.reject(r, a, "The sign-in link expired.", errMagicLinkExpired)
	}
	if a.app, err = h.target(ctx, link.AuthRequest); err != nil {
		return err
	}
	return h.finish(w, r, a, nil, func(tx *store.Store) error {
		err := tx.UseMagicLink(ctx, in.Token)
		if errors.Is(err, store.ErrNotFound) {
			return errMagicLinkExpired
		}
		if err != nil {
			return err
		}
		return tx.MarkEmailVerified(ctx, u.ID)
	})
}

func (h *handler) devOutbox(w http.ResponseWriter, r *http.Request) error {
	if _, err := httpx.RequireRole(r); err != nil {
		return err
	}
	messages, err := h.st.ListMail(r.Context(), 50)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, messages)
}
