package auth

import (
	"context"
	"errors"
	"net/http"

	"halo/internal/httpx"
	"halo/internal/store"
)

func (h *handler) accessPassSignIn(w http.ResponseWriter, r *http.Request) error {
	return h.codeSignIn(w, r, "access-pass", errPassMismatch)
}

func (h *handler) matchAccessPass(ctx context.Context, userID, pass string) (string, error) {
	matched, err := h.st.MatchAccessPass(ctx, userID, pass)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return "No temporary access pass is active for the account.", nil
	case err != nil:
		return "", err
	case !matched:
		return "The temporary access pass did not match.", nil
	}
	return "", nil
}

func useAccessPass(ctx context.Context, userID, pass string) func(*store.Store) error {
	return func(tx *store.Store) error {
		used, err := tx.UseAccessPass(ctx, userID, pass)
		if err == nil && !used {
			return errPassMismatch
		}
		return err
	}
}

func (h *handler) enrollWithPass(w http.ResponseWriter, r *http.Request, a attempt, extra func(*store.Store) error) error {
	ctx := r.Context()
	u := *a.user
	var token string
	err := h.st.Tx(ctx, func(tx *store.Store) error {
		if extra != nil {
			if err := extra(tx); err != nil {
				return err
			}
		}
		var err error
		if token, _, err = tx.CreateEnrollmentToken(ctx, u.ID, "access-pass", nil); err != nil {
			return err
		}
		if err := audit(ctx, tx, r, u, "user.access-pass.use", u.Name+" used a temporary access pass to set up a passkey"); err != nil {
			return err
		}
		return tx.RecordSignIn(ctx, a.event(r, "success", ""))
	})
	if errors.Is(err, errPassMismatch) {
		return h.reject(r, a, "The temporary access pass was already used.", errPassMismatch)
	}
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]string{"redirect": store.EnrollPath(token)})
}
