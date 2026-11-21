package api

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/oidc"
	"halo/internal/secret"
	"halo/internal/store"
)

const (
	registrationBodyLimit = 16 << 10
	registrationLimit     = 20
	registrationWindow    = time.Hour
)

var (
	registrationScopes    = []string{"openid", "profile", "email"}
	errRegistrationOff    = httpx.Fail(http.StatusNotFound, "invalid_request", "Client registration is turned off. Ask an administrator to register the client in the Halo console.")
	errInitialAccessToken = httpx.Fail(http.StatusUnauthorized, "invalid_token", "Send an API key with the registration scope as the initial access token: Authorization: Bearer hlk_…")
	errRegistrationAccess = httpx.Fail(http.StatusUnauthorized, "invalid_token", "The registration access token is not valid for this client.")
)

type clientMetadata struct {
	ClientName              string   `json:"client_name"`
	ClientURI               string   `json:"client_uri,omitempty"`
	RedirectURIs            []string `json:"redirect_uris"`
	PostLogoutRedirectURIs  []string `json:"post_logout_redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	ApplicationType         string   `json:"application_type"`
	Scope                   string   `json:"scope"`
	IDTokenSignedAlg        string   `json:"id_token_signed_response_alg"`

	JWKS                  json.RawMessage `json:"jwks,omitempty"`
	JWKSURI               string          `json:"jwks_uri,omitempty"`
	DPoPBoundAccessTokens bool            `json:"dpop_bound_access_tokens,omitempty"`
	RequirePushedRequests bool            `json:"require_pushed_authorization_requests,omitempty"`
}

type registeredClient struct {
	ClientID                string `json:"client_id"`
	ClientSecret            string `json:"client_secret,omitempty"`
	ClientIDIssuedAt        int64  `json:"client_id_issued_at"`
	ClientSecretExpiresAt   *int64 `json:"client_secret_expires_at,omitempty"`
	RegistrationAccessToken string `json:"registration_access_token,omitempty"`
	RegistrationClientURI   string `json:"registration_client_uri"`
	clientMetadata
}

type registrar struct {
	st  *store.Store
	cfg config.Config
}

func Registration(d httpx.Deps) http.Handler {
	h := &registrar{st: d.Store, cfg: d.Config}
	mux := http.NewServeMux()
	mux.Handle("POST /oauth2/register", oauthHandle(h.register))
	mux.Handle("GET /oauth2/register/{clientId}", oauthHandle(h.read))
	mux.Handle("PUT /oauth2/register/{clientId}", oauthHandle(h.update))
	mux.Handle("DELETE /oauth2/register/{clientId}", oauthHandle(h.remove))
	return mux
}

func oauthHandle(fn httpx.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := fn(w, r)
		if err == nil {
			return
		}
		var e *httpx.Error
		if !errors.As(err, &e) {
			slog.ErrorContext(r.Context(), "client registration failed", "method", r.Method, "path", r.URL.Path, "error", err)
			e = httpx.Fail(http.StatusInternalServerError, "server_error", "Halo could not complete the request. Try again later.")
		}
		if e.Status == http.StatusUnauthorized {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		}
		_ = httpx.JSON(w, e.Status, map[string]string{"error": e.Code, "error_description": e.Message})
	})
}

func metadataError(code string, err error) error {
	var e *httpx.Error
	if errors.As(err, &e) {
		return httpx.Fail(http.StatusBadRequest, code, e.Message)
	}
	return err
}

func bearer(r *http.Request) string {
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return token
}

func decodeMetadata(w http.ResponseWriter, r *http.Request, dst any) error {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, registrationBodyLimit)).Decode(dst)
	if tooLarge := new(http.MaxBytesError); errors.As(err, &tooLarge) {
		return httpx.Fail(http.StatusBadRequest, "invalid_client_metadata", "The client metadata is larger than 16 KB.")
	}
	if err != nil {
		return httpx.Fail(http.StatusBadRequest, "invalid_client_metadata", "Send the client metadata as a JSON object, as RFC 7591 describes.")
	}
	return nil
}

func (m *clientMetadata) check(allowed []string) (string, []string, error) {
	invalid := func(format string, args ...any) error {
		return httpx.Fail(http.StatusBadRequest, "invalid_client_metadata", fmt.Sprintf(format, args...))
	}
	if len(m.GrantTypes) == 0 {
		m.GrantTypes = []string{"authorization_code"}
	}
	for _, grant := range m.GrantTypes {
		if grant != "authorization_code" && grant != "refresh_token" {
			return "", nil, invalid("Registered clients use the authorization code flow, so the grant type %q is not available. Ask an administrator to create clients that need it.", grant)
		}
	}
	if !slices.Contains(m.GrantTypes, "authorization_code") {
		return "", nil, invalid("Include authorization_code in grant_types.")
	}
	for _, responseType := range m.ResponseTypes {
		if responseType != "code" {
			return "", nil, invalid("Registered clients use the code response type, so %q is not available.", responseType)
		}
	}
	if m.ApplicationType != "" && m.ApplicationType != "web" && m.ApplicationType != "native" {
		return "", nil, invalid(`Set application_type to "web" or "native".`)
	}
	appType := "spa"
	switch m.TokenEndpointAuthMethod {
	case "", "client_secret_basic", "client_secret_post":
		if m.ApplicationType == "native" {
			return "", nil, invalid(`Native clients can't keep a secret. Register with token_endpoint_auth_method "none" and use PKCE.`)
		}
		appType = "web"
	case oidc.PrivateKeyJWT:
		if m.ApplicationType == "native" {
			return "", nil, invalid(`Native clients can't keep a private key. Register with token_endpoint_auth_method "none" and use PKCE.`)
		}
		appType = "web"
	case "none":
		if m.ApplicationType == "native" {
			appType = "native"
		}
	default:
		return "", nil, invalid(`Set token_endpoint_auth_method to "client_secret_basic", "client_secret_post", "private_key_jwt" or "none".`)
	}
	if m.TokenEndpointAuthMethod != oidc.PrivateKeyJWT && (len(m.JWKS) > 0 || m.JWKSURI != "") {
		return "", nil, invalid(`Halo only uses jwks and jwks_uri to check private_key_jwt client assertions. Leave them out, or set token_endpoint_auth_method to "private_key_jwt".`)
	}
	m.JWKS = compactJSON(m.JWKS)
	if err := clientKeys(m.TokenEndpointAuthMethod, m.JWKS, m.JWKSURI); err != nil {
		return "", nil, metadataError("invalid_client_metadata", err)
	}
	m.IDTokenSignedAlg = cmp.Or(m.IDTokenSignedAlg, "RS256")
	if !slices.Contains(store.SigningAlgorithms, m.IDTokenSignedAlg) {
		return "", nil, invalid(`Set id_token_signed_response_alg to "RS256", "ES256" or "EdDSA".`)
	}
	if len(m.RedirectURIs) == 0 || len(m.RedirectURIs) > 10 {
		return "", nil, httpx.Fail(http.StatusBadRequest, "invalid_redirect_uri", "Register between 1 and 10 redirect URIs.")
	}
	for _, uri := range m.RedirectURIs {
		if err := checkURI(uri, "redirect URI"); err != nil {
			return "", nil, metadataError("invalid_redirect_uri", err)
		}
	}
	if len(m.PostLogoutRedirectURIs) > 10 {
		return "", nil, invalid("Register at most 10 post-logout redirect URIs.")
	}
	for _, uri := range m.PostLogoutRedirectURIs {
		if err := checkURI(uri, "post-logout URI"); err != nil {
			return "", nil, metadataError("invalid_client_metadata", err)
		}
	}
	m.ClientName = strings.TrimSpace(m.ClientName)
	if m.ClientName == "" {
		u, _ := url.Parse(m.RedirectURIs[0])
		m.ClientName = u.Host
	}
	if utf8.RuneCountInString(m.ClientName) > 200 || strings.ContainsFunc(m.ClientName, unicode.IsControl) {
		return "", nil, invalid("Keep client_name under 200 characters, on one line.")
	}
	if tooLong(200, m.ClientURI) {
		return "", nil, invalid("Keep client_uri under 200 characters.")
	}
	if err := checkHomepage(m.ClientURI); err != nil {
		return "", nil, metadataError("invalid_client_metadata", err)
	}
	scopes := slices.Clone(registrationScopes)
	if requested := strings.Fields(m.Scope); len(requested) > 0 {
		scopes = []string{"openid"}
		for _, scope := range allowed {
			if slices.Contains(requested, scope) && !slices.Contains(scopes, scope) {
				scopes = append(scopes, scope)
			}
		}
	}
	return appType, scopes, nil
}

func (m clientMetadata) registered(app *store.Application) {
	if m.TokenEndpointAuthMethod == oidc.PrivateKeyJWT {
		app.TokenEndpointAuthMethod = oidc.PrivateKeyJWT
	}
	app.JWKS, app.JWKSURI = m.JWKS, m.JWKSURI
	app.RequireDPoP, app.RequirePAR = app.RequireDPoP || m.DPoPBoundAccessTokens, app.RequirePAR || m.RequirePushedRequests
}

func (h *registrar) describe(app store.Application) registeredClient {
	method, kind := "none", "web"
	if app.Type == "web" {
		method = app.TokenEndpointAuthMethod
	}
	if app.Type == "native" {
		kind = "native"
	}
	return registeredClient{
		ClientID:              app.ClientID,
		ClientIDIssuedAt:      app.CreatedAt.Unix(),
		RegistrationClientURI: h.cfg.Issuer() + "/oauth2/register/" + url.PathEscape(app.ClientID),
		clientMetadata: clientMetadata{
			ClientName:              app.Name,
			ClientURI:               app.Homepage,
			RedirectURIs:            app.RedirectURIs,
			PostLogoutRedirectURIs:  app.PostLogoutURIs,
			GrantTypes:              []string{"authorization_code", "refresh_token"},
			ResponseTypes:           []string{"code"},
			TokenEndpointAuthMethod: method,
			ApplicationType:         kind,
			Scope:                   strings.Join(app.Scopes, " "),
			IDTokenSignedAlg:        app.TokenPolicy.IDTokenAlg,
			JWKS:                    app.JWKS,
			JWKSURI:                 app.JWKSURI,
			DPoPBoundAccessTokens:   app.RequireDPoP,
			RequirePushedRequests:   app.RequirePAR,
		},
	}
}

func (h *registrar) register(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	settings, err := h.st.Settings(ctx)
	if err != nil {
		return err
	}
	if !settings.ClientRegistrationOn() {
		return errRegistrationOff
	}
	if n, reset, err := h.st.CountRequest(ctx, httpx.ClientIP(r), "registration", registrationLimit, registrationWindow); err == nil && n > registrationLimit {
		w.Header().Set("Retry-After", strconv.Itoa(int(time.Until(reset).Seconds())+1))
		return httpx.Fail(http.StatusTooManyRequests, "temporarily_unavailable", "Too many clients were registered from this network. Wait an hour, then try again.")
	}
	var actor *string
	if settings.ClientRegistration == "token" {
		account, key, err := h.st.ResolveAPIKey(ctx, bearer(r))
		if errors.Is(err, store.ErrNotFound) || err == nil && !slices.Contains(key.Scopes, "registration") {
			return errInitialAccessToken
		}
		if err != nil {
			return err
		}
		actor = &account.ID
	}
	var in clientMetadata
	if err := decodeMetadata(w, r, &in); err != nil {
		return err
	}
	appType, scopes, err := in.check(registrationScopes)
	if err != nil {
		return err
	}
	token := "hlr_" + secret.Token(32)
	awaiting := settings.ClientRegistration == "approval"
	out := registeredClient{}
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		app, err := tx.CreateApplication(ctx, store.NewApplication{
			Name:             in.ClientName,
			Protocol:         "oidc",
			Type:             appType,
			Homepage:         in.ClientURI,
			RedirectURIs:     in.RedirectURIs,
			PostLogoutURIs:   in.PostLogoutRedirectURIs,
			Scopes:           scopes,
			RegistrationHash: secret.Hash(token),
			AwaitingApproval: awaiting,
			IDTokenAlg:       in.IDTokenSignedAlg,
		})
		if err != nil {
			return err
		}
		if in.TokenEndpointAuthMethod == oidc.PrivateKeyJWT || in.DPoPBoundAccessTokens || in.RequirePushedRequests {
			in.registered(&app)
			if err := tx.UpdateApplication(ctx, app); err != nil {
				return err
			}
		}
		out = h.describe(app)
		out.RegistrationAccessToken = token
		if appType == "web" && app.TokenEndpointAuthMethod != oidc.PrivateKeyJWT {
			credential, value, err := tx.AddClientSecret(ctx, app.ID, "Registration", secretLifetime)
			if err != nil {
				return err
			}
			expires := credential.ExpiresAt.Unix()
			out.ClientSecret, out.ClientSecretExpiresAt = value, &expires
		}
		summary := "Registered an OpenID Connect " + appType + " client through dynamic client registration"
		if awaiting {
			summary += "; sign-in stays off until an administrator approves it"
		}
		return tx.RecordAudit(ctx, store.AuditEvent{ActorID: actor, Action: "application.register", Summary: summary, TargetType: "application", TargetID: app.ID, TargetLabel: app.Name, IP: httpx.ClientIP(r)})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, out)
}

func (h *registrar) registered(r *http.Request) (store.Application, store.Settings, error) {
	ctx := r.Context()
	settings, err := h.st.Settings(ctx)
	if err != nil {
		return store.Application{}, settings, err
	}
	if !settings.ClientRegistrationOn() {
		return store.Application{}, settings, errRegistrationOff
	}
	token := bearer(r)
	app, err := h.st.GetApplicationByClientID(ctx, r.PathValue("clientId"))
	if errors.Is(err, store.ErrNotFound) || err == nil && (token == "" || !secret.Equal(app.RegistrationHash, secret.Hash(token))) {
		return store.Application{}, settings, errRegistrationAccess
	}
	return app, settings, err
}

func (h *registrar) read(w http.ResponseWriter, r *http.Request) error {
	app, _, err := h.registered(r)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, h.describe(app))
}

func (h *registrar) update(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	app, settings, err := h.registered(r)
	if err != nil {
		return err
	}
	var in registeredClient
	if err := decodeMetadata(w, r, &in); err != nil {
		return err
	}
	if in.ClientID != app.ClientID {
		return httpx.Fail(http.StatusBadRequest, "invalid_request", "Send this client's client_id in the body, as RFC 7592 requires.")
	}
	appType, scopes, err := in.check(append(slices.Clone(registrationScopes), app.Scopes...))
	if err != nil {
		return err
	}
	if appType != app.Type || appType == "web" && (in.TokenEndpointAuthMethod == oidc.PrivateKeyJWT) != (app.TokenEndpointAuthMethod == oidc.PrivateKeyJWT) {
		return httpx.Fail(http.StatusBadRequest, "invalid_client_metadata", "A registered client keeps its token_endpoint_auth_method and application_type. Register a new client to change them.")
	}
	next := app
	next.Name, next.Homepage, next.RedirectURIs, next.PostLogoutURIs, next.Scopes = in.ClientName, in.ClientURI, in.RedirectURIs, nonNil(in.PostLogoutRedirectURIs), scopes
	next.TokenPolicy.IDTokenAlg = in.IDTokenSignedAlg
	in.registered(&next)
	if next.FAPI2Profile {
		if _, err := fapi2(app, &next); err != nil {
			return metadataError("invalid_client_metadata", err)
		}
	}
	if next.TokenPolicy.IDTokenAlg == app.TokenPolicy.IDTokenAlg && next.Name == app.Name && next.Homepage == app.Homepage && slices.Equal(next.RedirectURIs, app.RedirectURIs) && slices.Equal(next.PostLogoutURIs, app.PostLogoutURIs) && slices.Equal(next.Scopes, app.Scopes) &&
		bytes.Equal(next.JWKS, app.JWKS) && next.JWKSURI == app.JWKSURI && next.RequireDPoP == app.RequireDPoP && next.RequirePAR == app.RequirePAR {
		return httpx.JSON(w, http.StatusOK, h.describe(app))
	}
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.UpdateApplication(ctx, next); err != nil {
			return err
		}
		summary := "The client updated its registration"
		if settings.ClientRegistration == "approval" || len(app.GroupIDs) > 0 {
			if err := tx.SetApplicationStatus(ctx, app.ID, "disabled", true); err != nil {
				return err
			}
			summary += "; sign-in stays off until an administrator approves it again"
		}
		return tx.RecordAudit(ctx, store.AuditEvent{Action: "application.registration.update", Summary: summary, TargetType: "application", TargetID: app.ID, TargetLabel: next.Name, IP: httpx.ClientIP(r)})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, h.describe(next))
}

func (h *registrar) remove(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	app, _, err := h.registered(r)
	if err != nil {
		return err
	}
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.DeleteApplication(ctx, app.ID); err != nil {
			return err
		}
		return tx.RecordAudit(ctx, store.AuditEvent{Action: "application.registration.delete", Summary: "The client deleted its registration, credentials and group assignments", TargetType: "application", TargetID: app.ID, TargetLabel: app.Name, IP: httpx.ClientIP(r)})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}
