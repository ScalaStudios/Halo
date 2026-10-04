package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"halo/internal/httpx"
	"halo/internal/id"
	"halo/internal/store"
)

const (
	maxAttempts = 8
	settle      = 5 * time.Second
	lease       = 2 * time.Minute
)

var (
	readers      = []string{"security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"}
	writers      = []string{"security_admin"}
	eventPattern = regexp.MustCompile(`^(\*|[a-z_]+(\.[a-z_]+)*(\.\*)?)$`)
	families     = []string{"access_package", "access_request", "access_review", "application", "branding", "device", "domain", "group", "lifecycle", "method", "network_zone", "organization", "policy", "provisioning", "role", "scim", "session", "settings", "sign_in", "user", "webhook"}
)

type handler struct {
	st     *store.Store
	client *http.Client
}

func Register(mux *http.ServeMux, d httpx.Deps) error {
	h := &handler{st: d.Store, client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	if d.Jobs != nil {
		d.Jobs.Every("deliver webhooks", 10*time.Second, func(ctx context.Context) error { return h.run(ctx, time.Now()) })
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
	route("GET /api/v1/webhooks", readers, h.list)
	route("POST /api/v1/webhooks", writers, h.create)
	route("GET /api/v1/webhooks/event-types", readers, h.eventTypes)
	route("GET /api/v1/webhooks/{id}", readers, h.get)
	route("PUT /api/v1/webhooks/{id}", writers, h.update)
	route("DELETE /api/v1/webhooks/{id}", writers, h.delete)
	route("POST /api/v1/webhooks/{id}/test", writers, h.test)
	return nil
}

func sign(secret []byte, at time.Time, body []byte) string {
	t := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(t + "."))
	mac.Write(body)
	return "t=" + t + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func backoff(attempt int) time.Duration {
	return 30 * time.Second << (attempt - 1)
}

func subscribed(patterns []string, eventType string) bool {
	return slices.ContainsFunc(patterns, func(p string) bool {
		return p == "*" || p == eventType || strings.HasSuffix(p, ".*") && strings.HasPrefix(eventType, strings.TrimSuffix(p, "*"))
	})
}

func (h *handler) run(ctx context.Context, now time.Time) error {
	err := h.st.Tx(ctx, func(tx *store.Store) error {
		events, err := tx.PollWebhookEvents(ctx, now.Add(-settle), 500)
		if err != nil || len(events) == 0 {
			return err
		}
		endpoints, err := tx.ActiveWebhooks(ctx)
		if err != nil {
			return err
		}
		for _, e := range events {
			for _, endpoint := range endpoints {
				if e.Time.Before(endpoint.CreatedAt) || !subscribed(endpoint.Events, e.Type) {
					continue
				}
				if _, err := tx.QueueWebhookDelivery(ctx, endpoint.ID, e, now); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	due, err := h.st.ClaimWebhookDeliveries(ctx, now, lease, 100)
	if err != nil {
		return err
	}
	for _, d := range due {
		if err := h.attempt(ctx, d, now); err != nil {
			return err
		}
	}
	return nil
}

func (h *handler) attempt(ctx context.Context, d store.WebhookDelivery, now time.Time) error {
	endpoint, err := h.st.GetWebhook(ctx, d.EndpointID)
	if err != nil {
		return err
	}
	secret, err := h.st.Sealer.Open(endpoint.Sealed)
	if err != nil {
		return err
	}
	code, message := h.send(ctx, endpoint.URL, secret, []byte(d.Body))
	status, next := "delivered", now
	if code < 200 || code > 299 {
		status = "pending"
		if d.Attempts+1 >= maxAttempts {
			status = "failed"
		} else {
			next = now.Add(backoff(d.Attempts + 1))
		}
	}
	var response *int
	if code > 0 {
		response = &code
	}
	return h.st.RecordWebhookAttempt(ctx, d.ID, status, response, message, now, next)
}

func (h *handler) send(ctx context.Context, target string, secret, body []byte) (int, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return 0, err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Halo-Webhooks")
	req.Header.Set("Halo-Signature", sign(secret, time.Now(), body))
	res, err := h.client.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return res.StatusCode, "The endpoint answered " + res.Status + "."
	}
	return res.StatusCode, ""
}

type input struct {
	URL         string   `json:"url"`
	Description string   `json:"description"`
	Events      []string `json:"events"`
	Enabled     *bool    `json:"enabled"`
}

func (in input) validate() (store.NewWebhook, error) {
	w := store.NewWebhook{URL: strings.TrimSpace(in.URL), Description: strings.TrimSpace(in.Description), Enabled: in.Enabled == nil || *in.Enabled}
	u, err := url.Parse(w.URL)
	if err != nil || u.Host == "" || u.User != nil || !(u.Scheme == "https" || u.Scheme == "http" && local(u.Hostname())) {
		return w, httpx.Invalid("Enter an https:// URL. Plain http:// is allowed only for localhost while you test.")
	}
	if len(w.URL) > 2048 || len(w.Description) > 200 {
		return w, httpx.Invalid("Keep the URL under 2,048 characters and the description under 200.")
	}
	for _, e := range in.Events {
		if !eventPattern.MatchString(e) {
			return w, httpx.Invalid(fmt.Sprintf("%q is not an event type. Use a type such as user.invite, a family such as user.*, or * for every event.", e))
		}
		if !slices.Contains(w.Events, e) {
			w.Events = append(w.Events, e)
		}
	}
	if len(w.Events) == 0 || len(w.Events) > 50 {
		return w, httpx.Invalid("Choose between 1 and 50 event types to send to this endpoint.")
	}
	return w, nil
}

func local(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || strings.HasSuffix(host, ".localhost") || ip != nil && ip.IsLoopback()
}

func record(r *http.Request, st *store.Store, actor store.User, action, summary string, w store.Webhook) error {
	return st.RecordAudit(r.Context(), store.AuditEvent{ActorID: &actor.ID, IP: httpx.ClientIP(r), Action: action, Summary: summary, TargetType: "webhook", TargetID: w.ID, TargetLabel: w.URL})
}

func found(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("The webhook endpoint")
	}
	return err
}

func (h *handler) list(w http.ResponseWriter, r *http.Request, _ store.User) error {
	endpoints, err := h.st.ListWebhooks(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, endpoints)
}

func (h *handler) eventTypes(w http.ResponseWriter, r *http.Request, _ store.User) error {
	seen, err := h.st.WebhookEventFamilies(r.Context())
	if err != nil {
		return err
	}
	all := slices.Concat(families, seen)
	slices.Sort(all)
	types := []string{}
	for _, family := range slices.Compact(all) {
		if eventPattern.MatchString(family) {
			types = append(types, family+".*")
		}
	}
	return httpx.JSON(w, http.StatusOK, types)
}

func (h *handler) get(w http.ResponseWriter, r *http.Request, _ store.User) error {
	ctx := r.Context()
	endpoint, err := h.st.GetWebhook(ctx, r.PathValue("id"))
	if err != nil {
		return found(err)
	}
	deliveries, err := h.st.ListWebhookDeliveries(ctx, endpoint.ID, 50)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"webhook": endpoint, "deliveries": deliveries})
}

func (h *handler) create(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in input
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	fields, err := in.validate()
	if err != nil {
		return err
	}
	ctx := r.Context()
	var endpoint store.Webhook
	var secret string
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		webhookID, s, err := tx.CreateWebhook(ctx, fields)
		if err != nil {
			return err
		}
		if endpoint, err = tx.GetWebhook(ctx, webhookID); err != nil {
			return err
		}
		secret = s
		return record(r, tx, actor, "webhook.create", "Added a webhook endpoint for "+strings.Join(fields.Events, ", "), endpoint)
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, map[string]any{"webhook": endpoint, "secret": secret})
}

func (h *handler) update(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in input
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	fields, err := in.validate()
	if err != nil {
		return err
	}
	ctx := r.Context()
	var endpoint store.Webhook
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.UpdateWebhook(ctx, r.PathValue("id"), fields); err != nil {
			return err
		}
		var err error
		if endpoint, err = tx.GetWebhook(ctx, r.PathValue("id")); err != nil {
			return err
		}
		state := "Updated"
		if !fields.Enabled {
			state = "Updated and paused"
		}
		return record(r, tx, actor, "webhook.update", state+" the webhook endpoint", endpoint)
	})
	if err != nil {
		return found(err)
	}
	return httpx.JSON(w, http.StatusOK, endpoint)
}

func (h *handler) delete(w http.ResponseWriter, r *http.Request, actor store.User) error {
	ctx := r.Context()
	err := h.st.Tx(ctx, func(tx *store.Store) error {
		endpoint, err := tx.GetWebhook(ctx, r.PathValue("id"))
		if err != nil {
			return err
		}
		if err := tx.DeleteWebhook(ctx, endpoint.ID); err != nil {
			return err
		}
		return record(r, tx, actor, "webhook.delete", "Deleted the webhook endpoint and its delivery log", endpoint)
	})
	if err != nil {
		return found(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *handler) test(w http.ResponseWriter, r *http.Request, actor store.User) error {
	ctx := r.Context()
	endpoint, err := h.st.GetWebhook(ctx, r.PathValue("id"))
	if err != nil {
		return found(err)
	}
	now := time.Now()
	data, _ := json.Marshal(map[string]string{"message": "This is a test event sent from the Halo console.", "endpointId": endpoint.ID, "sentBy": actor.Email})
	deliveryID, err := h.st.QueueWebhookDelivery(ctx, endpoint.ID, store.WebhookEvent{ID: id.New("evt"), Type: "webhook.test", Time: now.UTC(), Data: data}, now.Add(lease))
	if err != nil {
		return err
	}
	delivery, err := h.st.GetWebhookDelivery(ctx, deliveryID)
	if err != nil {
		return err
	}
	if err := h.attempt(ctx, delivery, now); err != nil {
		return err
	}
	if delivery, err = h.st.GetWebhookDelivery(ctx, deliveryID); err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, delivery)
}
