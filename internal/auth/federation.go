package auth

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"golang.org/x/oauth2"

	"halo/internal/httpx"
	"halo/internal/secret"
	"halo/internal/store"
)

const (
	FederationCallbackPath = "/api/v1/auth/federated/callback"
	federationCookie       = "halo_federation"
)

var (
	errProviderOff         = httpx.Fail(http.StatusNotFound, "ERR_PROVIDER_UNAVAILABLE", "This sign-in option is turned off. Choose another way to sign in.")
	errProviderUnreachable = httpx.Fail(http.StatusBadGateway, "ERR_PROVIDER_UNREACHABLE", "Halo could not reach the identity provider. Try again in a few minutes, or sign in another way.")
	errFederationState     = httpx.Fail(http.StatusBadRequest, "ERR_FEDERATION_STATE", "This sign-in attempt expired or was already used. Start again from the sign-in page.")
	errFederationCancelled = httpx.Fail(http.StatusUnauthorized, "ERR_FEDERATION_CANCELLED", "The identity provider did not sign you in. Try again, or choose another way to sign in.")
	errFederationFailed    = httpx.Fail(http.StatusUnauthorized, "ERR_FEDERATION_FAILED", "Halo could not verify the response from the identity provider. Try again; if it keeps failing, ask your administrator to check the provider settings.")
	errFederationEmail     = httpx.Fail(http.StatusForbidden, "ERR_FEDERATION_EMAIL", "The identity provider did not confirm your email address, so Halo could not find your account. Sign in another way, or ask your administrator for help.")
	errFederationDomain    = httpx.Fail(http.StatusForbidden, "ERR_FEDERATION_DOMAIN", "Your email domain is not allowed with this identity provider. Use your work account, or ask your administrator to allow your domain.")
	errFederationNoAccount = httpx.Fail(http.StatusForbidden, "ERR_FEDERATION_NO_ACCOUNT", "No Halo account matches that account. Ask your administrator to invite you, then sign in again.")
)

var federationClient = &http.Client{Timeout: 10 * time.Second}

type federatedProfile struct {
	subject, email, name string
	verified             bool
}

func (h *handler) providers(w http.ResponseWriter, r *http.Request) error {
	all, err := h.st.ListIdentityProviders(r.Context())
	if err != nil {
		return err
	}
	out := []map[string]string{}
	for _, p := range all {
		if p.Enabled && p.ShowOnSignIn {
			out = append(out, map[string]string{"id": p.ID, "name": p.Name, "kind": p.Kind})
		}
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h *handler) browser(fn func(http.ResponseWriter, *http.Request, *store.FederationState) (string, error)) httpx.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		var s store.FederationState
		location, err := fn(w, r, &s)
		if err != nil {
			var e *httpx.Error
			if !errors.As(err, &e) {
				slog.ErrorContext(r.Context(), "federated sign-in failed", "error", err)
				e = errFederationFailed
			}
			q := url.Values{"error": {e.Code}}
			if s.AuthRequest != "" {
				q.Set("authRequest", s.AuthRequest)
			}
			if s.Next != "" {
				q.Set("next", s.Next)
			}
			location = "/sign-in?" + q.Encode()
		}
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, location, http.StatusFound)
		return nil
	}
}

func (h *handler) federatedStart(w http.ResponseWriter, r *http.Request, s *store.FederationState) (string, error) {
	ctx := r.Context()
	q := r.URL.Query()
	s.AuthRequest, s.Next = q.Get("authRequest"), localPath(q.Get("next"))
	if _, err := h.target(ctx, s.AuthRequest); err != nil {
		return "", err
	}
	p, err := h.st.GetIdentityProvider(ctx, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) || err == nil && !p.Enabled {
		return "", errProviderOff
	}
	if err != nil {
		return "", err
	}
	party, err := h.relyingParty(ctx, p)
	if err != nil {
		slog.WarnContext(ctx, "identity provider unreachable", "provider", p.ID, "error", err)
		return "", errProviderUnreachable
	}
	state := secret.Token(32)
	s.ProviderID, s.CodeVerifier, s.Nonce = p.ID, secret.Token(32), secret.Token(16)
	if err := h.st.CreateFederationState(ctx, state, *s); err != nil {
		return "", err
	}
	http.SetCookie(w, h.federationCookie(state, int(store.FederationStateLifetime.Seconds())))
	opts := []rp.AuthURLOpt{rp.WithCodeChallenge(oidc.NewSHACodeChallenge(s.CodeVerifier))}
	if p.Kind != "github" {
		opts = append(opts, rp.AuthURLOpt(rp.WithURLParam("nonce", s.Nonce)))
	}
	return rp.AuthURL(state, party, opts...), nil
}

func (h *handler) federatedCallback(w http.ResponseWriter, r *http.Request, s *store.FederationState) (string, error) {
	ctx := r.Context()
	q := r.URL.Query()
	state := q.Get("state")
	cookie, _ := r.Cookie(federationCookie)
	http.SetCookie(w, h.federationCookie("", -1))
	if state == "" || cookie == nil || !secret.Equal([]byte(cookie.Value), []byte(state)) {
		return "", errFederationState
	}
	taken, err := h.st.TakeFederationState(ctx, state)
	if errors.Is(err, store.ErrNotFound) {
		return "", errFederationState
	}
	if err != nil {
		return "", err
	}
	*s = taken
	p, err := h.st.GetIdentityProvider(ctx, s.ProviderID)
	if errors.Is(err, store.ErrNotFound) || err == nil && !p.Enabled {
		return "", errProviderOff
	}
	if err != nil {
		return "", err
	}
	app, err := h.target(ctx, s.AuthRequest)
	if err != nil {
		return "", err
	}
	a := attempt{method: "federated", app: app, request: s.AuthRequest}
	if code := q.Get("error"); code != "" {
		return "", h.reject(r, a, p.Name+" returned "+code+".", errFederationCancelled)
	}
	profile, err := h.exchange(ctx, p, q.Get("code"), *s)
	if err != nil {
		slog.WarnContext(ctx, "federated sign-in not verified", "provider", p.ID, "error", err)
		return "", h.reject(r, a, "Halo could not verify the response from "+p.Name+".", errFederationFailed)
	}
	a.email = profile.email
	link, err := h.federatedUser(r, p, profile, &a)
	if err != nil {
		return "", err
	}
	verified := profile.verified && strings.EqualFold(profile.email, a.user.Email)
	rec := httptest.NewRecorder()
	if err := h.finish(rec, r, a, nil, func(tx *store.Store) error {
		if link != nil {
			if err := link(tx); err != nil {
				return err
			}
		}
		if verified {
			return tx.MarkEmailVerified(ctx, a.user.ID)
		}
		return nil
	}); err != nil {
		return "", err
	}
	for _, c := range rec.Header().Values("Set-Cookie") {
		w.Header().Add("Set-Cookie", c)
	}
	var out struct{ Redirect string }
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		return "", err
	}
	if s.AuthRequest == "" && s.Next != "" {
		return s.Next, nil
	}
	return out.Redirect, nil
}

func (h *handler) federatedUser(r *http.Request, p store.IdentityProvider, profile federatedProfile, a *attempt) (func(*store.Store) error, error) {
	ctx := r.Context()
	use := func(tx *store.Store) error {
		return tx.UseFederatedIdentity(ctx, p.ID, a.user.ID, profile.subject, profile.email)
	}
	linked, err := h.st.FindFederatedIdentity(ctx, p.ID, profile.subject)
	if err == nil {
		u, err := h.st.GetUser(ctx, linked.UserID)
		a.user = &u
		return use, err
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if !profile.verified || profile.email == "" {
		return nil, h.reject(r, *a, p.Name+" did not confirm the email address.", errFederationEmail)
	}
	if !domainAllowed(p.AllowedDomains, profile.email) {
		return nil, h.reject(r, *a, "The email domain is not allowed for "+p.Name+".", errFederationDomain)
	}
	u, err := h.st.GetUserByEmail(ctx, profile.email)
	if err == nil && u.Kind != "service" {
		a.user = &u
		return func(tx *store.Store) error {
			if err := use(tx); err != nil {
				return err
			}
			return audit(ctx, tx, r, u, "user.identity.link", u.Name+" linked "+p.Name+" ("+profile.email+")")
		}, nil
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if err == nil || !p.JIT || len(p.AllowedDomains) == 0 {
		return nil, h.reject(r, *a, "No Halo account uses "+profile.email+" and "+p.Name+" does not create accounts.", errFederationNoAccount)
	}
	created, err := h.provision(r, p, profile)
	a.user = &created
	return nil, err
}

func (h *handler) provision(r *http.Request, p store.IdentityProvider, profile federatedProfile) (store.User, error) {
	ctx := r.Context()
	groups, err := h.st.ListGroupsRaw(ctx)
	if err != nil {
		return store.User{}, err
	}
	groupIDs := []string{}
	for _, g := range groups {
		if g.Kind == "assigned" && slices.Contains(p.JITGroupIDs, g.ID) {
			groupIDs = append(groupIDs, g.ID)
		}
	}
	name := cmp.Or(profile.name, strings.Split(profile.email, "@")[0])
	var u store.User
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if u, err = tx.CreateUser(ctx, store.NewUser{Email: profile.email, Name: name, Status: "active", Source: p.Name, GroupIDs: groupIDs}); err != nil {
			return err
		}
		if err := tx.UseFederatedIdentity(ctx, p.ID, u.ID, profile.subject, profile.email); err != nil {
			return err
		}
		return tx.RecordAudit(ctx, store.AuditEvent{Action: "user.create", Summary: "Created on first sign-in with " + p.Name, TargetType: "user", TargetID: u.ID, TargetLabel: u.Name, IP: httpx.ClientIP(r)})
	})
	return u, err
}

func (h *handler) relyingParty(ctx context.Context, p store.IdentityProvider, opts ...rp.Option) (rp.RelyingParty, error) {
	clientSecret := ""
	if p.HasSecret {
		plain, err := h.st.Sealer.Open(p.SecretSealed)
		if err != nil {
			return nil, err
		}
		clientSecret = string(plain)
	}
	callback := h.cfg.Issuer() + FederationCallbackPath
	opts = append(opts, rp.WithHTTPClient(federationClient))
	if p.Kind == "github" {
		endpoint := oauth2.Endpoint{AuthURL: p.Issuer + "/login/oauth/authorize", TokenURL: p.Issuer + "/login/oauth/access_token"}
		return rp.NewRelyingPartyOAuth(&oauth2.Config{ClientID: p.ClientID, ClientSecret: clientSecret, RedirectURL: callback, Scopes: p.Scopes, Endpoint: endpoint}, opts...)
	}
	return rp.NewRelyingPartyOIDC(ctx, p.Issuer, p.ClientID, clientSecret, callback, p.Scopes, opts...)
}

func (h *handler) exchange(ctx context.Context, p store.IdentityProvider, code string, s store.FederationState) (federatedProfile, error) {
	party, err := h.relyingParty(ctx, p, rp.WithVerifierOpts(rp.WithNonce(func(context.Context) string { return s.Nonce })))
	if err != nil {
		return federatedProfile{}, err
	}
	tokens, err := rp.CodeExchange[*oidc.IDTokenClaims](ctx, code, party, rp.WithCodeVerifier(s.CodeVerifier))
	if err != nil {
		return federatedProfile{}, err
	}
	if p.Kind == "github" {
		return githubProfile(ctx, p.Issuer, tokens.AccessToken)
	}
	c := tokens.IDTokenClaims
	return federatedProfile{
		subject:  c.Subject,
		email:    c.Email,
		name:     cmp.Or(c.Name, strings.TrimSpace(c.GivenName+" "+c.FamilyName)),
		verified: bool(c.EmailVerified) || c.Claims["xms_edov"] == true,
	}, nil
}

func githubProfile(ctx context.Context, base, token string) (federatedProfile, error) {
	api := base + "/api/v3"
	if base == "https://github.com" {
		api = "https://api.github.com"
	}
	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	for path, dst := range map[string]any{"/user": &user, "/user/emails": &emails} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+path, nil)
		if err != nil {
			return federatedProfile{}, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		res, err := federationClient.Do(req)
		if err != nil {
			return federatedProfile{}, err
		}
		if res.StatusCode == http.StatusOK {
			err = json.NewDecoder(res.Body).Decode(dst)
		} else {
			err = fmt.Errorf("github %s: %s", path, res.Status)
		}
		res.Body.Close()
		if err != nil {
			return federatedProfile{}, err
		}
	}
	if user.ID == 0 {
		return federatedProfile{}, errors.New("github returned no user id")
	}
	profile := federatedProfile{subject: strconv.FormatInt(user.ID, 10), name: cmp.Or(user.Name, user.Login)}
	for _, e := range emails {
		if e.Primary && e.Verified {
			profile.email, profile.verified = e.Email, true
		}
	}
	return profile, nil
}

func (h *handler) federationCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{Name: federationCookie, Value: value, Path: "/api/v1/auth/federated", MaxAge: maxAge, HttpOnly: true, Secure: !h.auth.Dev, SameSite: http.SameSiteLaxMode}
}

func domainAllowed(domains []string, email string) bool {
	domain := email[strings.LastIndex(email, "@")+1:]
	return len(domains) == 0 || slices.ContainsFunc(domains, func(d string) bool { return strings.EqualFold(d, domain) })
}

func localPath(next string) string {
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.Contains(next, `\`) {
		return ""
	}
	return next
}
