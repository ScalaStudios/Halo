package api

import (
	"fmt"
	"net/http"
	"slices"

	"halo/internal/httpx"
	"halo/internal/store"
)

const maxBulkIDs = 500

type itemResult struct {
	ID    string       `json:"id"`
	OK    bool         `json:"ok"`
	Error *httpx.Error `json:"error,omitempty"`
}

func (h *handlers) bulkUsers(apply func(*http.Request, store.User, string) error) handler {
	return func(w http.ResponseWriter, r *http.Request, actor store.User) error {
		return h.eachUser(w, r, func(userID string) error { return apply(r, actor, userID) })
	}
}

func (h *handlers) bulkMembers(apply func(*http.Request, store.User, store.Group, string) error) handler {
	return func(w http.ResponseWriter, r *http.Request, actor store.User) error {
		g, err := h.assignedGroup(r, actor)
		if err != nil {
			return err
		}
		return h.eachUser(w, r, func(userID string) error { return apply(r, actor, g, userID) })
	}
}

func (h *handlers) bulkRevokeSessions(r *http.Request, actor store.User, userID string) error {
	_, err := h.revokeSessions(r, actor, userID)
	return err
}

func (h *handlers) eachUser(w http.ResponseWriter, r *http.Request, apply func(string) error) error {
	if e := h.auth.Throttle(w, r, "bulk"); e != nil {
		return e
	}
	var in struct {
		UserIDs []string `json:"userIds"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ids := slices.Compact(slices.Sorted(slices.Values(in.UserIDs)))
	if len(ids) == 0 || len(ids) > maxBulkIDs {
		return httpx.Invalid(fmt.Sprintf("Send between 1 and %d user ids in userIds. Split larger changes into several requests.", maxBulkIDs))
	}
	results, failed := make([]itemResult, len(ids)), 0
	for i, userID := range ids {
		results[i] = itemResult{ID: userID, OK: true}
		if err := apply(userID); err != nil {
			results[i] = itemResult{ID: userID, Error: httpx.Problem(r, err)}
			failed++
		}
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"results": results, "succeeded": len(ids) - failed, "failed": failed})
}
