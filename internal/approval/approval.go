package approval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"time"

	"halo/internal/httpx"
	"halo/internal/mail"
	"halo/internal/store"
)

const lifetime = 72 * time.Hour

var readers = []string{"security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"}

type replayKey struct{}

type Change struct {
	Action      string
	Summary     string
	TargetType  string
	TargetID    string
	TargetLabel string
	State       any
}

type Gate struct {
	st     *store.Store
	mailer mail.Mailer
	issuer string
}

func New(d httpx.Deps) *Gate {
	mailer := d.Mailer
	if mailer == nil {
		mailer = mail.Log{}
	}
	return &Gate{st: d.Store, mailer: mailer, issuer: d.Config.Issuer()}
}

func required(s store.Settings, action string) bool {
	return slices.Contains(s.ApprovalActions, action) || (action == "approval-settings" && len(s.ApprovalActions) > 0)
}

func (g *Gate) RefuseGlobalAdmin(ctx context.Context, roles []string) error {
	if !slices.Contains(roles, "global_admin") {
		return nil
	}
	s, err := g.st.CurrentSettings(ctx)
	if err == nil && required(s, "global-admin") {
		err = httpx.Invalid("Granting the global administrator role needs approval from another global administrator. Save without that role, then grant it on its own.")
	}
	return err
}

func (g *Gate) Hold(w http.ResponseWriter, r *http.Request, actor store.User, c Change) (bool, error) {
	ctx := r.Context()
	state, err := json.Marshal(c.State)
	if err != nil {
		return false, err
	}
	if requested, replay := ctx.Value(replayKey{}).(string); replay {
		if requested != string(state) {
			return false, httpx.Fail(http.StatusConflict, "ERR_STALE", c.TargetLabel+" changed after this change was requested. Ask for it again if it is still needed.")
		}
		return false, nil
	}
	if c.Action == "" {
		return false, nil
	}
	settings, err := g.st.CurrentSettings(ctx)
	if err != nil || !required(settings, c.Action) {
		return false, err
	}
	if slices.Contains(settings.EmergencyAccessIDs, actor.ID) {
		return false, g.bypass(r, actor, c)
	}
	if actor.Kind != "person" {
		return false, httpx.Fail(http.StatusForbidden, "ERR_APPROVAL_REQUIRED", "This change needs approval from another global administrator, so a person must ask for it in the console. API keys can't.")
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return false, httpx.Fail(http.StatusRequestEntityTooLarge, "ERR_TOO_LARGE", "The request body is larger than 1 MB, so it can't wait for approval. Send a smaller one.")
	}
	if err != nil {
		return false, err
	}
	req := store.ApprovalRequest{Action: c.Action, Summary: c.Summary, TargetType: c.TargetType, TargetID: c.TargetID, TargetLabel: c.TargetLabel,
		Requester: store.NamedRef{ID: actor.ID}, ExpiresAt: time.Now().Add(lifetime), Method: r.Method, Path: r.URL.Path, Body: body, State: string(state)}
	err = g.st.Tx(ctx, func(tx *store.Store) error {
		var err error
		if req.ID, err = tx.CreateApprovalRequest(ctx, req); err != nil {
			return err
		}
		return tx.RecordAudit(ctx, store.AuditEvent{ActorID: &actor.ID, Action: "approval.request", Summary: "Asked for approval: " + c.Summary, TargetType: c.TargetType, TargetID: c.TargetID, TargetLabel: c.TargetLabel, IP: httpx.ClientIP(r)})
	})
	if err != nil {
		return false, err
	}
	created, err := g.st.GetApprovalRequest(ctx, req.ID)
	if err != nil {
		return false, err
	}
	return true, httpx.JSON(w, http.StatusAccepted, created)
}

func (g *Gate) bypass(r *http.Request, actor store.User, c Change) error {
	ctx := r.Context()
	err := g.st.RecordAudit(ctx, store.AuditEvent{ActorID: &actor.ID, Action: "approval.bypass", Summary: "Skipped approval with an emergency access account: " + c.Summary, TargetType: c.TargetType, TargetID: c.TargetID, TargetLabel: c.TargetLabel, IP: httpx.ClientIP(r)})
	if err != nil {
		return err
	}
	emails, err := g.st.GlobalAdminEmails(ctx, actor.ID)
	if err != nil {
		slog.WarnContext(ctx, "could not look up global administrators to alert", "error", err)
	}
	for _, to := range emails {
		m := mail.Message{To: to, Subject: actor.Name + " skipped approval with an emergency access account", Text: actor.Name + " (" + actor.Email + ") used an emergency access account to make a change that normally needs approval from a second global administrator:\n\n" +
			c.Summary + " (" + c.TargetLabel + ")\n\nIf you did not expect this, check the audit log now: " + g.issuer + "/admin/audit"}
		if err := g.mailer.Send(ctx, m); err != nil {
			slog.WarnContext(ctx, "emergency access alert not delivered", "to", to, "error", err)
		}
	}
	return nil
}

type handler struct {
	st  *store.Store
	mux *http.ServeMux
}

func Register(mux *http.ServeMux, d httpx.Deps) error {
	h := &handler{st: d.Store, mux: mux}
	mux.Handle("GET /api/v1/approvals", httpx.Handle(h.list))
	mux.Handle("POST /api/v1/approvals/{id}/approve", httpx.Handle(h.decide(true)))
	mux.Handle("POST /api/v1/approvals/{id}/reject", httpx.Handle(h.decide(false)))
	return nil
}

type notApplied struct{ message string }

func (n notApplied) Error() string { return n.message }

func conflict(message string) error {
	return httpx.Fail(http.StatusConflict, "ERR_CONFLICT", message)
}

func decisionError(r *http.Request, actor store.User, req store.ApprovalRequest) error {
	_, viaKey := httpx.KeyScopes(r)
	switch {
	case req.Status == "expired":
		return conflict("This request expired before anyone decided it. Ask for the change again if it is still needed.")
	case req.Status != "pending":
		return conflict("This request was already decided. Refresh the page to see the outcome.")
	case !actor.HasRole("global_admin"):
		return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "Only a global administrator can approve or reject changes.")
	case viaKey || actor.Kind != "person":
		return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "Approve or reject changes in the console, signed in as yourself. API keys can't decide them.")
	case actor.ID == req.Requester.ID:
		return httpx.Fail(http.StatusForbidden, "ERR_SELF_APPROVAL", "You asked for this change yourself, so another global administrator must approve or reject it.")
	}
	return nil
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) error {
	actor, err := httpx.RequireRole(r, readers...)
	if err != nil {
		return err
	}
	list, err := h.st.ListApprovalRequests(r.Context())
	if err != nil {
		return err
	}
	for i := range list {
		list[i].CanDecide = decisionError(r, actor, list[i]) == nil
	}
	return httpx.JSON(w, http.StatusOK, list)
}

func (h *handler) decide(approve bool) httpx.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		actor, err := httpx.RequireRole(r)
		if err != nil {
			return err
		}
		ctx := r.Context()
		req, err := h.st.GetApprovalRequest(ctx, r.PathValue("id"))
		if errors.Is(err, store.ErrNotFound) {
			return httpx.NotFound("This approval request")
		}
		if err != nil {
			return err
		}
		if err := decisionError(r, actor, req); err != nil {
			return err
		}
		status, action, verb := "rejected", "approval.reject", "Rejected: "
		if approve {
			status, action, verb = "approved", "approval.approve", "Approved: "
		}
		settle := func(tx *store.Store, status, outcome string, events ...store.AuditEvent) error {
			if err := tx.DecideApprovalRequest(ctx, req.ID, status, actor.ID, outcome); errors.Is(err, store.ErrConflict) {
				return conflict("Someone decided this request a moment ago, or it expired. Refresh the page to see its current state.")
			} else if err != nil {
				return err
			}
			for _, e := range events {
				e.ActorID, e.TargetType, e.TargetID, e.TargetLabel, e.IP = &actor.ID, req.TargetType, req.TargetID, req.TargetLabel, httpx.ClientIP(r)
				if err := tx.RecordAudit(ctx, e); err != nil {
					return err
				}
			}
			return nil
		}
		decided := store.AuditEvent{Action: action, Summary: verb + req.Summary}
		err = h.st.Tx(ctx, func(tx *store.Store) error {
			if err := settle(tx, status, "", decided); err != nil || !approve {
				return err
			}
			return h.apply(tx, r, req)
		})
		var failure notApplied
		if errors.As(err, &failure) {
			err = h.st.Tx(ctx, func(tx *store.Store) error {
				return settle(tx, "failed", failure.message, decided, store.AuditEvent{Action: "approval.fail", Summary: "Not applied: " + req.Summary + ". " + failure.message})
			})
			if err == nil {
				return httpx.Fail(http.StatusConflict, "ERR_NOT_APPLIED", "Approved, but Halo could not apply the change. "+failure.message)
			}
		}
		if err != nil {
			return err
		}
		h.st.ForgetSettings()
		updated, err := h.st.GetApprovalRequest(ctx, req.ID)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, updated)
	}
}

func (h *handler) apply(tx *store.Store, r *http.Request, req store.ApprovalRequest) error {
	ctx := r.Context()
	if err := tx.LockApprovalTarget(ctx, req.TargetType, req.TargetID); err != nil {
		return err
	}
	requester, err := tx.GetUser(ctx, req.Requester.ID)
	if errors.Is(err, store.ErrNotFound) || requester.Status == "suspended" || requester.Status == "deprovisioned" {
		return notApplied{req.Requester.Name + "'s account could not be used to make the change. It may be suspended."}
	}
	if err != nil {
		return err
	}
	replay, err := http.NewRequestWithContext(context.WithValue(httpx.WithUser(tx.Bind(ctx), requester), replayKey{}, req.State), req.Method, req.Path, bytes.NewReader(req.Body))
	if err != nil {
		return err
	}
	replay.RemoteAddr = r.RemoteAddr
	replay.Header["X-Forwarded-For"] = r.Header.Values("X-Forwarded-For")
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, replay)
	if rec.Code < http.StatusMultipleChoices {
		return nil
	}
	if rec.Code >= http.StatusInternalServerError {
		return fmt.Errorf("replaying approval request %s: status %d", req.ID, rec.Code)
	}
	var out struct {
		Error httpx.Error `json:"error"`
	}
	if json.Unmarshal(rec.Body.Bytes(), &out) != nil || out.Error.Message == "" {
		out.Error.Message = "Try again; if it keeps failing, check the server log."
	}
	return notApplied{out.Error.Message}
}
