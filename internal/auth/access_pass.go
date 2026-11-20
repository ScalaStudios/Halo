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

func (h *handler) useAccessPass(ctx context.Context, userID, pass string) (string, error) {
	used, err := h.st.UseAccessPass(ctx, userID, pass)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return "No temporary access pass is active for the account.", nil
	case err != nil:
		return "", err
	case !used:
		return "The temporary access pass did not match.", nil
	}
	return "", nil
}

func (h *handler) enrollWithPass(w http.ResponseWriter, r *http.Request, a attempt) error {
	ctx := r.Context()
	u := *a.user
	var token string
	err := h.st.Tx(ctx, func(tx *store.Store) error {
		var err error
		if token, _, err = tx.CreateEnrollmentToken(ctx, u.ID, "access-pass", nil); err != nil {
			return err
		}
		if err := audit(ctx, tx, r, u, "user.access-pass.use", u.Name+" used a temporary access pass to set up a passkey"); err != nil {
			return err
		}
		return tx.RecordSignIn(ctx, a.event(r, "success", ""))
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]string{"redirect": store.EnrollPath(token)})
}
