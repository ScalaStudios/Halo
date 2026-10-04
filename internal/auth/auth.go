package auth

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"halo/internal/access"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/mail"
	"halo/internal/secret"
	"halo/internal/store"
)

var (
	errPasskeyRejected = httpx.Fail(http.StatusUnauthorized, "ERR_PASSKEY_REJECTED", "Halo could not verify that passkey. Try again, or sign in another way.")
	errSuspended       = httpx.Fail(http.StatusForbidden, "ERR_ACCOUNT_SUSPENDED", "This account is suspended. Contact your administrator to restore access.")
	errCodeMismatch    = httpx.Fail(http.StatusUnauthorized, "ERR_CODE_MISMATCH", "That code didn't match. Codes change every 30 seconds — enter the current one.")
	errRecoveryInvalid = httpx.Fail(http.StatusUnauthorized, "ERR_CODE_MISMATCH", "That recovery code didn't match or was already used. Check it and try another one.")
	errTooManyAttempts = httpx.Fail(http.StatusTooManyRequests, "ERR_TOO_MANY_ATTEMPTS", "Too many incorrect codes. Wait a few minutes, then try again.")
	errNotAssigned     = httpx.Fail(http.StatusForbidden, "ERR_NOT_ASSIGNED", "You don't have access to this application. Ask your administrator to assign it to you.")
	errRequestExpired  = httpx.Fail(http.StatusNotFound, "ERR_AUTH_REQUEST_EXPIRED", "This sign-in request expired or was already used. Go back to the application and sign in again.")
	errLinkExpired     = httpx.Fail(http.StatusNotFound, "ERR_LINK_EXPIRED", "This link has expired or was already used. Ask an administrator to send you a new one.")
	errCeremonyExpired = httpx.Fail(http.StatusBadRequest, "ERR_CEREMONY_EXPIRED", "This setup step expired. Start again.")
	errRegistration    = httpx.Fail(http.StatusBadRequest, "ERR_REGISTRATION_FAILED", "Halo could not verify the new passkey. Try again with the same device.")
	errReauthRequired  = httpx.Fail(http.StatusUnauthorized, "ERR_REAUTH_REQUIRED", "This application asks you to sign in again before continuing.")
	errBlockedByPolicy = httpx.Fail(http.StatusForbidden, "ERR_BLOCKED_BY_POLICY", "Your organization's access policy blocked this sign-in. If you think this is wrong, contact your administrator.")
	errStrongerAuth    = httpx.Fail(http.StatusForbidden, "ERR_STRONGER_AUTH_REQUIRED", "This application requires a passkey or security key. Sign in with one of those instead.")
	errLastMethod      = httpx.Fail(http.StatusConflict, "ERR_LAST_METHOD", "This is your last way to sign in, so it can't be removed. Add another passkey or authenticator app first.")
)

const notAssignedReason = "User is not assigned to this application."

type handler struct {
	st     *store.Store
	auth   *httpx.Auth
	wa     *webauthn.WebAuthn
	policy access.Engine
	mailer mail.Mailer
	cfg    config.Config
}

func Register(mux *http.ServeMux, d httpx.Deps) error {
	wa, err := webauthn.New(&webauthn.Config{
		RPID:                   d.Config.Hostname(),
		RPDisplayName:          "Halo",
		RPOrigins:              []string{d.Config.Issuer()},
		AuthenticatorSelection: protocol.AuthenticatorSelection{UserVerification: protocol.VerificationPreferred},
	})
	if err != nil {
		return err
	}
	policy := d.Policy
	if policy == nil {
		policy = access.AllowAll{}
	}
	mailer := d.Mailer
	if mailer == nil {
		mailer = mail.Log{}
	}
	h := &handler{st: d.Store, auth: d.Auth, wa: wa, policy: policy, mailer: mailer, cfg: d.Config}
	for pattern, fn := range map[string]httpx.HandlerFunc{
		"GET /api/v1/auth/request/{id}":          h.authRequest,
		"POST /api/v1/auth/passkey/begin":        h.passkeyBegin,
		"POST /api/v1/auth/passkey/finish":       h.passkeyFinish,
		"POST /api/v1/auth/identify":             h.identify,
		"POST /api/v1/auth/totp":                 h.totpSignIn,
		"POST /api/v1/auth/recovery":             h.recoverySignIn,
		"POST /api/v1/auth/continue":             h.continueRequest,
		"GET /api/v1/auth/enroll":                h.enrollInfo,
		"POST /api/v1/auth/enroll/begin":         h.enrollBegin,
		"POST /api/v1/auth/enroll/finish":        h.enrollFinish,
		"POST /api/v1/auth/sign-out":             h.signOut,
		"GET /api/v1/me":                         h.me,
		"POST /api/v1/me/passkeys/begin":         h.addPasskeyBegin,
		"POST /api/v1/me/passkeys/finish":        h.addPasskeyFinish,
		"DELETE /api/v1/me/methods/{id}":         h.removeMethod,
		"POST /api/v1/me/totp/begin":             h.totpBegin,
		"POST /api/v1/me/totp/confirm":           h.totpConfirm,
		"POST /api/v1/me/recovery-codes":         h.recoveryCodes,
		"GET /api/v1/me/sessions":                h.sessions,
		"DELETE /api/v1/me/sessions/{id}":        h.revokeSession,
		"POST /api/v1/me/sessions/revoke-others": h.revokeOtherSessions,
		"GET /api/v1/me/applications":            h.applications,
		"GET /api/v1/me/groups":                  h.groups,
		"GET /api/v1/me/sign-ins":                h.signIns,
		"POST /api/v1/auth/magic-link":           h.magicLink,
		"POST /api/v1/auth/magic-link/redeem":    h.redeemMagicLink,

		"GET /api/v1/auth/providers":                   h.providers,
		"GET /api/v1/auth/federated/{id}/start":        h.browser(h.federatedStart),
		"GET " + FederationCallbackPath:                h.browser(h.federatedCallback),
		"GET /api/v1/me/identities":                    h.myIdentities,
		"DELETE /api/v1/me/identities/{id}":            h.unlinkIdentity,
		"GET /api/v1/identity-providers":               h.listProviders,
		"POST /api/v1/identity-providers":              h.createProvider,
		"PUT /api/v1/identity-providers/{id}":          h.updateProvider,
		"DELETE /api/v1/identity-providers/{id}":       h.deleteProvider,
		"POST /api/v1/identity-providers/{id}/enable":  h.setProviderEnabled(true),
		"POST /api/v1/identity-providers/{id}/disable": h.setProviderEnabled(false),
	} {
		mux.Handle(pattern, httpx.Handle(fn))
	}
	if d.Config.Dev {
		mux.Handle("GET /api/v1/dev/outbox", httpx.Handle(h.devOutbox))
	}
	if d.Jobs != nil {
		d.Jobs.Every("retry undelivered email", time.Minute, mail.New(d.Config, d.Store).Retry)
	}
	return nil
}

type attempt struct {
	method  string
	email   string
	user    *store.User
	app     *store.Application
	request string
	eventID string
	risk    string
}

func (a attempt) event(r *http.Request, result, reason string) store.SignInEvent {
	_, os, browser := store.ParseUserAgent(r.UserAgent())
	e := store.SignInEvent{ID: a.eventID, Result: result, Method: a.method, Email: a.email, IP: httpx.ClientIP(r), Device: browser + " · " + os, Risk: a.risk, Reason: reason}
	if a.user != nil {
		e.UserID, e.Email = &a.user.ID, a.user.Email
	}
	if a.app != nil {
		e.AppID = &a.app.ID
	}
	return e
}

func (h *handler) reject(r *http.Request, a attempt, reason string, e *httpx.Error) error {
	if err := h.st.RecordSignIn(r.Context(), a.event(r, "failure", reason)); err != nil {
		return err
	}
	return e
}

func (h *handler) target(ctx context.Context, requestID string) (*store.Application, error) {
	if requestID == "" {
		return nil, nil
	}
	req, err := h.st.GetAuthRequest(ctx, requestID)
	if errors.Is(err, store.ErrNotFound) || err == nil && req.Done {
		return nil, errRequestExpired
	}
	if err != nil {
		return nil, err
	}
	app, err := h.st.GetApplicationByClientID(ctx, req.ClientID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errRequestExpired
	}
	return &app, err
}

func (h *handler) finish(w http.ResponseWriter, r *http.Request, a attempt, current *store.Session, extra func(*store.Store) error) error {
	u := *a.user
	if u.Status == "suspended" || u.Status == "deprovisioned" {
		return h.reject(r, a, "The account is suspended.", errSuspended)
	}
	if a.app != nil && !assigned(u, *a.app) {
		return h.reject(r, a, notAssignedReason, errNotAssigned)
	}
	ctx := r.Context()
	decision, err := h.policy.Evaluate(ctx, access.Input{User: u, App: a.app, Method: a.method, IP: httpx.ClientIP(r), UserAgent: r.UserAgent(), DeviceToken: httpx.DeviceToken(r), Session: current})
	if err != nil {
		return err
	}
	a.eventID, a.risk = decision.EventID, decision.Risk
	switch decision.Effect {
	case access.Block:
		return h.reject(r, a, policyReason(decision), errBlockedByPolicy)
	case access.RequirePhishingResistant:
		if a.method != "passkey" && a.method != "security-key" {
			return h.reject(r, a, policyReason(decision), errStrongerAuth)
		}
	}
	redirect, token := "/account", ""
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if extra != nil {
			if err := extra(tx); err != nil {
				return err
			}
		}
		sess := current
		if sess == nil {
			var device []byte
			if token := httpx.DeviceToken(r); token != "" {
				device = secret.Hash(token)
			}
			created, t, err := tx.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: a.method, IP: httpx.ClientIP(r), UserAgent: r.UserAgent(), DeviceHash: device})
			if err != nil {
				return err
			}
			sess, token = &created, t
			if err := tx.TouchSignIn(ctx, u.ID, created.CreatedAt); err != nil {
				return err
			}
		}
		if a.request != "" {
			if err := tx.CompleteAuthRequest(ctx, a.request, u.ID, sess.ID, amr(sess.Method), sess.CreatedAt); err != nil {
				return err
			}
			redirect = store.AuthorizeCallbackPath(a.request)
		}
		return tx.RecordSignIn(ctx, a.event(r, "success", ""))
	})
	if errors.Is(err, store.ErrNotFound) {
		return errRequestExpired
	}
	if err != nil {
		return err
	}
	if token != "" {
		h.auth.SetSession(w, token)
	}
	return httpx.JSON(w, http.StatusOK, map[string]string{"redirect": redirect})
}

func policyReason(d access.Decision) string {
	if d.Policy == "" {
		return d.Reason
	}
	return "Policy “" + d.Policy + "”: " + d.Reason
}

func assigned(u store.User, app store.Application) bool {
	return app.Status == "active" && slices.ContainsFunc(app.GroupIDs, func(g string) bool { return slices.Contains(u.GroupIDs, g) })
}

func amr(method string) []string {
	if method == "passkey" || method == "security-key" {
		return []string{"hwk", "mfa"}
	}
	if method == "federated" {
		return []string{"fed"}
	}
	return []string{"otp"}
}

func audit(ctx context.Context, st *store.Store, r *http.Request, u store.User, action, summary string) error {
	return st.RecordAudit(ctx, store.AuditEvent{ActorID: &u.ID, Action: action, Summary: summary, TargetType: "user", TargetID: u.ID, TargetLabel: u.Name, IP: httpx.ClientIP(r)})
}

func defaultLabel(kind, userAgent string) string {
	title := map[string]string{"passkey": "Passkey", "security-key": "Security key"}[kind]
	_, os, browser := store.ParseUserAgent(userAgent)
	name := strings.Fields(browser)[0]
	switch {
	case os == "Unknown":
		return title
	case name == "Unknown":
		return title + " · " + os
	}
	return title + " · " + name + " on " + os
}

func noContent(w http.ResponseWriter) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *handler) authRequest(w http.ResponseWriter, r *http.Request) error {
	app, err := h.target(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"application": map[string]string{"name": app.Name, "homepage": app.Homepage}})
}

func (h *handler) identify(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Email string `json:"email"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	u, err := h.st.GetUserByEmail(r.Context(), in.Email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	methods := []string{}
	if err == nil && u.Status != "suspended" && u.Status != "deprovisioned" {
		for _, kind := range []string{"passkey", "security-key", "totp", "recovery-codes"} {
			if slices.ContainsFunc(u.Methods, func(m store.Method) bool { return m.Kind == kind }) {
				methods = append(methods, kind)
			}
		}
	}
	if len(methods) == 0 {
		methods = []string{"passkey", "totp"}
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"methods": methods})
}

func (h *handler) continueRequest(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		AuthRequest string `json:"authRequest"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	u, sess, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	if in.AuthRequest == "" {
		return httpx.Invalid("authRequest is required. Start again from the application.")
	}
	app, err := h.target(r.Context(), in.AuthRequest)
	if err != nil {
		return err
	}
	req, err := h.st.GetAuthRequest(r.Context(), in.AuthRequest)
	if err != nil {
		return err
	}
	if slices.Contains(req.Prompt, "login") || req.MaxAge != nil && time.Since(sess.CreatedAt) > time.Duration(*req.MaxAge)*time.Second {
		return errReauthRequired
	}
	return h.finish(w, r, attempt{method: sess.Method, user: &u, app: app, request: in.AuthRequest}, &sess, nil)
}

func (h *handler) signOut(w http.ResponseWriter, r *http.Request) error {
	if sess, ok := httpx.CurrentSession(r); ok {
		if err := h.st.RevokeSession(r.Context(), sess.ID, ""); err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}
	h.auth.ClearSession(w)
	return noContent(w)
}
