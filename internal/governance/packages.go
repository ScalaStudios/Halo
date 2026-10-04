package governance

import (
	"errors"
	"net/http"
	"strings"

	"halo/internal/httpx"
	"halo/internal/store"
)

func (h *handler) listPackages(w http.ResponseWriter, r *http.Request, _ store.User) error {
	packages, err := h.st.ListAccessPackages(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, nonNil(packages))
}

func (h *handler) myPackages(w http.ResponseWriter, r *http.Request, _ store.User) error {
	packages, err := h.st.ListAccessPackages(r.Context())
	if err != nil {
		return err
	}
	requestable := []store.AccessPackage{}
	for _, p := range packages {
		if !p.Archived {
			requestable = append(requestable, p)
		}
	}
	return httpx.JSON(w, http.StatusOK, requestable)
}

func (h *handler) packageInput(r *http.Request) (store.AccessPackageInput, error) {
	var in struct {
		Name                 string   `json:"name"`
		Description          string   `json:"description"`
		OwnerID              *string  `json:"ownerId"`
		GroupIDs             []string `json:"groupIds"`
		ApproverIDs          []string `json:"approverIds"`
		MaxDays              *int     `json:"maxDays"`
		RequireJustification bool     `json:"requireJustification"`
		Archived             bool     `json:"archived"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return store.AccessPackageInput{}, err
	}
	out := store.AccessPackageInput{
		Name:                 strings.TrimSpace(in.Name),
		Description:          strings.TrimSpace(in.Description),
		GroupIDs:             unique(in.GroupIDs),
		ApproverIDs:          unique(in.ApproverIDs),
		MaxDays:              in.MaxDays,
		RequireJustification: in.RequireJustification,
		Archived:             in.Archived,
	}
	switch {
	case out.Name == "":
		return out, httpx.Invalid("Enter a package name, for example Production access.")
	case len(out.Name) > 200 || len(out.Description) > 2000:
		return out, httpx.Invalid("Keep the name under 200 characters and the description under 2,000.")
	case out.MaxDays != nil && (*out.MaxDays < 1 || *out.MaxDays > 365):
		return out, httpx.Invalid("Set the maximum duration between 1 and 365 days, or send null so access lasts until it is revoked.")
	case len(out.GroupIDs) == 0:
		return out, httpx.Invalid("Choose at least one group. A package grants membership of its groups.")
	}
	if _, err := h.assignedGroups(r.Context(), out.GroupIDs); err != nil {
		return out, err
	}
	if _, err := h.people(r.Context(), out.ApproverIDs, "approver"); err != nil {
		return out, err
	}
	if in.OwnerID != nil && *in.OwnerID != "" {
		if _, err := h.people(r.Context(), []string{*in.OwnerID}, "owner"); err != nil {
			return out, err
		}
		out.OwnerID = in.OwnerID
	}
	return out, nil
}

func (h *handler) savePackage(w http.ResponseWriter, r *http.Request, actor store.User, current *store.AccessPackage) error {
	in, err := h.packageInput(r)
	if err != nil {
		return err
	}
	packageID, action, summary, status := "", "access_package.create", "Created the package with "+plural(len(in.GroupIDs), "group")+" and "+plural(len(in.ApproverIDs), "approver"), http.StatusCreated
	if current != nil {
		packageID, action, summary, status = current.ID, "access_package.update", "Updated the package", http.StatusOK
		if in.Archived != current.Archived {
			summary = "Restored the package, so it can be requested again"
			if in.Archived {
				summary = "Archived the package, so it can no longer be requested"
			}
		}
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		packageID, err = tx.SaveAccessPackage(r.Context(), packageID, in)
		if errors.Is(err, store.ErrConflict) {
			return conflict("A package named " + in.Name + " already exists. Pick a different name.")
		}
		if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: action, Summary: summary, TargetType: "access_package", TargetID: packageID, TargetLabel: in.Name})
	})
	if err != nil {
		return err
	}
	p, err := h.st.GetAccessPackage(r.Context(), packageID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, status, p)
}

func (h *handler) createPackage(w http.ResponseWriter, r *http.Request, actor store.User) error {
	return h.savePackage(w, r, actor, nil)
}

func (h *handler) updatePackage(w http.ResponseWriter, r *http.Request, actor store.User) error {
	current, err := h.st.GetAccessPackage(r.Context(), r.PathValue("id"))
	if err != nil {
		return found(err, "This access package")
	}
	return h.savePackage(w, r, actor, &current)
}

func (h *handler) deletePackage(w http.ResponseWriter, r *http.Request, actor store.User) error {
	current, err := h.st.GetAccessPackage(r.Context(), r.PathValue("id"))
	if err != nil {
		return found(err, "This access package")
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		err := tx.DeleteAccessPackage(r.Context(), current.ID)
		if errors.Is(err, store.ErrConflict) {
			return conflict(current.Name + " has request history, so it can't be deleted. Archive it instead to stop new requests.")
		}
		if err != nil {
			return found(err, "This access package")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "access_package.delete", Summary: "Deleted the package", TargetType: "access_package", TargetID: current.ID, TargetLabel: current.Name})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
