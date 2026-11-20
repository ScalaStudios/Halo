package api

import (
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"halo/internal/httpx"
	"halo/internal/posture"
	"halo/internal/store"
)

func (h *handlers) listRecommendations(w http.ResponseWriter, r *http.Request, _ store.User) error {
	checks, err := posture.List(r.Context(), h.st, time.Now())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, checks)
}

func (h *handlers) recommendation(r *http.Request) (posture.Check, error) {
	checks, err := posture.List(r.Context(), h.st, time.Now())
	if err != nil {
		return posture.Check{}, err
	}
	for _, c := range checks {
		if c.ID == r.PathValue("id") {
			return c, nil
		}
	}
	return posture.Check{}, httpx.NotFound("This recommendation")
}

func (h *handlers) dismissRecommendation(w http.ResponseWriter, r *http.Request, actor store.User) error {
	c, err := h.recommendation(r)
	if err != nil {
		return err
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	in.Reason = strings.TrimSpace(in.Reason)
	switch {
	case c.Status == "pass":
		return httpx.Invalid("This check passes, so there is nothing to dismiss.")
	case in.Reason == "" || utf8.RuneCountInString(in.Reason) > 500:
		return httpx.Invalid("Say why this recommendation doesn't apply, in at most 500 characters. Other administrators see it here and in the audit log.")
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.DismissRecommendation(r.Context(), c.ID, in.Reason, actor.ID); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "recommendation.dismiss", Summary: "Dismissed the security recommendation “" + c.Title + "”: " + in.Reason, TargetType: "recommendation", TargetID: c.ID, TargetLabel: c.Title})
	})
	if err != nil {
		return err
	}
	return h.listRecommendations(w, r, actor)
}

func (h *handlers) restoreRecommendation(w http.ResponseWriter, r *http.Request, actor store.User) error {
	c, err := h.recommendation(r)
	if err != nil {
		return err
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.RestoreRecommendation(r.Context(), c.ID); err != nil {
			return found(err, "This dismissal")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "recommendation.restore", Summary: "Restored the security recommendation “" + c.Title + "”", TargetType: "recommendation", TargetID: c.ID, TargetLabel: c.Title})
	})
	if err != nil {
		return err
	}
	return h.listRecommendations(w, r, actor)
}
