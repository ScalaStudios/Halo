package api

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"halo/internal/httpx"
	"halo/internal/store"
)

func (h *handlers) listCollections(w http.ResponseWriter, r *http.Request, _ store.User) error {
	collections, err := h.st.ListCollections(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, nonNil(collections))
}

func (h *handlers) saveCollection(w http.ResponseWriter, r *http.Request, actor store.User, current *store.Collection) error {
	var in struct {
		Name           string   `json:"name"`
		ApplicationIDs []string `json:"applicationIds"`
		GroupIDs       []string `json:"groupIds"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return httpx.Invalid("Enter a collection name, for example Engineering.")
	}
	if tooLong(100, name) {
		return httpx.Invalid("Keep the collection name under 100 characters.")
	}
	apps, err := h.st.ListApplications(r.Context())
	if err != nil {
		return err
	}
	appIDs := []string{}
	for _, appID := range in.ApplicationIDs {
		i := slices.IndexFunc(apps, func(a store.Application) bool { return a.ID == appID })
		if i < 0 || apps[i].Type == "service" {
			return httpx.Invalid(fmt.Sprintf("Application %q does not exist or nobody signs in to it. Refresh the page and choose applications from the list.", appID))
		}
		if !slices.Contains(appIDs, appID) {
			appIDs = append(appIDs, appID)
		}
	}
	groups, err := h.groupsByID(r.Context(), in.GroupIDs)
	if err != nil {
		return err
	}
	collectionID, action, summary, status := "", "collection.create", "Created the collection with "+plural(len(appIDs), "application")+" for "+plural(len(groups), "group"), http.StatusCreated
	if current != nil {
		collectionID, action, summary, status = current.ID, "collection.update", "Updated the collection: "+plural(len(appIDs), "application")+" for "+plural(len(groups), "group"), http.StatusOK
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		collectionID, err = tx.SaveCollection(r.Context(), collectionID, name, appIDs, groupIDs(groups))
		if errors.Is(err, store.ErrConflict) {
			return conflict("A collection named " + name + " already exists. Pick a different name.")
		}
		if err != nil {
			return found(err, "This collection")
		}
		return record(r, tx, actor, store.AuditEvent{Action: action, Summary: summary, TargetType: "collection", TargetID: collectionID, TargetLabel: name})
	})
	if err != nil {
		return err
	}
	c, err := h.st.GetCollection(r.Context(), collectionID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, status, c)
}

func (h *handlers) createCollection(w http.ResponseWriter, r *http.Request, actor store.User) error {
	return h.saveCollection(w, r, actor, nil)
}

func (h *handlers) updateCollection(w http.ResponseWriter, r *http.Request, actor store.User) error {
	current, err := h.st.GetCollection(r.Context(), r.PathValue("id"))
	if err != nil {
		return found(err, "This collection")
	}
	return h.saveCollection(w, r, actor, &current)
}

func (h *handlers) deleteCollection(w http.ResponseWriter, r *http.Request, actor store.User) error {
	current, err := h.st.GetCollection(r.Context(), r.PathValue("id"))
	if err != nil {
		return found(err, "This collection")
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.DeleteCollection(r.Context(), current.ID); err != nil {
			return found(err, "This collection")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "collection.delete", Summary: "Deleted the collection", TargetType: "collection", TargetID: current.ID, TargetLabel: current.Name})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}
