package oidc

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"halo/internal/httpx"
	"halo/internal/secret"
	"halo/internal/store"
)

const requestURIPrefix = "urn:ietf:params:oauth:request_uri:"

type pushedKey struct{}

func (s *storage) pushAuthRequest(provider *op.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		requestURI, err := s.push(r.WithContext(op.ContextWithIssuer(r.Context(), s.issuer)), provider)
		var oidcErr *oidc.Error
		if err != nil && !errors.As(err, &oidcErr) {
			slog.ErrorContext(r.Context(), "pushed authorization request failed", "error", err)
			err = oidc.ErrServerError().WithDescription("Halo could not save the authorization request. Try again; if it keeps failing, check the server log.")
		}
		if err != nil {
			op.RequestError(w, r, err, nil)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		_ = httpx.JSON(w, http.StatusCreated, map[string]any{"request_uri": requestURI, "expires_in": int(store.PushedRequestLifetime.Seconds())})
	}
}

func (s *storage) push(r *http.Request, provider *op.Provider) (string, error) {
	ctx := r.Context()
	clientID, authenticated, err := op.ClientIDFromRequest(r, provider)
	if err != nil {
		return "", err
	}
	form := r.PostForm
	if !authenticated && form.Get("client_secret") != "" {
		if err := s.AuthorizeClientIDSecret(ctx, clientID, form.Get("client_secret")); err != nil {
			return "", oidc.ErrInvalidClient().WithDescription("The client secret is wrong or expired.")
		}
		authenticated = true
	}
	app, err := s.app(ctx, clientID)
	if err != nil {
		return "", err
	}
	c, err := s.client(ctx, app)
	if err != nil {
		return "", err
	}
	switch {
	case !authenticated && !c.public():
		return "", oidc.ErrInvalidClient().WithDescription("Authenticate the pushed authorization request with the application's client secret.")
	case form.Get("client_id") != "" && form.Get("client_id") != clientID:
		return "", oidc.ErrInvalidRequest().WithDescription("client_id in the body must match the authenticated application.")
	case form.Has("request_uri"):
		return "", oidc.ErrInvalidRequest().WithDescription("Leave request_uri out of a pushed authorization request.")
	case form.Has("request"):
		return "", oidc.ErrRequestNotSupported()
	}
	params := url.Values{}
	for key, values := range form {
		if key != "client_secret" && !strings.HasPrefix(key, "client_assertion") {
			params[key] = values
		}
	}
	params.Set("client_id", clientID)
	req := new(oidc.AuthRequest)
	if err := provider.Decoder().Decode(req, params); err != nil {
		return "", oidc.ErrInvalidRequest().WithDescription("The pushed authorization request has malformed parameters.")
	}
	if _, err := op.ValidateAuthRequestClient(ctx, req, c, provider.IDTokenHintVerifier(ctx)); err != nil {
		return "", err
	}
	if err := checkPKCE(app, req); err != nil {
		return "", err
	}
	value := secret.Token(32)
	if err := s.st.PushAuthRequest(ctx, secret.Hash(value), clientID, params.Encode()); err != nil {
		return "", err
	}
	return requestURIPrefix + value, nil
}

func (s *storage) pushedAuthRequest(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	value, ok := strings.CutPrefix(r.Form.Get("request_uri"), requestURIPrefix)
	params, err := s.st.TakePushedAuthRequest(r.Context(), secret.Hash(value), r.Form.Get("client_id"))
	switch {
	case !ok || errors.Is(err, store.ErrNotFound):
		http.Error(w, "This sign-in request expired or was already used. Go back to the application and start again.", http.StatusBadRequest)
		return nil, false
	case err != nil:
		http.Error(w, "Halo could not load this sign-in request. Go back to the application and try again.", http.StatusInternalServerError)
		return nil, false
	}
	next := r.Clone(r.Context())
	next.Method, next.Body, next.ContentLength, next.Form, next.PostForm = http.MethodGet, http.NoBody, 0, nil, nil
	next.URL.RawQuery = params
	return next.WithContext(context.WithValue(next.Context(), pushedKey{}, true)), true
}
