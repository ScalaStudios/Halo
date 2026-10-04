package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/mail"
	"halo/internal/store"
)

var readers = []string{"security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"}

type handlers struct {
	st     *store.Store
	cfg    config.Config
	mailer mail.Mailer
}

type handler func(http.ResponseWriter, *http.Request, store.User) error

func Register(mux *http.ServeMux, d httpx.Deps) error {
	mailer := d.Mailer
	if mailer == nil {
		mailer = mail.New(d.Config, d.Store)
	}
	h := &handlers{st: d.Store, cfg: d.Config, mailer: mailer}
	route := func(pattern string, roles []string, fn handler) {
		mux.Handle(pattern, httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
			actor, err := httpx.RequireRole(r, roles...)
			if err != nil {
				return err
			}
			return fn(w, r, actor)
		}))
	}

	route("GET /api/v1/users", readers, h.listUsers)
	route("GET /api/v1/users/{id}", readers, h.getUser)
	route("GET /api/v1/users/{id}/sessions", readers, h.userSessions)
	route("GET /api/v1/users/{id}/sign-ins", readers, h.userSignIns)
	route("POST /api/v1/users", []string{"user_admin"}, h.inviteUser)
	route("POST /api/v1/users/{id}/suspend", []string{"user_admin", "security_admin"}, h.suspendUser)
	route("POST /api/v1/users/{id}/restore", []string{"user_admin"}, h.restoreUser)
	route("POST /api/v1/users/{id}/revoke-sessions", []string{"user_admin", "helpdesk_admin", "security_admin"}, h.revokeUserSessions)
	route("POST /api/v1/users/{id}/reset-authentication", []string{"user_admin", "helpdesk_admin"}, h.resetAuthentication)
	route("DELETE /api/v1/users/{id}/methods/{methodId}", []string{"user_admin", "helpdesk_admin"}, h.deleteMethod)
	route("PUT /api/v1/users/{id}/roles", nil, h.setRoles)
	route("PATCH /api/v1/users/{id}", []string{"user_admin", "helpdesk_admin"}, h.updateUser)

	route("GET /api/v1/groups", readers, h.listGroups)
	route("POST /api/v1/groups", []string{"user_admin"}, h.createGroup)
	route("POST /api/v1/groups/{id}/members", []string{"user_admin"}, h.addMember)
	route("DELETE /api/v1/groups/{id}/members/{userId}", []string{"user_admin"}, h.removeMember)
	route("GET /api/v1/groups/{id}", readers, h.getGroup)
	route("GET /api/v1/groups/{id}/members", readers, h.groupMembers)
	route("PATCH /api/v1/groups/{id}", []string{"user_admin"}, h.updateGroup)
	route("DELETE /api/v1/groups/{id}", []string{"user_admin"}, h.deleteGroup)

	route("GET /api/v1/applications", readers, h.listApplications)
	route("GET /api/v1/applications/{id}", readers, h.getApplication)
	route("POST /api/v1/applications", []string{"app_admin"}, h.createApplication)
	route("POST /api/v1/applications/{id}/secrets", []string{"app_admin"}, h.rotateSecret)
	route("DELETE /api/v1/applications/{id}/secrets/{credentialId}", []string{"app_admin"}, h.revokeSecret)
	route("POST /api/v1/applications/{id}/disable", []string{"app_admin"}, h.setApplicationStatus("disabled"))
	route("POST /api/v1/applications/{id}/enable", []string{"app_admin"}, h.setApplicationStatus("active"))
	route("DELETE /api/v1/applications/{id}", []string{"app_admin"}, h.deleteApplication)
	route("PUT /api/v1/applications/{id}/groups", []string{"app_admin"}, h.setApplicationGroups)
	route("PUT /api/v1/applications/{id}/saml", []string{"app_admin"}, h.setApplicationSAML)
	route("PATCH /api/v1/applications/{id}", []string{"app_admin"}, h.updateApplication)

	route("GET /api/v1/sessions", readers, h.listSessions)
	route("DELETE /api/v1/sessions/{id}", []string{"security_admin", "user_admin"}, h.revokeSession)
	route("GET /api/v1/sign-ins", readers, h.listSignIns)
	route("GET /api/v1/audit", readers, h.listAudit)
	route("GET /api/v1/overview", readers, h.overview)

	route("GET /api/v1/signing-keys", readers, h.listSigningKeys)
	route("POST /api/v1/signing-keys/rotate", nil, h.rotateSigningKey)
	route("POST /api/v1/saml/certificate/rotate", nil, h.rotateSAMLCertificate)
	route("DELETE /api/v1/saml/certificates/{id}", nil, h.deleteSAMLCertificate)
	return nil
}

func record(r *http.Request, st *store.Store, actor store.User, e store.AuditEvent) error {
	e.ActorID = &actor.ID
	e.IP = httpx.ClientIP(r)
	return st.RecordAudit(r.Context(), e)
}

func found(err error, what string) error {
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound(what)
	}
	return err
}

func conflict(message string) error {
	return httpx.Fail(http.StatusConflict, "ERR_CONFLICT", message)
}

func noContent(w http.ResponseWriter) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func tooLong(size int, values ...string) bool {
	return slices.ContainsFunc(values, func(v string) bool { return len(v) > size })
}

func (h *handlers) groupsByID(ctx context.Context, ids []string) ([]store.Group, error) {
	all, err := h.st.ListGroupsRaw(ctx)
	if err != nil {
		return nil, err
	}
	groups := []store.Group{}
	for _, groupID := range ids {
		i := slices.IndexFunc(all, func(g store.Group) bool { return g.ID == groupID })
		if i < 0 {
			return nil, httpx.Invalid(fmt.Sprintf("Group %q does not exist. Refresh the page and choose groups from the list.", groupID))
		}
		if !slices.ContainsFunc(groups, func(g store.Group) bool { return g.ID == groupID }) {
			groups = append(groups, all[i])
		}
	}
	return groups, nil
}

func groupIDs(groups []store.Group) []string {
	ids := make([]string, len(groups))
	for i, g := range groups {
		ids[i] = g.ID
	}
	return ids
}
