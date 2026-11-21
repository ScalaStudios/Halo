package api

import (
	"errors"
	"net/http"
	"strings"

	"halo/internal/hostd"
	"halo/internal/httpx"
	"halo/internal/store"
)

func (h *handlers) setPOSIX(w http.ResponseWriter, r *http.Request, actor store.User) error {
	target, err := h.user(r)
	if err != nil {
		return err
	}
	if err := guardAdmin(actor, target); err != nil {
		return err
	}
	if target.POSIX == nil {
		return httpx.NotFound("This person's Linux account")
	}
	var in struct {
		Username      string `json:"username"`
		LoginShell    string `json:"loginShell"`
		HomeDirectory string `json:"homeDirectory"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	next := *target.POSIX
	next.Username, next.LoginShell, next.HomeDirectory = strings.TrimSpace(in.Username), strings.TrimSpace(in.LoginShell), strings.TrimSpace(in.HomeDirectory)
	switch {
	case !hostd.NamePattern.MatchString(next.Username):
		return httpx.Invalid("Use up to 32 lowercase letters, digits, _ . or - for the user name, starting with a letter or _.")
	case !hostd.PathPattern.MatchString(next.LoginShell) || next.LoginShell == "/":
		return httpx.Invalid("Set the login shell to an absolute path, such as /bin/bash.")
	case !hostd.PathPattern.MatchString(next.HomeDirectory) || next.HomeDirectory == "/":
		return httpx.Invalid("Set the home directory to an absolute path, such as /home/" + next.Username + ".")
	}
	if next == *target.POSIX {
		return httpx.JSON(w, http.StatusOK, target)
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		switch err := tx.SetPOSIXAccount(r.Context(), target.ID, next); {
		case errors.Is(err, store.ErrConflict):
			return conflict("Another person or group already uses the Linux name " + next.Username + ". Choose a different one.")
		case errors.Is(err, store.ErrPOSIXName):
			return httpx.Invalid(next.Username + " is reserved for a system account. Choose a different user name.")
		case err != nil:
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "user.posix", Summary: "Updated the Linux account " + next.Username, TargetType: "user", TargetID: target.ID, TargetLabel: target.Name, Changes: store.Diff(*target.POSIX, next)})
	})
	if err != nil {
		return err
	}
	return h.respondUser(w, r, target.ID)
}
