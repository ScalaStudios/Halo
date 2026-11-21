package forwardauth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"unicode"

	"halo/internal/access"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/saml"
	"halo/internal/secret"
	"halo/internal/store"
)

const (
	CallbackPath = "/_halo/callback"
	SignOutPath  = "/_halo/sign-out"
	StateCookie  = "__Host-halo_state"
)

func CookieName(appID string) string {
	return "__Host-halo_" + appID
}

type handler struct {
	st     *store.Store
	cfg    config.Config
	policy access.Engine
}

func New(d httpx.Deps) http.Handler {
	h := &handler{st: d.Store, cfg: d.Config, policy: d.Policy}
	if h.policy == nil {
		h.policy = access.AllowAll{}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /forward-auth/{id}/verify", h.verify)
	mux.Handle("GET /forward-auth/{id}/start", d.Auth.Resolve(http.HandlerFunc(h.start)))
	return mux
}

func (h *handler) verify(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Cache-Control", "no-store")
	app, err := h.st.ForwardAuthApplication(ctx, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "Halo has no forward-auth application with this ID. Copy the address from the application's Overview tab in the Halo console.", http.StatusNotFound)
		return
	}
	if err != nil {
		failed(w, r, err)
		return
	}
	origin, uri := original(r, app)
	if origin == "" {
		http.Error(w, "Halo can't tell which address of "+app.Name+" this request is for. The proxy must send X-Forwarded-Uri, and X-Forwarded-Host when the application has more than one protected address, from an address in HALO_TRUSTED_PROXIES.", http.StatusForbidden)
		return
	}
	cookie := CookieName(app.ID)
	switch path, query, _ := strings.Cut(uri, "?"); path {
	case CallbackPath:
		values, _ := url.ParseQuery(query)
		token, returnTo, expires, err := h.st.ExchangeForwardAuthCode(ctx, app.ID, values.Get("code"), value(r, StateCookie), origin)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "This sign-in link has expired, was already used, or was opened in another browser. Open "+origin+" again.", http.StatusForbidden)
			return
		}
		if err != nil {
			failed(w, r, err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: cookie, Value: token, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: expires})
		redirect(w, r, returnTo)
		return
	case SignOutPath:
		if token := value(r, cookie); token != "" {
			if err := h.st.EndForwardAuthSession(ctx, app.ID, token); err != nil {
				failed(w, r, err)
				return
			}
		}
		http.SetCookie(w, &http.Cookie{Name: cookie, Value: "", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
		redirect(w, r, h.cfg.Issuer()+"/signed-out")
		return
	}
	sess, user, err := h.session(ctx, app.ID, value(r, cookie))
	if errors.Is(err, store.ErrNotFound) || err == nil && app.ReauthRequired(sess.CreatedAt) {
		state := value(r, StateCookie)
		if len(state) < 32 || len(state) > 64 {
			state = secret.Token(32)
		}
		http.SetCookie(w, &http.Cookie{Name: StateCookie, Value: state, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 900})
		redirect(w, r, h.cfg.Issuer()+"/forward-auth/"+url.PathEscape(app.ID)+"/start?"+url.Values{"rd": {origin + uri}, "state": {state}}.Encode())
		return
	}
	if err != nil {
		failed(w, r, err)
		return
	}
	if app.Status != "active" || !slices.Contains(user.AppIDs, app.ID) {
		http.Error(w, user.Email+" doesn't have access to "+app.Name+". Ask your administrator to assign it to you.", http.StatusForbidden)
		return
	}
	groups, err := h.groups(ctx, user)
	if err != nil {
		failed(w, r, err)
		return
	}
	for _, claim := range app.Claims {
		w.Header().Set(claim.Name, headerValue(user.ClaimValue(claim.Source, groups, app.RoleKeys(user))))
	}
	w.WriteHeader(http.StatusOK)
}

func headerValue(value any) string {
	values := slices.Clone(store.ClaimStrings(value))
	if _, text := value.(string); !text {
		for i := range values {
			values[i] = strings.ReplaceAll(values[i], ",", " ")
		}
	}
	return strings.Map(func(c rune) rune {
		if unicode.IsControl(c) {
			return -1
		}
		return c
	}, strings.Join(values, ","))
}

func (h *handler) session(ctx context.Context, appID, token string) (store.Session, store.User, error) {
	if token == "" {
		return store.Session{}, store.User{}, store.ErrNotFound
	}
	sess, err := h.st.ForwardAuthSession(ctx, appID, token)
	if err != nil {
		return sess, store.User{}, err
	}
	user, err := h.st.GetUser(ctx, sess.UserID)
	if err == nil && (user.Status == "suspended" || user.Status == "deprovisioned") {
		err = store.ErrNotFound
	}
	return sess, user, err
}

func (h *handler) groups(ctx context.Context, user store.User) ([]string, error) {
	all, err := h.st.ListGroupsRaw(ctx)
	var names []string
	for _, g := range all {
		if slices.Contains(user.GroupIDs, g.ID) {
			names = append(names, g.Name)
		}
	}
	return names, err
}

func (h *handler) start(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app, err := h.st.ForwardAuthApplication(ctx, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		saml.Page(w, http.StatusNotFound, "Halo can't open this application", "It may have been deleted. Open it from your account portal, or ask your administrator.", "")
		return
	}
	if err != nil {
		internal(w, r, err)
		return
	}
	returnTo, origin := target(app, r.URL.Query().Get("rd"))
	state := r.URL.Query().Get("state")
	if returnTo == "" || len(state) < 32 || len(state) > 64 {
		saml.Page(w, http.StatusBadRequest, "Halo won't send you to this address", "It isn't one of the addresses of "+app.Name+". Open "+app.Name+" from its own address or from your account portal.", "")
		return
	}
	sess, signedIn := httpx.CurrentSession(r)
	if !signedIn || app.ReauthRequired(sess.CreatedAt) {
		http.Redirect(w, r, "/sign-in?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
		return
	}
	user, _ := httpx.CurrentUser(r)
	_, system, browser := store.ParseUserAgent(r.UserAgent())
	event := store.SignInEvent{UserID: &user.ID, Email: user.Email, AppID: &app.ID, Result: "failure", Method: sess.Method, IP: httpx.ClientIP(r), Device: browser + " · " + system}
	refuse := func(reason, title, message, detail string) {
		event.Reason = reason
		if err := h.st.RecordSignIn(ctx, event); err != nil {
			internal(w, r, err)
			return
		}
		saml.Page(w, http.StatusForbidden, title, message, detail)
	}
	if app.Status != "active" {
		refuse("The application is disabled.", app.Name+" is turned off", "Your administrator turned off sign-in to "+app.Name+". Contact them if you still need it.", "")
		return
	}
	if !slices.Contains(user.AppIDs, app.ID) {
		refuse("User is not assigned to this application.", "You don't have access to "+app.Name, "Ask your administrator to assign "+app.Name+" to you, then open it again.", "")
		return
	}
	decision, err := h.policy.Evaluate(ctx, access.Input{User: user, App: &app, Method: sess.Method, IP: event.IP, UserAgent: r.UserAgent(), DeviceToken: httpx.DeviceToken(r), Session: &sess})
	if err != nil {
		internal(w, r, err)
		return
	}
	reason := decision.Reason
	if decision.Policy != "" {
		reason = "Policy “" + decision.Policy + "”: " + decision.Reason
	}
	switch {
	case decision.Effect == access.Block:
		refuse(reason, "Your organization's access policy blocked this sign-in", "If you think this is wrong, contact your administrator and mention the policy below.", reason)
		return
	case decision.Effect == access.RequirePhishingResistant && sess.Method != "passkey" && sess.Method != "security-key":
		refuse(reason, app.Name+" needs a passkey or security key", "Sign out of Halo, sign in again with a passkey or security key, then open "+app.Name+" again.", reason)
		return
	case len(decision.Terms) > 0:
		http.Redirect(w, r, "/account/terms?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
		return
	}
	code, err := h.st.CreateForwardAuthCode(ctx, app.ID, sess.ID, state, returnTo)
	if err == nil {
		event.Result = "success"
		err = h.st.RecordSignIn(ctx, event)
	}
	if err != nil {
		internal(w, r, err)
		return
	}
	http.Redirect(w, r, origin+CallbackPath+"?code="+url.QueryEscape(code), http.StatusSeeOther)
}

func original(r *http.Request, app store.Application) (string, string) {
	uri := httpx.ProxyHeader(r, "X-Forwarded-Uri")
	if !strings.HasPrefix(uri, "/") || strings.ContainsFunc(uri, unicode.IsControl) {
		return "", ""
	}
	host := httpx.ProxyHeader(r, "X-Forwarded-Host")
	if origin := originOf(app, "", host); origin != "" || len(app.RedirectURIs) != 1 {
		return origin, uri
	}
	u, err := url.Parse(app.RedirectURIs[0])
	if err != nil {
		return "", ""
	}
	return u.Scheme + "://" + u.Host, uri
}

func target(app store.Application, raw string) (string, string) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.User != nil || u.Opaque != "" || strings.ContainsFunc(raw, func(c rune) bool { return c == '\\' || unicode.IsControl(c) }) {
		return "", ""
	}
	origin := originOf(app, u.Scheme, u.Host)
	if origin == "" {
		return "", ""
	}
	return origin + (&url.URL{Path: u.Path, RawPath: u.RawPath, RawQuery: u.RawQuery}).RequestURI(), origin
}

func originOf(app store.Application, scheme, host string) string {
	for _, raw := range app.RedirectURIs {
		if u, err := url.Parse(raw); err == nil && host != "" && (scheme == "" || u.Scheme == scheme) && strings.EqualFold(u.Host, host) {
			return u.Scheme + "://" + u.Host
		}
	}
	return ""
}

func value(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}

func redirect(w http.ResponseWriter, r *http.Request, to string) {
	w.Header().Set("Location", to)
	if r.URL.Query().Has("nginx") {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusFound)
}

func failed(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "forward-auth check failed", "path", r.URL.Path, "error", err)
	http.Error(w, "Halo could not check this request. Try again; if it keeps failing, check the Halo server log.", http.StatusInternalServerError)
}

func internal(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "forward-auth sign-in failed", "path", r.URL.Path, "error", err)
	saml.Page(w, http.StatusInternalServerError, "Halo couldn't finish signing you in", "Try again in a minute. If it keeps failing, contact your administrator.", "")
}
