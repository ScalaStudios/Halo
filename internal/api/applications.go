package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"

	"halo/internal/httpx"
	"halo/internal/oidc"
	"halo/internal/saml"
	"halo/internal/store"
)

const (
	secretLifetime = 365 * 24 * time.Hour
	rotationGrace  = 24 * time.Hour
)

var setupGuides = []string{"grafana", "forgejo", "generic-oidc", "kubernetes", "generic-saml", "aws-iam-identity-center", "slack"}

func (h *handlers) application(r *http.Request) (store.Application, error) {
	a, err := h.st.GetApplication(r.Context(), r.PathValue("id"))
	return a, found(err, "This application")
}

func checkURI(raw, kind string) error {
	u, err := url.Parse(raw)
	local := err == nil && u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1")
	if err != nil || u.Host == "" || strings.Contains(raw, "#") || (u.Scheme != "https" && !local) {
		return httpx.Invalid(fmt.Sprintf("%q is not a valid %s. Use an absolute https:// address without a #fragment; plain http:// is only allowed for localhost and 127.0.0.1.", raw, kind))
	}
	return nil
}

func checkHomepage(raw string) error {
	if raw == "" {
		return nil
	}
	if u, err := url.Parse(raw); err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return httpx.Invalid(fmt.Sprintf("%q is not a valid homepage. Use an absolute address such as https://grafana.example.com.", raw))
	}
	return nil
}

func checkRedirects(protocol, appType string, redirects, postLogout []string) error {
	for _, uri := range redirects {
		if err := checkURI(uri, "redirect URI"); err != nil {
			return err
		}
	}
	for _, uri := range postLogout {
		if err := checkURI(uri, "post-logout URI"); err != nil {
			return err
		}
	}
	if protocol == "oidc" && appType != "service" && len(redirects) == 0 {
		return httpx.Invalid("Add at least one redirect URI. Halo only sends people back to addresses registered here, for example https://grafana.example.com/login/generic_oauth.")
	}
	return nil
}

func checkScopes(scopes []string) error {
	for _, scope := range scopes {
		if scope == "" || strings.ContainsAny(scope, " \t\r\n\"\\") {
			return httpx.Invalid(fmt.Sprintf("%q is not a valid scope. Scopes are single words without spaces or quotes, such as openid or billing:read.", scope))
		}
	}
	return nil
}

func (h *handlers) listApplications(w http.ResponseWriter, r *http.Request, _ store.User) error {
	apps, err := h.st.ListApplications(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, nonNil(apps))
}

func (h *handlers) getApplication(w http.ResponseWriter, r *http.Request, _ store.User) error {
	a, err := h.application(r)
	if err != nil {
		return err
	}
	view, err := h.view(r.Context(), a)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, view)
}

type samlView struct {
	EntityID string   `json:"entityId"`
	ACSURLs  []string `json:"acsUrls"`
	store.SAMLSettings
	IdP saml.Connection `json:"idp"`
}

func (h *handlers) view(ctx context.Context, app store.Application) (any, error) {
	if app.Protocol != "saml" {
		return app, nil
	}
	settings, err := h.st.SAMLSettings(ctx, app.ID)
	if err != nil {
		return nil, err
	}
	idp, err := saml.Describe(ctx, h.st, h.cfg)
	if err != nil {
		return nil, err
	}
	app.Claims = store.SAMLClaims(settings)
	return struct {
		store.Application
		SAML samlView `json:"saml"`
	}{app, samlView{app.ClientID, app.RedirectURIs, settings, idp}}, nil
}

type samlInput struct {
	EntityID     *string               `json:"entityId"`
	ACSURLs      *[]string             `json:"acsUrls"`
	NameIDFormat *string               `json:"nameIdFormat"`
	SignResponse *bool                 `json:"signResponse"`
	Attributes   *store.SAMLAttributes `json:"attributes"`
	MetadataXML  *string               `json:"metadataXml"`
}

func (in samlInput) apply(entityID string, acs []string, s store.SAMLSettings) (string, []string, store.SAMLSettings, error) {
	if in.MetadataXML != nil {
		s.MetadataXML = strings.TrimSpace(*in.MetadataXML)
		if s.MetadataXML != "" {
			var err error
			if entityID, acs, err = saml.ParseMetadata(s.MetadataXML); err != nil {
				return "", nil, s, httpx.Invalid(err.Error())
			}
		}
	}
	if in.EntityID != nil {
		entityID = strings.TrimSpace(*in.EntityID)
	}
	if in.ACSURLs != nil {
		acs = *in.ACSURLs
	}
	if in.NameIDFormat != nil {
		s.NameIDFormat = *in.NameIDFormat
	}
	if in.SignResponse != nil {
		s.SignResponse = *in.SignResponse
	}
	if in.Attributes != nil {
		s.Attributes = *in.Attributes
	}
	unprintable := func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }
	if entityID == "" || len(entityID) > 1024 || strings.ContainsFunc(entityID, unprintable) {
		return "", nil, s, httpx.Invalid("Enter the service provider's entity ID without spaces, for example https://slack.com. The application shows it in its SAML settings or metadata.")
	}
	if len(acs) == 0 || len(acs) > 10 {
		return "", nil, s, httpx.Invalid("Add between 1 and 10 ACS URLs. Halo only sends SAML responses to these addresses, for example https://example.slack.com/sso/saml.")
	}
	for i, uri := range acs {
		acs[i] = strings.TrimSpace(uri)
		if err := checkURI(acs[i], "ACS URL"); err != nil {
			return "", nil, s, err
		}
	}
	if !slices.Contains([]string{"email", "persistent", "unspecified"}, s.NameIDFormat) {
		return "", nil, s, httpx.Invalid(`Choose a NameID format: "email" sends the email address, "persistent" sends the Halo user ID, "unspecified" sends the email address without declaring a format.`)
	}
	seen := map[string]bool{}
	for _, name := range []*string{&s.Attributes.Email, &s.Attributes.Name, &s.Attributes.GivenName, &s.Attributes.FamilyName, &s.Attributes.Groups} {
		*name = strings.TrimSpace(*name)
		if len(*name) > 256 || strings.ContainsFunc(*name, unicode.IsControl) {
			return "", nil, s, httpx.Invalid(fmt.Sprintf("%q is not a usable attribute name. Keep attribute names under 256 characters, on one line.", *name))
		}
		if *name != "" && seen[*name] {
			return "", nil, s, httpx.Invalid(fmt.Sprintf("Two attributes are both named %q. Give each attribute its own name, or clear one to stop sending it.", *name))
		}
		seen[*name] = true
	}
	return entityID, acs, s, nil
}

func entityConflict(entityID string) error {
	return conflict("Another application already uses the entity ID " + entityID + ". Each service provider needs its own entity ID.")
}

func (h *handlers) setApplicationSAML(w http.ResponseWriter, r *http.Request, actor store.User) error {
	app, err := h.application(r)
	if err != nil {
		return err
	}
	if app.Protocol != "saml" {
		return httpx.Invalid(app.Name + " doesn't sign in with SAML, so it has no SAML settings.")
	}
	var in samlInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	current, err := h.st.SAMLSettings(r.Context(), app.ID)
	if err != nil {
		return err
	}
	entityID, acs, settings, err := in.apply(app.ClientID, slices.Clone(app.RedirectURIs), current)
	if err != nil {
		return err
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.SetSAMLSettings(r.Context(), app.ID, entityID, acs, settings); errors.Is(err, store.ErrConflict) {
			return entityConflict(entityID)
		} else if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "application.saml.update", Summary: "Updated SAML settings", TargetType: "application", TargetID: app.ID, TargetLabel: app.Name})
	})
	if err != nil {
		return err
	}
	return h.respondApplication(w, r, app.ID)
}

func (h *handlers) createApplication(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in struct {
		Name           string     `json:"name"`
		Description    string     `json:"description"`
		Protocol       string     `json:"protocol"`
		Type           string     `json:"type"`
		Homepage       string     `json:"homepage"`
		RedirectURIs   []string   `json:"redirectUris"`
		PostLogoutURIs []string   `json:"postLogoutUris"`
		Scopes         []string   `json:"scopes"`
		GroupIDs       []string   `json:"groupIds"`
		SetupGuide     string     `json:"setupGuide"`
		SAML           *samlInput `json:"saml"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return httpx.Invalid("Enter an application name.")
	}
	if tooLong(200, name, in.Homepage) || tooLong(2000, in.Description) {
		return httpx.Invalid("Keep the name and homepage under 200 characters and the description under 2,000.")
	}
	switch in.Protocol {
	case "oidc", "oauth":
	case "saml":
		in.Type = "web"
	default:
		return httpx.Invalid(`Choose a protocol: "oidc" for OpenID Connect sign-in, "oauth" for API access or "saml" for SAML 2.0 sign-in.`)
	}
	if !slices.Contains([]string{"web", "spa", "native", "service"}, in.Type) {
		return httpx.Invalid(`Choose an application type: "web", "spa", "native" or "service".`)
	}
	if err := checkHomepage(in.Homepage); err != nil {
		return err
	}
	if err := checkRedirects(in.Protocol, in.Type, in.RedirectURIs, in.PostLogoutURIs); err != nil {
		return err
	}
	if err := checkScopes(in.Scopes); err != nil {
		return err
	}
	if in.SetupGuide != "" && !slices.Contains(setupGuides, in.SetupGuide) {
		return httpx.Invalid("Choose a setup guide: " + strings.Join(setupGuides, ", ") + ".")
	}
	var entityID string
	var settings store.SAMLSettings
	if in.Protocol == "saml" {
		if len(in.RedirectURIs) > 0 || len(in.PostLogoutURIs) > 0 {
			return httpx.Invalid("SAML applications take their ACS URLs in saml.acsUrls, not in redirectUris or postLogoutUris.")
		}
		if in.SAML == nil {
			return httpx.Invalid(`Send the service provider's settings, for example {"saml": {"entityId": "https://slack.com", "acsUrls": ["https://example.slack.com/sso/saml"]}}.`)
		}
		var err error
		if entityID, in.RedirectURIs, settings, err = in.SAML.apply("", nil, store.DefaultSAMLSettings()); err != nil {
			return err
		}
		if in.SetupGuide == "" {
			in.SetupGuide = "generic-saml"
		}
	}
	groups, err := h.groupsByID(r.Context(), in.GroupIDs)
	if err != nil {
		return err
	}

	var app store.Application
	var secret *string
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		created, err := tx.CreateApplication(r.Context(), store.NewApplication{
			Name:           name,
			Description:    strings.TrimSpace(in.Description),
			Protocol:       in.Protocol,
			Type:           in.Type,
			Homepage:       in.Homepage,
			RedirectURIs:   in.RedirectURIs,
			PostLogoutURIs: in.PostLogoutURIs,
			Scopes:         in.Scopes,
			GroupIDs:       groupIDs(groups),
			SetupGuide:     in.SetupGuide,
			OwnerID:        &actor.ID,
			ClientID:       entityID,
		})
		if errors.Is(err, store.ErrConflict) && in.Protocol == "saml" {
			return entityConflict(entityID)
		}
		if err != nil {
			return err
		}
		if in.Protocol == "saml" {
			if err := tx.SetSAMLSettings(r.Context(), created.ID, entityID, in.RedirectURIs, settings); err != nil {
				return err
			}
		} else if in.Type == "web" || in.Type == "service" {
			_, value, err := tx.AddClientSecret(r.Context(), created.ID, "Initial", secretLifetime)
			if err != nil {
				return err
			}
			secret = &value
		}
		protocol := map[string]string{"oidc": "OpenID Connect", "oauth": "OAuth", "saml": "SAML"}[in.Protocol]
		if err := record(r, tx, actor, store.AuditEvent{Action: "application.create", Summary: "Registered " + protocol + " " + in.Type + " application", TargetType: "application", TargetID: created.ID, TargetLabel: created.Name}); err != nil {
			return err
		}
		app, err = tx.GetApplication(r.Context(), created.ID)
		return err
	})
	if err != nil {
		return err
	}
	view, err := h.view(r.Context(), app)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, map[string]any{"application": view, "clientSecret": secret})
}

func (h *handlers) rotateSecret(w http.ResponseWriter, r *http.Request, actor store.User) error {
	app, err := h.application(r)
	if err != nil {
		return err
	}
	var in struct {
		Label string `json:"label"`
	}
	if err := httpx.Decode(r, &in); err != nil && r.ContentLength != 0 {
		return err
	}
	if app.Protocol == "saml" || (app.Type != "web" && app.Type != "service") {
		return httpx.Invalid(app.Name + " is a public client that signs in without a client secret, so there is nothing to rotate.")
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = "Created " + time.Now().UTC().Format("2 January 2006")
	}
	if tooLong(100, label) {
		return httpx.Invalid("Keep the secret label under 100 characters.")
	}
	var credential store.Credential
	var value string
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		graceEnds := time.Now().Add(rotationGrace)
		for _, c := range app.Credentials {
			if c.Kind == "secret" && c.ExpiresAt.After(time.Now()) {
				if err := tx.ExpireCredential(r.Context(), app.ID, c.ID, graceEnds); err != nil {
					return err
				}
			}
		}
		var err error
		credential, value, err = tx.AddClientSecret(r.Context(), app.ID, label, secretLifetime)
		if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "application.secret.rotate", Summary: "Created client secret “" + label + "”; older secrets stop working in 24 hours", TargetType: "application", TargetID: app.ID, TargetLabel: app.Name})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, map[string]any{"credential": credential, "clientSecret": value})
}

func (h *handlers) revokeSecret(w http.ResponseWriter, r *http.Request, actor store.User) error {
	app, err := h.application(r)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(app.Credentials, func(c store.Credential) bool { return c.ID == r.PathValue("credentialId") })
	if i < 0 {
		return httpx.NotFound("This credential")
	}
	c := app.Credentials[i]
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.RevokeCredential(r.Context(), app.ID, c.ID); err != nil {
			return found(err, "This credential")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "application.secret.revoke", Summary: "Revoked " + c.Kind + " “" + c.Label + "”", TargetType: "application", TargetID: app.ID, TargetLabel: app.Name})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

func (h *handlers) setApplicationStatus(status string) handler {
	action, summary := "application.enable", "Enabled sign-in"
	if status == "disabled" {
		action, summary = "application.disable", "Disabled sign-in"
	}
	return func(w http.ResponseWriter, r *http.Request, actor store.User) error {
		app, err := h.application(r)
		if err != nil {
			return err
		}
		err = h.st.Tx(r.Context(), func(tx *store.Store) error {
			if err := tx.SetApplicationStatus(r.Context(), app.ID, status); err != nil {
				return err
			}
			return record(r, tx, actor, store.AuditEvent{Action: action, Summary: summary, TargetType: "application", TargetID: app.ID, TargetLabel: app.Name})
		})
		if err != nil {
			return err
		}
		return h.respondApplication(w, r, app.ID)
	}
}

func (h *handlers) deleteApplication(w http.ResponseWriter, r *http.Request, actor store.User) error {
	app, err := h.application(r)
	if err != nil {
		return err
	}
	if app.ClientID == oidc.CLIClientID {
		return httpx.Fail(http.StatusConflict, "ERR_BUILT_IN", "The Halo CLI is built in, so it can't be deleted. Disable it to stop people signing in from the command line.")
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.DeleteApplication(r.Context(), app.ID); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "application.delete", Summary: "Deleted the application and its credentials", TargetType: "application", TargetID: app.ID, TargetLabel: app.Name})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

func (h *handlers) setApplicationGroups(w http.ResponseWriter, r *http.Request, actor store.User) error {
	app, err := h.application(r)
	if err != nil {
		return err
	}
	var in struct {
		GroupIDs *[]string `json:"groupIds"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if in.GroupIDs == nil {
		return httpx.Invalid(`Send the complete list of groups, for example {"groupIds": ["grp_…"]}. Send an empty list to remove every assignment.`)
	}
	groups, err := h.groupsByID(r.Context(), *in.GroupIDs)
	if err != nil {
		return err
	}
	summary := "Removed every group assignment"
	if len(groups) > 0 {
		names := make([]string, len(groups))
		for i, g := range groups {
			names[i] = g.Name
		}
		summary = "Assigned to " + strings.Join(names, ", ")
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.SetApplicationGroups(r.Context(), app.ID, groupIDs(groups)); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "application.groups.update", Summary: summary, TargetType: "application", TargetID: app.ID, TargetLabel: app.Name})
	})
	if err != nil {
		return err
	}
	return h.respondApplication(w, r, app.ID)
}

func (h *handlers) respondApplication(w http.ResponseWriter, r *http.Request, appID string) error {
	app, err := h.st.GetApplication(r.Context(), appID)
	if err != nil {
		return err
	}
	view, err := h.view(r.Context(), app)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, view)
}

func (h *handlers) updateApplication(w http.ResponseWriter, r *http.Request, actor store.User) error {
	app, err := h.application(r)
	if err != nil {
		return err
	}
	var in struct {
		Name           *string   `json:"name"`
		Description    *string   `json:"description"`
		Homepage       *string   `json:"homepage"`
		RedirectURIs   *[]string `json:"redirectUris"`
		PostLogoutURIs *[]string `json:"postLogoutUris"`
		Scopes         *[]string `json:"scopes"`
		TokenPolicy    *struct {
			AccessTokenTTL  *int  `json:"accessTokenTtl"`
			RefreshTokenTTL *int  `json:"refreshTokenTtl"`
			IDTokenTTL      *int  `json:"idTokenTtl"`
			Rotation        *bool `json:"rotation"`
		} `json:"tokenPolicy"`
		Owner *string `json:"owner"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if app.Protocol == "saml" && (in.RedirectURIs != nil || in.PostLogoutURIs != nil || in.Scopes != nil || in.TokenPolicy != nil) {
		return httpx.Invalid(app.Name + " signs in with SAML, so it has no redirect URIs, scopes or token lifetimes. Change its ACS URLs in the SAML settings.")
	}
	next, changed := app, []string{}
	for _, f := range []struct {
		label string
		value *string
		field *string
	}{{"name", in.Name, &next.Name}, {"description", in.Description, &next.Description}, {"homepage", in.Homepage, &next.Homepage}} {
		if f.value != nil && strings.TrimSpace(*f.value) != *f.field {
			*f.field = strings.TrimSpace(*f.value)
			changed = append(changed, f.label)
		}
	}
	for _, f := range []struct {
		label string
		value *[]string
		field *[]string
	}{{"redirect URIs", in.RedirectURIs, &next.RedirectURIs}, {"post-logout URIs", in.PostLogoutURIs, &next.PostLogoutURIs}, {"scopes", in.Scopes, &next.Scopes}} {
		if f.value == nil {
			continue
		}
		clean := []string{}
		for _, v := range *f.value {
			if v = strings.TrimSpace(v); !slices.Contains(clean, v) {
				clean = append(clean, v)
			}
		}
		if !slices.Equal(clean, *f.field) {
			*f.field = clean
			changed = append(changed, f.label)
		}
	}
	if p := in.TokenPolicy; p != nil {
		before := app.TokenPolicy
		for _, f := range []struct {
			value *int
			field *int
		}{{p.AccessTokenTTL, &next.TokenPolicy.AccessTokenTTL}, {p.RefreshTokenTTL, &next.TokenPolicy.RefreshTokenTTL}, {p.IDTokenTTL, &next.TokenPolicy.IDTokenTTL}} {
			if f.value != nil {
				*f.field = *f.value
			}
		}
		if p.Rotation != nil {
			next.TokenPolicy.Rotation = *p.Rotation
		}
		if next.TokenPolicy.Rotation != before.Rotation {
			changed = append(changed, "refresh token rotation")
		}
		before.Rotation = next.TokenPolicy.Rotation
		if next.TokenPolicy != before {
			changed = append(changed, "token lifetimes")
		}
	}
	current := ""
	if app.Owner != nil {
		current = *app.Owner
	}
	if in.Owner != nil && strings.TrimSpace(*in.Owner) != current {
		next.Owner = nil
		if ownerID := strings.TrimSpace(*in.Owner); ownerID != "" {
			owner, err := h.st.GetUser(r.Context(), ownerID)
			if errors.Is(err, store.ErrNotFound) || (err == nil && owner.Kind != "person") {
				return httpx.Invalid("That owner is not a person in the directory. Refresh the page and pick someone from the list.")
			}
			if err != nil {
				return err
			}
			next.Owner = &owner.ID
		}
		changed = append(changed, "owner")
	}
	if len(changed) == 0 {
		return h.respondApplication(w, r, app.ID)
	}
	if next.Name == "" {
		return httpx.Invalid("Enter an application name.")
	}
	if tooLong(200, next.Name, next.Homepage) || tooLong(2000, next.Description) {
		return httpx.Invalid("Keep the name and homepage under 200 characters and the description under 2,000.")
	}
	if err := checkHomepage(next.Homepage); err != nil {
		return err
	}
	if slices.Contains(changed, "redirect URIs") || slices.Contains(changed, "post-logout URIs") {
		if err := checkRedirects(app.Protocol, app.Type, next.RedirectURIs, next.PostLogoutURIs); err != nil {
			return err
		}
	}
	if slices.Contains(changed, "scopes") {
		if err := checkScopes(next.Scopes); err != nil {
			return err
		}
		if app.Protocol == "oidc" && !slices.Contains(next.Scopes, "openid") {
			return httpx.Invalid("Keep the openid scope. OpenID Connect sign-in fails without it.")
		}
	}
	if t := next.TokenPolicy; slices.Contains(changed, "token lifetimes") {
		switch {
		case t.AccessTokenTTL < 60 || t.AccessTokenTTL > 86400:
			return httpx.Invalid("Access tokens must last between 1 minute and 24 hours (60 to 86400 seconds).")
		case t.IDTokenTTL < 0 || t.IDTokenTTL > 86400 || (app.Type != "service" && t.IDTokenTTL < 60):
			return httpx.Invalid("ID tokens must last between 1 minute and 24 hours (60 to 86400 seconds).")
		case t.RefreshTokenTTL < 0 || t.RefreshTokenTTL > 90*86400:
			return httpx.Invalid("Refresh tokens can last up to 90 days (7776000 seconds). Use 0 to stop issuing them.")
		}
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.UpdateApplication(r.Context(), next); err != nil {
			return found(err, "This application")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "application.update", Summary: "Updated " + strings.Join(changed, ", "), TargetType: "application", TargetID: app.ID, TargetLabel: next.Name})
	})
	if err != nil {
		return err
	}
	return h.respondApplication(w, r, app.ID)
}
