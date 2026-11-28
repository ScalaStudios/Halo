package api

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"halo/internal/httpx"
	"halo/internal/store"
)

const (
	maxAppRoles     = 100
	maxRoleHolders  = 200
	maxRoleDescribe = 500
)

var roleKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:-]{0,63}$`)

type roleInput struct {
	Key         *string   `json:"key"`
	Name        *string   `json:"name"`
	Description *string   `json:"description"`
	GroupIDs    *[]string `json:"groupIds"`
	UserIDs     *[]string `json:"userIds"`
}

func (h *handlers) roleApplication(r *http.Request) (store.Application, error) {
	app, err := h.application(r)
	switch {
	case err != nil:
		return app, err
	case app.Protocol == "bookmark":
		return app, httpx.Invalid(app.Name + " is a bookmark. Nobody signs in to it through Halo, so it has no roles.")
	case app.Protocol == "remote":
		return app, httpx.Invalid(app.Name + " opens sessions to a host, which receives no roles from Halo. Assign groups to it instead.")
	case app.Protocol == "ldap":
		return app, httpx.Invalid(app.Name + " reads groups over LDAP, so it has no application roles. Assign groups to it instead.")
	case app.Type == "service":
		return app, httpx.Invalid(app.Name + " is a service application. Nobody signs in to it, so it has no roles to assign.")
	case store.BuiltInClient(app.ClientID):
		return app, httpx.Invalid(app.Name + " is built in and uses Halo's administrator roles, so it has no application roles.")
	}
	return app, nil
}

func appRole(app store.Application, roleID string) (store.AppRole, error) {
	i := slices.IndexFunc(app.Roles, func(r store.AppRole) bool { return r.ID == roleID })
	if i < 0 {
		return store.AppRole{}, httpx.NotFound("This role")
	}
	return app.Roles[i], nil
}

func (h *handlers) applyRole(r *http.Request, in roleInput, current store.AppRole) (store.AppRole, string, error) {
	next := current
	if in.Name != nil {
		next.Name = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		next.Description = strings.TrimSpace(*in.Description)
	}
	if in.GroupIDs != nil {
		next.GroupIDs = slices.Sorted(slices.Values(*in.GroupIDs))
	}
	if in.UserIDs != nil {
		next.UserIDs = nonNil(slices.Compact(slices.Sorted(slices.Values(*in.UserIDs))))
	}
	switch {
	case next.Name == "" || utf8.RuneCountInString(next.Name) > 100:
		return next, "", httpx.Invalid("Enter a role name of up to 100 characters, such as Editor.")
	case utf8.RuneCountInString(next.Description) > maxRoleDescribe:
		return next, "", httpx.Invalid(fmt.Sprintf("Keep the description to %d characters.", maxRoleDescribe))
	case len(next.UserIDs) > maxRoleHolders:
		return next, "", httpx.Invalid(fmt.Sprintf("Give a role to up to %d people directly. Assign a group to reach more.", maxRoleHolders))
	}
	groups, err := h.groupsByID(r.Context(), next.GroupIDs)
	if err != nil {
		return next, "", err
	}
	next.GroupIDs = groupIDs(groups)
	var holders []string
	for _, g := range groups {
		holders = append(holders, g.Name)
	}
	for _, userID := range next.UserIDs {
		u, err := h.st.GetUser(r.Context(), userID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return next, "", err
		}
		if !slices.Contains(current.UserIDs, userID) && (err != nil || u.Kind != "person" || u.Status == "deprovisioned") {
			return next, "", httpx.Invalid("That person can't hold a role. Refresh the page and pick someone from the directory.")
		}
		holders = append(holders, u.Name)
	}
	if len(holders) == 0 {
		return next, "nobody", nil
	}
	return next, strings.Join(holders, ", "), nil
}

func (h *handlers) createAppRole(w http.ResponseWriter, r *http.Request, actor store.User) error {
	app, err := h.roleApplication(r)
	if err != nil {
		return err
	}
	var in roleInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	key := ""
	if in.Key != nil {
		key = strings.TrimSpace(*in.Key)
	}
	if !roleKey.MatchString(key) {
		return httpx.Invalid("Start the key with a letter and use up to 64 letters, digits, dots, colons, dashes and underscores, such as editor or billing.admin. " + app.Name + " receives it in the roles claim.")
	}
	if len(app.Roles) >= maxAppRoles {
		return httpx.Invalid(fmt.Sprintf("An application holds up to %d roles. Delete one %s no longer uses first.", maxAppRoles, app.Name))
	}
	role, holders, err := h.applyRole(r, in, store.AppRole{Key: key, GroupIDs: []string{}, UserIDs: []string{}})
	if err != nil {
		return err
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		created, err := tx.CreateAppRole(r.Context(), app.ID, role)
		if errors.Is(err, store.ErrConflict) {
			return conflict(app.Name + " already has a role with the key " + key + ". Pick another key.")
		}
		if err != nil {
			return err
		}
		role = created
		return record(r, tx, actor, store.AuditEvent{Action: "application.role.create", Summary: fmt.Sprintf("Added the role %s (%s), held by %s", role.Name, role.Key, holders),
			TargetType: "application", TargetID: app.ID, TargetLabel: app.Name, Changes: store.Diff(store.AppRole{}, role)})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, role)
}

func (h *handlers) updateAppRole(w http.ResponseWriter, r *http.Request, actor store.User) error {
	app, err := h.roleApplication(r)
	if err != nil {
		return err
	}
	current, err := appRole(app, r.PathValue("roleId"))
	if err != nil {
		return err
	}
	var in roleInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if in.Key != nil && strings.TrimSpace(*in.Key) != current.Key {
		return httpx.Invalid("A role's key can't be changed, because " + app.Name + " already receives it. Add a new role and delete this one instead.")
	}
	next, holders, err := h.applyRole(r, in, current)
	if err != nil {
		return err
	}
	changes := store.Diff(current, next)
	if len(changes) == 0 {
		return httpx.JSON(w, http.StatusOK, current)
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.UpdateAppRole(r.Context(), next); err != nil {
			return found(err, "This role")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "application.role.update", Summary: fmt.Sprintf("Updated the role %s (%s), held by %s", next.Name, next.Key, holders),
			TargetType: "application", TargetID: app.ID, TargetLabel: app.Name, Changes: changes})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, next)
}

func (h *handlers) deleteAppRole(w http.ResponseWriter, r *http.Request, actor store.User) error {
	app, err := h.roleApplication(r)
	if err != nil {
		return err
	}
	role, err := appRole(app, r.PathValue("roleId"))
	if err != nil {
		return err
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.DeleteAppRole(r.Context(), role.ID); err != nil {
			return found(err, "This role")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "application.role.delete", Summary: fmt.Sprintf("Deleted the role %s (%s)", role.Name, role.Key),
			TargetType: "application", TargetID: app.ID, TargetLabel: app.Name, Changes: store.Diff(role, store.AppRole{})})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}
