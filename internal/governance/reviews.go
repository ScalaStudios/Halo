package governance

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"halo/internal/httpx"
	"halo/internal/store"
)

func (h *handler) listReviews(w http.ResponseWriter, r *http.Request, _ store.User) error {
	list, err := h.st.ListAccessReviews(r.Context(), store.AccessReviewFilter{})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, nonNil(list))
}

func (h *handler) myReviews(w http.ResponseWriter, r *http.Request, u store.User) error {
	list, err := h.st.ListAccessReviews(r.Context(), store.AccessReviewFilter{ReviewerID: u.ID, Open: true})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, nonNil(list))
}

func (h *handler) loadReview(ctx context.Context, reviewID string, u store.User) (store.AccessReview, error) {
	review, err := h.st.GetAccessReview(ctx, reviewID)
	if err != nil {
		return review, found(err, "This access review")
	}
	reviewer := hasRef(review.Reviewers, u.ID)
	if !reviewer && !u.HasRole("global_admin") && !u.HasRole(readers...) {
		return review, httpx.NotFound("This access review")
	}
	open := review.Status != "completed"
	review.CanDecide = open && (reviewer || u.HasRole("global_admin"))
	review.CanComplete = open && (reviewer || u.HasRole("global_admin", "user_admin"))
	return review, nil
}

func (h *handler) respondReview(w http.ResponseWriter, r *http.Request, reviewID string, u store.User, status int) error {
	review, err := h.loadReview(r.Context(), reviewID, u)
	if err != nil {
		return err
	}
	return httpx.JSON(w, status, review)
}

func (h *handler) getReview(w http.ResponseWriter, r *http.Request, u store.User) error {
	return h.respondReview(w, r, r.PathValue("id"), u, http.StatusOK)
}

func (h *handler) createReview(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in struct {
		Name        string    `json:"name"`
		GroupID     string    `json:"groupId"`
		ReviewerIDs []string  `json:"reviewerIds"`
		DueAt       time.Time `json:"dueAt"`
		AutoApply   bool      `json:"autoApply"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	name := strings.TrimSpace(in.Name)
	switch {
	case name == "":
		return httpx.Invalid("Enter a review name, for example Production access · Q4.")
	case len(name) > 200:
		return httpx.Invalid("Keep the review name under 200 characters.")
	case len(in.ReviewerIDs) == 0:
		return httpx.Invalid("Choose at least one reviewer. Reviewers decide whether each member keeps access.")
	case !in.DueAt.After(time.Now()):
		return httpx.Invalid("Choose a due date in the future.")
	case in.DueAt.After(time.Now().AddDate(1, 0, 0)):
		return httpx.Invalid("Choose a due date within the next year.")
	}
	g, err := h.st.GetGroup(r.Context(), in.GroupID)
	if errors.Is(err, store.ErrNotFound) {
		return httpx.Invalid("That group does not exist. Refresh the page and choose a group from the list.")
	}
	if err != nil {
		return err
	}
	if g.Kind == "dynamic" {
		return httpx.Invalid(g.Name + " is a rule-based group: its members come from the rule " + *g.Rule + ", so they can't be removed one by one. Access reviews cover assigned groups only; change the rule or people's profiles instead.")
	}
	if g.MemberCount == 0 {
		return httpx.Invalid(g.Name + " has no members, so there is nothing to review.")
	}
	reviewers := unique(in.ReviewerIDs)
	if _, err := h.people(r.Context(), reviewers, "reviewer"); err != nil {
		return err
	}
	var reviewID string
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		reviewID, err = tx.CreateAccessReview(r.Context(), store.NewAccessReview{Name: name, GroupID: g.ID, ReviewerIDs: reviewers, DueAt: in.DueAt, AutoApply: in.AutoApply, CreatedBy: actor.ID})
		if err != nil {
			return err
		}
		summary := fmt.Sprintf("Started a review of %s in %s, due %s", plural(g.MemberCount, "member"), g.Name, formatTime(in.DueAt))
		return record(r, tx, actor, store.AuditEvent{Action: "access_review.create", Summary: summary, TargetType: "access_review", TargetID: reviewID, TargetLabel: name})
	})
	if err != nil {
		return err
	}
	return h.respondReview(w, r, reviewID, actor, http.StatusCreated)
}

func (h *handler) decideReviewItem(w http.ResponseWriter, r *http.Request, u store.User) error {
	var in struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	note := strings.TrimSpace(in.Note)
	if in.Decision != "keep" && in.Decision != "remove" {
		return httpx.Invalid(`Send "keep" to confirm this person's access or "remove" to take it away.`)
	}
	if len(note) > 2000 {
		return httpx.Invalid("Keep the note under 2,000 characters.")
	}
	review, err := h.loadReview(r.Context(), r.PathValue("id"), u)
	if err != nil {
		return err
	}
	if review.Status == "completed" {
		return conflict("This review is complete, so its decisions can't change.")
	}
	if !review.CanDecide {
		return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "Only the reviewers of "+review.Name+" ("+names(review.Reviewers)+") or a global administrator can record decisions.")
	}
	i := slices.IndexFunc(review.Items, func(item store.ReviewItem) bool { return item.User.ID == r.PathValue("userId") })
	if i < 0 {
		return httpx.Fail(http.StatusNotFound, "ERR_NOT_FOUND", "This person is not part of "+review.Name+". Refresh the page to see who is being reviewed.")
	}
	member := review.Items[i].User
	summary := "Kept " + member.Name + " in " + review.Group.Name
	if in.Decision == "remove" {
		summary = "Marked " + member.Name + " for removal from " + review.Group.Name
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.DecideReviewItem(r.Context(), review.ID, member.ID, in.Decision, note, u.ID, time.Now()); errors.Is(err, store.ErrNotFound) {
			return conflict("This review was completed a moment ago, so its decisions can't change.")
		} else if err != nil {
			return err
		}
		return record(r, tx, u, store.AuditEvent{Action: "access_review.decide", Summary: summary, TargetType: "access_review", TargetID: review.ID, TargetLabel: review.Name})
	})
	if err != nil {
		return err
	}
	return h.respondReview(w, r, review.ID, u, http.StatusOK)
}

func (h *handler) completeReview(w http.ResponseWriter, r *http.Request, u store.User) error {
	review, err := h.loadReview(r.Context(), r.PathValue("id"), u)
	if err != nil {
		return err
	}
	if review.Status == "completed" {
		return conflict("This review was already completed.")
	}
	if !review.CanComplete {
		return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "Only the reviewers of "+review.Name+" ("+names(review.Reviewers)+") or a user administrator can complete this review.")
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		out, err := tx.CompleteAccessReview(r.Context(), review.ID, u.ID, time.Now())
		if errors.Is(err, store.ErrConflict) {
			return conflict("This review was completed a moment ago. Refresh the page to see the results.")
		}
		if err != nil {
			return err
		}
		summary := fmt.Sprintf("Completed the review: %d kept, %d removed", out.Kept, len(out.Removed))
		if out.NotRemoved > 0 {
			summary += fmt.Sprintf(", %d marked for removal but left in place because automatic removal is off", out.NotRemoved)
		}
		if out.NoDecision > 0 {
			summary += fmt.Sprintf(", %d kept access without a decision", out.NoDecision)
		}
		if err := record(r, tx, u, store.AuditEvent{Action: "access_review.complete", Summary: summary, TargetType: "access_review", TargetID: review.ID, TargetLabel: review.Name}); err != nil {
			return err
		}
		for _, m := range out.Removed {
			if err := record(r, tx, u, store.AuditEvent{Action: "group.member.remove", Summary: "Removed " + m.Name + " after the access review " + review.Name, TargetType: "group", TargetID: review.Group.ID, TargetLabel: review.Group.Name}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return h.respondReview(w, r, review.ID, u, http.StatusOK)
}

func markOverdueReviews(ctx context.Context, st *store.Store, now time.Time) error {
	return st.Tx(ctx, func(tx *store.Store) error {
		overdue, err := tx.MarkOverdueReviews(ctx, now)
		if err != nil {
			return err
		}
		for _, v := range overdue {
			if err := tx.RecordAudit(ctx, store.AuditEvent{Action: "access_review.overdue", Summary: "Passed its due date before every decision was made", TargetType: "access_review", TargetID: v.ID, TargetLabel: v.Name}); err != nil {
				return err
			}
		}
		return nil
	})
}
