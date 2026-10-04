package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"slices"
	"strings"
	"time"

	"halo/internal/httpx"
	halomail "halo/internal/mail"
	"halo/internal/store"
)

func (h *handlers) user(r *http.Request) (store.User, error) {
	u, err := h.st.GetUser(r.Context(), r.PathValue("id"))
	return u, found(err, "This user")
}

func (h *handlers) respondUser(w http.ResponseWriter, r *http.Request, userID string) error {
	u, err := h.st.GetUser(r.Context(), userID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, u)
}

func (h *handlers) email(r *http.Request, m halomail.Message) bool {
	if h.mailer == nil {
		return false
	}
	if err := h.mailer.Send(r.Context(), m); err != nil {
		slog.WarnContext(r.Context(), "email not delivered", "to", m.To, "subject", m.Subject, "error", err)
		return false
	}
	return h.mailer.Configured()
}

func (h *handlers) emailLink(r *http.Request, token string, m halomail.Message) (bool, error) {
	if !h.email(r, m) {
		return false, nil
	}
	return true, h.st.MarkEnrollmentEmailed(r.Context(), token)
}

func guardAdmin(actor, target store.User) error {
	if len(target.RoleKeys) > 0 && !actor.HasRole("global_admin") {
		return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", target.Name+" holds an administrator role, so only a global administrator can change their account.")
	}
	return nil
}

func (h *handlers) listUsers(w http.ResponseWriter, r *http.Request, _ store.User) error {
	users, err := h.st.ListUsers(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, nonNil(users))
}

func (h *handlers) getUser(w http.ResponseWriter, r *http.Request, _ store.User) error {
	u, err := h.user(r)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, u)
}

func (h *handlers) userSessions(w http.ResponseWriter, r *http.Request, _ store.User) error {
	u, err := h.user(r)
	if err != nil {
		return err
	}
	sessions, err := h.st.ListSessions(r.Context(), u.ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, markCurrent(r, sessions))
}

func (h *handlers) userSignIns(w http.ResponseWriter, r *http.Request, _ store.User) error {
	u, err := h.user(r)
	if err != nil {
		return err
	}
	events, err := h.st.ListSignIns(r.Context(), store.SignInFilter{UserID: u.ID})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, events)
}

func (h *handlers) inviteUser(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in struct {
		Email      string   `json:"email"`
		Name       string   `json:"name"`
		Title      string   `json:"title"`
		Department string   `json:"department"`
		Location   string   `json:"location"`
		ManagerID  string   `json:"managerId"`
		GroupIDs   []string `json:"groupIds"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	email, err := checkEmail(in.Email)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return httpx.Invalid(nameRequired)
	}
	if tooLong(200, name, in.Title, in.Department, in.Location) {
		return httpx.Invalid(profileTooLong)
	}
	var managerID *string
	if in.ManagerID != "" {
		manager, err := h.manager(r, in.ManagerID)
		if err != nil {
			return err
		}
		managerID = &manager.ID
	}
	groups, err := h.groupsByID(r.Context(), in.GroupIDs)
	if err != nil {
		return err
	}
	for _, g := range groups {
		if g.Kind == "dynamic" {
			return httpx.Invalid(g.Name + " is a dynamic group, so people join it through its rule. Leave it out of the invitation.")
		}
	}

	var user store.User
	var token string
	var expires time.Time
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		user, err = tx.CreateUser(r.Context(), store.NewUser{
			Email:      email,
			Name:       name,
			Title:      strings.TrimSpace(in.Title),
			Department: strings.TrimSpace(in.Department),
			Location:   strings.TrimSpace(in.Location),
			ManagerID:  managerID,
			Status:     "invited",
			GroupIDs:   groupIDs(groups),
		})
		if errors.Is(err, store.ErrConflict) {
			return conflict("Someone with the email " + email + " already exists. Open their profile instead of inviting them again.")
		}
		if err != nil {
			return err
		}
		token, expires, err = tx.CreateEnrollmentToken(r.Context(), user.ID, "invite", &actor.ID)
		if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "user.invite", Summary: "Invited " + user.Name + " <" + user.Email + ">", TargetType: "user", TargetID: user.ID, TargetLabel: user.Name})
	})
	if err != nil {
		return err
	}
	link := h.cfg.Issuer() + store.EnrollPath(token)
	emailed, err := h.emailLink(r, token, halomail.Invite(user.Email, user.Name, actor.Name, link, expires))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, map[string]any{"user": user, "enrollUrl": link, "expiresAt": expires, "emailed": emailed})
}

func (h *handlers) suspendUser(w http.ResponseWriter, r *http.Request, actor store.User) error {
	target, err := h.user(r)
	if err != nil {
		return err
	}
	if target.ID == actor.ID {
		return httpx.Invalid("You cannot suspend your own account. Ask another administrator if this account must be locked.")
	}
	if err := guardAdmin(actor, target); err != nil {
		return err
	}
	if target.HasRole("global_admin") && target.Status == "active" {
		users, err := h.st.ListUsers(r.Context())
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(users, func(u store.User) bool {
			return u.ID != target.ID && u.Status == "active" && u.HasRole("global_admin")
		}) {
			return httpx.Invalid(target.Name + " is the last active global administrator. Make someone else a global administrator before suspending them.")
		}
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.SetUserStatus(r.Context(), target.ID, "suspended"); err != nil {
			return err
		}
		revoked, err := tx.RevokeUserSessions(r.Context(), target.ID, "")
		if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "user.suspend", Summary: "Suspended the account and ended " + plural(revoked, "session"), TargetType: "user", TargetID: target.ID, TargetLabel: target.Name})
	})
	if err != nil {
		return err
	}
	return h.respondUser(w, r, target.ID)
}

func (h *handlers) restoreUser(w http.ResponseWriter, r *http.Request, actor store.User) error {
	target, err := h.user(r)
	if err != nil {
		return err
	}
	if err := guardAdmin(actor, target); err != nil {
		return err
	}
	if target.Status != "suspended" {
		return conflict(target.Name + " is not suspended, so there is nothing to restore.")
	}
	status := "active"
	if target.LastSignInAt == nil && len(target.Methods) == 0 {
		status = "invited"
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.SetUserStatus(r.Context(), target.ID, status); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "user.restore", Summary: "Restored access", TargetType: "user", TargetID: target.ID, TargetLabel: target.Name})
	})
	if err != nil {
		return err
	}
	return h.respondUser(w, r, target.ID)
}

func (h *handlers) revokeUserSessions(w http.ResponseWriter, r *http.Request, actor store.User) error {
	target, err := h.user(r)
	if err != nil {
		return err
	}
	keep := ""
	if current, ok := httpx.CurrentSession(r); ok && target.ID == actor.ID {
		keep = current.ID
	}
	var revoked int
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		revoked, err = tx.RevokeUserSessions(r.Context(), target.ID, keep)
		if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "session.revoke", Summary: "Ended " + plural(revoked, "session"), TargetType: "user", TargetID: target.ID, TargetLabel: target.Name})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]int{"revoked": revoked})
}

func (h *handlers) resetAuthentication(w http.ResponseWriter, r *http.Request, actor store.User) error {
	target, err := h.user(r)
	if err != nil {
		return err
	}
	if err := guardAdmin(actor, target); err != nil {
		return err
	}
	var token string
	var expires time.Time
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.DeleteAuthenticationMethods(r.Context(), target.ID); err != nil {
			return err
		}
		if _, err := tx.RevokeUserSessions(r.Context(), target.ID, ""); err != nil {
			return err
		}
		var err error
		token, expires, err = tx.CreateEnrollmentToken(r.Context(), target.ID, "reset", &actor.ID)
		if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "user.authentication.reset", Summary: "Reset authentication methods and created a new setup link", TargetType: "user", TargetID: target.ID, TargetLabel: target.Name})
	})
	if err != nil {
		return err
	}
	link := h.cfg.Issuer() + store.EnrollPath(token)
	emailed, err := h.emailLink(r, token, halomail.Reset(target.Email, target.Name, actor.Name, link, expires))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"enrollUrl": link, "expiresAt": expires, "emailed": emailed})
}

func (h *handlers) deleteMethod(w http.ResponseWriter, r *http.Request, actor store.User) error {
	target, err := h.user(r)
	if err != nil {
		return err
	}
	if err := guardAdmin(actor, target); err != nil {
		return err
	}
	i := slices.IndexFunc(target.Methods, func(m store.Method) bool { return m.ID == r.PathValue("methodId") })
	if i < 0 {
		return httpx.NotFound("This sign-in method")
	}
	method := target.Methods[i]
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.DeleteMethod(r.Context(), target.ID, method.ID); err != nil {
			return found(err, "This sign-in method")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "user.method.delete", Summary: "Removed sign-in method “" + method.Label + "”", TargetType: "user", TargetID: target.ID, TargetLabel: target.Name})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

func (h *handlers) setRoles(w http.ResponseWriter, r *http.Request, actor store.User) error {
	target, err := h.user(r)
	if err != nil {
		return err
	}
	var in struct {
		Roles *[]string `json:"roles"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if in.Roles == nil {
		return httpx.Invalid(`Send the complete list of roles, for example {"roles": ["auditor"]}. Send an empty list to remove every role.`)
	}
	roles, labels := []string{}, []string{}
	for _, role := range *in.Roles {
		label, ok := store.RoleLabels[role]
		if !ok {
			return httpx.Invalid(fmt.Sprintf("%q is not a role. Use global_admin, security_admin, user_admin, helpdesk_admin, app_admin or auditor.", role))
		}
		if !slices.Contains(roles, role) {
			roles = append(roles, role)
			labels = append(labels, label)
		}
	}
	if target.ID == actor.ID && target.HasRole("global_admin") && !slices.Contains(roles, "global_admin") {
		return httpx.Invalid("You cannot remove your own global administrator role. Ask another global administrator to do it, so the organization always keeps one.")
	}
	summary := "Removed every administrator role"
	if len(labels) > 0 {
		summary = "Set roles to " + strings.Join(labels, ", ")
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.SetUserRoles(r.Context(), target.ID, roles); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "role.update", Summary: summary, TargetType: "user", TargetID: target.ID, TargetLabel: target.Name})
	})
	if err != nil {
		return err
	}
	return h.respondUser(w, r, target.ID)
}

const (
	nameRequired   = "Enter the person's name so other administrators can recognise them."
	profileTooLong = "Keep the name, title, department and location under 200 characters each."
)

func checkEmail(raw string) (string, error) {
	email := strings.TrimSpace(raw)
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || len(email) > 254 {
		return "", httpx.Invalid(fmt.Sprintf("%q is not a valid email address. Enter a single address such as sam@example.com.", raw))
	}
	return email, nil
}

func (h *handlers) manager(r *http.Request, managerID string) (store.User, error) {
	m, err := h.st.GetUser(r.Context(), managerID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && m.Kind != "person") {
		return m, httpx.Invalid("That manager is not a person in the directory. Refresh the page and pick someone from the list.")
	}
	return m, err
}

func (h *handlers) updateUser(w http.ResponseWriter, r *http.Request, actor store.User) error {
	target, err := h.user(r)
	if err != nil {
		return err
	}
	if err := guardAdmin(actor, target); err != nil {
		return err
	}
	var in struct {
		Email      *string `json:"email"`
		Name       *string `json:"name"`
		Title      *string `json:"title"`
		Department *string `json:"department"`
		Location   *string `json:"location"`
		ManagerID  *string `json:"managerId"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	next, changed := target, []string{}
	for _, f := range []struct {
		label string
		value *string
		field *string
	}{{"name", in.Name, &next.Name}, {"email", in.Email, &next.Email}, {"title", in.Title, &next.Title}, {"department", in.Department, &next.Department}, {"location", in.Location, &next.Location}} {
		if f.value != nil && strings.TrimSpace(*f.value) != *f.field {
			*f.field = strings.TrimSpace(*f.value)
			changed = append(changed, f.label)
		}
	}
	current := ""
	if target.ManagerID != nil {
		current = *target.ManagerID
	}
	if in.ManagerID != nil && strings.TrimSpace(*in.ManagerID) != current {
		next.ManagerID = nil
		if managerID := strings.TrimSpace(*in.ManagerID); managerID != "" {
			next.ManagerID = &managerID
		}
		changed = append(changed, "manager")
	}
	if !actor.HasRole("global_admin", "user_admin") && slices.ContainsFunc(changed, func(f string) bool { return f == "name" || f == "email" || f == "manager" }) {
		return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "Helpdesk administrators can change title, department and location only. Ask a user administrator to change the name, email or manager.")
	}
	if len(changed) == 0 {
		return httpx.JSON(w, http.StatusOK, target)
	}
	if slices.Contains(changed, "email") {
		if _, err := checkEmail(next.Email); err != nil {
			return err
		}
	}
	if next.Name == "" {
		return httpx.Invalid(nameRequired)
	}
	if tooLong(200, next.Name, next.Title, next.Department, next.Location) {
		return httpx.Invalid(profileTooLong)
	}
	if next.ManagerID != nil && *next.ManagerID != current {
		if *next.ManagerID == target.ID {
			return httpx.Invalid(target.Name + " cannot be their own manager. Pick the person they report to.")
		}
		manager, err := h.manager(r, *next.ManagerID)
		if err != nil {
			return err
		}
		loop, err := h.st.ManagementChainIncludes(r.Context(), manager.ID, target.ID)
		if err != nil {
			return err
		}
		if loop {
			return httpx.Invalid(manager.Name + " already reports to " + target.Name + ", so making them " + target.Name + "'s manager would create a loop. Pick someone higher in the organization.")
		}
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.UpdateUserProfile(r.Context(), next); errors.Is(err, store.ErrConflict) {
			return conflict("Someone else already uses the email " + next.Email + ". Each person needs their own address.")
		} else if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "user.update", Summary: "Updated " + strings.Join(changed, ", "), TargetType: "user", TargetID: target.ID, TargetLabel: next.Name})
	})
	if err != nil {
		return err
	}
	return h.respondUser(w, r, target.ID)
}
