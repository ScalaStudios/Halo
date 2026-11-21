package policy

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"halo/internal/httpx"
	"halo/internal/store"
)

var contextIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,99}$`)

func readContext(r *http.Request, existing string) (store.AuthContext, error) {
	var in struct {
		ID      string   `json:"id"`
		Name    string   `json:"name"`
		Methods []string `json:"methods"`
		MaxAge  int      `json:"maxAge"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return store.AuthContext{}, err
	}
	c := store.AuthContext{ID: cmp.Or(existing, strings.TrimSpace(in.ID)), Name: strings.TrimSpace(in.Name), MaxAge: in.MaxAge}
	c.Methods = slices.DeleteFunc(slices.Clone(store.MethodKinds), func(m string) bool { return !slices.Contains(in.Methods, m) })
	switch {
	case !contextIDPattern.MatchString(c.ID):
		return c, httpx.Invalid("Enter an identifier of up to 100 letters, digits, dots, colons, hyphens or underscores, such as passkey-recent. Applications send it in acr_values.")
	case c.Name == "":
		return c, httpx.Invalid("Enter a name, for example “Passkey in the last 5 minutes”.")
	case len(c.Name) > 200:
		return c, httpx.Invalid("Keep the name under 200 characters.")
	case slices.ContainsFunc(in.Methods, func(v string) bool { return !slices.Contains(store.MethodKinds, v) }):
		return c, httpx.Invalid("Sign-in methods must be " + methodNames + ".")
	case c.MaxAge != 0 && (c.MaxAge < 60 || c.MaxAge > 90*86400):
		return c, httpx.Invalid("The maximum sign-in age must be between 1 minute and 90 days (60 to 7776000 seconds). Use 0 for no limit.")
	case len(c.Methods) == 0 && c.MaxAge == 0:
		return c, httpx.Invalid("Choose the sign-in methods that meet this context, a maximum sign-in age, or both.")
	}
	return c, nil
}

func (h *handlers) listContexts(w http.ResponseWriter, r *http.Request, _ store.User) error {
	contexts, err := h.st.ListAuthContexts(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, contexts)
}

func (h *handlers) createContext(w http.ResponseWriter, r *http.Request, actor store.User) error {
	c, err := readContext(r, "")
	if err != nil {
		return err
	}
	var created store.AuthContext
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		created, err = tx.CreateAuthContext(r.Context(), c)
		if errors.Is(err, store.ErrConflict) {
			return conflict("An authentication context with the identifier " + c.ID + " already exists. Pick a different identifier.")
		}
		if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "authentication_context.create", Summary: "Added authentication context " + created.ID, TargetType: "authentication_context", TargetID: created.ID, TargetLabel: created.Name})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, created)
}

func (h *handlers) updateContext(w http.ResponseWriter, r *http.Request, actor store.User) error {
	old, err := h.st.GetAuthContext(r.Context(), r.PathValue("id"))
	if err != nil {
		return found(err, "This authentication context")
	}
	c, err := readContext(r, old.ID)
	if err != nil {
		return err
	}
	parts := []string{}
	if c.Name != old.Name {
		parts = append(parts, "renamed from “"+old.Name+"”")
	}
	if !slices.Equal(c.Methods, old.Methods) {
		parts = append(parts, "changed the sign-in methods")
	}
	if c.MaxAge != old.MaxAge {
		parts = append(parts, "changed the maximum sign-in age")
	}
	summary := "Saved without changes"
	if len(parts) > 0 {
		summary = capitalize(strings.Join(parts, ", "))
	}
	var updated store.AuthContext
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		if updated, err = tx.UpdateAuthContext(r.Context(), c); err != nil {
			return found(err, "This authentication context")
		}
		if err := h.refuseLockout(r, tx, actor); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "authentication_context.update", Summary: summary, TargetType: "authentication_context", TargetID: updated.ID, TargetLabel: updated.Name, Changes: store.Diff(old, updated)})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, updated)
}

func (h *handlers) deleteContext(w http.ResponseWriter, r *http.Request, actor store.User) error {
	c, err := h.st.GetAuthContext(r.Context(), r.PathValue("id"))
	if err != nil {
		return found(err, "This authentication context")
	}
	policies, err := h.st.ListAccessPolicies(r.Context())
	if err != nil {
		return err
	}
	users := []string{}
	for _, p := range policies {
		if p.ContextID != nil && *p.ContextID == c.ID {
			users = append(users, "“"+p.Name+"”")
		}
	}
	if len(users) > 0 {
		those := map[bool]string{true: "that policy", false: "those policies"}[len(users) == 1]
		return conflict(fmt.Sprintf("“%s” is used by %s: %s. Change the effect of %s first.", c.Name, plural(len(users), "policy", "policies"), strings.Join(users, ", "), those))
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.DeleteAuthContext(r.Context(), c.ID); err != nil {
			return found(err, "This authentication context")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "authentication_context.delete", Summary: "Deleted authentication context " + c.ID, TargetType: "authentication_context", TargetID: c.ID, TargetLabel: c.Name})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
