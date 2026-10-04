package provisioning

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"halo/internal/httpx"
	"halo/internal/scim"
	"halo/internal/store"
)

var readers = []string{"security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"}

type handlers struct {
	st     *store.Store
	client *http.Client
	mu     sync.Mutex
}

func Register(mux *http.ServeMux, d httpx.Deps) error {
	h := &handlers{st: d.Store, client: &http.Client{Timeout: 15 * time.Second}}
	if d.Jobs != nil {
		d.Jobs.Every("push assigned people to applications over SCIM", 2*time.Minute, h.syncDue)
	}
	route := func(pattern string, roles []string, fn func(http.ResponseWriter, *http.Request, store.User) error) {
		mux.Handle(pattern, httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
			actor, err := httpx.RequireRole(r, roles...)
			if err != nil {
				return err
			}
			return fn(w, r, actor)
		}))
	}
	route("GET /api/v1/provisioning/applications", readers, h.list)
	route("PUT /api/v1/provisioning/applications/{id}", []string{"app_admin"}, h.configure)
	route("POST /api/v1/provisioning/applications/{id}/test", []string{"app_admin"}, h.test)
	route("POST /api/v1/provisioning/applications/{id}/sync", []string{"app_admin"}, h.syncNow)
	return nil
}

func (h *handlers) application(r *http.Request) (store.Application, error) {
	app, err := h.st.GetApplication(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		return app, httpx.NotFound("This application")
	}
	if err == nil && (app.Protocol != "oidc" && app.Protocol != "saml" || app.Type == "service") {
		return app, httpx.Invalid(app.Name + " authenticates as itself, so it has no people to provision. Provisioning works with OpenID Connect and SAML applications that people sign in to.")
	}
	return app, err
}

func (h *handlers) config(r *http.Request, app store.Application) (store.AppProvisioning, error) {
	c, err := h.st.GetAppProvisioning(r.Context(), app.ID)
	if errors.Is(err, store.ErrNotFound) {
		return c, httpx.Fail(http.StatusNotFound, "ERR_NOT_CONFIGURED", "Provisioning is not set up for "+app.Name+". Save its SCIM base URL and bearer token first.")
	}
	return c, err
}

func (h *handlers) list(w http.ResponseWriter, r *http.Request, _ store.User) error {
	configs, err := h.st.ListAppProvisioning(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, configs)
}

func (h *handlers) configure(w http.ResponseWriter, r *http.Request, actor store.User) error {
	app, err := h.application(r)
	if err != nil {
		return err
	}
	var in struct {
		BaseURL string `json:"baseUrl"`
		Token   string `json:"token"`
		Enabled bool   `json:"enabled"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	base := strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	u, err := url.Parse(base)
	loopback := err == nil && u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1")
	if err != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !loopback) {
		return httpx.Invalid(fmt.Sprintf("%q is not a valid SCIM base URL. Use the https:// address from %s's SCIM settings, such as https://app.example.com/scim/v2, without a query string.", in.BaseURL, app.Name))
	}
	_, err = h.st.GetAppProvisioning(r.Context(), app.ID)
	configured := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	token := strings.TrimSpace(in.Token)
	if token == "" && !configured {
		return httpx.Invalid("Enter the bearer token " + app.Name + " issued for SCIM provisioning.")
	}
	if len(token) > 4096 {
		return httpx.Invalid("The bearer token is longer than 4,096 characters. Check that you pasted only the token.")
	}
	var sealed []byte
	if token != "" {
		sealed = h.st.Sealer.Seal([]byte(token))
	}
	summary := "Set the SCIM endpoint to " + base + " with provisioning paused"
	if in.Enabled {
		summary = "Set the SCIM endpoint to " + base + " with provisioning on"
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.SaveAppProvisioning(r.Context(), app.ID, base, sealed, in.Enabled); err != nil {
			return err
		}
		return tx.RecordAudit(r.Context(), store.AuditEvent{ActorID: &actor.ID, Action: "provisioning.configure", Summary: summary, TargetType: "application", TargetID: app.ID, TargetLabel: app.Name, IP: httpx.ClientIP(r)})
	})
	if err != nil {
		return err
	}
	c, err := h.st.GetAppProvisioning(r.Context(), app.ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, c)
}

func (h *handlers) test(w http.ResponseWriter, r *http.Request, _ store.User) error {
	app, err := h.application(r)
	if err != nil {
		return err
	}
	c, err := h.config(r, app)
	if err != nil {
		return err
	}
	remote, err := h.remote(c)
	if err != nil {
		return httpx.Fail(http.StatusUnprocessableEntity, "ERR_TOKEN_UNREADABLE", err.Error())
	}
	var config map[string]any
	if err := remote.do(r.Context(), http.MethodGet, "/ServiceProviderConfig", nil, &config); err != nil {
		return httpx.Fail(http.StatusBadGateway, "ERR_SCIM_CONNECTION", "Testing "+app.Name+" failed: "+err.Error()+".")
	}
	return httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *handlers) syncNow(w http.ResponseWriter, r *http.Request, actor store.User) error {
	app, err := h.application(r)
	if err != nil {
		return err
	}
	c, err := h.config(r, app)
	if err != nil {
		return err
	}
	users, err := h.st.ListUsers(r.Context())
	if err != nil {
		return err
	}
	if err := h.sync(r.Context(), c, users); err != nil {
		return err
	}
	if c, err = h.st.GetAppProvisioning(r.Context(), app.ID); err != nil {
		return err
	}
	summary := fmt.Sprintf("Pushed people over SCIM: %d created, %d updated, %d deactivated", c.Created, c.Updated, c.Deactivated)
	if c.LastError != "" {
		summary += ". Some changes failed: " + c.LastError
	}
	if err := h.st.RecordAudit(r.Context(), store.AuditEvent{ActorID: &actor.ID, Action: "provisioning.sync", Summary: summary, TargetType: "application", TargetID: app.ID, TargetLabel: app.Name, IP: httpx.ClientIP(r)}); err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, c)
}

func (h *handlers) syncDue(ctx context.Context) error {
	configs, err := h.st.ListAppProvisioning(ctx)
	if err != nil {
		return err
	}
	var users []store.User
	loaded := false
	for _, c := range configs {
		if !c.Enabled || c.AppStatus != "active" {
			continue
		}
		if !loaded {
			if users, err = h.st.ListUsers(ctx); err != nil {
				return err
			}
			loaded = true
		}
		if err := h.sync(ctx, c, users); err != nil {
			return err
		}
	}
	return nil
}

type run struct {
	created, updated, deactivated, failed int
	first                                 string
}

func (r *run) fail(who string, err error) bool {
	r.failed++
	var status *statusError
	transport := !errors.As(err, &status)
	if r.first == "" {
		r.first = who + ": " + err.Error()
		if transport {
			r.first = err.Error()
		}
	}
	return transport
}

func (r *run) message() string {
	if r.failed > 1 {
		return fmt.Sprintf("%s (and %d more)", r.first, r.failed-1)
	}
	return r.first
}

func (h *handlers) remote(c store.AppProvisioning) (*client, error) {
	token, err := h.st.Sealer.Open(c.TokenSealed)
	if err != nil {
		return nil, errors.New("Halo could not decrypt the saved bearer token, which happens when HALO_SECRET_KEY changes. Save the token again")
	}
	return &client{http: h.client, base: c.BaseURL, token: string(token)}, nil
}

func (h *handlers) sync(ctx context.Context, c store.AppProvisioning, users []store.User) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	remote, err := h.remote(c)
	if err != nil {
		return h.st.RecordProvisioningRun(ctx, c.AppID, 0, 0, 0, err.Error())
	}
	known, err := h.st.ListProvisionedUsers(ctx, c.AppID)
	if err != nil {
		return err
	}
	var result run
	desired, emails := map[string]bool{}, map[string]string{}
	aborted := false
	for _, u := range users {
		emails[u.ID] = u.Email
		if aborted || u.Status != "active" || !slices.Contains(u.AppIDs, c.AppID) {
			continue
		}
		desired[u.ID] = true
		resource := scim.NewUser(u.Email, u.Name, u.Title, u.Department, true)
		resource.ExternalID = u.ID
		encoded, _ := json.Marshal(resource)
		sum := sha256.Sum256(encoded)
		hash := hex.EncodeToString(sum[:])
		prev, seen := known[u.ID]
		if seen && prev.Active && prev.Hash == hash {
			continue
		}
		remoteID, created, err := remote.push(ctx, prev.RemoteID, resource)
		if err != nil {
			aborted = result.fail(u.Email, err)
			continue
		}
		if err := h.st.SaveProvisionedUser(ctx, c.AppID, u.ID, store.ProvisionedUser{RemoteID: remoteID, Hash: hash, Active: true}); err != nil {
			return err
		}
		if created {
			result.created++
		} else {
			result.updated++
		}
	}
	for userID, prev := range known {
		if aborted || !prev.Active || desired[userID] {
			continue
		}
		if err := remote.deactivate(ctx, prev.RemoteID); err != nil && !isStatus(err, http.StatusNotFound) {
			aborted = result.fail(emails[userID], err)
			continue
		}
		prev.Active = false
		if err := h.st.SaveProvisionedUser(ctx, c.AppID, userID, prev); err != nil {
			return err
		}
		result.deactivated++
	}
	return h.st.RecordProvisioningRun(ctx, c.AppID, result.created, result.updated, result.deactivated, result.message())
}

type statusError struct {
	status int
	detail string
}

func (e *statusError) Error() string {
	msg := fmt.Sprintf("the SCIM endpoint answered %d %s", e.status, http.StatusText(e.status))
	if e.detail != "" {
		msg += " (" + e.detail + ")"
	}
	switch e.status {
	case http.StatusUnauthorized, http.StatusForbidden:
		msg += ". Check the bearer token"
	case http.StatusNotFound:
		msg += ". Check the base URL"
	}
	return msg
}

func isStatus(err error, status int) bool {
	var e *statusError
	return errors.As(err, &e) && e.status == status
}

type client struct {
	http  *http.Client
	base  string
	token string
}

func (c *client) do(ctx context.Context, method, path string, body, out any) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/scim+json, application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/scim+json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Halo could not connect to the SCIM endpoint: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		var problem struct {
			Detail string `json:"detail"`
		}
		_ = json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&problem)
		return &statusError{status: res.StatusCode, detail: problem.Detail}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(out); err != nil {
		return &statusError{status: res.StatusCode, detail: "the response is not SCIM JSON"}
	}
	return nil
}

func (c *client) push(ctx context.Context, remoteID string, u scim.User) (string, bool, error) {
	if remoteID != "" {
		err := c.do(ctx, http.MethodPut, "/Users/"+url.PathEscape(remoteID), u, nil)
		if !isStatus(err, http.StatusNotFound) {
			return remoteID, false, err
		}
	}
	var created struct {
		ID string `json:"id"`
	}
	err := c.do(ctx, http.MethodPost, "/Users", u, &created)
	if isStatus(err, http.StatusConflict) {
		userName, _ := json.Marshal(u.UserName)
		var found struct {
			Resources []struct {
				ID string `json:"id"`
			} `json:"Resources"`
		}
		if err := c.do(ctx, http.MethodGet, "/Users?filter="+url.QueryEscape("userName eq "+string(userName)), nil, &found); err != nil {
			return "", false, err
		}
		if len(found.Resources) == 0 {
			return "", false, &statusError{status: http.StatusConflict, detail: "the user exists, but filtering by userName does not find it"}
		}
		return found.Resources[0].ID, false, c.do(ctx, http.MethodPut, "/Users/"+url.PathEscape(found.Resources[0].ID), u, nil)
	}
	if err == nil && created.ID == "" {
		err = &statusError{status: http.StatusCreated, detail: "the response has no id"}
	}
	return created.ID, err == nil, err
}

func (c *client) deactivate(ctx context.Context, remoteID string) error {
	return c.do(ctx, http.MethodPatch, "/Users/"+url.PathEscape(remoteID), map[string]any{
		"schemas":    []string{scim.PatchSchema},
		"Operations": []map[string]any{{"op": "replace", "path": "active", "value": false}},
	}, nil)
}
