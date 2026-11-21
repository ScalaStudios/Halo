package oidc

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/rs/cors"
	httphelper "github.com/zitadel/oidc/v3/pkg/http"
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

var ReservedClaims = []string{"sub", "iss", "aud", "exp", "iat", "nbf", "jti", "nonce", "azp", "acr", "amr", "sid", "auth_time", "at_hash", "c_hash", "act", "client_id", "scope", "cnf"}

var supportedClaims = []string{"sub", "aud", "exp", "iat", "iss", "auth_time", "nonce", "acr", "amr", "at_hash", "azp", "client_id", "scope", "sid", "email", "email_verified", "name", "preferred_username", "picture", groupsScope, "roles"}

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
		op.WithIDTokenHintVerifierOpts(op.WithSupportedIDTokenHintSigningAlgorithms(store.SigningAlgorithms...)),
		op.WithAccessTokenVerifierOpts(op.WithSupportedAccessTokenSigningAlgorithms(store.SigningAlgorithms...)),
		func(o *op.Provider) error {
			o.CORSOptions().AllowedHeaders = append(o.CORSOptions().AllowedHeaders, oidc.DPoPHeader)
			return nil
		},
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
		if r.Form.Get("grant_type") == string(oidc.GrantTypeTokenExchange) {
			r.Form["audience"] = slices.Compact(slices.Sorted(slices.Values(append(r.Form["audience"], r.Form["resource"]...))))
		}
		provider.ServeHTTP(w, r.WithContext(withAlgorithms(context.WithValue(r.Context(), requestKey{}, r))))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", serve)
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		rewrite(w, r, serve, func(body []byte) []byte {
			var c oidc.DiscoveryConfiguration
			if json.Unmarshal(body, &c) != nil {
				return body
			}
			c.ResponseTypesSupported = []string{string(oidc.ResponseTypeCode)}
			c.GrantTypesSupported = slices.DeleteFunc(c.GrantTypesSupported, func(g oidc.GrantType) bool { return g == oidc.GrantTypeImplicit || g == oidc.GrantTypeBearer })
			c.BackChannelLogoutSupported, c.BackChannelLogoutSessionSupported = true, true
			if contexts, err := d.Store.ListAuthContexts(r.Context()); err == nil {
				for _, ac := range contexts {
					c.ACRValuesSupported = append(c.ACRValuesSupported, ac.ID)
				}
			}
			if settings, err := d.Store.Settings(r.Context()); err == nil && settings.ClientRegistrationOn() {
				c.RegistrationEndpoint = d.Config.Issuer() + "/oauth2/register"
			}
			methods := []oidc.AuthMethod{oidc.AuthMethodNone, oidc.AuthMethodBasic, oidc.AuthMethodPost, oidc.AuthMethodPrivateKeyJWT}
			c.TokenEndpointAuthMethodsSupported, c.RevocationEndpointAuthMethodsSupported = methods, methods
			c.IntrospectionEndpointAuthMethodsSupported = []oidc.AuthMethod{oidc.AuthMethodBasic, oidc.AuthMethodPrivateKeyJWT}
			algs := []string{}
			for _, alg := range assertionAlgorithms {
				algs = append(algs, string(alg))
			}
			c.TokenEndpointAuthSigningAlgValuesSupported, c.IntrospectionEndpointAuthSigningAlgValuesSupported, c.RevocationEndpointAuthSigningAlgValuesSupported = algs, algs, algs
			body, _ = json.Marshal(struct {
				oidc.DiscoveryConfiguration
				PushedAuthorizationRequestEndpoint string                    `json:"pushed_authorization_request_endpoint"`
				DPoPSigningAlgValuesSupported      []jose.SignatureAlgorithm `json:"dpop_signing_alg_values_supported"`
				AuthorizationResponseIssSupported  bool                      `json:"authorization_response_iss_parameter_supported"`
			}{c, d.Config.Issuer() + "/oauth2/par", dpopAlgorithms, true})
			return body
		})
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
	workload := &workloads{st: d.Store, dev: d.Config.Dev}
	token := st.authenticateClient(func(w http.ResponseWriter, r *http.Request) {
		if len(r.Header.Values(oidc.DPoPHeader)) == 0 {
			serve(w, r)
			return
		}
		jkt, err := st.proof(r, "")
		if err != nil {
			op.RequestError(w, r, err, nil)
			return
		}
		rewrite(w, r.WithContext(withDPoP(r.Context(), jkt)), serve, func(body []byte) []byte {
			var resp map[string]any
			if json.Unmarshal(body, &resp) != nil {
				return body
			}
			resp["token_type"] = dpopTokenType
			body, _ = json.Marshal(resp)
			return body
		})
	})
	mux.HandleFunc("POST /oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("grant_type") == string(oidc.GrantTypeClientCredentials) && r.PostForm.Get("client_assertion_type") == oidc.ClientAssertionTypeJWTAssertion && !st.application(r.Context(), r.PostForm.Get("client_id")) {
			workload.ServeHTTP(w, r)
			return
		}
		token(w, r)
	})
	mux.HandleFunc("POST /oauth2/introspect", st.authenticateClient(serve))
	mux.HandleFunc("POST /oauth2/revoke", st.authenticateClient(serve))
	mux.HandleFunc("/oauth2/userinfo", func(w http.ResponseWriter, r *http.Request) {
		scheme, token, _ := strings.Cut(r.Header.Get("Authorization"), " ")
		if !strings.EqualFold(scheme, dpopTokenType) {
			serve(w, r)
			return
		}
		jkt, err := st.proof(r, token)
		var e *oidc.Error
		if errors.As(err, &e) {
			w.Header().Set("WWW-Authenticate", `DPoP error="invalid_dpop_proof"`)
			httphelper.MarshalJSONWithStatus(w, e, http.StatusUnauthorized)
			return
		}
		if err != nil {
			http.Error(w, "Halo could not check the DPoP proof. Try again.", http.StatusInternalServerError)
			return
		}
		r.Header.Set("Authorization", oidc.PrefixBearer+token)
		serve(w, r.WithContext(withDPoP(r.Context(), jkt)))
	})
	mux.HandleFunc("/oauth2/authorize", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Has("request_uri") {
			var ok bool
			if r, ok = st.pushedAuthRequest(w, r); !ok {
				return
			}
		}
		serve(withIssuer(w, st.issuer, r.FormValue("redirect_uri")), r)
	})
	mux.Handle("POST /oauth2/par", cors.New(*provider.CORSOptions()).Handler(http.MaxBytesHandler(st.authenticateClient(st.pushAuthRequest(provider)), 64<<10)))
	mux.HandleFunc("GET /oauth2/logout/return", st.logoutReturn)
	registerTransmitter(mux, d.Store, d.Config.Issuer())
	mux.Handle("GET /oauth2/revoked", httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		revoked, err := d.Store.RevokedAccessTokens(r.Context())
		if err != nil {
			return err
		}
		w.Header().Set("Cache-Control", "no-store")
		return httpx.JSON(w, http.StatusOK, map[string]any{"revoked": revoked})
	}))
	mux.HandleFunc("/oauth2/authorize/callback", func(w http.ResponseWriter, r *http.Request) {
		req, err := d.Store.GetAuthRequest(r.Context(), r.FormValue("id"))
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			http.Error(w, "Halo could not load this sign-in. Go back to the application and try again.", http.StatusInternalServerError)
			return
		}
		sess, signedIn := httpx.CurrentSession(r)
		if err == nil && req.Done && (!signedIn || req.SessionID == nil || *req.SessionID != sess.ID) {
			http.Error(w, "This sign-in was completed in a different browser session. Start again from the application.", http.StatusForbidden)
			return
		}
		if err == nil {
			w = withIssuer(w, st.issuer, req.RedirectURI)
		}
		if err == nil && req.Done && st.awaitConsent(w, r, req, provider) {
			return
		}
		if err == nil && req.Done && req.ResponseMode == string(oidc.ResponseModeFormPost) && st.formPost(w, r, req, provider) {
			return
		}
		provider.ServeHTTP(w, r)
	})
	return mux, nil
}

func rewrite(w http.ResponseWriter, r *http.Request, next http.HandlerFunc, edit func([]byte) []byte) {
	rec := httptest.NewRecorder()
	next(rec, r)
	body := rec.Body.Bytes()
	if rec.Code == http.StatusOK {
		body = edit(body)
	}
	maps.Copy(w.Header(), rec.Header())
	w.WriteHeader(rec.Code)
	_, _ = w.Write(body)
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
		d.Jobs.Every("send back-channel logouts", 10*time.Second, func(ctx context.Context) error { return DeliverLogouts(ctx, d.Store, d.Config.Issuer(), time.Now()) })
		d.Jobs.Every("queue back-channel logouts for expired sessions", time.Minute, d.Store.QueueExpiredSessionLogouts)
		d.Jobs.Every("push shared signals", 10*time.Second, func(ctx context.Context) error { return DeliverSignals(ctx, d.Store, d.Config.Issuer(), time.Now()) })
	}
	resources := &resourceHandlers{st: d.Store}
	route("GET /api/v1/api-resources", readers, resources.list)
	route("POST /api/v1/api-resources", writers, resources.create)
	route("GET /api/v1/api-resources/{id}", readers, resources.get)
	route("PUT /api/v1/api-resources/{id}", writers, resources.update)
	route("DELETE /api/v1/api-resources/{id}", writers, resources.remove)
	route("PUT /api/v1/api-resources/{id}/grants/{appId}", writers, resources.grant)
	signals := &signalHandlers{st: d.Store}
	route("GET /api/v1/applications/{id}/shared-signals", readers, signals.get)
	route("PUT /api/v1/applications/{id}/shared-signals", []string{"security_admin"}, signals.put)
	route("DELETE /api/v1/applications/{id}/shared-signals", []string{"security_admin"}, signals.remove)
	return nil
}
