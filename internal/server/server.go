package server

import (
	"context"
	"net/http"
	"time"

	"halo/internal/api"
	"halo/internal/apikeys"
	"halo/internal/approval"
	"halo/internal/auth"
	"halo/internal/avatars"
	"halo/internal/compliance"
	"halo/internal/config"
	"halo/internal/governance"
	"halo/internal/httpx"
	"halo/internal/jobs"
	"halo/internal/logexport"
	"halo/internal/mail"
	"halo/internal/metrics"
	"halo/internal/oidc"
	"halo/internal/policy"
	"halo/internal/provisioning"
	"halo/internal/saml"
	"halo/internal/scim"
	"halo/internal/settings"
	"halo/internal/sshca"
	"halo/internal/store"
	"halo/internal/webhooks"
)

var sensitiveRoutes = []string{
	"PUT /api/v1/users/{id}/roles",
	"PUT /api/v1/users/{id}/eligible-roles",
	"PUT /api/v1/users/{id}/guest",
	"POST /api/v1/me/role-activations",
	"POST /api/v1/service-accounts",
	"PUT /api/v1/service-accounts/{id}/roles",
	"POST /api/v1/service-accounts/{id}/keys",
	"POST /api/v1/service-accounts/{id}/federated-credentials",
	"DELETE /api/v1/service-accounts/{id}/federated-credentials/{credentialId}",
	"POST /api/v1/applications/{id}/secrets",
	"POST /api/v1/signing-keys/rotate",
	"POST /api/v1/saml/certificate/rotate",
	"POST /api/v1/policies",
	"PUT /api/v1/policies/order",
	"PUT /api/v1/policies/{id}",
	"DELETE /api/v1/policies/{id}",
	"PUT /api/v1/authentication-contexts/{id}",
	"DELETE /api/v1/authentication-contexts/{id}",
	"PUT /api/v1/methods/{method}",
	"PATCH /api/v1/settings",
	"POST /api/v1/users/{id}/reset-authentication",
	"POST /api/v1/users/{id}/access-pass",
	"PUT /api/v1/users/{id}/password",
	"DELETE /api/v1/users/{id}/methods/{methodId}",
	"POST /api/v1/identity-providers",
	"PUT /api/v1/identity-providers/{id}",
	"DELETE /api/v1/identity-providers/{id}",
	"POST /api/v1/identity-providers/{id}/enable",
	"POST /api/v1/identity-providers/{id}/disable",
	"POST /api/v1/certificate-authorities",
	"DELETE /api/v1/certificate-authorities/{id}",
	"GET /api/v1/export",
	"POST /api/v1/approvals/{id}/approve",
	"POST /api/v1/impersonations/{id}/start",
	"GET /api/v1/users/{id}/export",
	"DELETE /api/v1/users/{id}",
	"POST /api/v1/notification-rules",
	"PUT /api/v1/notification-rules/{id}",
	"DELETE /api/v1/notification-rules/{id}",
	"POST /api/v1/compliance-sources",
	"PUT /api/v1/compliance-sources/{id}",
	"DELETE /api/v1/compliance-sources/{id}",
	"PUT /api/v1/applications/{id}/hook",
	"DELETE /api/v1/applications/{id}/hook",
}

var ownMethodRoutes = []string{
	"POST /api/v1/me/passkeys/begin",
	"POST /api/v1/me/totp/begin",
	"POST /api/v1/me/recovery-codes",
	"DELETE /api/v1/me/methods/{id}",
	"PUT /api/v1/me/security-emails",
}

type App struct {
	Handler http.Handler
	Metrics http.Handler
	Jobs    *jobs.Scheduler
}

func New(cfg config.Config, st *store.Store) (*App, error) {
	authn := &httpx.Auth{Store: st, Public: cfg.PublicURL, Dev: cfg.Dev, Keys: apikeys.Resolver(st), AccessTokens: oidc.AccessTokenResolver(st)}
	scheduler := &jobs.Scheduler{}
	httpx.TrustProxies(cfg.TrustedProxies)
	deps := httpx.Deps{Config: cfg, Store: st, Auth: authn, Policy: policy.NewEngine(st), Mailer: mail.New(cfg, st), Jobs: scheduler}
	scheduler.Every("purge expired records", time.Hour, func(ctx context.Context) error { return st.Purge(ctx) })
	scheduler.Every("weekly security digest", time.Hour, func(ctx context.Context) error {
		return mail.SendWeeklyDigest(ctx, deps.Mailer, cfg, st)
	})

	apiMux := http.NewServeMux()
	for _, register := range []func(*http.ServeMux, httpx.Deps) error{auth.Register, api.Register, governance.Register, policy.Register, compliance.Register, apikeys.Register, provisioning.Register, settings.Register, webhooks.Register, logexport.Register, oidc.Register, sshca.Register, avatars.Register, approval.Register} {
		if err := register(apiMux, deps); err != nil {
			return nil, err
		}
	}
	apiMux.Handle("GET /api/v1/organization", settings.Organization(deps))
	apiMux.Handle("/api/", httpx.Handle(func(http.ResponseWriter, *http.Request) error {
		return httpx.Fail(http.StatusNotFound, "ERR_NO_ROUTE", "This API route does not exist. Check the path and method.")
	}))

	provider, err := oidc.New(deps)
	if err != nil {
		return nil, err
	}

	samlHandler, err := saml.New(deps)
	if err != nil {
		return nil, err
	}
	scimHandler, err := scim.New(deps)
	if err != nil {
		return nil, err
	}

	root := http.NewServeMux()
	root.Handle("/api/", authn.Resolve(authn.KeyScope("api", authn.RateLimit(authn.SameOrigin(authn.RecentSignIn(apiMux, sensitiveRoutes, ownMethodRoutes))))))
	root.Handle("/oauth2/", authn.Resolve(authn.KeyScope("", provider)))
	registration := api.Registration(deps)
	root.Handle("/oauth2/register", registration)
	root.Handle("/oauth2/register/", registration)
	root.Handle("/.well-known/", provider)
	root.Handle("/saml/", authn.Resolve(authn.KeyScope("", samlHandler)))
	root.Handle("POST /saml/sp/{id}/acs", samlHandler)
	root.Handle("/scim/", authn.Resolve(authn.KeyScope("scim", scimHandler)))
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := st.Ping(r.Context()); err != nil {
			http.Error(w, "database unreachable", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	})
	app := &App{Handler: httpx.Logging(root), Jobs: scheduler}
	if cfg.MetricsListen != "" || cfg.MetricsToken != "" {
		exporter := metrics.Handler(cfg.MetricsToken, func() []metrics.Gauge {
			pool := st.PoolStat()
			return []metrics.Gauge{
				{Name: "halo_db_pool_acquired_connections", Help: "Database connections in use.", Value: float64(pool.AcquiredConns())},
				{Name: "halo_db_pool_idle_connections", Help: "Idle database connections.", Value: float64(pool.IdleConns())},
				{Name: "halo_db_pool_total_connections", Help: "Open database connections.", Value: float64(pool.TotalConns())},
				{Name: "halo_db_pool_max_connections", Help: "Largest number of database connections the pool opens.", Value: float64(pool.MaxConns())},
			}
		})
		if cfg.MetricsListen != "" {
			app.Metrics = exporter
		} else {
			root.Handle("GET /metrics", exporter)
		}
	}
	return app, nil
}
