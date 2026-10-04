package oidc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/zitadel/oidc/v3/pkg/oidc"

	"halo/internal/httpx"
	"halo/internal/store"
)

var (
	scopePattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9:._/-]{0,99}$`)
	reservedScopes = []string{oidc.ScopeOpenID, oidc.ScopeProfile, oidc.ScopeEmail, oidc.ScopePhone, oidc.ScopeAddress, oidc.ScopeOfflineAccess, groupsScope}
)

func audience(granted []store.GrantedResource, clientID string, scopes []string) ([]string, int) {
	aud, ttl := []string{}, 0
	for _, g := range granted {
		if slices.ContainsFunc(g.Scopes, func(scope string) bool { return slices.Contains(scopes, scope) }) {
			aud = append(aud, g.Identifier)
			if ttl == 0 || g.AccessTokenTTL < ttl {
				ttl = g.AccessTokenTTL
			}
		}
	}
	if len(aud) == 0 {
		aud = []string{clientID}
	}
	return aud, ttl
}

func isGranted(granted []store.GrantedResource, scope string) bool {
	return slices.ContainsFunc(granted, func(g store.GrantedResource) bool { return slices.Contains(g.Scopes, scope) })
}

func withResources(ctx context.Context, granted []store.GrantedResource, scopes []string) ([]string, error) {
	r, _ := ctx.Value(requestKey{}).(*http.Request)
	if r == nil {
		return scopes, nil
	}
	for _, target := range r.Form["resource"] {
		i := slices.IndexFunc(granted, func(g store.GrantedResource) bool { return g.Identifier == target })
		if i < 0 {
			return nil, oidc.ErrInvalidTarget().WithDescription("This application has no access to the API %q. Ask an administrator to grant it on the API resources page.", target)
		}
		for _, scope := range granted[i].Scopes {
			if !slices.Contains(scopes, scope) {
				scopes = append(scopes, scope)
			}
		}
	}
	return scopes, nil
}

type resourceHandlers struct {
	st *store.Store
}

func (h *resourceHandlers) resource(r *http.Request) (store.APIResource, error) {
	res, err := h.st.GetAPIResource(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		return res, httpx.NotFound("This API")
	}
	return res, err
}

func (h *resourceHandlers) list(w http.ResponseWriter, r *http.Request, _ store.User) error {
	resources, err := h.st.ListAPIResources(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, resources)
}

func (h *resourceHandlers) get(w http.ResponseWriter, r *http.Request, _ store.User) error {
	res, err := h.resource(r)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, res)
}

func (h *resourceHandlers) read(r *http.Request, current string) (store.APIResource, error) {
	var in struct {
		Name           string           `json:"name"`
		Identifier     string           `json:"identifier"`
		Description    string           `json:"description"`
		AccessTokenTTL int              `json:"accessTokenTtl"`
		Scopes         []store.APIScope `json:"scopes"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return store.APIResource{}, err
	}
	res := store.APIResource{ID: current, Name: strings.TrimSpace(in.Name), Identifier: strings.TrimSpace(in.Identifier), Description: strings.TrimSpace(in.Description), AccessTokenTTL: in.AccessTokenTTL}
	u, err := url.Parse(res.Identifier)
	switch {
	case res.Name == "" || len(res.Name) > 200:
		return res, httpx.Invalid("Enter a name for the API of up to 200 characters, for example “Billing API”.")
	case err != nil || u.Scheme == "" || strings.Contains(res.Identifier, "#") || len(res.Identifier) > 300:
		return res, httpx.Invalid("Enter the identifier as an absolute URI without a #fragment, for example https://billing.example.com. Access tokens carry it in the aud claim.")
	case len(res.Description) > 500:
		return res, httpx.Invalid("Keep the description under 500 characters.")
	case res.AccessTokenTTL < 60 || res.AccessTokenTTL > 86400:
		return res, httpx.Invalid("Set the access token lifetime between 60 seconds and 24 hours.")
	case len(in.Scopes) == 0 || len(in.Scopes) > 100:
		return res, httpx.Invalid("Add between 1 and 100 scopes, such as billing:read.")
	}
	for _, scope := range in.Scopes {
		scope.Name, scope.Description = strings.TrimSpace(scope.Name), strings.TrimSpace(scope.Description)
		switch {
		case !scopePattern.MatchString(scope.Name):
			return res, httpx.Invalid(fmt.Sprintf("“%s” is not a valid scope. Use lowercase letters, digits and : . _ / - such as billing:read.", scope.Name))
		case slices.Contains(reservedScopes, scope.Name):
			return res, httpx.Invalid(fmt.Sprintf("“%s” is an OpenID Connect scope Halo handles itself. Pick a name specific to this API, such as billing:read.", scope.Name))
		case len(scope.Description) > 200:
			return res, httpx.Invalid(fmt.Sprintf("Keep the description of %s under 200 characters.", scope.Name))
		case slices.ContainsFunc(res.Scopes, func(s store.APIScope) bool { return s.Name == scope.Name }):
			return res, httpx.Invalid(fmt.Sprintf("%s is listed twice. Remove one of them.", scope.Name))
		}
		res.Scopes = append(res.Scopes, scope)
	}
	others, err := h.st.ListAPIResources(r.Context())
	if err != nil {
		return res, err
	}
	for _, other := range others {
		if other.ID == current {
			continue
		}
		if other.Identifier == res.Identifier {
			return res, httpx.Fail(http.StatusConflict, "ERR_CONFLICT", fmt.Sprintf("%s already uses the identifier %s. Each API needs its own.", other.Name, res.Identifier))
		}
		for _, scope := range other.Scopes {
			if slices.ContainsFunc(res.Scopes, func(s store.APIScope) bool { return s.Name == scope.Name }) {
				return res, httpx.Fail(http.StatusConflict, "ERR_CONFLICT", fmt.Sprintf("The scope %s already belongs to %s. Scope names are unique across APIs, so prefix them with the API, such as billing:read.", scope.Name, other.Name))
			}
		}
	}
	return res, nil
}

func (h *resourceHandlers) save(w http.ResponseWriter, r *http.Request, actor store.User, res store.APIResource, action, summary string, status int) error {
	var saved store.APIResource
	err := h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		if saved, err = tx.SaveAPIResource(r.Context(), res); errors.Is(err, store.ErrConflict) {
			return httpx.Fail(http.StatusConflict, "ERR_CONFLICT", "An API named "+res.Name+" already exists. Pick a different name.")
		} else if err != nil {
			return err
		}
		return audit(r, tx, actor, store.AuditEvent{Action: action, Summary: summary, TargetType: "api_resource", TargetID: saved.ID, TargetLabel: saved.Name})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, status, saved)
}

func (h *resourceHandlers) create(w http.ResponseWriter, r *http.Request, actor store.User) error {
	res, err := h.read(r, "")
	if err != nil {
		return err
	}
	return h.save(w, r, actor, res, "api_resource.create", fmt.Sprintf("Added the API %s with %s", res.Identifier, plural(len(res.Scopes), "scope", "scopes")), http.StatusCreated)
}

func (h *resourceHandlers) update(w http.ResponseWriter, r *http.Request, actor store.User) error {
	old, err := h.resource(r)
	if err != nil {
		return err
	}
	res, err := h.read(r, old.ID)
	if err != nil {
		return err
	}
	return h.save(w, r, actor, res, "api_resource.update", fmt.Sprintf("Updated the API %s, now %s", res.Identifier, plural(len(res.Scopes), "scope", "scopes")), http.StatusOK)
}

func (h *resourceHandlers) remove(w http.ResponseWriter, r *http.Request, actor store.User) error {
	res, err := h.resource(r)
	if err != nil {
		return err
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.DeleteAPIResource(r.Context(), res.ID); err != nil {
			return err
		}
		return audit(r, tx, actor, store.AuditEvent{Action: "api_resource.delete", Summary: fmt.Sprintf("Deleted the API %s and its grants to %s", res.Identifier, plural(len(res.Grants), "application", "applications")), TargetType: "api_resource", TargetID: res.ID, TargetLabel: res.Name})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *resourceHandlers) grant(w http.ResponseWriter, r *http.Request, actor store.User) error {
	res, err := h.resource(r)
	if err != nil {
		return err
	}
	app, err := h.st.GetApplication(r.Context(), r.PathValue("appId"))
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("This application")
	}
	if err != nil {
		return err
	}
	if app.Protocol == "saml" {
		return httpx.Invalid(app.Name + " signs in with SAML, which has no access tokens. Grant API scopes to OpenID Connect or OAuth applications.")
	}
	var in struct {
		Scopes *[]string `json:"scopes"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if in.Scopes == nil {
		return httpx.Invalid(`Send the complete list of granted scopes, for example {"scopes": ["billing:read"]}. Send an empty list to remove the grant.`)
	}
	scopes := []string{}
	for _, scope := range *in.Scopes {
		if !slices.ContainsFunc(res.Scopes, func(s store.APIScope) bool { return s.Name == scope }) {
			return httpx.Invalid(fmt.Sprintf("%s has no scope named %q. Add it to the API first.", res.Name, scope))
		}
		if !slices.Contains(scopes, scope) {
			scopes = append(scopes, scope)
		}
	}
	summary := fmt.Sprintf("Granted %s %s on %s", app.Name, strings.Join(scopes, ", "), res.Name)
	if len(scopes) == 0 {
		summary = fmt.Sprintf("Removed every %s scope from %s", res.Name, app.Name)
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.SetAPIResourceGrant(r.Context(), res.ID, app.ID, scopes); err != nil {
			return err
		}
		return audit(r, tx, actor, store.AuditEvent{Action: "api_resource.grant", Summary: summary, TargetType: "application", TargetID: app.ID, TargetLabel: app.Name})
	})
	if err != nil {
		return err
	}
	res, err = h.st.GetAPIResource(r.Context(), res.ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, res)
}

func audit(r *http.Request, tx *store.Store, actor store.User, e store.AuditEvent) error {
	e.ActorID, e.IP = &actor.ID, httpx.ClientIP(r)
	return tx.RecordAudit(r.Context(), e)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
