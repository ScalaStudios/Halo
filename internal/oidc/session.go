package oidc

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"halo/internal/access"
	"halo/internal/httpx"
	"halo/internal/id"
	"halo/internal/store"
)

const silentPrefix = "ars"

type requestKey struct{}

func (s *storage) silent(ctx context.Context, a *store.AuthRequest, app store.Application, hintSubject string) error {
	r, _ := ctx.Value(requestKey{}).(*http.Request)
	if r == nil {
		return oidc.ErrLoginRequired().WithDescription("The user is not signed in to Halo.")
	}
	u, _ := httpx.CurrentUser(r)
	sess, signedIn := httpx.CurrentSession(r)
	switch {
	case !signedIn:
		return oidc.ErrLoginRequired().WithDescription("The user is not signed in to Halo.")
	case hintSubject != "" && hintSubject != u.ID:
		return oidc.ErrLoginRequired().WithDescription("A different user is signed in to Halo than the one in id_token_hint.")
	case a.MaxAge != nil && time.Since(sess.CreatedAt) > time.Duration(*a.MaxAge)*time.Second:
		return oidc.ErrLoginRequired().WithDescription("The Halo session is older than max_age or the application's sign-in frequency allows.")
	}
	_, os, browser := store.ParseUserAgent(r.UserAgent())
	event := store.SignInEvent{Result: "failure", Method: sess.Method, UserID: &u.ID, Email: u.Email, AppID: &app.ID, IP: httpx.ClientIP(r), Device: browser + " · " + os}
	fail := func(reason string, e *oidc.Error) error {
		event.Reason = reason
		if err := s.st.RecordSignIn(ctx, event); err != nil {
			return err
		}
		return e
	}
	if !slices.Contains(u.AppIDs, app.ID) {
		return fail("User is not assigned to this application.", oidc.ErrInteractionRequired().WithDescription("The user is not assigned to this application."))
	}
	decision, err := s.policy.Evaluate(ctx, access.Input{User: u, App: &app, Method: sess.Method, IP: event.IP, UserAgent: r.UserAgent(), DeviceToken: httpx.DeviceToken(r), Session: &sess})
	if err != nil {
		return err
	}
	switch {
	case decision.Effect == access.Block:
		return fail(policyReason(decision), oidc.ErrInteractionRequired().WithDescription("The organization's access policy blocked this sign-in."))
	case decision.Effect == access.RequirePhishingResistant && sess.Method != "passkey" && sess.Method != "security-key":
		return fail(policyReason(decision), oidc.ErrLoginRequired().WithDescription("This application requires signing in with a passkey or security key."))
	case len(decision.Terms) > 0:
		return oidc.ErrInteractionRequired().WithDescription("The user must accept the organization's terms of use first.")
	}
	requested, err := s.st.RequestedAuthContexts(ctx, a.ACRValues)
	if err != nil {
		return err
	}
	acr, unmet := decision.StepUp(requested, sess)
	if unmet != nil {
		return oidc.ErrLoginRequired().WithDescription("The Halo session does not meet the authentication context %s.", unmet.ID)
	}
	needed, err := s.needsConsent(ctx, app, u.ID, *a)
	if err != nil {
		return err
	}
	if needed {
		return consentRequired()
	}
	a.ID, a.UserID, a.SessionID, a.AuthTime, a.AMR, a.ACR, a.Done = id.New(silentPrefix), &u.ID, &sess.ID, &sess.CreatedAt, sess.AMR(), acr, true
	event.Result = "success"
	return s.st.RecordSignIn(ctx, event)
}

func policyReason(decision access.Decision) string {
	if decision.Policy == "" {
		return decision.Reason
	}
	return "Policy “" + decision.Policy + "”: " + decision.Reason
}

func (s *storage) TerminateSessionFromRequest(ctx context.Context, req *op.EndSessionRequest) (string, error) {
	if err := s.TerminateSession(ctx, req.UserID, req.ClientID); err != nil {
		return "", err
	}
	sid := ""
	if req.IDTokenHintClaims != nil {
		if sid, _ = req.IDTokenHintClaims.Claims["sid"].(string); sid != "" {
			if err := s.st.RevokeSession(ctx, sid, req.UserID); err != nil && !errors.Is(err, store.ErrNotFound) {
				return "", err
			}
		}
	}
	q := url.Values{}
	confirm := req.IDTokenHintClaims == nil
	if r, ok := ctx.Value(requestKey{}).(*http.Request); ok {
		if current, signedIn := httpx.CurrentSession(r); signedIn && current.ID != sid {
			confirm = true
		}
	}
	if confirm {
		q.Set("confirm", "1")
	}
	if !strings.HasPrefix(req.RedirectURI, "/") {
		q.Set("next", req.RedirectURI)
	}
	if len(q) == 0 {
		return "/signed-out", nil
	}
	return "/signed-out?" + q.Encode(), nil
}

func (s *storage) logoutReturn(w http.ResponseWriter, r *http.Request) {
	next := r.URL.Query().Get("next")
	ok, err := s.postLogoutAllowed(r.Context(), next)
	if err != nil {
		http.Error(w, "Halo could not check where to send you after signing out. Go back to the application.", http.StatusInternalServerError)
		return
	}
	if !ok {
		next = "/signed-out"
	}
	http.Redirect(w, r, next, http.StatusFound)
}

func (s *storage) postLogoutAllowed(ctx context.Context, next string) (bool, error) {
	u, err := url.Parse(next)
	if err != nil || !u.IsAbs() {
		return false, nil
	}
	if q := u.Query(); q.Has("state") {
		q.Del("state")
		u.RawQuery = q.Encode()
	}
	apps, err := s.st.ListApplications(ctx)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(apps, func(app store.Application) bool {
		_, err := usable(app, nil)
		return err == nil && op.ValidateEndSessionPostLogoutRedirectURI(u.String(), client{app: app, dev: s.dev}) == nil
	}), nil
}
