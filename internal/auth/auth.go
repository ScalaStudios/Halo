package auth

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"halo/internal/access"
	"halo/internal/approval"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/mail"
	"halo/internal/secret"
	"halo/internal/store"
	"halo/internal/webhooks"
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
	errSessionLimit    = httpx.Fail(http.StatusForbidden, "ERR_SESSION_LIMIT", "You're already signed in on as many devices as your organization allows. Sign out on a device you no longer use, or ask your administrator to end one of your sessions, then try again.")
	errTermsExpired    = httpx.Fail(http.StatusNotFound, "ERR_TERMS_EXPIRED", "This step expired or was already used. Sign in again to read and accept the terms.")
	errPassMismatch    = httpx.Fail(http.StatusUnauthorized, "ERR_CODE_MISMATCH", "That temporary access pass didn't match, expired or was already used. Check it, or ask your administrator for a new one.")
	mismatches         = []*httpx.Error{errPasskeyRejected, errCodeMismatch, errRecoveryInvalid, errPassMismatch, errPasswordMismatch, errEmailCodeMismatch}
)

const notAssignedReason = "User is not assigned to this application."

type handler struct {
	st     *store.Store
	auth   *httpx.Auth
	wa     *webauthn.WebAuthn
	policy access.Engine
	mailer mail.Mailer
	cfg    config.Config
	gate   *approval.Gate
	trust  *trustList
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
	h := &handler{st: d.Store, auth: d.Auth, wa: wa, policy: policy, mailer: mailer, cfg: d.Config, gate: approval.New(d), trust: &trustList{}}
	for pattern, fn := range map[string]httpx.HandlerFunc{
		"GET /api/v1/auth/request/{id}":          h.authRequest,
		"POST /api/v1/auth/passkey/begin":        h.passkeyBegin,
		"POST /api/v1/auth/passkey/finish":       h.passkeyFinish,
		"POST /api/v1/auth/identify":             h.identify,
		"POST /api/v1/auth/totp":                 h.totpSignIn,
		"POST /api/v1/auth/recovery":             h.recoverySignIn,
		"POST /api/v1/auth/access-pass":          h.accessPassSignIn,
		"POST /api/v1/auth/password":             h.passwordSignIn,
		"POST /api/v1/auth/certificate":          h.certificateSignIn,
		"POST /api/v1/auth/kerberos":             h.kerberosSignIn,
		"POST /api/v1/auth/continue":             h.continueRequest,
		"GET /api/v1/auth/terms/{ceremony}":      h.pendingTerms,
		"POST /api/v1/auth/terms":                h.acceptTerms,
		"GET /api/v1/auth/consent/{id}":          h.showConsent,
		"POST /api/v1/auth/consent":              h.decideConsent,
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
		"POST /api/v1/me/reauthenticate/begin":   h.reauthBegin,
		"POST /api/v1/me/reauthenticate":         h.reauthenticate,
		"GET /api/v1/me/password":                h.myPassword,
		"GET /api/v1/me/passkey-rules":           h.myPasskeyRules,
		"PUT /api/v1/me/password":                h.setMyPassword,
		"GET /api/v1/me/sessions":                h.sessions,
		"DELETE /api/v1/me/sessions/{id}":        h.revokeSession,
		"POST /api/v1/me/sessions/revoke-others": h.revokeOtherSessions,
		"GET /api/v1/me/security-emails":         h.securityEmails,
		"PUT /api/v1/me/security-emails":         h.setSecurityEmails,
		"PUT /api/v1/me/locale":                  h.setLocale,
		"GET /api/v1/me/applications":            h.applications,
		"PUT /api/v1/me/applications":            h.arrangeApplications,
		"GET /api/v1/me/collections":             h.collections,
		"GET /api/v1/me/groups":                  h.groups,
		"GET /api/v1/me/sign-ins":                h.signIns,
		"GET /api/v1/me/consents":                h.consents,
		"DELETE /api/v1/me/consents/{id}":        h.revokeConsent,
		"POST /api/v1/auth/magic-link":           h.magicLink,
		"POST /api/v1/auth/magic-link/redeem":    h.redeemMagicLink,
		"POST /api/v1/auth/magic-link/code":      h.emailCodeSignIn,
		"POST /api/v1/auth/sign-up":              h.signUp,
		"GET /api/v1/auth/sign-up":               h.signUpInfo,
		"POST /api/v1/auth/sign-up/begin":        h.signUpBegin,
		"POST /api/v1/auth/sign-up/finish":       h.signUpFinish,

		"POST /api/v1/auth/enroll/identity-proof":  h.enrollProof,
		"POST /api/v1/auth/sign-up/identity-proof": h.signUpProof,

		"GET /api/v1/auth/providers":                   h.providers,
		"GET /api/v1/auth/federated/{id}/start":        h.browser(h.federatedStart),
		"GET " + FederationCallbackPath:                h.browser(h.federatedCallback),
		"POST /api/v1/auth/federated/{id}/acs":         h.browser(h.samlCallback),
		"GET /api/v1/me/identities":                    h.myIdentities,
		"DELETE /api/v1/me/identities/{id}":            h.unlinkIdentity,
		"GET /api/v1/identity-providers":               h.listProviders,
		"POST /api/v1/identity-providers":              h.createProvider,
		"PUT /api/v1/identity-providers/{id}":          h.updateProvider,
		"DELETE /api/v1/identity-providers/{id}":       h.deleteProvider,
		"POST /api/v1/identity-providers/{id}/enable":  h.setProviderEnabled(true),
		"POST /api/v1/identity-providers/{id}/disable": h.setProviderEnabled(false),
		"GET /api/v1/certificate-authorities":          h.listCertificateAuthorities,
		"POST /api/v1/certificate-authorities":         h.createCertificateAuthority,
		"DELETE /api/v1/certificate-authorities/{id}":  h.deleteCertificateAuthority,
		"GET /api/v1/kerberos":                         h.getKerberos,
		"PUT /api/v1/kerberos":                         h.saveKerberos,
		"DELETE /api/v1/kerberos":                      h.deleteKerberos,
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
	method       string
	secondFactor bool
	email        string
	user         *store.User
	app          *store.Application
	request      string
	eventID      string
	risk         string
	challenge    string
	hook         webhooks.Verdict
	mismatch     bool
}

func (a attempt) event(r *http.Request, result, reason string) store.SignInEvent {
	_, os, browser := store.ParseUserAgent(r.UserAgent())
	return a.eventFrom(httpx.ClientIP(r), browser+" · "+os, result, reason)
}

func (a attempt) eventFrom(ip, device, result, reason string) store.SignInEvent {
	e := store.SignInEvent{ID: a.eventID, Result: result, Method: a.method, Email: a.email, IP: ip, Device: device, Risk: a.risk, Reason: cmp.Or(reason, a.hook.Log), Challenge: a.challenge, Hook: a.hook.Decision, Mismatch: a.mismatch}
	if a.user != nil {
		e.UserID, e.Email = &a.user.ID, a.user.Email
	}
	if a.app != nil {
		e.AppID = &a.app.ID
	}
	return e
}

func (h *handler) reject(r *http.Request, a attempt, reason string, e *httpx.Error) error {
	a.mismatch = slices.Contains(mismatches, e)
	if err := h.fail(r.Context(), a, a.event(r, "failure", reason)); err != nil {
		return err
	}
	return e
}

func (h *handler) fail(ctx context.Context, a attempt, e store.SignInEvent) error {
	if err := h.st.RecordSignIn(ctx, e); err != nil {
		return err
	}
	if a.user != nil && slices.Contains(guessedMethods, a.method) {
		go h.notifyLocked(context.WithoutCancel(ctx), *a.user)
	}
	return nil
}

func (h *handler) notifyLocked(ctx context.Context, u store.User) {
	settings, err := h.st.Settings(ctx)
	if err != nil {
		slog.WarnContext(ctx, "lockout email not sent", "user", u.ID, "error", err)
		return
	}
	failures, err := h.st.RecentFailures(ctx, u.ID, u.Email, guessedMethods, lockoutStart(settings))
	if err != nil || failures < settings.LockoutThreshold {
		return
	}
	until := time.Now().Add(time.Duration(settings.LockoutMinutes) * time.Minute)
	msgs := []mail.Message{mail.AccountLocked(u.Email, u.Name, settings.LockoutThreshold, until, h.cfg.Issuer()+"/account/activity")}
	if settings.LockoutEmailAdmins {
		admins, err := h.st.GlobalAdminEmails(ctx, u.ID)
		if err != nil {
			slog.WarnContext(ctx, "lockout email to administrators not sent", "user", u.ID, "error", err)
		}
		for _, to := range admins {
			msgs = append(msgs, mail.AccountLockedAdmin(to, u.Name, u.Email, settings.LockoutThreshold, until, h.cfg.Issuer()+"/admin/users/"+u.ID))
		}
	}
	mail.SendSecurity(ctx, h.mailer, h.st, u.ID, "", msgs...)
}

func (h *handler) notify(r *http.Request, userID, subject string, msgs ...mail.Message) {
	go mail.SendSecurity(context.WithoutCancel(r.Context()), h.mailer, h.st, userID, subject, msgs...)
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
		if a.method != "passkey" && a.method != "security-key" && a.method != "access-pass" {
			return h.reject(r, a, policyReason(decision), errStrongerAuth)
		}
	}
	var req store.AuthRequest
	var requested []store.AuthContext
	if a.app != nil {
		if req, err = h.st.GetAuthRequest(ctx, a.request); errors.Is(err, store.ErrNotFound) {
			return errRequestExpired
		}
		if err != nil {
			return err
		}
		if requested, err = h.st.RequestedAuthContexts(ctx, req.ACRValues); err != nil {
			return err
		}
	}
	probe := store.Session{Method: a.method, SecondFactor: a.secondFactor, CreatedAt: time.Now()}
	if current != nil {
		probe = *current
	}
	acr, unmet := decision.StepUp(requested, probe)
	switch {
	case unmet == nil || a.method == "access-pass":
	case current != nil:
		return stepUpRequired(*unmet)
	default:
		return h.reject(r, a, "The sign-in does not meet the authentication context “"+unmet.Name+"”.", stepUpRequired(*unmet))
	}
	if a.app != nil && len(decision.Terms) == 0 {
		if a.hook, err = webhooks.RunHook(ctx, h.st, *a.app, webhooks.HookCall{Stage: webhooks.HookSignIn, User: u, Scopes: req.Scopes}); err != nil {
			return err
		}
		if a.hook.Denied() {
			return h.reject(r, a, a.hook.Log, httpx.Fail(http.StatusForbidden, "ERR_BLOCKED_BY_HOOK", a.hook.Reason))
		}
	}
	if a.method == "access-pass" {
		return h.enrollWithPass(w, r, a, extra)
	}
	if missing, err := h.unproven(ctx, u); err != nil {
		return err
	} else if missing {
		return h.reject(r, a, "The account is waiting for an identity check.", errProofRequired)
	}
	redirect, token, ceremony := "/account", "", ""
	emergency := false
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if extra != nil {
			if err := extra(tx); err != nil {
				return err
			}
		}
		if len(decision.Terms) > 0 {
			c := termsCeremony{Method: a.method, SecondFactor: a.secondFactor, Request: a.request, Terms: decision.Terms}
			if current != nil {
				c.SessionID = current.ID
			}
			var err error
			ceremony, err = tx.CreateCeremony(ctx, "terms", &u.ID, c)
			return err
		}
		sess := current
		if sess == nil {
			var device []byte
			if token := httpx.DeviceToken(r); token != "" {
				device = secret.Hash(token)
			}
			replacing := ""
			if prev, ok := httpx.CurrentSession(r); ok && prev.UserID == u.ID {
				replacing = prev.ID
			}
			if err := tx.MakeRoomForSession(ctx, u, false, replacing); err != nil {
				return err
			}
			created, t, err := tx.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: a.method, SecondFactor: a.secondFactor, IP: httpx.ClientIP(r), UserAgent: r.UserAgent(), DeviceHash: device, AppOnly: decision.AppOnly})
			if err != nil {
				return err
			}
			sess, token = &created, t
			if err := tx.TouchSignIn(ctx, u.ID, created.CreatedAt); err != nil {
				return err
			}
			if emergency, err = h.emergencySignIn(ctx, tx, r, u); err != nil {
				return err
			}
		}
		if a.request != "" {
			if err := tx.CompleteAuthRequest(ctx, a.request, u.ID, sess.ID, sess.AMR(), sess.CreatedAt, acr); err != nil {
				return err
			}
			redirect = store.AuthorizeCallbackPath(a.request)
		}
		return tx.RecordSignIn(ctx, a.event(r, "success", ""))
	})
	if errors.Is(err, store.ErrSessionLimit) {
		return h.reject(r, a, store.SessionLimitReason, errSessionLimit)
	}
	if errors.Is(err, store.ErrProofRequired) {
		return h.reject(r, a, "The account is waiting for an identity check.", errProofRequired)
	}
	if errors.Is(err, store.ErrNotFound) {
		return errRequestExpired
	}
	if err != nil {
		return err
	}
	if ceremony != "" {
		return httpx.JSON(w, http.StatusOK, map[string]string{"redirect": "/sign-in?terms=" + ceremony, "terms": ceremony})
	}
	if emergency {
		h.gate.Alert(ctx, u, u.Name+" signed in with an emergency access account", u.Name+" ("+u.Email+") signed in to Halo with an emergency access account from "+httpx.ClientIP(r)+
			". Access policies don't block emergency access accounts, so use them only when no one else can sign in.\n\nIf you did not expect this, check the sign-in log now: "+h.cfg.Issuer()+"/admin/sign-ins")
	}
	if token != "" {
		h.auth.SetSession(w, token)
		if slices.ContainsFunc(decision.Signals, func(s string) bool { return s == "new-device" || s == "new-network" }) {
			appName := ""
			if a.app != nil {
				appName = a.app.Name
			}
			e := a.event(r, "success", "")
			h.notify(r, u.ID, e.IP, mail.NewSignIn(u.Email, u.Name, appName, e.Device, e.IP, h.cfg.Issuer()+"/account/sessions", time.Now()))
		}
	}
	return httpx.JSON(w, http.StatusOK, map[string]string{"redirect": redirect})
}

func stepUpRequired(c store.AuthContext) *httpx.Error {
	return httpx.Fail(http.StatusForbidden, "ERR_STEP_UP_REQUIRED", "This sign-in needs to meet “"+c.Name+"”. "+c.SignInAgain())
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

func (h *handler) emergencySignIn(ctx context.Context, tx *store.Store, r *http.Request, u store.User) (bool, error) {
	org, err := h.st.Settings(ctx)
	if err != nil || !slices.Contains(org.EmergencyAccessIDs, u.ID) {
		return false, err
	}
	return true, audit(ctx, tx, r, u, "emergency_access.sign_in", u.Name+" signed in with an emergency access account")
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
	ctx := r.Context()
	app, err := h.target(ctx, r.PathValue("id"))
	if err != nil {
		return err
	}
	req, err := h.st.GetAuthRequest(ctx, r.PathValue("id"))
	if err != nil {
		return err
	}
	out := map[string]any{"application": map[string]any{"name": app.Name, "homepage": app.Homepage, "logoUrl": app.LogoURL, "kerberosSignIn": app.KerberosSignIn}, "loginHint": req.LoginHint}
	provider, err := h.route(ctx, cmp.Or(req.DomainHint, req.LoginHint))
	if err != nil {
		return err
	}
	if provider != nil {
		out["provider"] = provider
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h *handler) identify(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Email string `json:"email"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	provider, err := h.route(r.Context(), in.Email)
	if err != nil {
		return err
	}
	if provider != nil {
		return httpx.JSON(w, http.StatusOK, map[string]any{"methods": []string{}, "provider": provider})
	}
	u, err := h.st.GetUserByEmail(r.Context(), in.Email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	methods := []string{}
	if err == nil && u.Status != "suspended" && u.Status != "deprovisioned" {
		settings, err := h.st.Settings(r.Context())
		if err != nil {
			return err
		}
		for _, kind := range []string{"access-pass", "passkey", "security-key", "totp", "password", "recovery-codes"} {
			if slices.ContainsFunc(u.Methods, func(m store.Method) bool { return m.Kind == kind }) && (kind != "password" || settings.PasswordAllowed(u)) {
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
