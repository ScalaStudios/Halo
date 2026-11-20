package policy

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"halo/internal/access"
	"halo/internal/httpx"
	"halo/internal/store"
)

func readTerms(r *http.Request) (store.Terms, error) {
	var in struct {
		Name string `json:"name"`
		Text string `json:"text"`
		URL  string `json:"url"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return store.Terms{}, err
	}
	t := store.Terms{Name: strings.TrimSpace(in.Name), Text: strings.TrimSpace(in.Text), URL: strings.TrimSpace(in.URL)}
	link, err := url.Parse(t.URL)
	switch {
	case t.Name == "":
		return t, httpx.Invalid("Enter a name for the terms, for example “Acceptable use policy”.")
	case len(t.Name) > 200:
		return t, httpx.Invalid("Keep the name under 200 characters.")
	case t.Text == "" && t.URL == "":
		return t, httpx.Invalid("Add the text of the terms, a link to them, or both.")
	case len(t.Text) > 100000:
		return t, httpx.Invalid("Keep the text under 100,000 characters, or link to the full document instead.")
	case t.URL != "" && (err != nil || link.Scheme != "https" && link.Scheme != "http" || link.Host == ""):
		return t, httpx.Invalid("Enter the link as a full web address, such as https://example.com/terms.")
	}
	return t, nil
}

func (h *handlers) listTerms(w http.ResponseWriter, r *http.Request, _ store.User) error {
	terms, err := h.st.ListTerms(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, terms)
}

func (h *handlers) saveTerms(w http.ResponseWriter, r *http.Request, actor store.User, t store.Terms, action string, summary func(store.Terms) string, status int) error {
	var saved store.Terms
	err := h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		saved, err = tx.SaveTerms(r.Context(), t)
		if errors.Is(err, store.ErrConflict) {
			return conflict("Terms named " + t.Name + " already exist. Pick a different name.")
		}
		if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: action, Summary: summary(saved), TargetType: "terms", TargetID: saved.ID, TargetLabel: saved.Name})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, status, saved)
}

func (h *handlers) createTerms(w http.ResponseWriter, r *http.Request, actor store.User) error {
	t, err := readTerms(r)
	if err != nil {
		return err
	}
	return h.saveTerms(w, r, actor, t, "terms.create", func(store.Terms) string { return "Added terms of use, version 1" }, http.StatusCreated)
}

func (h *handlers) updateTerms(w http.ResponseWriter, r *http.Request, actor store.User) error {
	old, err := h.st.GetTerms(r.Context(), r.PathValue("id"))
	if err != nil {
		return found(err, "These terms")
	}
	t, err := readTerms(r)
	if err != nil {
		return err
	}
	t.ID = old.ID
	return h.saveTerms(w, r, actor, t, "terms.update", func(saved store.Terms) string {
		switch {
		case saved.Version > old.Version:
			return fmt.Sprintf("Published version %d", saved.Version)
		case saved.Name != old.Name:
			return "Renamed from “" + old.Name + "”"
		}
		return "Saved without changes"
	}, http.StatusOK)
}

func (h *handlers) deleteTerms(w http.ResponseWriter, r *http.Request, actor store.User) error {
	t, err := h.st.GetTerms(r.Context(), r.PathValue("id"))
	if err != nil {
		return found(err, "These terms")
	}
	policies, err := h.st.ListAccessPolicies(r.Context())
	if err != nil {
		return err
	}
	users := []string{}
	for _, p := range policies {
		if p.TermsID != nil && *p.TermsID == t.ID {
			users = append(users, "“"+p.Name+"”")
		}
	}
	if len(users) > 0 {
		those := map[bool]string{true: "that policy", false: "those policies"}[len(users) == 1]
		return conflict(fmt.Sprintf("“%s” is used by %s: %s. Change the effect of %s first.", t.Name, plural(len(users), "policy", "policies"), strings.Join(users, ", "), those))
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.DeleteTerms(r.Context(), t.ID); err != nil {
			return found(err, "These terms")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "terms.delete", Summary: "Deleted terms of use", TargetType: "terms", TargetID: t.ID, TargetLabel: t.Name})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func covers(c store.PolicyConditions, u store.User) bool {
	included := c.AllUsers || slices.Contains(c.UserIDs, u.ID) || shared(c.GroupIDs, u.GroupIDs) != ""
	return included && !slices.Contains(c.ExcludeUserIDs, u.ID) && shared(c.ExcludeGroupIDs, u.GroupIDs) == ""
}

type myTerms struct {
	store.Terms
	AcceptedAt *time.Time `json:"acceptedAt"`
}

func (h *handlers) myTerms(w http.ResponseWriter, r *http.Request) error {
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	policies, err := h.st.ListAccessPolicies(ctx)
	if err != nil {
		return err
	}
	terms, err := h.st.ListTerms(ctx)
	if err != nil {
		return err
	}
	accepted, err := h.st.TermsAcceptances(ctx, u.ID)
	if err != nil {
		return err
	}
	required := []myTerms{}
	for _, t := range terms {
		applies := slices.ContainsFunc(policies, func(p store.AccessPolicy) bool {
			return p.Enabled && p.Mode == "enforce" && p.Effect == access.RequireTerms && p.TermsID != nil && *p.TermsID == t.ID && covers(p.Conditions, u)
		})
		if !applies {
			continue
		}
		view := myTerms{Terms: t}
		if i := slices.IndexFunc(accepted, func(a store.TermsAcceptance) bool { return a.TermsID == t.ID && a.Version == t.Version }); i >= 0 {
			view.AcceptedAt = &accepted[i].AcceptedAt
		}
		required = append(required, view)
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"terms": required, "accepted": accepted})
}

func (h *handlers) acceptTerms(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Version int `json:"version"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	t, err := h.st.GetTerms(r.Context(), r.PathValue("id"))
	if err != nil {
		return found(err, "These terms")
	}
	if in.Version != t.Version {
		return conflict(fmt.Sprintf("Version %d of “%s” was published while you were reading. Reload the page, read it, then accept it.", t.Version, t.Name))
	}
	if err := h.st.AcceptTerms(r.Context(), u.ID, []store.Terms{t}); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
