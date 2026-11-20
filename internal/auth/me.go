package auth

import (
	"cmp"
	"context"
	"errors"
	"fmt"
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
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Homepage    string  `json:"homepage"`
	LaunchURL   string  `json:"launchUrl"`
	LogoURL     *string `json:"logoUrl"`
	Favourite   bool    `json:"favourite"`
}

func (h *handler) myApplications(ctx context.Context, u store.User) ([]myApplication, error) {
	apps, err := h.st.ListApplications(ctx)
	if err != nil {
		return nil, err
	}
	prefs, err := h.st.AppPreferences(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	out := []myApplication{}
	for _, a := range apps {
		if a.Status == "active" && slices.Contains(u.AppIDs, a.ID) {
			launch := a.Homepage
			if a.Protocol == "saml" {
				launch = "/saml/launch/" + a.ID
			}
			out = append(out, myApplication{ID: a.ID, Name: a.Name, Description: a.Description, Homepage: a.Homepage, LaunchURL: launch, LogoURL: a.LogoURL, Favourite: prefs[a.ID].Favourite})
		}
	}
	slices.SortStableFunc(out, func(a, b myApplication) int {
		pa, pb := prefs[a.ID].Position, prefs[b.ID].Position
		switch {
		case pa != nil && pb != nil:
			return cmp.Compare(*pa, *pb)
		case pa != nil:
			return -1
		case pb != nil:
			return 1
		}
		return 0
	})
	return out, nil
}

func (h *handler) applications(w http.ResponseWriter, r *http.Request) error {
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	out, err := h.myApplications(r.Context(), u)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h *handler) arrangeApplications(w http.ResponseWriter, r *http.Request) error {
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	var in struct {
		Order      []string `json:"order"`
		Favourites []string `json:"favourites"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	mine, err := h.myApplications(r.Context(), u)
	if err != nil {
		return err
	}
	for _, appID := range slices.Concat(in.Order, in.Favourites) {
		if !slices.ContainsFunc(mine, func(a myApplication) bool { return a.ID == appID }) {
			return httpx.Invalid(fmt.Sprintf("%q is not one of your applications. Refresh the page and try again.", appID))
		}
	}
	if err := h.st.SetAppPreferences(r.Context(), u.ID, in.Order, in.Favourites); err != nil {
		return err
	}
	return h.applications(w, r)
}

type myCollection struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	ApplicationIDs []string `json:"applicationIds"`
}

func (h *handler) collections(w http.ResponseWriter, r *http.Request) error {
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	all, err := h.st.ListCollections(r.Context())
	if err != nil {
		return err
	}
	mine, err := h.myApplications(r.Context(), u)
	if err != nil {
		return err
	}
	out := []myCollection{}
	for _, c := range all {
		if !slices.ContainsFunc(c.GroupIDs, func(groupID string) bool { return slices.Contains(u.GroupIDs, groupID) }) {
			continue
		}
		visible := []string{}
		for _, a := range mine {
			if slices.Contains(c.ApplicationIDs, a.ID) {
				visible = append(visible, a.ID)
			}
		}
		if len(visible) > 0 {
			out = append(out, myCollection{ID: c.ID, Name: c.Name, ApplicationIDs: visible})
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
