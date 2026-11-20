package governance

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"halo/internal/httpx"
	"halo/internal/store"
)

const maxActivationHours = 8

var errNotEligible = httpx.Fail(http.StatusForbidden, "ERR_NOT_ELIGIBLE", "You aren't eligible for that role. Ask a global administrator to make you eligible first.")

func (h *handler) activateRole(w http.ResponseWriter, r *http.Request, u store.User) error {
	if _, ok := httpx.CurrentSession(r); !ok {
		return httpx.ErrUnauthenticated
	}
	var in struct {
		Role          string `json:"role"`
		DurationHours int    `json:"durationHours"`
		Justification string `json:"justification"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	label := store.RoleLabels[in.Role]
	if !slices.Contains(u.EligibleRoles, in.Role) {
		return errNotEligible
	}
	if u.HasRole(in.Role) {
		return conflict("You already hold " + label + ". Refresh the page to see your roles.")
	}
	if in.DurationHours < 1 || in.DurationHours > maxActivationHours {
		return httpx.Invalid(fmt.Sprintf("Choose a duration between 1 and %d hours.", maxActivationHours))
	}
	justification := strings.TrimSpace(in.Justification)
	if justification == "" {
		return httpx.Invalid("Explain why you need " + label + ". The reason is recorded in the audit log.")
	}
	if len(justification) > 2000 {
		return httpx.Invalid("Keep the justification under 2,000 characters.")
	}
	if err := expireActivations(r.Context(), h.st, time.Now()); err != nil {
		return err
	}
	var activation store.RoleActivation
	err := h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		activation, err = tx.ActivateRole(r.Context(), u.ID, in.Role, justification, time.Now().Add(time.Duration(in.DurationHours)*time.Hour))
		switch {
		case errors.Is(err, store.ErrNotFound):
			return errNotEligible
		case errors.Is(err, store.ErrConflict):
			return conflict(label + " was activated a moment ago. Refresh the page to see until when.")
		case err != nil:
			return err
		}
		return record(r, tx, u, store.AuditEvent{Action: "role.activate", Summary: fmt.Sprintf("Activated %s for %s: %s", label, plural(in.DurationHours, "hour"), justification), TargetType: "user", TargetID: u.ID, TargetLabel: u.Name})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, activation)
}

func (h *handler) deactivateRole(w http.ResponseWriter, r *http.Request, u store.User) error {
	err := h.st.Tx(r.Context(), func(tx *store.Store) error {
		role, err := tx.DeactivateRole(r.Context(), r.PathValue("id"), u.ID)
		if errors.Is(err, store.ErrNotFound) {
			return conflict("This role is no longer active. Refresh the page to see your roles.")
		}
		if err != nil {
			return err
		}
		return record(r, tx, u, store.AuditEvent{Action: "role.deactivate", Summary: "Deactivated " + store.RoleLabels[role] + " before its time limit", TargetType: "user", TargetID: u.ID, TargetLabel: u.Name})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func expireActivations(ctx context.Context, st *store.Store, now time.Time) error {
	return st.Tx(ctx, func(tx *store.Store) error {
		expired, err := tx.ExpireRoleActivations(ctx, now)
		if err != nil {
			return err
		}
		for _, a := range expired {
			if err := tx.RecordAudit(ctx, store.AuditEvent{Action: "role.expire", Summary: store.RoleLabels[a.Role] + " activation reached its time limit and ended", TargetType: "user", TargetID: a.UserID, TargetLabel: a.UserName}); err != nil {
				return err
			}
		}
		return nil
	})
}
