package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"halo/internal/approval"
	"halo/internal/httpx"
	"halo/internal/store"
)

func (h *handlers) person(r *http.Request) (store.User, error) {
	u, err := h.user(r)
	if err == nil && u.Kind != "person" {
		return u, httpx.NotFound("This person")
	}
	return u, err
}

func (h *handlers) exportUser(w http.ResponseWriter, r *http.Request, actor store.User) error {
	target, err := h.person(r)
	if err != nil {
		return err
	}
	return h.exportPersonalData(w, r, actor, target, "Exported their personal data as JSON")
}

func (h *handlers) exportMe(w http.ResponseWriter, r *http.Request, actor store.User) error {
	if sess, ok := httpx.CurrentSession(r); !ok || sess.ImpersonationID != "" || actor.Kind != "person" {
		return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "Download your data from the account portal, signed in as yourself.")
	}
	return h.exportPersonalData(w, r, actor, actor, "Downloaded their personal data as JSON")
}

func (h *handlers) exportPersonalData(w http.ResponseWriter, r *http.Request, actor, person store.User, summary string) error {
	now := time.Now().UTC()
	out := map[string]any{"exportedAt": now, "person": person}
	for _, section := range store.PersonalDataSections {
		rows := []json.RawMessage{}
		if _, err := h.st.ExportRows(r.Context(), section, func(row json.RawMessage) error {
			rows = append(rows, row)
			return nil
		}, person.ID); err != nil {
			return err
		}
		out[section.Name] = rows
	}
	if err := record(r, h.st, actor, store.AuditEvent{Action: "user.export", Summary: summary, TargetType: "user", TargetID: person.ID, TargetLabel: person.Name}); err != nil {
		return err
	}
	w.Header().Set("Content-Disposition", `attachment; filename="halo-personal-data-`+person.ID+`-`+now.Format("2006-01-02")+`.json"`)
	return httpx.JSON(w, http.StatusOK, out)
}

func (h *handlers) deleteUser(w http.ResponseWriter, r *http.Request, actor store.User) error {
	ctx := r.Context()
	target, err := h.person(r)
	if err != nil {
		return err
	}
	check := func(st *store.Store, u store.User) error {
		if u.ID == actor.ID {
			return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "You cannot delete your own account. Ask another global administrator to do it.")
		}
		settings, err := st.CurrentSettings(ctx)
		if err != nil {
			return err
		}
		if slices.Contains(settings.EmergencyAccessIDs, u.ID) {
			return httpx.Invalid(u.Name + " is an emergency access account. Remove them from the emergency access accounts under Approvals first.")
		}
		provisioned, err := st.ActiveProvisionedAccounts(ctx, u.ID)
		if err != nil {
			return err
		}
		if provisioned > 0 {
			return conflict("Halo still keeps " + plural(provisioned, "account") + " for " + u.Name + " in other applications through provisioning. Suspend them and sync provisioning so those accounts are deactivated, then delete them.")
		}
		if !u.HasRole("global_admin") {
			return nil
		}
		others, err := st.GlobalAdminEmails(ctx, u.ID)
		if err == nil && len(others) == 0 {
			err = httpx.Invalid(u.Name + " is the only active global administrator. Grant the role to another person first, so the organization always keeps one.")
		}
		return err
	}
	if err := check(h.st, target); err != nil {
		return err
	}
	change := approval.Change{Action: "user-deletion", Summary: "Delete the person permanently", TargetType: "user", TargetID: target.ID, TargetLabel: target.Name, State: []any{target.Name, target.Email, target.RoleKeys}}
	if held, err := h.gate.Hold(w, r, actor, change); held || err != nil {
		return err
	}
	var pseudonym string
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.LockApprovalTarget(ctx, "user", target.ID); err != nil {
			return err
		}
		current, err := tx.GetUser(ctx, target.ID)
		if err != nil {
			return found(err, "This person")
		}
		if err := check(tx, current); err != nil {
			return err
		}
		if pseudonym, err = tx.ErasePerson(ctx, current); err != nil {
			return err
		}
		if current.AvatarURL != nil {
			if err := h.removePicture(ctx, current.ID); err != nil {
				return err
			}
		}
		return record(r, tx, actor, store.AuditEvent{Action: "user.delete", Summary: "Deleted permanently, and replaced their name and email address in the logs with a pseudonym", TargetType: "user", TargetID: pseudonym, TargetLabel: pseudonym})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]string{"pseudonym": pseudonym})
}
