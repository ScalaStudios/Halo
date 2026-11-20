package auth

import (
	"errors"
	"net/http"

	"halo/internal/httpx"
	"halo/internal/store"
)

type termsCeremony struct {
	Method       string        `json:"method"`
	SecondFactor bool          `json:"secondFactor"`
	Request      string        `json:"request"`
	SessionID    string        `json:"sessionId"`
	Terms        []store.Terms `json:"terms"`
}

func (h *handler) pendingTerms(w http.ResponseWriter, r *http.Request) error {
	var c termsCeremony
	if err := h.st.PeekCeremony(r.Context(), r.PathValue("ceremony"), "terms", &c); errors.Is(err, store.ErrNotFound) {
		return errTermsExpired
	} else if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"terms": c.Terms})
}

func (h *handler) acceptTerms(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Ceremony string `json:"ceremony"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	var c termsCeremony
	userID, err := h.st.TakeCeremony(ctx, in.Ceremony, "terms", &c)
	if errors.Is(err, store.ErrNotFound) || err == nil && userID == nil {
		return errTermsExpired
	}
	if err != nil {
		return err
	}
	var current *store.Session
	if c.SessionID != "" {
		sess, ok := httpx.CurrentSession(r)
		if !ok || sess.ID != c.SessionID {
			return errTermsExpired
		}
		current = &sess
	}
	u, err := h.st.GetUser(ctx, *userID)
	if errors.Is(err, store.ErrNotFound) {
		return errTermsExpired
	}
	if err != nil {
		return err
	}
	if err := h.st.AcceptTerms(ctx, u.ID, c.Terms); err != nil {
		return err
	}
	app, err := h.target(ctx, c.Request)
	if err != nil {
		return err
	}
	return h.finish(w, r, attempt{method: c.Method, secondFactor: c.SecondFactor, email: u.Email, user: &u, app: app, request: c.Request}, current, nil)
}
