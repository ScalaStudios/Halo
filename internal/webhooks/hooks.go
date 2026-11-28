package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"halo/internal/httpx"
	"halo/internal/store"
)

const (
	HookSignIn  = "sign_in"
	HookToken   = "token"
	maxHookBody = 64 << 10
)

var (
	HookClient     = httpx.GuardedClient()
	hookTimeout    = 3 * time.Second
	claimName      = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:/-]{0,99}$`)
	reservedClaims = []string{"iss", "sub", "aud", "exp", "nbf", "iat", "jti", "auth_time", "nonce", "acr", "amr", "azp", "at_hash", "c_hash", "s_hash", "sid",
		"client_id", "scope", "cnf", "act", "may_act", "email", "email_verified", "name", "preferred_username", "picture", "groups", "roles"}
)

type HookCall struct {
	Stage     string
	User      store.User
	Scopes    []string
	GrantType string
}

type Verdict struct {
	Decision string
	Reason   string
	Log      string
	Claims   map[string]any
}

func (v Verdict) Denied() bool { return v.Decision == "denied" || v.Decision == "failed_closed" }

type hookAnswer struct {
	Decision string         `json:"decision"`
	Reason   string         `json:"reason"`
	Claims   map[string]any `json:"claims"`
}

func RunHook(ctx context.Context, st *store.Store, app store.Application, call HookCall) (Verdict, error) {
	hook, err := st.GetApplicationHook(ctx, app.ID)
	if errors.Is(err, store.ErrNotFound) {
		return Verdict{}, nil
	}
	if err != nil {
		return Verdict{}, err
	}
	payload := map[string]any{"type": call.Stage, "time": time.Now().UTC(), "scopes": slices.Concat([]string{}, call.Scopes),
		"user":        map[string]string{"id": call.User.ID, "email": call.User.Email, "name": call.User.Name},
		"application": map[string]string{"id": app.ID, "clientId": app.ClientID, "name": app.Name}}
	if call.GrantType != "" {
		payload["grantType"] = call.GrantType
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Verdict{}, err
	}
	answer, cause := callHook(ctx, st, hook, body)
	switch {
	case cause != "" && hook.FailOpen:
		return Verdict{Decision: "failed_open", Log: "The hook failed: " + cause + ". Allowed because the hook fails open."}, nil
	case cause != "":
		return Verdict{Decision: "failed_closed", Log: "The hook failed: " + cause + ". Blocked because the hook fails closed.",
			Reason: "Halo could not check your access to " + app.Name + ". Try again in a few minutes, or contact your administrator."}, nil
	case answer.Decision == "deny":
		reason := clip(strings.TrimSpace(answer.Reason), 200)
		if reason == "" {
			reason = "Your organization's access check denied this sign-in to " + app.Name + "."
		}
		return Verdict{Decision: "denied", Reason: reason, Log: "The hook denied access: " + reason}, nil
	}
	claims := map[string]any{}
	for name, value := range answer.Claims {
		if slices.Contains(hook.Claims, name) && !slices.Contains(reservedClaims, name) {
			claims[name] = value
		}
	}
	return Verdict{Decision: "allowed", Claims: claims}, nil
}

func callHook(ctx context.Context, st *store.Store, hook store.ApplicationHook, body []byte) (hookAnswer, string) {
	target, err := st.Sealer.Open(hook.URLSealed)
	if err != nil {
		return hookAnswer{}, "its URL could not be decrypted"
	}
	key, err := st.Sealer.Open(hook.SecretSealed)
	if err != nil {
		return hookAnswer{}, "its signing secret could not be decrypted"
	}
	ctx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, string(target), bytes.NewReader(body))
	if err != nil {
		return hookAnswer{}, withoutURL(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Halo-Hooks")
	req.Header.Set("Halo-Signature", sign(key, time.Now(), body))
	res, err := HookClient.Do(req)
	if err != nil {
		return hookAnswer{}, unreachable(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return hookAnswer{}, "it answered " + res.Status + " instead of 200 OK"
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxHookBody+1))
	switch {
	case err != nil:
		return hookAnswer{}, unreachable(err)
	case len(raw) > maxHookBody:
		return hookAnswer{}, "its answer is larger than 64 KiB"
	case !verified(key, res.Header.Get("Halo-Signature"), raw, time.Now()):
		return hookAnswer{}, "its answer has no valid Halo-Signature header"
	}
	var answer hookAnswer
	if json.Unmarshal(raw, &answer) != nil || answer.Decision != "allow" && answer.Decision != "deny" {
		return hookAnswer{}, `its answer is not JSON with "decision" set to allow or deny`
	}
	return answer, ""
}

func unreachable(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "it did not answer within " + hookTimeout.String()
	}
	return withoutURL(err)
}

func verified(key []byte, header string, body []byte, now time.Time) bool {
	stamp, _, _ := strings.Cut(strings.TrimPrefix(header, "t="), ",")
	unix, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		return false
	}
	at := time.Unix(unix, 0)
	return now.Sub(at).Abs() <= 5*time.Minute && hmac.Equal([]byte(header), []byte(sign(key, at, body)))
}

type hookInput struct {
	URL      string   `json:"url"`
	Secret   string   `json:"secret"`
	Claims   []string `json:"claims"`
	FailOpen bool     `json:"failOpen"`
}

func PublicURL(raw, name, caller string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || len(raw) > 2048 {
		return nil, httpx.Invalid("Enter the https:// URL of the " + name + ", under 2,048 characters.")
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !httpx.PublicAddress(ip) || local(u.Hostname()) {
		return nil, httpx.Invalid(caller + " public addresses. Private, loopback and link-local addresses are refused.")
	}
	return u, nil
}

func hookable(app store.Application) error {
	if app.Protocol != "oidc" && app.Protocol != "oauth" || app.Type == "service" || app.BuiltIn() {
		return httpx.Invalid("Hooks run when a person signs in to an OpenID Connect application. Service applications, SAML applications, bookmarks and Halo's built-in apps can't have one.")
	}
	return nil
}

func (in hookInput) apply(hook *store.ApplicationHook, seal func([]byte) []byte, creating bool) error {
	in.URL, in.Secret = strings.TrimSpace(in.URL), strings.TrimSpace(in.Secret)
	if creating || in.URL != "" {
		u, err := PublicURL(in.URL, "hook", "Hooks only call")
		if err != nil {
			return err
		}
		hook.Host, hook.URLSealed = u.Host, seal([]byte(in.URL))
	}
	if creating || in.Secret != "" {
		if len(in.Secret) < 32 || len(in.Secret) > 256 {
			return httpx.Invalid("Enter a signing secret of 32 to 256 characters. Halo signs its requests with it, and the hook signs its answers with it.")
		}
		hook.SecretSealed = seal([]byte(in.Secret))
	}
	hook.Claims, hook.FailOpen = []string{}, in.FailOpen
	for _, name := range in.Claims {
		name = strings.TrimSpace(name)
		if !claimName.MatchString(name) || slices.Contains(reservedClaims, name) {
			return httpx.Invalid(fmt.Sprintf("A hook can't add %q. Use up to 100 letters, digits and _ . : / -, and none of the claims Halo sets itself, such as sub, iss, aud, exp, email, groups or roles.", name))
		}
		if !slices.Contains(hook.Claims, name) {
			hook.Claims = append(hook.Claims, name)
		}
	}
	if len(hook.Claims) > 20 {
		return httpx.Invalid("A hook can add up to 20 claims.")
	}
	return nil
}

func recordHook(r *http.Request, st *store.Store, actor store.User, action, summary string, app store.Application) error {
	return st.RecordAudit(r.Context(), store.AuditEvent{ActorID: &actor.ID, IP: httpx.ClientIP(r), Action: action, Summary: summary, TargetType: "application", TargetID: app.ID, TargetLabel: app.Name})
}

func (h *handler) getHook(w http.ResponseWriter, r *http.Request, _ store.User) error {
	ctx := r.Context()
	app, err := h.st.GetApplication(ctx, r.PathValue("id"))
	if err != nil {
		return err
	}
	hook, err := h.st.GetApplicationHook(ctx, app.ID)
	if errors.Is(err, store.ErrNotFound) {
		return httpx.JSON(w, http.StatusOK, nil)
	}
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, hook)
}

func (h *handler) logoutDeliveries(w http.ResponseWriter, r *http.Request, _ store.User) error {
	app, err := h.st.GetApplication(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	deliveries, err := h.st.ListWebhookDeliveries(r.Context(), app.ID, 50)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, deliveries)
}

func (h *handler) putHook(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in hookInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	var hook store.ApplicationHook
	err := h.st.Tx(ctx, func(tx *store.Store) error {
		app, err := tx.GetApplication(ctx, r.PathValue("id"))
		if err != nil {
			return err
		}
		if err := hookable(app); err != nil {
			return err
		}
		hook, err = tx.GetApplicationHook(ctx, app.ID)
		creating := errors.Is(err, store.ErrNotFound)
		if err != nil && !creating {
			return err
		}
		hook.AppID = app.ID
		if err := in.apply(&hook, tx.Sealer.Seal, creating); err != nil {
			return err
		}
		if err := tx.SaveApplicationHook(ctx, hook); err != nil {
			return err
		}
		mode := "fails closed"
		if hook.FailOpen {
			mode = "fails open"
		}
		summary := "Set the hook to call " + hook.Host + ". It " + mode
		if len(hook.Claims) > 0 {
			summary += " and can add " + strings.Join(hook.Claims, ", ")
		}
		return recordHook(r, tx, actor, "application.hook.update", summary+".", app)
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, hook)
}

func (h *handler) deleteHook(w http.ResponseWriter, r *http.Request, actor store.User) error {
	ctx := r.Context()
	err := h.st.Tx(ctx, func(tx *store.Store) error {
		app, err := tx.GetApplication(ctx, r.PathValue("id"))
		if err != nil {
			return err
		}
		hook, err := tx.GetApplicationHook(ctx, app.ID)
		if err != nil {
			return err
		}
		if err := tx.DeleteApplicationHook(ctx, app.ID); err != nil {
			return err
		}
		return recordHook(r, tx, actor, "application.hook.delete", "Removed the hook that called "+hook.Host+".", app)
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
