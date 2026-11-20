package auth

import (
	"errors"
	"net/http"
	"strings"

	"halo/internal/httpx"
	"halo/internal/store"
)

type consentStep struct {
	req  store.AuthRequest
	app  store.Application
	user store.User
	sess store.Session
}

func (h *handler) pendingConsent(r *http.Request, requestID string) (consentStep, error) {
	u, sess, err := httpx.RequireUser(r)
	if err != nil {
		return consentStep{}, err
	}
	req, err := h.st.GetAuthRequest(r.Context(), requestID)
	if errors.Is(err, store.ErrNotFound) || err == nil && (!req.Done || req.Consent != nil || req.SessionID == nil || *req.SessionID != sess.ID) {
		return consentStep{}, errRequestExpired
	}
	if err != nil {
		return consentStep{}, err
	}
	app, err := h.st.GetApplicationByClientID(r.Context(), req.ClientID)
	if errors.Is(err, store.ErrNotFound) {
		return consentStep{}, errRequestExpired
	}
	return consentStep{req: req, app: app, user: u, sess: sess}, err
}

func (h *handler) showConsent(w http.ResponseWriter, r *http.Request) error {
	c, err := h.pendingConsent(r, r.PathValue("id"))
	if err != nil {
		return err
	}
	scopes, err := h.st.DescribeScopes(r.Context(), c.req.Scopes)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{
		"application": map[string]any{"name": c.app.Name, "homepage": c.app.Homepage, "logoUrl": c.app.LogoURL},
		"email":       c.user.Email,
		"scopes":      scopes,
	})
}

func (h *handler) decideConsent(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		AuthRequest string `json:"authRequest"`
		Allow       bool   `json:"allow"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	c, err := h.pendingConsent(r, in.AuthRequest)
	if err != nil {
		return err
	}
	ctx := r.Context()
	decision := "denied"
	if in.Allow {
		decision = "granted"
	}
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.DecideConsent(ctx, c.req.ID, c.sess.ID, decision); err != nil {
			return err
		}
		if !in.Allow {
			return tx.RecordSignIn(ctx, attempt{method: c.sess.Method, user: &c.user, app: &c.app}.event(r, "interrupted", "Did not allow the application to access their information."))
		}
		if err := tx.GrantConsent(ctx, c.user.ID, c.app.ID, c.req.Scopes); err != nil {
			return err
		}
		return tx.RecordAudit(ctx, store.AuditEvent{ActorID: &c.user.ID, Action: "consent.grant", Summary: "Allowed access to " + strings.Join(c.req.Scopes, " "), TargetType: "application", TargetID: c.app.ID, TargetLabel: c.app.Name, IP: httpx.ClientIP(r)})
	})
	if errors.Is(err, store.ErrNotFound) {
		return errRequestExpired
	}
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]string{"redirect": store.AuthorizeCallbackPath(c.req.ID)})
}

func (h *handler) consents(w http.ResponseWriter, r *http.Request) error {
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	consents, err := h.st.ListConsents(r.Context(), u.ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, consents)
}

func (h *handler) revokeConsent(w http.ResponseWriter, r *http.Request) error {
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	app, err := h.st.GetApplication(ctx, r.PathValue("id"))
	if err == nil {
		err = h.st.Tx(ctx, func(tx *store.Store) error {
			if err := tx.RevokeConsent(ctx, u.ID, app); err != nil {
				return err
			}
			return tx.RecordAudit(ctx, store.AuditEvent{ActorID: &u.ID, Action: "consent.revoke", Summary: "Took back access and revoked the application's tokens", TargetType: "application", TargetID: app.ID, TargetLabel: app.Name, IP: httpx.ClientIP(r)})
		})
	}
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("Access for that application")
	}
	if err != nil {
		return err
	}
	return noContent(w)
}
