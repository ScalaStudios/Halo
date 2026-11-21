package features

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"halo/internal/httpx"
	"halo/internal/store"
)

type Feature struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Paths       []string `json:"-"`
	Jobs        []string `json:"-"`
	usage       func(context.Context, *store.Store) ([]string, error)
}

var readers = []string{"security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"}

var All = []Feature{
	{
		ID: "saml", Name: "SAML identity provider",
		Description: "Single sign-on to applications over SAML 2.0, with Halo's SAML metadata, signing certificate and sign-in endpoints under /saml.",
		Paths:       []string{"/saml/metadata", "/saml/certificate", "/saml/sso", "/saml/resume", "/saml/launch", "/api/v1/saml", "/api/v1/applications/{id}/saml"},
		Jobs:        []string{"purge expired SAML requests"},
		usage: func(ctx context.Context, st *store.Store) ([]string, error) {
			apps, err := st.ListApplications(ctx)
			return tally(apps, err, func(a store.Application) bool { return a.Protocol == "saml" }, "application signs in with SAML", "applications sign in with SAML")
		},
	},
	{
		ID: "scim", Name: "SCIM server",
		Description: "HR systems, Okta or Microsoft Entra ID create, update and deactivate people and groups in Halo through /scim/v2.",
		Paths:       []string{"/scim"},
		usage: func(ctx context.Context, st *store.Store) ([]string, error) {
			keys, err := st.ListAPIKeys(ctx, "")
			return tally(keys, err, func(k store.APIKey) bool {
				return k.RevokedAt == nil && (k.ExpiresAt == nil || k.ExpiresAt.After(time.Now())) && slices.Contains(k.Scopes, "scim")
			}, "active API key has the SCIM scope", "active API keys have the SCIM scope")
		},
	},
	{
		ID: "provisioning", Name: "Outbound SCIM",
		Description: "Halo creates, updates and deactivates the people and groups assigned to an application in that application, every 2 minutes or on demand.",
		Paths:       []string{"/api/v1/provisioning"},
		Jobs:        []string{"push assigned people and groups to applications over SCIM"},
		usage: func(ctx context.Context, st *store.Store) ([]string, error) {
			configs, err := st.ListAppProvisioning(ctx)
			return tally(configs, err, func(c store.AppProvisioning) bool { return c.Enabled }, "application is provisioned", "applications are provisioned")
		},
	},
	{
		ID: "ssh", Name: "SSH certificate authority",
		Description: "Short-lived SSH certificates for people in mapped groups through halo ssh-cert, and the CA public key your servers trust.",
		Paths:       []string{"/api/v1/ssh", "/api/v1/me/ssh"},
		usage: func(ctx context.Context, st *store.Store) ([]string, error) {
			mappings, err := st.ListSSHPrincipalMappings(ctx)
			return tally(mappings, err, func(store.SSHPrincipalMapping) bool { return true }, "group is mapped to SSH principals", "groups are mapped to SSH principals")
		},
	},
	{
		ID: "governance", Name: "Governance",
		Description: "Access packages and access requests, access reviews, and joiner, mover, leaver, date and inactivity lifecycle rules. Access that was already granted still expires on time, and lifecycle rules act on what changed while it was off once you turn it back on.",
		Paths: []string{"/api/v1/access-packages", "/api/v1/access-requests", "/api/v1/access-reviews", "/api/v1/lifecycle",
			"/api/v1/me/access-packages", "/api/v1/me/access-requests", "/api/v1/me/access-grants", "/api/v1/me/approvals", "/api/v1/me/reviews"},
		Jobs: []string{"mark overdue access reviews", "run lifecycle rules", "run date and inactivity lifecycle rules"},
		usage: func(ctx context.Context, st *store.Store) ([]string, error) {
			packages, err := st.ListAccessPackages(ctx)
			used, err := tally(packages, err, func(store.AccessPackage) bool { return true }, "access package", "access packages")
			if err != nil {
				return nil, err
			}
			reviews, err := st.ListAccessReviews(ctx, store.AccessReviewFilter{Open: true})
			open, err := tally(reviews, err, func(store.AccessReview) bool { return true }, "open access review", "open access reviews")
			if err != nil {
				return nil, err
			}
			rules, err := st.ListLifecycleRules(ctx)
			active, err := tally(rules, err, func(r store.LifecycleRule) bool { return r.Enabled }, "lifecycle rule is on", "lifecycle rules are on")
			return slices.Concat(used, open, active), err
		},
	},
	{
		ID: "webhooks", Name: "Webhooks and notification rules",
		Description: "Signed webhook deliveries, and Slack, Discord and webhook messages for the security events you choose. Events from while it was off are delivered when you turn it back on.",
		Paths:       []string{"/api/v1/webhooks", "/api/v1/notification-rules"},
		Jobs:        []string{"deliver webhooks"},
		usage: func(ctx context.Context, st *store.Store) ([]string, error) {
			hooks, err := st.ListWebhooks(ctx)
			used, err := tally(hooks, err, func(w store.Webhook) bool { return w.Enabled }, "webhook is on", "webhooks are on")
			if err != nil {
				return nil, err
			}
			rules, err := st.ListNotificationRules(ctx)
			active, err := tally(rules, err, func(r store.NotificationRule) bool { return r.Enabled }, "notification rule is on", "notification rules are on")
			return slices.Concat(used, active), err
		},
	},
	{
		ID: "forward-auth", Name: "Forward auth",
		Description: "Sign-in for applications behind Traefik, nginx or Caddy, which ask Halo under /forward-auth before they let each request through.",
		Paths:       []string{"/forward-auth"},
		usage: func(ctx context.Context, st *store.Store) ([]string, error) {
			apps, err := st.ListApplications(ctx)
			return tally(apps, err, func(a store.Application) bool { return a.Protocol == "forward-auth" }, "application uses forward auth", "applications use forward auth")
		},
	},
	{
		ID: "radius", Name: "RADIUS server",
		Description: "Sign-in for VPN gateways, firewalls and network devices with a Halo password over RADIUS. While it is off, the RADIUS listener answers nothing.",
		usage: func(ctx context.Context, st *store.Store) ([]string, error) {
			apps, err := st.ListApplications(ctx)
			return tally(apps, err, func(a store.Application) bool { return a.Protocol == "radius" }, "RADIUS client is registered", "RADIUS clients are registered")
		},
	},
	{
		ID: "log-streaming", Name: "Log streaming",
		Description: "Every sign-in and audit event sent to syslog or an OpenTelemetry collector.",
		Paths:       []string{"/api/v1/log-export"},
		Jobs:        []string{"export logs"},
		usage: func(ctx context.Context, st *store.Store) ([]string, error) {
			x, err := st.GetLogExport(ctx)
			if errors.Is(err, store.ErrNotFound) {
				return nil, nil
			}
			return tally([]store.LogExport{x}, err, func(store.LogExport) bool { return true }, "destination receives every event", "destinations receive every event")
		},
	},
}

type owner string

func (owner) ServeHTTP(http.ResponseWriter, *http.Request) {}

func tally[T any](list []T, err error, keep func(T) bool, one, many string) ([]string, error) {
	n := 0
	for _, x := range list {
		if keep(x) {
			n++
		}
	}
	if err != nil || n == 0 {
		return nil, err
	}
	if n == 1 {
		return []string{"1 " + one}, nil
	}
	return []string{fmt.Sprintf("%d %s", n, many)}, nil
}

func Known(id string) bool {
	return slices.ContainsFunc(All, func(f Feature) bool { return f.ID == id })
}

func Name(id string) string {
	if i := slices.IndexFunc(All, func(f Feature) bool { return f.ID == id }); i >= 0 {
		return All[i].Name
	}
	return id
}

func Off(ctx context.Context, st *store.Store, id string) (bool, error) {
	s, err := st.Settings(ctx)
	return slices.Contains(s.FeaturesOff, id), err
}

func Gate(st *store.Store, next http.Handler) http.Handler {
	routes := http.NewServeMux()
	for _, f := range All {
		for _, p := range f.Paths {
			routes.Handle(p, owner(f.ID))
			routes.Handle(p+"/", owner(f.ID))
		}
	}
	return httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		h, _ := routes.Handler(r)
		if id, ok := h.(owner); ok {
			gone, err := Off(r.Context(), st, string(id))
			if err != nil {
				return err
			}
			if gone && strings.HasPrefix(r.URL.Path, "/api/") {
				return httpx.ErrNoRoute
			}
			if gone {
				http.NotFound(w, r)
				return nil
			}
		}
		next.ServeHTTP(w, r)
		return nil
	})
}

func SkipJob(st *store.Store) func(context.Context, string) bool {
	owners := map[string]string{}
	for _, f := range All {
		for _, job := range f.Jobs {
			owners[job] = f.ID
		}
	}
	return func(ctx context.Context, job string) bool {
		id, ok := owners[job]
		if !ok {
			return false
		}
		gone, err := Off(ctx, st, id)
		return gone || err != nil
	}
}

func InUse(ctx context.Context, st *store.Store, ids []string) (string, error) {
	var found []string
	for _, f := range All {
		if !slices.Contains(ids, f.ID) {
			continue
		}
		usage, err := f.usage(ctx, st)
		if err != nil {
			return "", err
		}
		if len(usage) > 0 {
			found = append(found, f.Name+" is in use: "+strings.Join(usage, ", "))
		}
	}
	return strings.Join(found, ". "), nil
}

func Register(mux *http.ServeMux, d httpx.Deps) error {
	type view struct {
		Feature
		Enabled bool     `json:"enabled"`
		Usage   []string `json:"usage"`
	}
	mux.Handle("GET /api/v1/features", httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		if _, err := httpx.RequireRole(r, readers...); err != nil {
			return err
		}
		s, err := d.Store.Settings(r.Context())
		if err != nil {
			return err
		}
		list := make([]view, len(All))
		for i, f := range All {
			usage, err := f.usage(r.Context(), d.Store)
			if err != nil {
				return err
			}
			list[i] = view{Feature: f, Enabled: !slices.Contains(s.FeaturesOff, f.ID), Usage: append([]string{}, usage...)}
		}
		return httpx.JSON(w, http.StatusOK, list)
	}))
	return nil
}
