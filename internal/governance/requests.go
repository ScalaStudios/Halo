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
	"halo/internal/mail"
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
	asked := req.RequestedBy != nil && actor.ID == req.RequestedBy.ID
	switch {
	case req.Status != "pending":
		return conflict("This request was already " + req.Status + ". Refresh the page to see its current state.")
	case actor.ID == req.Requester.ID || asked:
		who := map[bool]string{true: "another global administrator", false: "a global administrator"}[actor.HasRole("global_admin")]
		if len(req.Approvers) > 0 {
			who = names(req.Approvers) + " or " + who
		}
		why := "You made this request yourself"
		if asked && actor.ID != req.Requester.ID {
			why = "You asked for this access for " + req.Requester.Name
		}
		return httpx.Fail(http.StatusForbidden, "ERR_SELF_APPROVAL", why+", so you can't decide it. Ask "+who+" to approve or deny it.")
	case actor.HasRole("global_admin"):
		return nil
	case hasRef(req.Approvers, actor.ID):
		return nil
	case len(req.Approvers) == 0:
		return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "Nobody can decide this stage of the request for "+req.Package.Name+", so only a global administrator can.")
	}
	return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "Only "+names(req.Approvers)+" or a global administrator can decide this stage of the request for "+req.Package.Name+".")
}

func managerOf(u store.User) string {
	if u.ManagerID != nil {
		return *u.ManagerID
	}
	if u.Sponsor != nil {
		return u.Sponsor.ID
	}
	return ""
}

func (h *handler) beneficiary(ctx context.Context, actor store.User, userID string) (store.User, error) {
	refused := httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "You can request access for people who report to you, and for guests you sponsor who have no manager. Choose someone from your team.")
	if !actor.CanSponsor() {
		return store.User{}, refused
	}
	target, err := h.st.GetUser(ctx, userID)
	if errors.Is(err, store.ErrNotFound) {
		return store.User{}, refused
	}
	if err != nil {
		return store.User{}, err
	}
	if target.Kind != "person" || (target.Status != "active" && target.Status != "invited") || managerOf(target) != actor.ID {
		return store.User{}, refused
	}
	return target, nil
}

func answersFor(pkg store.AccessPackage, given map[string]string) ([]store.AccessAnswer, error) {
	answers := []store.AccessAnswer{}
	for _, q := range pkg.Questions {
		answer := strings.TrimSpace(given[q.ID])
		switch {
		case answer == "" && q.Required:
			return nil, httpx.Invalid("Answer " + q.Text + ". " + pkg.Name + " asks it of everyone who requests it.")
		case answer == "":
			continue
		case len(answer) > 500:
			return nil, httpx.Invalid("Keep the answer to " + q.Text + " under 500 characters.")
		case q.Kind == "choice" && !slices.Contains(q.Choices, answer):
			return nil, httpx.Invalid("Choose one of the listed answers to " + q.Text + ".")
		}
		answers = append(answers, store.AccessAnswer{QuestionID: q.ID, Question: q.Text, Answer: answer})
	}
	return answers, nil
}

func answersText(answers []store.AccessAnswer) string {
	lines := make([]string, len(answers))
	for i, a := range answers {
		lines[i] = a.Question + ": " + a.Answer
	}
	return strings.Join(lines, "\n")
}

func requestDuration(req store.AccessRequest) string {
	if req.Extends != nil && req.DurationDays != nil {
		return "for " + plural(*req.DurationDays, "more day")
	}
	return durationText(req.DurationDays)
}

func requestState(req store.AccessRequest) map[string]any {
	return map[string]any{"status": req.Status, "stage": req.Stage, "expiresAt": req.ExpiresAt}
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
			if !slices.Contains([]string{"pending", "approved", "denied", "cancelled", "expired"}, status) {
				return httpx.Invalid("status must be pending, approved, denied, cancelled or expired, or a comma-separated list such as approved,denied.")
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
	list, err := h.st.ListAccessRequests(r.Context(), store.AccessRequestFilter{Person: u.ID})
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

func (h *handler) myReports(w http.ResponseWriter, r *http.Request, u store.User) error {
	if !u.CanSponsor() {
		return httpx.JSON(w, http.StatusOK, []store.NamedRef{})
	}
	list, err := h.st.DirectReports(r.Context(), u.ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, nonNil(list))
}

func (h *handler) createRequest(w http.ResponseWriter, r *http.Request, u store.User) error {
	var in struct {
		PackageID     string            `json:"packageId"`
		BeneficiaryID string            `json:"beneficiaryId"`
		ExtendsID     string            `json:"extendsId"`
		DurationDays  *int              `json:"durationDays"`
		Justification string            `json:"justification"`
		Answers       map[string]string `json:"answers"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	beneficiary := u
	if in.BeneficiaryID != "" && in.BeneficiaryID != u.ID {
		var err error
		if beneficiary, err = h.beneficiary(r.Context(), u, in.BeneficiaryID); err != nil {
			return err
		}
	}
	var extends *string
	if in.ExtendsID != "" {
		grant, err := h.st.GetAccessRequest(r.Context(), in.ExtendsID)
		if errors.Is(err, store.ErrNotFound) || (err == nil && grant.Requester.ID != beneficiary.ID) {
			return httpx.Invalid("That access does not exist. Refresh the page and choose access from the list.")
		}
		if err != nil {
			return err
		}
		switch {
		case in.PackageID != "" && in.PackageID != grant.Package.ID:
			return httpx.Invalid("That access comes from " + grant.Package.Name + ", so send its packageId or leave packageId out.")
		case grant.Status != "approved" || grant.EndedAt != nil:
			return conflict(grant.Package.Name + " already ended, so request it again instead of extending it.")
		case grant.ExpiresAt == nil:
			return conflict(grant.Package.Name + " lasts until it is revoked, so there is nothing to extend.")
		}
		in.PackageID, extends = grant.Package.ID, &grant.ID
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
		return httpx.Invalid("Explain why " + map[bool]string{true: "you need", false: beneficiary.Name + " needs"}[beneficiary.ID == u.ID] + " " + pkg.Name + ". Approvers read this when they decide.")
	}
	if len(justification) > 2000 {
		return httpx.Invalid("Keep the justification under 2,000 characters.")
	}
	answers, err := answersFor(pkg, in.Answers)
	if err != nil {
		return err
	}
	if err := checkDuration(pkg, in.DurationDays); err != nil {
		return err
	}
	var requestedBy *string
	summary := "Requested " + pkg.Name + " " + durationText(in.DurationDays)
	if beneficiary.ID != u.ID {
		requestedBy, summary = &u.ID, summary+" for "+beneficiary.Name
	}
	if extends != nil {
		summary = "Asked to extend " + pkg.Name + " " + durationText(in.DurationDays)
	}
	var requestID string
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		requestID, err = tx.CreateAccessRequest(r.Context(), store.NewAccessRequest{PackageID: pkg.ID, RequesterID: beneficiary.ID, RequestedBy: requestedBy, ExtendsID: extends,
			Justification: justification, Answers: answers, DurationDays: in.DurationDays})
		if errors.Is(err, store.ErrConflict) {
			return conflict(map[bool]string{true: "You already have", false: beneficiary.Name + " already has"}[beneficiary.ID == u.ID] + " a pending request for " + pkg.Name + ". Wait for a decision or cancel it first.")
		}
		if err != nil {
			return err
		}
		return record(r, tx, u, store.AuditEvent{Action: "access_request.create", Summary: summary, TargetType: "user", TargetID: beneficiary.ID, TargetLabel: beneficiary.Name})
	})
	if err != nil {
		return err
	}
	req, err := h.st.GetAccessRequest(r.Context(), requestID)
	if err != nil {
		return err
	}
	h.notifyApprovers(r.Context(), req, false)
	if requestedBy != nil {
		h.send(r.Context(), mail.AccessRequestedForYou(beneficiary.Email, u.Name, pkg.Name, requestDuration(req), h.cfg.Issuer()+"/account/access"))
	}
	return httpx.JSON(w, http.StatusCreated, req)
}

func (h *handler) cancelRequest(w http.ResponseWriter, r *http.Request, u store.User) error {
	list, err := h.st.ListAccessRequests(r.Context(), store.AccessRequestFilter{ID: r.PathValue("id"), Person: u.ID})
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
		return record(r, tx, u, store.AuditEvent{Action: "access_request.cancel", Summary: "Cancelled the request for " + req.Package.Name, TargetType: "user", TargetID: req.Requester.ID, TargetLabel: req.Requester.Name})
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
	stage := ""
	if req.Stages > 1 {
		stage = fmt.Sprintf(" at stage %d of %d", req.Stage, req.Stages)
	}
	var updated store.AccessRequest
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		if approve {
			_, err = tx.ApproveAccessRequest(r.Context(), req.ID, req.Stage, actor.ID, note, time.Now())
		} else {
			err = tx.DenyAccessRequest(r.Context(), req.ID, req.Stage, actor.ID, note, time.Now())
		}
		if errors.Is(err, store.ErrConflict) {
			return conflict("Someone decided this request a moment ago. Refresh the page to see the decision.")
		}
		if err != nil {
			return err
		}
		if updated, err = tx.GetAccessRequest(r.Context(), req.ID); err != nil {
			return err
		}
		action, summary := "access_request.deny", "Denied "+req.Package.Name+stage
		switch {
		case approve && updated.Status == "pending":
			action, summary = "access_request.approve", "Approved "+req.Package.Name+stage+", so it moved to the next stage"
		case approve && updated.Extends != nil:
			until := "it is revoked"
			if updated.ExpiresAt != nil {
				until = formatTime(*updated.ExpiresAt)
			}
			action, summary = "access_request.approve", "Approved the extension of "+req.Package.Name+stage+" until "+until
		case approve:
			action, summary = "access_request.approve", "Approved "+req.Package.Name+" "+durationText(req.DurationDays)+stage
		}
		return record(r, tx, actor, store.AuditEvent{Action: action, Summary: summary, TargetType: "user", TargetID: req.Requester.ID, TargetLabel: req.Requester.Name,
			Changes: store.Diff(requestState(req), requestState(updated))})
	})
	if err != nil {
		return err
	}
	if updated.Status == "pending" {
		h.notifyApprovers(r.Context(), updated, false)
	} else {
		h.notifyRequester(r.Context(), updated)
	}
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

func (h *handler) notifyApprovers(ctx context.Context, req store.AccessRequest, fallbackOnly bool) {
	contacts, err := h.st.AccessApproverContacts(ctx, req.ID, fallbackOnly)
	if err != nil {
		slog.WarnContext(ctx, "could not look up approvers to email", "request", req.ID, "error", err)
		return
	}
	requestedBy := ""
	if req.RequestedBy != nil {
		requestedBy = req.RequestedBy.Name
	}
	for _, c := range contacts {
		link := h.cfg.Issuer() + "/account/access"
		if c.Admin {
			link = h.cfg.Issuer() + "/admin/requests"
		}
		h.send(ctx, mail.AccessRequested(c.Email, req.Requester.Name, req.Requester.Email, requestedBy, req.Package.Name, requestDuration(req), req.Justification, answersText(req.Answers), link))
	}
}

func (h *handler) notifyRequester(ctx context.Context, req store.AccessRequest) {
	link := h.cfg.Issuer() + "/account/access"
	if req.Status == "expired" {
		h.send(ctx, mail.AccessExpired(req.Requester.Email, req.Package.Name, link))
		return
	}
	decider := "An approver"
	if req.DecidedBy != nil {
		decider = req.DecidedBy.Name
	}
	expires := ""
	if req.ExpiresAt != nil {
		expires = formatTime(*req.ExpiresAt)
	}
	h.send(ctx, mail.AccessDecided(req.Requester.Email, req.Status == "approved", decider, req.Package.Name, expires, names(req.Groups), req.DecisionNote, link))
}

func (h *handler) timeOutRequests(ctx context.Context, now time.Time) error {
	due, err := h.st.TimedOutAccessRequests(ctx, now)
	if err != nil {
		return err
	}
	var errs []error
	for _, t := range due {
		var updated store.AccessRequest
		err := h.st.Tx(ctx, func(tx *store.Store) error {
			req, err := tx.GetAccessRequest(ctx, t.RequestID)
			if err != nil {
				return err
			}
			action, summary, change := "access_request.timeout", "Closed the request for "+req.Package.Name+" because nobody decided within "+plural(t.TimeoutDays, "day"), tx.ExpireAccessRequest
			if t.Escalate {
				action, summary, change = "access_request.escalate", "Sent the request for "+req.Package.Name+" to the fallback approvers because nobody decided within "+plural(t.TimeoutDays, "day"), tx.EscalateAccessRequest
			}
			if err := change(ctx, t, now); err != nil {
				return err
			}
			if updated, err = tx.GetAccessRequest(ctx, t.RequestID); err != nil {
				return err
			}
			return tx.RecordAudit(ctx, store.AuditEvent{Action: action, Summary: summary, TargetType: "user", TargetID: req.Requester.ID, TargetLabel: req.Requester.Name,
				Changes: store.Diff(map[string]any{"status": req.Status, "stage": req.Stage, "escalated": t.Escalated}, map[string]any{"status": updated.Status, "stage": updated.Stage, "escalated": t.Escalate || t.Escalated})})
		})
		if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if t.Escalate {
			h.notifyApprovers(ctx, updated, true)
		} else {
			h.notifyRequester(ctx, updated)
		}
	}
	return errors.Join(errs...)
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
