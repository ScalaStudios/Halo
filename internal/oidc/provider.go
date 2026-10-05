package oidc

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
	"golang.org/x/text/language"

	"halo/internal/access"
	"halo/internal/httpx"
	"halo/internal/store"
)

const (
	groupsScope               = "groups"
	devicePollInterval        = 5 * time.Second
	deviceAuthorizationWindow = 10 * time.Minute
	deviceAuthorizationLimit  = 20
)

var supportedClaims = []string{"sub", "aud", "exp", "iat", "iss", "auth_time", "nonce", "amr", "at_hash", "azp", "client_id", "scope", "sid", "email", "email_verified", "name", "preferred_username", "picture", groupsScope}

func New(d httpx.Deps) (http.Handler, error) {
	config := &op.Config{
		CryptoKey:                sha256.Sum256(append([]byte("halo/oidc/token-encryption\x00"), d.Config.SecretKey...)),
		DefaultLogoutRedirectURI: "/",
		CodeMethodS256:           true,
		AuthMethodPost:           true,
		GrantTypeRefreshToken:    true,
		SupportedUILocales:       []language.Tag{language.English},
		SupportedScopes:          []string{oidc.ScopeOpenID, oidc.ScopeProfile, oidc.ScopeEmail, oidc.ScopeOfflineAccess, groupsScope},
		SupportedClaims:          supportedClaims,
		DeviceAuthorization:      op.DeviceAuthorizationConfig{Lifetime: 10 * time.Minute, PollInterval: devicePollInterval, UserFormPath: "/device", UserCode: op.UserCodeBase20},
	}
	options := []op.Option{
		op.WithCustomAuthEndpoint(op.NewEndpoint("oauth2/authorize")),
		op.WithCustomTokenEndpoint(op.NewEndpoint("oauth2/token")),
		op.WithCustomUserinfoEndpoint(op.NewEndpoint("oauth2/userinfo")),
		op.WithCustomKeysEndpoint(op.NewEndpoint("oauth2/keys")),
		op.WithCustomEndSessionEndpoint(op.NewEndpoint("oauth2/logout")),
		op.WithCustomRevocationEndpoint(op.NewEndpoint("oauth2/revoke")),
		op.WithCustomIntrospectionEndpoint(op.NewEndpoint("oauth2/introspect")),
		op.WithCustomDeviceAuthorizationEndpoint(op.NewEndpoint("oauth2/device_authorization")),
	}
	if d.Config.Dev {
		options = append(options, op.WithAllowInsecure())
	}
	policy := d.Policy
	if policy == nil {
		policy = access.AllowAll{}
	}
	if err := ensureCLI(context.Background(), d.Store); err != nil {
		return nil, err
	}
	st := &storage{st: d.Store, dev: d.Config.Dev, policy: policy, issuer: d.Config.Issuer()}
	provider, err := op.NewOpenIDProvider(d.Config.Issuer(), config, st, options...)
	if err != nil {
		return nil, err
	}

	serve := func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		provider.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestKey{}, r)))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", serve)
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		serve(rec, r)
		body := rec.Body.Bytes()
		var c oidc.DiscoveryConfiguration
		if rec.Code == http.StatusOK && json.Unmarshal(body, &c) == nil {
			c.ResponseTypesSupported = []string{string(oidc.ResponseTypeCode)}
			c.GrantTypesSupported = slices.DeleteFunc(c.GrantTypesSupported, func(g oidc.GrantType) bool { return g == oidc.GrantTypeImplicit || g == oidc.GrantTypeBearer })
			c.IntrospectionEndpointAuthSigningAlgValuesSupported, c.RevocationEndpointAuthSigningAlgValuesSupported = nil, nil
			body, _ = json.Marshal(c)
		}
		maps.Copy(w.Header(), rec.Header())
		w.WriteHeader(rec.Code)
		_, _ = w.Write(body)
	})
	mux.HandleFunc("POST /oauth2/device_authorization", func(w http.ResponseWriter, r *http.Request) {
		recent, err := d.Store.CountDeviceAuthorizations(r.Context(), httpx.ClientIP(r), time.Now().Add(-deviceAuthorizationWindow))
		if err == nil && recent >= deviceAuthorizationLimit {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", strconv.Itoa(int(deviceAuthorizationWindow.Seconds())))
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "slow_down", "error_description": "Too many sign-in requests came from this network. Wait 10 minutes, then run halo login again."})
			return
		}
		serve(w, r)
	})
	mux.HandleFunc("GET /oauth2/logout/return", st.logoutReturn)
	mux.HandleFunc("/oauth2/authorize/callback", func(w http.ResponseWriter, r *http.Request) {
		req, err := d.Store.GetAuthRequest(r.Context(), r.FormValue("id"))
		sess, signedIn := httpx.CurrentSession(r)
		if err == nil && req.Done && (!signedIn || req.SessionID == nil || *req.SessionID != sess.ID) {
			http.Error(w, "This sign-in was completed in a different browser session. Start again from the application.", http.StatusForbidden)
			return
		}
		provider.ServeHTTP(w, r)
	})
	return mux, nil
}

func Register(mux *http.ServeMux, d httpx.Deps) error {
	policy := d.Policy
	if policy == nil {
		policy = access.AllowAll{}
	}
	devices := &deviceHandlers{st: d.Store, policy: policy}
	mux.Handle("GET /api/v1/device/{code}", httpx.Handle(devices.show))
	mux.Handle("POST /api/v1/device/{code}/approve", httpx.Handle(devices.decide(true)))
	mux.Handle("POST /api/v1/device/{code}/deny", httpx.Handle(devices.decide(false)))

	readers, writers := []string{"security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"}, []string{"app_admin"}
	route := func(pattern string, roles []string, fn func(http.ResponseWriter, *http.Request, store.User) error) {
		mux.Handle(pattern, httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
			actor, err := httpx.RequireRole(r, roles...)
			if err != nil {
				return err
			}
			return fn(w, r, actor)
		}))
	}
	if d.Jobs != nil {
		d.Jobs.Every("retire superseded signing keys", time.Hour, func(ctx context.Context) error { return d.Store.RetireSigningKeys(ctx, time.Now()) })
	}
	resources := &resourceHandlers{st: d.Store}
	route("GET /api/v1/api-resources", readers, resources.list)
	route("POST /api/v1/api-resources", writers, resources.create)
	route("GET /api/v1/api-resources/{id}", readers, resources.get)
	route("PUT /api/v1/api-resources/{id}", writers, resources.update)
	route("DELETE /api/v1/api-resources/{id}", writers, resources.remove)
	route("PUT /api/v1/api-resources/{id}/grants/{appId}", writers, resources.grant)
	return nil
}
