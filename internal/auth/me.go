package auth

import (
	"errors"
	"net/http"
	"slices"

	"halo/internal/httpx"
	"halo/internal/store"
)

func (h *handler) me(w http.ResponseWriter, r *http.Request) error {
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, u)
}

func (h *handler) removeMethod(w http.ResponseWriter, r *http.Request) error {
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	methodID := r.PathValue("id")
	i := slices.IndexFunc(u.Methods, func(m store.Method) bool { return m.ID == methodID })
	if i < 0 {
		return httpx.NotFound("The sign-in method")
	}
	ctx := r.Context()
	identities, err := h.st.ListUserIdentities(ctx, u.ID)
	if err != nil {
		return err
	}
	linked := slices.ContainsFunc(identities, func(f store.FederatedIdentity) bool { return f.ProviderEnabled })
	if !linked && !slices.ContainsFunc(u.Methods, func(m store.Method) bool { return m.ID != methodID && m.Kind != "recovery-codes" }) {
		return errLastMethod
	}
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.DeleteMethod(ctx, u.ID, methodID); err != nil {
			return err
		}
		return audit(ctx, tx, r, u, "user.method.remove", u.Name+" removed "+u.Methods[i].Label)
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

func (h *handler) sessions(w http.ResponseWriter, r *http.Request) error {
	u, current, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	list, err := h.st.ListSessions(r.Context(), u.ID)
	if err != nil {
		return err
	}
	for i := range list {
		list[i].Current = list[i].ID == current.ID
	}
	return httpx.JSON(w, http.StatusOK, list)
}

func (h *handler) revokeSession(w http.ResponseWriter, r *http.Request) error {
	u, current, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	sessionID := r.PathValue("id")
	if err := h.st.RevokeSession(r.Context(), sessionID, u.ID); errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("The session")
	} else if err != nil {
		return err
	}
	if sessionID == current.ID {
		h.auth.ClearSession(w)
	}
	return noContent(w)
}

func (h *handler) revokeOtherSessions(w http.ResponseWriter, r *http.Request) error {
	u, current, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	n, err := h.st.RevokeUserSessions(r.Context(), u.ID, current.ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]int{"revoked": n})
}

type myApplication struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
	LaunchURL   string `json:"launchUrl"`
}

func (h *handler) applications(w http.ResponseWriter, r *http.Request) error {
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	apps, err := h.st.ListApplications(r.Context())
	if err != nil {
		return err
	}
	out := []myApplication{}
	for _, a := range apps {
		if a.Status == "active" && slices.Contains(u.AppIDs, a.ID) {
			launch := a.Homepage
			if a.Protocol == "saml" {
				launch = "/saml/launch/" + a.ID
			}
			out = append(out, myApplication{ID: a.ID, Name: a.Name, Description: a.Description, Homepage: a.Homepage, LaunchURL: launch})
		}
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h *handler) groups(w http.ResponseWriter, r *http.Request) error {
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	all, err := h.st.ListGroups(r.Context())
	if err != nil {
		return err
	}
	out := []store.Group{}
	for _, g := range all {
		if slices.Contains(u.GroupIDs, g.ID) {
			out = append(out, g)
		}
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h *handler) signIns(w http.ResponseWriter, r *http.Request) error {
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	events, err := h.st.ListSignIns(r.Context(), store.SignInFilter{UserID: u.ID, Limit: 100})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, events)
}
