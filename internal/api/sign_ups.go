package api

import (
	"errors"
	"net/http"

	"halo/internal/httpx"
	"halo/internal/store"
)

func (h *handlers) listSignUps(w http.ResponseWriter, r *http.Request, _ store.User) error {
	list, err := h.st.ListSignUps(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, nonNil(list))
}

func (h *handlers) approveSignUp(w http.ResponseWriter, r *http.Request, actor store.User) error {
	ctx := r.Context()
	settings, err := h.st.Settings(ctx)
	if err != nil {
		return err
	}
	var u store.User
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		var err error
		u, err = tx.ActivateSignUp(ctx, r.PathValue("id"), settings.SignUpGroupIDs)
		if errors.Is(err, store.ErrConflict) {
			return conflict("Someone already has an account with this email address, so the sign-up can't become one. Reject it instead.")
		}
		if err != nil {
			return found(err, "This sign-up")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "user.sign_up.approve", Summary: "Approved the sign-up of " + u.Name + " <" + u.Email + ">", TargetType: "user", TargetID: u.ID, TargetLabel: u.Name})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, u)
}

func (h *handlers) rejectSignUp(w http.ResponseWriter, r *http.Request, actor store.User) error {
	ctx := r.Context()
	err := h.st.Tx(ctx, func(tx *store.Store) error {
		su, err := tx.DeleteSignUp(ctx, r.PathValue("id"))
		if err != nil {
			return found(err, "This sign-up")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "user.sign_up.reject", Summary: "Rejected the sign-up of " + su.Name + " <" + su.Email + ">", TargetType: "sign_up", TargetID: su.ID, TargetLabel: su.Name})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}
