package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"halo/internal/secret"
	"halo/internal/store"
)

type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Message
}

func Fail(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

var (
	ErrUnauthenticated = Fail(http.StatusUnauthorized, "ERR_UNAUTHENTICATED", "Sign in to continue. Your session may have expired.")
	ErrForbidden       = Fail(http.StatusForbidden, "ERR_FORBIDDEN", "Your role does not allow this change. Ask a global administrator for access.")
	ErrCrossSite       = Fail(http.StatusForbidden, "ERR_CROSS_SITE", "This request did not come from Halo, so it was blocked. Reload the page and try again.")
	ErrBadJSON         = Fail(http.StatusBadRequest, "ERR_INVALID_JSON", "The request body is not valid JSON.")
)

func NotFound(what string) *Error {
	return Fail(http.StatusNotFound, "ERR_NOT_FOUND", what+" was not found. It may have been deleted.")
}

func Invalid(message string) *Error {
	return Fail(http.StatusUnprocessableEntity, "ERR_INVALID", message)
}

type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

func Handle(fn HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := fn(w, r)
		if err == nil {
			return
		}
		var e *Error
		if !errors.As(err, &e) {
			if errors.Is(err, store.ErrNotFound) {
				e = NotFound("The item")
			} else if errors.Is(err, store.ErrConflict) {
				e = Fail(http.StatusConflict, "ERR_CONFLICT", "Something with the same name or address already exists.")
			} else {
				slog.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
				e = Fail(http.StatusInternalServerError, "ERR_INTERNAL", "Halo could not complete the request. Try again; if it keeps failing, check the server log.")
			}
		}
		JSON(w, e.Status, map[string]any{"error": e})
	}
}

func JSON(w http.ResponseWriter, status int, body any) error {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(body)
}

func Decode(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return ErrBadJSON
	}
	return nil
}

var trustedProxies atomic.Pointer[[]*net.IPNet]

func TrustProxies(cidrs []string) {
	nets := []*net.IPNet{}
	for _, cidr := range cidrs {
		if _, n, err := net.ParseCIDR(cidr); err == nil {
			nets = append(nets, n)
		}
	}
	trustedProxies.Store(&nets)
}

func trusted(ip net.IP) bool {
	nets := trustedProxies.Load()
	return ip != nil && nets != nil && slices.ContainsFunc(*nets, func(n *net.IPNet) bool { return n.Contains(ip) })
}

func ClientIP(r *http.Request) string {
	client, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		client = r.RemoteAddr
	}
	if !trusted(net.ParseIP(client)) {
		return client
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(hops[i]))
		if ip == nil {
			break
		}
		client = ip.String()
		if !trusted(ip) {
			break
		}
	}
	return client
}

type contextKey int

const (
	userKey contextKey = iota
	sessionKey
	deviceKey
	scopesKey
)

const (
	SessionCookie = "halo_session"
	DeviceCookie  = "halo_device"
)

type KeyResolver func(ctx context.Context, token string) (store.User, []string, error)

type AccessTokenResolver func(ctx context.Context, token string) (store.User, error)

type Auth struct {
	Store        *store.Store
	Public       *url.URL
	Dev          bool
	Keys         KeyResolver
	AccessTokens AccessTokenResolver
}

func (a *Auth) Resolve(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		device := ""
		if c, err := r.Cookie(DeviceCookie); err == nil && len(c.Value) >= 32 {
			device = c.Value
		} else {
			device = secret.Token(32)
			http.SetCookie(w, &http.Cookie{Name: DeviceCookie, Value: device, Path: "/", HttpOnly: true, Secure: !a.Dev, SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(2 * 365 * 24 * time.Hour)})
		}
		r = r.WithContext(context.WithValue(r.Context(), deviceKey, device))
		cookie, err := r.Cookie(SessionCookie)
		if err == nil && cookie.Value != "" {
			sess, err := a.Store.SessionByToken(r.Context(), cookie.Value)
			if err == nil {
				user, err := a.Store.GetUser(r.Context(), sess.UserID)
				if err == nil && user.Status != "suspended" && user.Status != "deprovisioned" {
					ctx := context.WithValue(r.Context(), userKey, user)
					ctx = context.WithValue(ctx, sessionKey, sess)
					r = r.WithContext(ctx)
				}
			}
		}
		if _, signedIn := CurrentUser(r); !signedIn && a.Keys != nil {
			if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && strings.HasPrefix(token, "hlk_") && !personalRoute(r.URL.Path) {
				user, scopes, err := a.Keys(r.Context(), token)
				if err == nil && user.Status == "active" {
					ctx := context.WithValue(r.Context(), userKey, user)
					r = r.WithContext(context.WithValue(ctx, scopesKey, scopes))
				}
			}
		}
		if _, signedIn := CurrentUser(r); !signedIn && a.AccessTokens != nil && strings.HasPrefix(r.URL.Path, "/api/v1/me/") {
			if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && token != "" && !strings.HasPrefix(token, "hlk_") {
				if user, err := a.AccessTokens(r.Context(), token); err == nil && user.Status == "active" {
					r = r.WithContext(context.WithValue(r.Context(), userKey, user))
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func personalRoute(path string) bool {
	return strings.HasPrefix(path, "/api/v1/me/") || path == "/api/v1/me" || strings.HasPrefix(path, "/api/v1/auth/")
}

func (a *Auth) KeyScope(scope string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if scopes, viaKey := KeyScopes(r); viaKey && !slices.Contains(scopes, scope) {
			ctx := context.WithValue(r.Context(), userKey, nil)
			r = r.WithContext(context.WithValue(ctx, scopesKey, nil))
		}
		next.ServeHTTP(w, r)
	})
}

func KeyScopes(r *http.Request) ([]string, bool) {
	scopes, ok := r.Context().Value(scopesKey).([]string)
	return scopes, ok
}

func (a *Auth) SetSession(w http.ResponseWriter, token string) {
	settings, err := a.Store.Settings(context.Background())
	if err != nil {
		settings = store.DefaultSettings
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   !a.Dev,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(settings.SessionLifetime()),
	})
}

func (a *Auth) ClearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: !a.Dev, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

func (a *Auth) SameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		site := r.Header.Get("Sec-Fetch-Site")
		allowed := site == "same-origin" || site == "none"
		if site == "" {
			origin := r.Header.Get("Origin")
			allowed = origin == "" || origin == a.Public.Scheme+"://"+a.Public.Host
		}
		if !allowed {
			Handle(func(http.ResponseWriter, *http.Request) error { return ErrCrossSite })(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func DeviceToken(r *http.Request) string {
	device, _ := r.Context().Value(deviceKey).(string)
	return device
}

func CurrentUser(r *http.Request) (store.User, bool) {
	u, ok := r.Context().Value(userKey).(store.User)
	return u, ok
}

func CurrentSession(r *http.Request) (store.Session, bool) {
	s, ok := r.Context().Value(sessionKey).(store.Session)
	return s, ok
}

func RequireUser(r *http.Request) (store.User, store.Session, error) {
	u, ok := CurrentUser(r)
	if !ok {
		return store.User{}, store.Session{}, ErrUnauthenticated
	}
	s, _ := CurrentSession(r)
	return u, s, nil
}

func RequireRole(r *http.Request, roles ...string) (store.User, error) {
	u, _, err := RequireUser(r)
	if err != nil {
		return store.User{}, err
	}
	if u.HasRole("global_admin") || u.HasRole(roles...) {
		return u, nil
	}
	return store.User{}, ErrForbidden
}

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if v := recover(); v != nil {
				slog.ErrorContext(r.Context(), "panic", "path", r.URL.Path, "panic", v)
				if rec.status == http.StatusOK {
					Handle(func(http.ResponseWriter, *http.Request) error { return errors.New("panic") })(rec, r)
				}
			}
			slog.InfoContext(r.Context(), "http", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration", time.Since(started).Round(time.Millisecond))
		}()
		next.ServeHTTP(rec, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}
