package apikeys

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"halo/internal/httpx"
	"halo/internal/store"
)

var (
	readers = []string{"security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"}
	writers = []string{"security_admin"}
	scopes  = []string{"api", "scim"}
)

func Resolver(st *store.Store) httpx.KeyResolver {
	return st.ResolveAPIKey
}

type handlers struct {
	st *store.Store
}

func Register(mux *http.ServeMux, d httpx.Deps) error {
	h := &handlers{st: d.Store}
	route := func(pattern string, roles []string, fn func(http.ResponseWriter, *http.Request, store.User) error) {
		mux.Handle(pattern, httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
			actor, err := httpx.RequireRole(r, roles...)
			if err != nil {
				return err
			}
			return fn(w, r, actor)
		}))
	}
	route("GET /api/v1/service-accounts", readers, h.list)
	route("POST /api/v1/service-accounts", writers, h.create)
	route("GET /api/v1/service-accounts/{id}", readers, h.get)
	route("PATCH /api/v1/service-accounts/{id}", writers, h.update)
	route("PUT /api/v1/service-accounts/{id}/roles", writers, h.setRoles)
	route("POST /api/v1/service-accounts/{id}/keys", writers, h.createKey)
	route("GET /api/v1/api-keys", readers, h.listKeys)
	route("DELETE /api/v1/api-keys/{id}", writers, h.revokeKey)
	return nil
}

func record(r *http.Request, st *store.Store, actor store.User, account store.ServiceAccount, action, summary string) error {
	return st.RecordAudit(r.Context(), store.AuditEvent{ActorID: &actor.ID, Action: action, Summary: summary, TargetType: "service_account", TargetID: account.ID, TargetLabel: account.Name, IP: httpx.ClientIP(r)})
}

func (h *handlers) account(r *http.Request) (store.ServiceAccount, error) {
	a, err := h.st.GetServiceAccount(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		return a, httpx.NotFound("This service account")
	}
	return a, err
}

func holds(actor store.User, role string) bool {
	return role == "auditor" || actor.HasRole("global_admin", role)
}

func manageable(actor store.User, account store.ServiceAccount) error {
	for _, role := range account.Roles {
		if !holds(actor, role) {
			return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", account.Name+" holds the "+store.RoleLabels[role]+" role, which you don't have. Ask a global administrator to change it or create keys for it.")
		}
	}
	return nil
}

func grantable(actor store.User, requested []string) ([]string, error) {
	roles := []string{}
	for _, role := range requested {
		label, ok := store.RoleLabels[role]
		if !ok {
			return nil, httpx.Invalid(fmt.Sprintf("%q is not a role. Use global_admin, security_admin, user_admin, helpdesk_admin, app_admin or auditor.", role))
		}
		if !holds(actor, role) {
			return nil, httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "You can only give a service account roles you hold yourself. Ask a global administrator to assign "+label+".")
		}
		if !slices.Contains(roles, role) {
			roles = append(roles, role)
		}
	}
	return roles, nil
}

func labels(roles []string) string {
	names := make([]string, len(roles))
	for i, role := range roles {
		names[i] = store.RoleLabels[role]
	}
	return strings.Join(names, ", ")
}

func (h *handlers) owner(r *http.Request, ownerID *string) (*string, error) {
	if ownerID == nil || *ownerID == "" {
		return nil, nil
	}
	_, err := h.st.GetUser(r.Context(), *ownerID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpx.Invalid("The owner does not exist. Refresh the page and pick someone from the directory.")
	}
	if err != nil {
		return nil, err
	}
	if _, err := h.st.GetServiceAccount(r.Context(), *ownerID); err == nil {
		return nil, httpx.Invalid("Choose a person as the owner. Service accounts cannot own other service accounts.")
	}
	return ownerID, nil
}

func (h *handlers) list(w http.ResponseWriter, r *http.Request, _ store.User) error {
	accounts, err := h.st.ListServiceAccounts(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, accounts)
}

func (h *handlers) get(w http.ResponseWriter, r *http.Request, _ store.User) error {
	account, err := h.account(r)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, account)
}

func (h *handlers) create(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		OwnerID     *string  `json:"ownerId"`
		Roles       []string `json:"roles"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	name, description := strings.TrimSpace(in.Name), strings.TrimSpace(in.Description)
	if name == "" {
		return httpx.Invalid("Enter a name that says what the service account automates, such as Terraform.")
	}
	if len(name) > 200 || len(description) > 2000 {
		return httpx.Invalid("Keep the name under 200 characters and the description under 2,000.")
	}
	roles, err := grantable(actor, in.Roles)
	if err != nil {
		return err
	}
	owner, err := h.owner(r, in.OwnerID)
	if err != nil {
		return err
	}
	var account store.ServiceAccount
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		account, err = tx.CreateServiceAccount(r.Context(), store.NewServiceAccount{Name: name, Description: description, OwnerID: owner, CreatedBy: &actor.ID, Roles: roles})
		if errors.Is(err, store.ErrConflict) {
			return httpx.Fail(http.StatusConflict, "ERR_CONFLICT", "A service account named "+name+" already exists. Pick a different name.")
		}
		if err != nil {
			return err
		}
		summary := "Created the service account without roles"
		if len(roles) > 0 {
			summary = "Created the service account with roles " + labels(roles)
		}
		return record(r, tx, actor, account, "service_account.create", summary)
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, account)
}

func (h *handlers) update(w http.ResponseWriter, r *http.Request, actor store.User) error {
	account, err := h.account(r)
	if err != nil {
		return err
	}
	var in struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		OwnerID     *string `json:"ownerId"`
		Disabled    *bool   `json:"disabled"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	disablingOnly := in.Disabled != nil && *in.Disabled && in.Name == nil && in.Description == nil && in.OwnerID == nil
	if !disablingOnly {
		if err := manageable(actor, account); err != nil {
			return err
		}
	}
	name, description, owner := account.Name, account.Description, account.OwnerID
	var changed []string
	if in.Name != nil && strings.TrimSpace(*in.Name) != name {
		name = strings.TrimSpace(*in.Name)
		changed = append(changed, "name")
	}
	if in.Description != nil && strings.TrimSpace(*in.Description) != description {
		description = strings.TrimSpace(*in.Description)
		changed = append(changed, "description")
	}
	if in.OwnerID != nil && *in.OwnerID != deref(owner) {
		if owner, err = h.owner(r, in.OwnerID); err != nil {
			return err
		}
		changed = append(changed, "owner")
	}
	if name == "" || len(name) > 200 || len(description) > 2000 {
		return httpx.Invalid("Keep the name between 1 and 200 characters and the description under 2,000.")
	}
	status := account.Status
	if in.Disabled != nil && *in.Disabled {
		status = "suspended"
	} else if in.Disabled != nil {
		status = "active"
	}
	if len(changed) == 0 && status == account.Status {
		return httpx.JSON(w, http.StatusOK, account)
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		action, summary := "service_account.update", "Updated "+strings.Join(changed, ", ")
		if len(changed) > 0 {
			if err := tx.UpdateServiceAccount(r.Context(), account.ID, name, description, owner); err != nil {
				return err
			}
		}
		if status != account.Status {
			if err := tx.SetUserStatus(r.Context(), account.ID, status); err != nil {
				return err
			}
			action, summary = "service_account.enable", "Enabled the service account; its API keys work again"
			if status == "suspended" {
				action, summary = "service_account.disable", "Disabled the service account; its API keys stopped working"
			}
			if len(changed) > 0 {
				summary += " and updated " + strings.Join(changed, ", ")
			}
		}
		return record(r, tx, actor, account, action, summary)
	})
	if err != nil {
		return err
	}
	return h.respond(w, r, account.ID)
}

func (h *handlers) setRoles(w http.ResponseWriter, r *http.Request, actor store.User) error {
	account, err := h.account(r)
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
		return httpx.Invalid(`Send the complete list of roles, for example {"roles": ["app_admin"]}. Send an empty list to remove every role.`)
	}
	if err := manageable(actor, account); err != nil {
		return err
	}
	roles, err := grantable(actor, *in.Roles)
	if err != nil {
		return err
	}
	summary := "Removed every role"
	if len(roles) > 0 {
		summary = "Set roles to " + labels(roles)
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.SetUserRoles(r.Context(), account.ID, roles); err != nil {
			return err
		}
		return record(r, tx, actor, account, "service_account.roles.update", summary)
	})
	if err != nil {
		return err
	}
	return h.respond(w, r, account.ID)
}

func (h *handlers) createKey(w http.ResponseWriter, r *http.Request, actor store.User) error {
	account, err := h.account(r)
	if err != nil {
		return err
	}
	var in struct {
		Label     string     `json:"label"`
		Scopes    []string   `json:"scopes"`
		ExpiresAt *time.Time `json:"expiresAt"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if err := manageable(actor, account); err != nil {
		return err
	}
	label := strings.TrimSpace(in.Label)
	if label == "" || len(label) > 100 {
		return httpx.Invalid("Enter a label of up to 100 characters that says where the key is used, such as Production CI.")
	}
	granted := []string{}
	for _, scope := range in.Scopes {
		if !slices.Contains(scopes, scope) {
			return httpx.Invalid(fmt.Sprintf("%q is not a scope. Use api for the management API or scim for SCIM provisioning.", scope))
		}
		if !slices.Contains(granted, scope) {
			granted = append(granted, scope)
		}
	}
	if len(granted) == 0 {
		return httpx.Invalid(`Choose at least one scope: "api" for the management API or "scim" for SCIM provisioning.`)
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(time.Now()) {
		return httpx.Invalid("The expiry date is in the past. Pick a future date, or leave it empty for a key that does not expire.")
	}
	var key store.APIKey
	var value string
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		key, value, err = tx.CreateAPIKey(r.Context(), account.ID, label, granted, in.ExpiresAt, &actor.ID)
		if err != nil {
			return err
		}
		return record(r, tx, actor, account, "api_key.create", "Created API key “"+label+"” ("+key.Prefix+"…) with scope "+strings.Join(granted, " and "))
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, map[string]any{"key": key, "secret": value})
}

func (h *handlers) listKeys(w http.ResponseWriter, r *http.Request, _ store.User) error {
	keys, err := h.st.ListAPIKeys(r.Context(), "")
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, keys)
}

func (h *handlers) revokeKey(w http.ResponseWriter, r *http.Request, actor store.User) error {
	key, err := h.st.GetAPIKey(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("This API key")
	}
	if err != nil {
		return err
	}
	if key.RevokedAt != nil {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.RevokeAPIKey(r.Context(), key.ID); err != nil {
			return err
		}
		account := store.ServiceAccount{ID: key.ServiceAccountID, Name: key.ServiceAccountName}
		return record(r, tx, actor, account, "api_key.revoke", "Revoked API key “"+key.Label+"” ("+key.Prefix+"…)")
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *handlers) respond(w http.ResponseWriter, r *http.Request, accountID string) error {
	account, err := h.st.GetServiceAccount(r.Context(), accountID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, account)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
