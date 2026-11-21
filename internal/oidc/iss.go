package oidc

import (
	"bytes"
	"context"
	"html/template"
	"net/http"
	"net/url"

	"github.com/zitadel/oidc/v3/pkg/op"

	"halo/internal/store"
)

var formPostPage = template.Must(template.New("form_post").Parse(`<!doctype html><html><head><meta charset="utf-8"></head><body onload="document.forms[0].submit()"><form method="post" action="{{.Action}}">{{range $name, $values := .Params}}{{range $values}}<input type="hidden" name="{{$name}}" value="{{.}}">{{end}}{{end}}<noscript><button type="submit">Continue</button></noscript></form></body></html>`))

type issuedResponse struct {
	http.ResponseWriter
	issuer   string
	redirect *url.URL
}

func withIssuer(w http.ResponseWriter, issuer, redirectURI string) http.ResponseWriter {
	u, err := url.Parse(redirectURI)
	if err != nil || redirectURI == "" {
		return w
	}
	return &issuedResponse{w, issuer, u}
}

func (w *issuedResponse) WriteHeader(code int) {
	loc, err := url.Parse(w.Header().Get("Location"))
	if code >= 300 && code < 400 && err == nil && loc.Scheme == w.redirect.Scheme && loc.Opaque == w.redirect.Opaque && loc.Host == w.redirect.Host && loc.Path == w.redirect.Path {
		if params, err := url.ParseQuery(loc.Fragment); err == nil && loc.Fragment != "" {
			params.Set("iss", w.issuer)
			loc.Fragment, loc.RawFragment = "", ""
			w.Header().Set("Location", loc.String()+"#"+params.Encode())
		} else {
			query := loc.Query()
			query.Set("iss", w.issuer)
			loc.RawQuery = query.Encode()
			w.Header().Set("Location", loc.String())
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

func (s *storage) formPost(w http.ResponseWriter, r *http.Request, req store.AuthRequest, authorizer op.Authorizer) bool {
	ctx := r.Context()
	authReq, err := s.wrap(ctx, req, nil)
	if err != nil || !authReq.Done() {
		return false
	}
	page, err := s.formPostPage(ctx, authReq, authorizer)
	if err != nil {
		op.AuthRequestError(w, r, authReq, err, authorizer)
		return true
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = page.WriteTo(w)
	return true
}

func (s *storage) formPostPage(ctx context.Context, authReq op.AuthRequest, authorizer op.Authorizer) (*bytes.Buffer, error) {
	if _, err := s.GetClientByClientID(ctx, authReq.GetClientID()); err != nil {
		return nil, err
	}
	resp, err := op.BuildAuthResponseCodeResponsePayload(ctx, authReq, authorizer)
	if err != nil {
		return nil, err
	}
	params := url.Values{"code": {resp.Code}, "iss": {s.issuer}}
	if resp.State != "" {
		params.Set("state", resp.State)
	}
	var page bytes.Buffer
	return &page, formPostPage.Execute(&page, map[string]any{"Action": authReq.GetRedirectURI(), "Params": params})
}
