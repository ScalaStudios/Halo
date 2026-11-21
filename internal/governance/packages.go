package governance

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"halo/internal/httpx"
	"halo/internal/id"
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

type stageInput struct {
	Approver    string   `json:"approver"`
	ApproverIDs []string `json:"approverIds"`
	GroupID     *string  `json:"groupId"`
	FallbackIDs []string `json:"fallbackIds"`
	TimeoutDays *int     `json:"timeoutDays"`
}

func ids(refs []store.NamedRef) []string {
	out := make([]string, len(refs))
	for i, ref := range refs {
		out[i] = ref.ID
	}
	return out
}

func (h *handler) packageInput(r *http.Request, current *store.AccessPackage) (store.AccessPackageInput, error) {
	var in struct {
		Name                 string                  `json:"name"`
		Description          string                  `json:"description"`
		OwnerID              *string                 `json:"ownerId"`
		GroupIDs             []string                `json:"groupIds"`
		ApproverIDs          *[]string               `json:"approverIds"`
		Stages               *[]stageInput           `json:"stages"`
		Questions            *[]store.AccessQuestion `json:"questions"`
		MaxDays              *int                    `json:"maxDays"`
		RequireJustification bool                    `json:"requireJustification"`
		Archived             bool                    `json:"archived"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return store.AccessPackageInput{}, err
	}
	out := store.AccessPackageInput{
		Name:                 strings.TrimSpace(in.Name),
		Description:          strings.TrimSpace(in.Description),
		GroupIDs:             unique(in.GroupIDs),
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
	if in.OwnerID != nil && *in.OwnerID != "" {
		if _, err := h.people(r.Context(), []string{*in.OwnerID}, "owner"); err != nil {
			return out, err
		}
		out.OwnerID = in.OwnerID
	}
	var err error
	if out.Stages, err = h.stagesInput(r.Context(), in.Stages, in.ApproverIDs, current); err != nil {
		return out, err
	}
	out.Questions, err = questionsInput(in.Questions, current)
	return out, err
}

func (h *handler) stagesInput(ctx context.Context, stages *[]stageInput, approverIDs *[]string, current *store.AccessPackage) ([]store.AccessStageInput, error) {
	if stages == nil {
		if current != nil && approverIDs == nil {
			list := []stageInput{}
			for _, st := range current.Stages {
				var group *string
				if st.Group != nil {
					group = &st.Group.ID
				}
				list = append(list, stageInput{Approver: st.Approver, ApproverIDs: ids(st.Approvers), GroupID: group, FallbackIDs: ids(st.Fallback), TimeoutDays: st.TimeoutDays})
			}
			stages = &list
		} else {
			if current != nil && (len(current.Stages) != 1 || current.Stages[0].Approver != "people" || len(current.Stages[0].Fallback) > 0 || current.Stages[0].TimeoutDays != nil) {
				return nil, httpx.Invalid(current.Name + " has approval stages, so send them in stages. approverIds alone only changes a package with one stage of named approvers.")
			}
			list := []stageInput{{Approver: "people"}}
			if approverIDs != nil {
				list[0].ApproverIDs = *approverIDs
			}
			stages = &list
		}
	}
	if len(*stages) < 1 || len(*stages) > 3 {
		return nil, httpx.Invalid("Set between one and three approval stages. Requests go through them in order.")
	}
	out := []store.AccessStageInput{}
	for i, st := range *stages {
		n := i + 1
		in := store.AccessStageInput{Approver: st.Approver, ApproverIDs: unique(st.ApproverIDs), FallbackIDs: unique(st.FallbackIDs), TimeoutDays: st.TimeoutDays}
		switch st.Approver {
		case "people":
		case "manager", "group-owners":
			if len(in.ApproverIDs) > 0 {
				return nil, httpx.Invalid(fmt.Sprintf("Stage %d is decided by the requester's manager or a group's owners, so leave its approverIds empty. Name extra people as fallback approvers instead.", n))
			}
		default:
			return nil, httpx.Invalid(fmt.Sprintf("Choose who decides stage %d: people, manager or group-owners.", n))
		}
		if st.Approver == "group-owners" {
			if st.GroupID == nil || *st.GroupID == "" {
				return nil, httpx.Invalid(fmt.Sprintf("Choose the group whose owners decide stage %d.", n))
			}
			if _, err := h.assignedGroups(ctx, []string{*st.GroupID}); err != nil {
				return nil, err
			}
			in.GroupID = st.GroupID
		}
		if st.TimeoutDays != nil && (*st.TimeoutDays < 1 || *st.TimeoutDays > 30) {
			return nil, httpx.Invalid(fmt.Sprintf("Set the deadline of stage %d between 1 and 30 days, or send null so the stage waits until someone decides.", n))
		}
		users, err := h.people(ctx, append(slices.Clone(in.ApproverIDs), in.FallbackIDs...), "approver")
		if err != nil {
			return nil, err
		}
		for _, u := range users {
			if u.Guest || u.Kind != "person" {
				return nil, httpx.Invalid(u.Name + " can't approve requests. Approvers are people in your organization, never guests or service accounts.")
			}
		}
		out = append(out, in)
	}
	return out, nil
}

func questionsInput(in *[]store.AccessQuestion, current *store.AccessPackage) ([]store.AccessQuestion, error) {
	if in == nil {
		if current != nil {
			return current.Questions, nil
		}
		return []store.AccessQuestion{}, nil
	}
	if len(*in) > 10 {
		return nil, httpx.Invalid("Ask at most 10 questions. Long forms slow requests down.")
	}
	out := []store.AccessQuestion{}
	for i, q := range *in {
		q.Text = strings.TrimSpace(q.Text)
		if q.ID == "" {
			q.ID = id.New("qst")
		}
		choices := []string{}
		for _, c := range q.Choices {
			if c = strings.TrimSpace(c); c != "" && !slices.Contains(choices, c) {
				choices = append(choices, c)
			}
		}
		q.Choices = choices
		switch {
		case len(q.ID) > 40 || slices.ContainsFunc(out, func(o store.AccessQuestion) bool { return o.ID == q.ID }):
			return nil, httpx.Invalid(fmt.Sprintf("Question %d has a duplicate or invalid id. Leave id empty to have Halo assign one.", i+1))
		case q.Text == "":
			return nil, httpx.Invalid(fmt.Sprintf("Enter the text of question %d, for example Ticket number.", i+1))
		case len(q.Text) > 200:
			return nil, httpx.Invalid("Keep each question under 200 characters.")
		case q.Kind == "text":
			q.Choices = []string{}
		case q.Kind != "choice":
			return nil, httpx.Invalid(fmt.Sprintf("Question %d needs a kind: text or choice.", i+1))
		case len(choices) < 2 || len(choices) > 20:
			return nil, httpx.Invalid("Give " + q.Text + " between 2 and 20 choices.")
		case slices.ContainsFunc(choices, func(c string) bool { return len(c) > 100 }):
			return nil, httpx.Invalid("Keep each choice of " + q.Text + " under 100 characters.")
		}
		out = append(out, q)
	}
	return out, nil
}

func (h *handler) savePackage(w http.ResponseWriter, r *http.Request, actor store.User, current *store.AccessPackage) error {
	in, err := h.packageInput(r, current)
	if err != nil {
		return err
	}
	before := store.AccessPackage{}
	packageID, action, summary, status := "", "access_package.create", "Created the package with "+plural(len(in.GroupIDs), "group")+" and "+plural(len(in.Stages), "approval stage"), http.StatusCreated
	if current != nil {
		before = *current
		packageID, action, summary, status = current.ID, "access_package.update", "Updated the package", http.StatusOK
		if in.Archived != current.Archived {
			summary = "Restored the package, so it can be requested again"
			if in.Archived {
				summary = "Archived the package, so it can no longer be requested"
			}
		}
	}
	var saved store.AccessPackage
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		packageID, err = tx.SaveAccessPackage(r.Context(), packageID, in)
		if errors.Is(err, store.ErrConflict) {
			return conflict("A package named " + in.Name + " already exists. Pick a different name.")
		}
		if err != nil {
			return err
		}
		if saved, err = tx.GetAccessPackage(r.Context(), packageID); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: action, Summary: summary, TargetType: "access_package", TargetID: packageID, TargetLabel: in.Name, Changes: store.Diff(before, saved)})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, status, saved)
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
