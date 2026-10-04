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

	"halo/internal/httpx"
	"halo/internal/store"
)

func durationText(days *int) string {
	if days == nil {
		return "until revoked"
	}
	return "for " + plural(*days, "day")
}

func membershipText(removed []store.NamedRef) string {
	if len(removed) == 0 {
		return "no group memberships removed"
	}
	return "removed from " + names(removed)
}

func decisionError(actor store.User, req store.AccessRequest) error {
	switch {
	case req.Status != "pending":
		return conflict("This request was already " + req.Status + ". Refresh the page to see its current state.")
	case actor.ID == req.Requester.ID:
		others := slices.DeleteFunc(slices.Clone(req.Approvers), func(ref store.NamedRef) bool { return ref.ID == actor.ID })
		who := map[bool]string{true: "another global administrator", false: "a global administrator"}[actor.HasRole("global_admin")]
		if len(others) > 0 {
			who = names(others) + " or " + who
		}
		return httpx.Fail(http.StatusForbidden, "ERR_SELF_APPROVAL", "You made this request yourself, so you can't decide it. Ask "+who+" to approve or deny it.")
	case actor.HasRole("global_admin"):
		return nil
	case hasRef(req.Approvers, actor.ID):
		return nil
	case len(req.Approvers) == 0:
		return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", req.Package.Name+" has no approvers, so only a global administrator can decide this request.")
	}
	return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "Only "+names(req.Approvers)+" or a global administrator can decide requests for "+req.Package.Name+".")
}

func checkDuration(pkg store.AccessPackage, days *int) error {
	if pkg.MaxDays != nil && (days == nil || *days < 1 || *days > *pkg.MaxDays) {
		return httpx.Invalid(fmt.Sprintf("Choose a duration between 1 and %s. %s can't be granted for longer.", plural(*pkg.MaxDays, "day"), pkg.Name))
	}
	if days != nil && (*days < 1 || *days > 365) {
		return httpx.Invalid("Choose a duration between 1 and 365 days, or send null to keep access until it is revoked.")
	}
	return nil
}

func (h *handler) listRequests(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var statuses []string
	if raw := r.URL.Query().Get("status"); raw != "" {
		statuses = strings.Split(raw, ",")
		for _, status := range statuses {
			if !slices.Contains([]string{"pending", "approved", "denied", "cancelled"}, status) {
				return httpx.Invalid("status must be pending, approved, denied or cancelled, or a comma-separated list such as approved,denied.")
			}
		}
	}
	list, err := h.st.ListAccessRequests(r.Context(), store.AccessRequestFilter{Statuses: statuses})
	if err != nil {
		return err
	}
	for i := range list {
		list[i].CanDecide = decisionError(actor, list[i]) == nil
	}
	return httpx.JSON(w, http.StatusOK, nonNil(list))
}

func (h *handler) myRequests(w http.ResponseWriter, r *http.Request, u store.User) error {
	list, err := h.st.ListAccessRequests(r.Context(), store.AccessRequestFilter{RequesterID: u.ID})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, nonNil(list))
}

func (h *handler) myGrants(w http.ResponseWriter, r *http.Request, u store.User) error {
	list, err := h.st.ListAccessRequests(r.Context(), store.AccessRequestFilter{RequesterID: u.ID, ActiveGrants: true})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, nonNil(list))
}

func (h *handler) myApprovals(w http.ResponseWriter, r *http.Request, u store.User) error {
	list, err := h.st.ListAccessRequests(r.Context(), store.AccessRequestFilter{Statuses: []string{"pending"}})
	if err != nil {
		return err
	}
	mine := []store.AccessRequest{}
	for _, req := range list {
		if decisionError(u, req) == nil {
			req.CanDecide = true
			mine = append(mine, req)
		}
	}
	return httpx.JSON(w, http.StatusOK, mine)
}

func (h *handler) createRequest(w http.ResponseWriter, r *http.Request, u store.User) error {
	var in struct {
		PackageID     string `json:"packageId"`
		DurationDays  *int   `json:"durationDays"`
		Justification string `json:"justification"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	pkg, err := h.st.GetAccessPackage(r.Context(), in.PackageID)
	if errors.Is(err, store.ErrNotFound) {
		return httpx.Invalid("That access package does not exist. Refresh the page and choose one from the list.")
	}
	if err != nil {
		return err
	}
	if pkg.Archived {
		return conflict(pkg.Name + " is archived, so it can't be requested anymore. Ask an administrator which package replaced it.")
	}
	justification := strings.TrimSpace(in.Justification)
	if pkg.RequireJustification && justification == "" {
		return httpx.Invalid("Explain why you need " + pkg.Name + ". Approvers read this when they decide.")
	}
	if len(justification) > 2000 {
		return httpx.Invalid("Keep the justification under 2,000 characters.")
	}
	if err := checkDuration(pkg, in.DurationDays); err != nil {
		return err
	}
	var requestID string
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		requestID, err = tx.CreateAccessRequest(r.Context(), pkg.ID, u.ID, justification, in.DurationDays)
		if errors.Is(err, store.ErrConflict) {
			return conflict("You already have a pending request for " + pkg.Name + ". Wait for a decision or cancel it first.")
		}
		if err != nil {
			return err
		}
		return record(r, tx, u, store.AuditEvent{Action: "access_request.create", Summary: "Requested " + pkg.Name + " " + durationText(in.DurationDays), TargetType: "user", TargetID: u.ID, TargetLabel: u.Name})
	})
	if err != nil {
		return err
	}
	req, err := h.st.GetAccessRequest(r.Context(), requestID)
	if err != nil {
		return err
	}
	h.notifyApprovers(r.Context(), req)
	return httpx.JSON(w, http.StatusCreated, req)
}

func (h *handler) cancelRequest(w http.ResponseWriter, r *http.Request, u store.User) error {
	list, err := h.st.ListAccessRequests(r.Context(), store.AccessRequestFilter{ID: r.PathValue("id"), RequesterID: u.ID})
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return httpx.NotFound("This request")
	}
	req := list[0]
	if req.Status != "pending" {
		return conflict("This request was already " + req.Status + ", so it can't be cancelled.")
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.CancelAccessRequest(r.Context(), req.ID, u.ID, time.Now()); errors.Is(err, store.ErrConflict) {
			return conflict("This request was decided a moment ago. Refresh the page to see the decision.")
		} else if err != nil {
			return err
		}
		return record(r, tx, u, store.AuditEvent{Action: "access_request.cancel", Summary: "Cancelled the request for " + req.Package.Name, TargetType: "user", TargetID: u.ID, TargetLabel: u.Name})
	})
	if err != nil {
		return err
	}
	updated, err := h.st.GetAccessRequest(r.Context(), req.ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, updated)
}

func (h *handler) approveRequest(w http.ResponseWriter, r *http.Request, actor store.User) error {
	return h.decide(w, r, actor, true)
}

func (h *handler) denyRequest(w http.ResponseWriter, r *http.Request, actor store.User) error {
	return h.decide(w, r, actor, false)
}

func (h *handler) decide(w http.ResponseWriter, r *http.Request, actor store.User, approve bool) error {
	var in struct {
		Note string `json:"note"`
	}
	if r.ContentLength != 0 {
		if err := httpx.Decode(r, &in); err != nil {
			return err
		}
	}
	note := strings.TrimSpace(in.Note)
	if len(note) > 2000 {
		return httpx.Invalid("Keep the note under 2,000 characters.")
	}
	req, err := h.st.GetAccessRequest(r.Context(), r.PathValue("id"))
	if err != nil {
		return found(err, "This access request")
	}
	if err := decisionError(actor, req); err != nil {
		return err
	}
	action, summary := "access_request.deny", "Denied "+req.Package.Name
	if approve {
		action, summary = "access_request.approve", "Approved "+req.Package.Name+" "+durationText(req.DurationDays)
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		decide := tx.DenyAccessRequest
		if approve {
			decide = tx.ApproveAccessRequest
		}
		if err := decide(r.Context(), req.ID, actor.ID, note, time.Now()); errors.Is(err, store.ErrConflict) {
			return conflict("Someone decided this request a moment ago. Refresh the page to see the decision.")
		} else if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: action, Summary: summary, TargetType: "user", TargetID: req.Requester.ID, TargetLabel: req.Requester.Name})
	})
	if err != nil {
		return err
	}
	updated, err := h.st.GetAccessRequest(r.Context(), req.ID)
	if err != nil {
		return err
	}
	h.notifyRequester(r.Context(), updated)
	return httpx.JSON(w, http.StatusOK, updated)
}

func (h *handler) revokeGrant(w http.ResponseWriter, r *http.Request, actor store.User) error {
	req, err := h.st.GetAccessRequest(r.Context(), r.PathValue("id"))
	if err != nil {
		return found(err, "This access request")
	}
	if req.Status != "approved" || req.EndedAt != nil {
		return conflict("This access isn't active, so there is nothing to revoke.")
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		removed, err := tx.EndAccessGrant(r.Context(), req.ID, "revoked", time.Now())
		if errors.Is(err, store.ErrNotFound) {
			return conflict("This access ended a moment ago. Refresh the page to see its current state.")
		}
		if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "access_request.revoke", Summary: "Revoked " + req.Package.Name + "; " + membershipText(removed), TargetType: "user", TargetID: req.Requester.ID, TargetLabel: req.Requester.Name})
	})
	if err != nil {
		return err
	}
	updated, err := h.st.GetAccessRequest(r.Context(), req.ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, updated)
}

func (h *handler) notifyApprovers(ctx context.Context, req store.AccessRequest) {
	contacts, err := h.st.AccessApproverContacts(ctx, req.Package.ID, req.Requester.ID)
	if err != nil {
		slog.WarnContext(ctx, "could not look up approvers to email", "request", req.ID, "error", err)
		return
	}
	text := fmt.Sprintf("%s (%s) requested the access package %s %s.", req.Requester.Name, req.Requester.Email, req.Package.Name, durationText(req.DurationDays))
	if req.Justification != "" {
		text += "\n\nReason: " + req.Justification
	}
	for _, c := range contacts {
		link := h.cfg.Issuer() + "/account/access"
		if c.Admin {
			link = h.cfg.Issuer() + "/admin/requests"
		}
		h.send(ctx, c.Email, req.Requester.Name+" requests "+req.Package.Name, text+"\n\nApprove or deny it in Halo: "+link)
	}
}

func (h *handler) notifyRequester(ctx context.Context, req store.AccessRequest) {
	decider := "An approver"
	if req.DecidedBy != nil {
		decider = req.DecidedBy.Name
	}
	subject := "Your request for " + req.Package.Name + " was denied"
	text := decider + " denied your request for " + req.Package.Name + "."
	if req.Status == "approved" {
		subject = "Your request for " + req.Package.Name + " was approved"
		text = decider + " approved your request for " + req.Package.Name + ". You have access until it is revoked."
		if req.ExpiresAt != nil {
			text = decider + " approved your request for " + req.Package.Name + ". You have access until " + formatTime(*req.ExpiresAt) + "."
		}
		text += "\n\nGroups: " + names(req.Groups) + ". Applications pick up the change the next time you sign in to them."
	}
	if req.DecisionNote != "" {
		text += "\n\nNote from " + decider + ": " + req.DecisionNote
	}
	h.send(ctx, req.Requester.Email, subject, text+"\n\nSee your requests: "+h.cfg.Issuer()+"/account/access")
}

func expireGrants(ctx context.Context, st *store.Store, now time.Time) error {
	ids, err := st.ExpiredAccessGrants(ctx, now)
	if err != nil {
		return err
	}
	var errs []error
	for _, requestID := range ids {
		errs = append(errs, st.Tx(ctx, func(tx *store.Store) error {
			req, err := tx.GetAccessRequest(ctx, requestID)
			if err != nil {
				return err
			}
			removed, err := tx.EndAccessGrant(ctx, requestID, "expired", now)
			if errors.Is(err, store.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			return tx.RecordAudit(ctx, store.AuditEvent{Action: "access_request.expire", Summary: req.Package.Name + " expired; " + membershipText(removed), TargetType: "user", TargetID: req.Requester.ID, TargetLabel: req.Requester.Name})
		}))
	}
	return errors.Join(errs...)
}
