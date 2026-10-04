package auth

import (
	"cmp"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"halo/internal/httpx"
	"halo/internal/store"
)

var (
	providerReaders = []string{"security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"}
	errLastLink     = httpx.Fail(http.StatusConflict, "ERR_LAST_METHOD", "This is your last way to sign in, so it can't be unlinked. Add a passkey or authenticator app first.")
	entraIssuer     = regexp.MustCompile(`^https://login\.microsoftonline\.com/[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}/v2\.0$`)
	domainPattern   = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)
	defaultScopes   = map[string][]string{"google": {"openid", "email", "profile"}, "microsoft": {"openid", "email", "profile"}, "github": {"read:user", "user:email"}, "oidc": {"openid", "email", "profile"}}
)

type providerInput struct {
	Kind           string   `json:"kind"`
	Name           string   `json:"name"`
	Issuer         string   `json:"issuer"`
	ClientID       string   `json:"clientId"`
	ClientSecret   string   `json:"clientSecret"`
	Scopes         []string `json:"scopes"`
	Enabled        bool     `json:"enabled"`
	ShowOnSignIn   bool     `json:"showOnSignIn"`
	AllowedDomains []string `json:"allowedDomains"`
	JIT            bool     `json:"jit"`
	JITGroupIDs    []string `json:"jitGroupIds"`
}

func (h *handler) listProviders(w http.ResponseWriter, r *http.Request) error {
	if _, err := httpx.RequireRole(r, providerReaders...); err != nil {
		return err
	}
	providers, err := h.st.ListIdentityProviders(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"callbackUrl": h.cfg.Issuer() + FederationCallbackPath, "providers": providers})
}

func (h *handler) createProvider(w http.ResponseWriter, r *http.Request) error {
	return h.saveProvider(w, r, store.IdentityProvider{})
}

func (h *handler) updateProvider(w http.ResponseWriter, r *http.Request) error {
	existing, err := h.st.GetIdentityProvider(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("This identity provider")
	}
	if err != nil {
		return err
	}
	return h.saveProvider(w, r, existing)
}

func (h *handler) saveProvider(w http.ResponseWriter, r *http.Request, existing store.IdentityProvider) error {
	actor, err := httpx.RequireRole(r, "security_admin")
	if err != nil {
		return err
	}
	var in providerInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if existing.ID != "" {
		in.Kind = existing.Kind
	}
	p, err := h.validProvider(r, in, existing.HasSecret)
	if err != nil {
		return err
	}
	p.ID = existing.ID
	action, summary := "identity_provider.create", "Added identity provider"
	if existing.ID != "" {
		action, summary = "identity_provider.update", "Changed identity provider settings"
	}
	return h.writeProvider(w, r, actor, action, summary, func(tx *store.Store) (store.IdentityProvider, error) {
		return tx.SaveIdentityProvider(r.Context(), p, strings.TrimSpace(in.ClientSecret))
	})
}

func (h *handler) setProviderEnabled(enabled bool) httpx.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		actor, err := httpx.RequireRole(r, "security_admin")
		if err != nil {
			return err
		}
		p, err := h.st.GetIdentityProvider(r.Context(), r.PathValue("id"))
		if errors.Is(err, store.ErrNotFound) {
			return httpx.NotFound("This identity provider")
		}
		if err != nil {
			return err
		}
		p.Enabled = enabled
		action, summary := "identity_provider.disable", "Turned off sign-in with "+p.Name
		if enabled {
			action, summary = "identity_provider.enable", "Turned on sign-in with "+p.Name
		}
		return h.writeProvider(w, r, actor, action, summary, func(tx *store.Store) (store.IdentityProvider, error) {
			return tx.SaveIdentityProvider(r.Context(), p, "")
		})
	}
}

func (h *handler) deleteProvider(w http.ResponseWriter, r *http.Request) error {
	actor, err := httpx.RequireRole(r, "security_admin")
	if err != nil {
		return err
	}
	ctx := r.Context()
	p, err := h.st.GetIdentityProvider(ctx, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("This identity provider")
	}
	if err != nil {
		return err
	}
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.DeleteIdentityProvider(ctx, p.ID); err != nil {
			return err
		}
		return tx.RecordAudit(ctx, store.AuditEvent{ActorID: &actor.ID, Action: "identity_provider.delete", Summary: "Deleted identity provider and unlinked its accounts", TargetType: "identity_provider", TargetID: p.ID, TargetLabel: p.Name, IP: httpx.ClientIP(r)})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

func (h *handler) writeProvider(w http.ResponseWriter, r *http.Request, actor store.User, action, summary string, save func(*store.Store) (store.IdentityProvider, error)) error {
	ctx := r.Context()
	var saved store.IdentityProvider
	err := h.st.Tx(ctx, func(tx *store.Store) error {
		var err error
		if saved, err = save(tx); err != nil {
			return err
		}
		return tx.RecordAudit(ctx, store.AuditEvent{ActorID: &actor.ID, Action: action, Summary: summary, TargetType: "identity_provider", TargetID: saved.ID, TargetLabel: saved.Name, IP: httpx.ClientIP(r)})
	})
	if errors.Is(err, store.ErrConflict) {
		return httpx.Fail(http.StatusConflict, "ERR_CONFLICT", "Another identity provider already uses that name. Pick a different name.")
	}
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("This identity provider")
	}
	if err != nil {
		return err
	}
	if action == "identity_provider.create" {
		return httpx.JSON(w, http.StatusCreated, saved)
	}
	return httpx.JSON(w, http.StatusOK, saved)
}

func (h *handler) validProvider(r *http.Request, in providerInput, hasSecret bool) (store.IdentityProvider, error) {
	p := store.IdentityProvider{
		Kind:         in.Kind,
		Name:         strings.TrimSpace(in.Name),
		Issuer:       strings.TrimSpace(in.Issuer),
		ClientID:     strings.TrimSpace(in.ClientID),
		Enabled:      in.Enabled,
		ShowOnSignIn: in.ShowOnSignIn,
		JIT:          in.JIT,
	}
	defaults, known := defaultScopes[p.Kind]
	switch {
	case !known:
		return p, httpx.Invalid(`Choose a provider kind: "google", "microsoft", "github" or "oidc".`)
	case p.Name == "" || len(p.Name) > 100:
		return p, httpx.Invalid("Enter a name of up to 100 characters. People see it on the sign-in button, as in “Continue with Google”.")
	case p.ClientID == "":
		return p, httpx.Invalid("Enter the client ID from the provider's app registration.")
	case strings.TrimSpace(in.ClientSecret) == "" && !hasSecret && p.Kind != "oidc":
		return p, httpx.Invalid("Enter the client secret from the provider's app registration.")
	}
	switch p.Kind {
	case "google":
		p.Issuer = "https://accounts.google.com"
	case "microsoft":
		if !entraIssuer.MatchString(p.Issuer) {
			return p, httpx.Invalid("Enter the directory (tenant) ID from Microsoft Entra, a value like 72f988bf-86f1-41af-91ab-2d7cd011db47. Domain names and “common” are not supported.")
		}
	case "github":
		p.Issuer = strings.TrimSuffix(cmp.Or(p.Issuer, "https://github.com"), "/")
	}
	if !validIssuer(p.Issuer, h.cfg.Dev) {
		return p, httpx.Invalid("Enter the provider URL starting with https://, for example https://auth.example.com.")
	}
	p.Scopes = []string{}
	for _, s := range in.Scopes {
		if s = strings.TrimSpace(s); s != "" && !slices.Contains(p.Scopes, s) {
			p.Scopes = append(p.Scopes, s)
		}
	}
	if len(p.Scopes) == 0 {
		p.Scopes = defaults
	}
	if p.Kind != "github" && !slices.Contains(p.Scopes, "openid") {
		p.Scopes = append([]string{"openid"}, p.Scopes...)
	}
	p.AllowedDomains = []string{}
	for _, d := range in.AllowedDomains {
		d = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(d), "@"))
		if d == "" || slices.Contains(p.AllowedDomains, d) {
			continue
		}
		if !domainPattern.MatchString(d) {
			return p, httpx.Invalid("“" + d + "” is not a domain. Enter domains like example.com, without the @ or a path.")
		}
		p.AllowedDomains = append(p.AllowedDomains, d)
	}
	if p.JIT && len(p.AllowedDomains) == 0 {
		return p, httpx.Invalid("Add at least one allowed email domain before creating accounts on first sign-in, so only people from your organization get an account.")
	}
	p.JITGroupIDs = []string{}
	if len(in.JITGroupIDs) > 0 {
		groups, err := h.st.ListGroupsRaw(r.Context())
		if err != nil {
			return p, err
		}
		for _, groupID := range in.JITGroupIDs {
			if !slices.ContainsFunc(groups, func(g store.Group) bool { return g.ID == groupID && g.Kind == "assigned" }) {
				return p, httpx.Invalid("New people can only be added to assigned groups that exist. Refresh the page and pick the groups again.")
			}
			if !slices.Contains(p.JITGroupIDs, groupID) {
				p.JITGroupIDs = append(p.JITGroupIDs, groupID)
			}
		}
	}
	return p, nil
}

func validIssuer(issuer string, dev bool) bool {
	u, err := url.Parse(issuer)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"
	return u.Scheme == "https" || u.Scheme == "http" && local && dev
}

func (h *handler) myIdentities(w http.ResponseWriter, r *http.Request) error {
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	identities, err := h.st.ListUserIdentities(r.Context(), u.ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, identities)
}

func (h *handler) unlinkIdentity(w http.ResponseWriter, r *http.Request) error {
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	identities, err := h.st.ListUserIdentities(ctx, u.ID)
	if err != nil {
		return err
	}
	identityID := r.PathValue("id")
	i := slices.IndexFunc(identities, func(f store.FederatedIdentity) bool { return f.ID == identityID })
	if i < 0 {
		return httpx.NotFound("The linked account")
	}
	otherLinks := slices.ContainsFunc(identities, func(f store.FederatedIdentity) bool { return f.ID != identityID && f.ProviderEnabled })
	otherMethods := slices.ContainsFunc(u.Methods, func(m store.Method) bool { return m.Kind != "recovery-codes" })
	if !otherLinks && !otherMethods {
		return errLastLink
	}
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.DeleteUserIdentity(ctx, u.ID, identityID); err != nil {
			return err
		}
		return audit(ctx, tx, r, u, "user.identity.unlink", u.Name+" unlinked "+identities[i].ProviderName+" ("+identities[i].Email+")")
	})
	if err != nil {
		return err
	}
	return noContent(w)
}
