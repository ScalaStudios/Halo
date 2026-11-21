package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"halo/internal/approval"
	"halo/internal/avatars"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/mail"
	"halo/internal/store"
)

var readers = []string{"security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"}

type handlers struct {
	st            *store.Store
	cfg           config.Config
	mailer        mail.Mailer
	gate          *approval.Gate
	auth          *httpx.Auth
	removePicture func(context.Context, string) error
}

type handler func(http.ResponseWriter, *http.Request, store.User) error

type scopeKey struct{}

func Register(mux *http.ServeMux, d httpx.Deps) error {
	mailer := d.Mailer
	if mailer == nil {
		mailer = mail.New(d.Config, d.Store)
	}
	removePicture, err := avatars.PictureRemover(d)
	if err != nil {
		return err
	}
	h := &handlers{st: d.Store, cfg: d.Config, mailer: mailer, gate: approval.New(d), auth: d.Auth, removePicture: removePicture}
	route := func(pattern string, roles []string, fn handler) {
		mux.Handle(pattern, httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
			actor, err := httpx.RequireRole(r, roles...)
			if err != nil {
				return err
			}
			return fn(w, r, actor)
		}))
	}
	scoped := func(pattern string, roles []string, fn handler) {
		mux.Handle(pattern, httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
			actor, _, err := httpx.RequireUser(r)
			if err != nil {
				return err
			}
			groups, ok := actor.Scope(roles...)
			if !ok {
				return httpx.ErrForbidden
			}
			if groups != nil {
				r = r.WithContext(context.WithValue(r.Context(), scopeKey{}, groups))
			}
			return fn(w, r, actor)
		}))
	}
	signedIn := func(pattern string, fn handler) {
		mux.Handle(pattern, httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
			actor, _, err := httpx.RequireUser(r)
			if err != nil {
				return err
			}
			return fn(w, r, actor)
		}))
	}

	scoped("GET /api/v1/users", readers, h.listUsers)
	scoped("GET /api/v1/users/{id}", readers, h.getUser)
	scoped("GET /api/v1/users/{id}/sessions", readers, h.userSessions)
	scoped("GET /api/v1/users/{id}/sign-ins", readers, h.userSignIns)
	route("GET /api/v1/users/{id}/export", nil, h.exportUser)
	signedIn("GET /api/v1/me/export", h.exportMe)
	scoped("POST /api/v1/users", []string{"user_admin"}, h.inviteUser)
	scoped("POST /api/v1/users/import", []string{"user_admin"}, h.importUsers)
	scoped("POST /api/v1/users/{id}/suspend", []string{"user_admin", "security_admin"}, h.suspendUser)
	scoped("POST /api/v1/users/bulk/suspend", []string{"user_admin", "security_admin"}, h.bulkUsers(h.suspend))
	scoped("POST /api/v1/users/{id}/restore", []string{"user_admin"}, h.restoreUser)
	scoped("POST /api/v1/users/bulk/restore", []string{"user_admin"}, h.bulkUsers(h.restore))
	scoped("POST /api/v1/users/{id}/revoke-sessions", []string{"user_admin", "helpdesk_admin", "security_admin"}, h.revokeUserSessions)
	scoped("POST /api/v1/users/bulk/revoke-sessions", []string{"user_admin", "helpdesk_admin", "security_admin"}, h.bulkUsers(h.bulkRevokeSessions))
	scoped("POST /api/v1/users/{id}/reset-authentication", []string{"user_admin", "helpdesk_admin"}, h.resetAuthentication)
	scoped("DELETE /api/v1/users/{id}/methods/{methodId}", []string{"user_admin", "helpdesk_admin"}, h.deleteMethod)
	scoped("PUT /api/v1/users/{id}/password", []string{"user_admin", "helpdesk_admin"}, h.setTemporaryPassword)
	scoped("POST /api/v1/users/{id}/access-pass", []string{"user_admin", "helpdesk_admin"}, h.createAccessPass)
	route("PUT /api/v1/users/{id}/roles", nil, h.setRoles)
	route("PUT /api/v1/users/{id}/eligible-roles", nil, h.setEligibleRoles)
	scoped("PATCH /api/v1/users/{id}", []string{"user_admin", "helpdesk_admin"}, h.updateUser)
	signedIn("PUT /api/v1/users/{id}/guest", h.updateGuest)
	signedIn("GET /api/v1/me/guests", h.mySponsoredGuests)
	route("POST /api/v1/users/{id}/impersonations", []string{"helpdesk_admin"}, h.requestImpersonation)
	route("DELETE /api/v1/users/{id}", nil, h.deleteUser)
	route("GET /api/v1/impersonations", readers, h.listImpersonations)
	route("POST /api/v1/impersonations/{id}/start", []string{"helpdesk_admin"}, h.startImpersonation)
	mux.Handle("POST /api/v1/impersonations/{id}/end", httpx.Handle(h.endImpersonation))
	route("GET /api/v1/sign-ups", readers, h.listSignUps)
	route("POST /api/v1/sign-ups/{id}/approve", []string{"user_admin"}, h.approveSignUp)
	route("DELETE /api/v1/sign-ups/{id}", []string{"user_admin"}, h.rejectSignUp)

	scoped("GET /api/v1/groups", readers, h.listGroups)
	route("POST /api/v1/groups", []string{"user_admin"}, h.createGroup)
	signedIn("POST /api/v1/groups/{id}/members", h.addMember)
	signedIn("DELETE /api/v1/groups/{id}/members/{userId}", h.removeMember)
	signedIn("POST /api/v1/groups/{id}/members/bulk/add", h.bulkMembers(h.addToGroup))
	signedIn("POST /api/v1/groups/{id}/members/bulk/remove", h.bulkMembers(h.removeFromGroup))
	route("PUT /api/v1/groups/{id}/owners", []string{"user_admin"}, h.setGroupOwners)
	signedIn("POST /api/v1/me/groups/{id}/join", h.joinGroup)
	signedIn("POST /api/v1/groups/{id}/join-requests/{userId}/approve", h.decideJoinRequest(true))
	signedIn("POST /api/v1/groups/{id}/join-requests/{userId}/deny", h.decideJoinRequest(false))
	signedIn("GET /api/v1/me/owned-groups", h.myOwnedGroups)
	signedIn("GET /api/v1/me/owned-groups/{id}", h.myOwnedGroup)
	signedIn("GET /api/v1/me/joinable-groups", h.myJoinableGroups)
	scoped("GET /api/v1/groups/{id}", readers, h.getGroup)
	scoped("GET /api/v1/groups/{id}/members", readers, h.groupMembers)
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

	route("GET /api/v1/collections", readers, h.listCollections)
	route("POST /api/v1/collections", []string{"app_admin"}, h.createCollection)
	route("PUT /api/v1/collections/{id}", []string{"app_admin"}, h.updateCollection)
	route("DELETE /api/v1/collections/{id}", []string{"app_admin"}, h.deleteCollection)

	route("GET /api/v1/sessions", readers, h.listSessions)
	scoped("DELETE /api/v1/sessions/{id}", []string{"security_admin", "user_admin"}, h.revokeSession)
	route("GET /api/v1/sign-ins", readers, h.listSignIns)
	route("GET /api/v1/audit", readers, h.listAudit)
	route("GET /api/v1/overview", readers, h.overview)
	route("GET /api/v1/reports/method-registration", readers, h.methodRegistrationReport)
	route("GET /api/v1/reports/method-usage", readers, h.methodUsageReport)
	route("GET /api/v1/reports/inactive-people", readers, h.inactivePeopleReport)
	route("GET /api/v1/reports/application-usage", readers, h.applicationUsageReport)
	route("GET /api/v1/recommendations", readers, h.listRecommendations)
	route("PUT /api/v1/recommendations/{id}/dismissal", []string{"security_admin"}, h.dismissRecommendation)
	route("DELETE /api/v1/recommendations/{id}/dismissal", []string{"security_admin"}, h.restoreRecommendation)

	route("GET /api/v1/signing-keys", readers, h.listSigningKeys)
	route("POST /api/v1/signing-keys/rotate", nil, h.rotateSigningKey)
	route("POST /api/v1/saml/certificate/rotate", nil, h.rotateSAMLCertificate)
	route("DELETE /api/v1/saml/certificates/{id}", nil, h.deleteSAMLCertificate)
	return nil
}

func inScope(r *http.Request, groupIDs ...string) bool {
	scope, limited := r.Context().Value(scopeKey{}).([]string)
	return !limited || slices.ContainsFunc(groupIDs, func(id string) bool { return slices.Contains(scope, id) })
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
