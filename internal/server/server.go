package server

import (
	"context"
	"net/http"
	"time"

	"halo/internal/api"
	"halo/internal/apikeys"
	"halo/internal/auth"
	"halo/internal/avatars"
	"halo/internal/config"
	"halo/internal/governance"
	"halo/internal/httpx"
	"halo/internal/jobs"
	"halo/internal/mail"
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

type App struct {
	Handler http.Handler
	Jobs    *jobs.Scheduler
}

func New(cfg config.Config, st *store.Store) (*App, error) {
	authn := &httpx.Auth{Store: st, Public: cfg.PublicURL, Dev: cfg.Dev, Keys: apikeys.Resolver(st), AccessTokens: oidc.AccessTokenResolver(st)}
	scheduler := &jobs.Scheduler{}
	httpx.TrustProxies(cfg.TrustedProxies)
	deps := httpx.Deps{Config: cfg, Store: st, Auth: authn, Policy: policy.NewEngine(st), Mailer: mail.New(cfg, st), Jobs: scheduler}
	scheduler.Every("purge expired records", time.Hour, func(ctx context.Context) error { return st.Purge(ctx) })

	apiMux := http.NewServeMux()
	for _, register := range []func(*http.ServeMux, httpx.Deps) error{auth.Register, api.Register, governance.Register, policy.Register, apikeys.Register, provisioning.Register, settings.Register, webhooks.Register, oidc.Register, sshca.Register, avatars.Register} {
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
	root.Handle("/api/", authn.Resolve(authn.KeyScope("api", authn.SameOrigin(apiMux))))
	root.Handle("/oauth2/", authn.Resolve(authn.KeyScope("", provider)))
	root.Handle("/.well-known/", provider)
	root.Handle("/saml/", authn.Resolve(authn.KeyScope("", samlHandler)))
	root.Handle("/scim/", authn.Resolve(authn.KeyScope("scim", scimHandler)))
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := st.Ping(r.Context()); err != nil {
			http.Error(w, "database unreachable", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	})
	return &App{Handler: httpx.Logging(root), Jobs: scheduler}, nil
}
