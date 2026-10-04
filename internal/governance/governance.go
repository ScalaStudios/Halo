package governance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/mail"
	"halo/internal/store"
)

var readers = []string{"security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"}

var managers = []string{"user_admin"}

type handler struct {
	st     *store.Store
	cfg    config.Config
	mailer mail.Mailer
}

type route func(http.ResponseWriter, *http.Request, store.User) error

func Register(mux *http.ServeMux, d httpx.Deps) error {
	h := &handler{st: d.Store, cfg: d.Config, mailer: d.Mailer}
	if h.mailer == nil {
		h.mailer = mail.Log{}
	}
	handle := func(pattern string, roles []string, fn route) {
		mux.Handle(pattern, httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
			u, _, err := httpx.RequireUser(r)
			if err == nil && roles != nil {
				u, err = httpx.RequireRole(r, roles...)
			}
			if err != nil {
				return err
			}
			return fn(w, r, u)
		}))
	}

	handle("GET /api/v1/access-packages", readers, h.listPackages)
	handle("POST /api/v1/access-packages", managers, h.createPackage)
	handle("PUT /api/v1/access-packages/{id}", managers, h.updatePackage)
	handle("DELETE /api/v1/access-packages/{id}", managers, h.deletePackage)

	handle("GET /api/v1/access-requests", readers, h.listRequests)
	handle("POST /api/v1/access-requests/{id}/approve", nil, h.approveRequest)
	handle("POST /api/v1/access-requests/{id}/deny", nil, h.denyRequest)
	handle("POST /api/v1/access-requests/{id}/revoke", managers, h.revokeGrant)

	handle("GET /api/v1/access-reviews", readers, h.listReviews)
	handle("POST /api/v1/access-reviews", managers, h.createReview)
	handle("GET /api/v1/access-reviews/{id}", nil, h.getReview)
	handle("PUT /api/v1/access-reviews/{id}/decisions/{userId}", nil, h.decideReviewItem)
	handle("POST /api/v1/access-reviews/{id}/complete", nil, h.completeReview)

	handle("GET /api/v1/lifecycle/rules", readers, h.listRules)
	handle("POST /api/v1/lifecycle/rules", managers, h.createRule)
	handle("PUT /api/v1/lifecycle/rules/{id}", managers, h.updateRule)
	handle("DELETE /api/v1/lifecycle/rules/{id}", managers, h.deleteRule)
	handle("GET /api/v1/lifecycle/runs", readers, h.listRuns)

	handle("GET /api/v1/me/access-packages", nil, h.myPackages)
	handle("GET /api/v1/me/access-requests", nil, h.myRequests)
	handle("POST /api/v1/me/access-requests", nil, h.createRequest)
	handle("POST /api/v1/me/access-requests/{id}/cancel", nil, h.cancelRequest)
	handle("GET /api/v1/me/access-grants", nil, h.myGrants)
	handle("GET /api/v1/me/approvals", nil, h.myApprovals)
	handle("GET /api/v1/me/reviews", nil, h.myReviews)

	if d.Jobs != nil {
		d.Jobs.Every("expire access grants", time.Minute, func(ctx context.Context) error { return expireGrants(ctx, d.Store, time.Now()) })
		d.Jobs.Every("mark overdue access reviews", time.Minute, func(ctx context.Context) error { return markOverdueReviews(ctx, d.Store, time.Now()) })
		d.Jobs.Every("run lifecycle rules", time.Minute, func(ctx context.Context) error { return runLifecycle(ctx, d.Store) })
	}
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

func names(refs []store.NamedRef) string {
	list := make([]string, len(refs))
	for i, ref := range refs {
		list[i] = ref.Name
	}
	if len(list) < 2 {
		return strings.Join(list, "")
	}
	return strings.Join(list[:len(list)-1], ", ") + " and " + list[len(list)-1]
}

func hasRef(refs []store.NamedRef, userID string) bool {
	return slices.ContainsFunc(refs, func(ref store.NamedRef) bool { return ref.ID == userID })
}

func unique(ids []string) []string {
	out := slices.Clone(ids)
	slices.Sort(out)
	return slices.Compact(out)
}

func (h *handler) people(ctx context.Context, ids []string, role string) ([]store.User, error) {
	var users []store.User
	for _, userID := range unique(ids) {
		u, err := h.st.GetUser(ctx, userID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.Invalid(fmt.Sprintf("The %s %q does not exist. Refresh the page and pick people from the directory.", role, userID))
		}
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (h *handler) assignedGroups(ctx context.Context, ids []string) ([]store.Group, error) {
	all, err := h.st.ListGroupsRaw(ctx)
	if err != nil {
		return nil, err
	}
	var groups []store.Group
	for _, groupID := range unique(ids) {
		i := slices.IndexFunc(all, func(g store.Group) bool { return g.ID == groupID })
		if i < 0 {
			return nil, httpx.Invalid(fmt.Sprintf("Group %q does not exist. Refresh the page and choose groups from the list.", groupID))
		}
		if g := all[i]; g.Kind != "assigned" {
			return nil, httpx.Invalid(fmt.Sprintf("%s is a rule-based group, so its members come from the rule %s. Choose an assigned group instead.", g.Name, *g.Rule))
		}
		groups = append(groups, all[i])
	}
	return groups, nil
}

func (h *handler) send(ctx context.Context, to, subject, text string) {
	if err := h.mailer.Send(ctx, mail.Message{To: to, Subject: subject, Text: text}); err != nil {
		slog.WarnContext(ctx, "governance email not sent", "to", to, "subject", subject, "error", err)
	}
}

func formatTime(t time.Time) string {
	return t.UTC().Format("2 January 2006, 15:04 UTC")
}
