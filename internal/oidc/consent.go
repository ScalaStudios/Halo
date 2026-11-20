package oidc

import (
	"context"
	"net/http"
	"net/url"
	"slices"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"halo/internal/store"
)

func consentRequired() *oidc.Error {
	return &oidc.Error{ErrorType: "consent_required", Description: "The user has not allowed this application to access the requested information."}
}

func (s *storage) needsConsent(ctx context.Context, app store.Application, userID string, a store.AuthRequest) (bool, error) {
	if slices.Contains(a.Prompt, oidc.PromptConsent) {
		return true, nil
	}
	if !app.RequiresConsent {
		return false, nil
	}
	granted, err := s.st.ConsentedScopes(ctx, userID, app.ID)
	return slices.ContainsFunc(a.Scopes, func(scope string) bool { return !slices.Contains(granted, scope) }), err
}

func (s *storage) awaitConsent(w http.ResponseWriter, r *http.Request, req store.AuthRequest, authorizer op.Authorizer) bool {
	if req.Consent != nil {
		if *req.Consent == "granted" {
			return false
		}
		_ = s.st.DeleteAuthRequest(r.Context(), req.ID)
		op.AuthRequestError(w, r, &authRequest{AuthRequest: req}, oidc.ErrAccessDenied().WithDescription("The user did not allow the application to access their information."), authorizer)
		return true
	}
	app, err := s.app(r.Context(), req.ClientID)
	needed := false
	if err == nil {
		needed, err = s.needsConsent(r.Context(), app, deref(req.UserID), req)
	}
	switch {
	case err != nil:
		http.Error(w, "Halo could not check what you allowed this application to access. Go back to the application and try again.", http.StatusInternalServerError)
	case !needed:
		return false
	case slices.Contains(req.Prompt, oidc.PromptNone):
		op.AuthRequestError(w, r, &authRequest{AuthRequest: req}, consentRequired(), authorizer)
	default:
		http.Redirect(w, r, "/sign-in?consent="+url.QueryEscape(req.ID), http.StatusFound)
	}
	return true
}
