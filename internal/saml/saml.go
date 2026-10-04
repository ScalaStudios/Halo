package saml

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/crewjam/saml"

	"halo/internal/access"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/store"
)

const rsaSHA256 = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"

var nameIDFormats = map[string]string{
	"email":       "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress",
	"persistent":  "urn:oasis:names:tc:SAML:2.0:nameid-format:persistent",
	"unspecified": "urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified",
}

var (
	_ saml.SessionProvider = (*flow)(nil)
	_ saml.AssertionMaker  = (*flow)(nil)
)

type handler struct {
	st     *store.Store
	cfg    config.Config
	policy access.Engine
}

func New(d httpx.Deps) (http.Handler, error) {
	h := &handler{st: d.Store, cfg: d.Config, policy: d.Policy}
	if h.policy == nil {
		h.policy = access.AllowAll{}
	}
	if d.Jobs != nil {
		d.Jobs.Every("purge expired SAML requests", time.Hour, d.Store.PurgeSAMLRequests)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /saml/metadata", h.metadata)
	mux.HandleFunc("GET /saml/certificate", h.certificate)
	mux.HandleFunc("GET /saml/sso", h.sso)
	mux.HandleFunc("POST /saml/sso", h.sso)
	mux.HandleFunc("GET /saml/resume", h.resume)
	mux.HandleFunc("GET /saml/launch/{appId}", h.launch)
	return mux, nil
}

type Connection struct {
	EntityID               string    `json:"entityId"`
	MetadataURL            string    `json:"metadataUrl"`
	SSOURL                 string    `json:"ssoUrl"`
	CertificateURL         string    `json:"certificateUrl"`
	CertificateFingerprint string    `json:"certificateFingerprint"`
	CertificateExpiresAt   time.Time `json:"certificateExpiresAt"`
}

func Describe(ctx context.Context, st *store.Store, cfg config.Config) (Connection, error) {
	_, cert, err := signingKey(ctx, st, cfg.Hostname())
	if err != nil {
		return Connection{}, err
	}
	metadata := endpoint(cfg, "metadata").String()
	return Connection{EntityID: metadata, MetadataURL: metadata, SSOURL: endpoint(cfg, "sso").String(), CertificateURL: endpoint(cfg, "certificate").String(),
		CertificateFingerprint: Fingerprint(cert.Raw), CertificateExpiresAt: cert.NotAfter}, nil
}

func Fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	hex := make([]string, len(sum))
	for i, b := range sum {
		hex[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(hex, ":")
}

func ParseMetadata(raw string) (string, []string, error) {
	var ed saml.EntityDescriptor
	if err := xml.Unmarshal([]byte(raw), &ed); err != nil || ed.EntityID == "" {
		return "", nil, errors.New("Halo couldn't read the metadata. Paste the complete <EntityDescriptor> document the application provides for its SAML service provider.")
	}
	var acs []string
	for _, sp := range ed.SPSSODescriptors {
		for _, service := range sp.AssertionConsumerServices {
			if service.Binding == saml.HTTPPostBinding {
				acs = append(acs, service.Location)
			}
		}
	}
	if len(acs) == 0 {
		return "", nil, errors.New("The metadata has no AssertionConsumerService with the HTTP-POST binding, which Halo uses to send SAML responses. Enter the ACS URL yourself instead.")
	}
	return ed.EntityID, acs, nil
}

func endpoint(cfg config.Config, name string) *url.URL {
	return cfg.PublicURL.JoinPath("saml", name)
}

func signingKey(ctx context.Context, st *store.Store, host string) (*rsa.PrivateKey, *x509.Certificate, error) {
	k, err := st.ActiveSAMLKey(ctx)
	if errors.Is(err, store.ErrNotFound) {
		if err := createKey(ctx, st, host); err != nil {
			return nil, nil, err
		}
		k, err = st.ActiveSAMLKey(ctx)
	}
	if err != nil {
		return nil, nil, err
	}
	parsed, err := x509.ParsePKCS8PrivateKey(k.PrivateKey)
	key, ok := parsed.(*rsa.PrivateKey)
	if err != nil || !ok {
		return nil, nil, fmt.Errorf("SAML key %s is not a readable RSA key: %v", k.ID, err)
	}
	cert, err := x509.ParseCertificate(k.Certificate)
	return key, cert, err
}

func createKey(ctx context.Context, st *store.Store, host string) error {
	der, cert, err := NewKey(host)
	if err != nil {
		return err
	}
	return st.CreateSAMLKey(ctx, der, cert)
}

func NewKey(host string) ([]byte, []byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	subject := &x509.Certificate{Subject: pkix.Name{CommonName: host}, NotBefore: now, NotAfter: now.AddDate(10, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, BasicConstraintsValid: true}
	cert, err := x509.CreateCertificate(rand.Reader, subject, subject, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	return der, cert, err
}

func (h *handler) provider(ctx context.Context) (*saml.IdentityProvider, error) {
	key, cert, err := signingKey(ctx, h.st, h.cfg.Hostname())
	if err != nil {
		return nil, err
	}
	return &saml.IdentityProvider{Key: key, Certificate: cert, MetadataURL: *endpoint(h.cfg, "metadata"), SSOURL: *endpoint(h.cfg, "sso"), ServiceProviderProvider: h, SignatureMethod: rsaSHA256}, nil
}

func (h *handler) GetServiceProvider(r *http.Request, entityID string) (*saml.EntityDescriptor, error) {
	app, err := h.st.GetApplicationByClientID(r.Context(), entityID)
	if errors.Is(err, store.ErrNotFound) || err == nil && app.Protocol != "saml" {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	return descriptor(app), nil
}

func descriptor(app store.Application) *saml.EntityDescriptor {
	acs := make([]saml.IndexedEndpoint, len(app.RedirectURIs))
	for i, uri := range app.RedirectURIs {
		acs[i] = saml.IndexedEndpoint{Binding: saml.HTTPPostBinding, Location: uri, Index: i}
	}
	return &saml.EntityDescriptor{EntityID: app.ClientID, SPSSODescriptors: []saml.SPSSODescriptor{{AssertionConsumerServices: acs}}}
}

func (h *handler) metadata(w http.ResponseWriter, r *http.Request) {
	idp, err := h.provider(r.Context())
	if err != nil {
		internal(w, r, err)
		return
	}
	md := idp.Metadata()
	md.ValidUntil = idp.Certificate.NotAfter
	md.IDPSSODescriptors[0].KeyDescriptors = md.IDPSSODescriptors[0].KeyDescriptors[:1]
	md.IDPSSODescriptors[0].NameIDFormats = []saml.NameIDFormat{saml.NameIDFormat(nameIDFormats["email"]), saml.NameIDFormat(nameIDFormats["persistent"]), saml.NameIDFormat(nameIDFormats["unspecified"])}
	body, err := xml.MarshalIndent(md, "", "  ")
	if err != nil {
		internal(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	_, _ = w.Write(body)
}

func (h *handler) certificate(w http.ResponseWriter, r *http.Request) {
	_, cert, err := signingKey(r.Context(), h.st, h.cfg.Hostname())
	if err != nil {
		internal(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="halo-saml-certificate.pem"`)
	_ = pem.Encode(w, &pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
}

func (h *handler) sso(w http.ResponseWriter, r *http.Request) {
	idp, err := h.provider(r.Context())
	if err != nil {
		internal(w, r, err)
		return
	}
	req, err := saml.NewIdpAuthnRequest(idp, r)
	if err == nil {
		err = req.Validate()
	}
	if err != nil {
		unreadable(w, err)
		return
	}
	h.respond(w, r, req)
}

func (h *handler) resume(w http.ResponseWriter, r *http.Request) {
	if _, signedIn := httpx.CurrentSession(r); !signedIn {
		redirectToSignIn(w, r, r.URL.RequestURI())
		return
	}
	pending, err := h.st.TakeSAMLRequest(r.Context(), r.URL.Query().Get("id"))
	if errors.Is(err, store.ErrNotFound) {
		page(w, http.StatusNotFound, "This sign-in request is no longer valid", "Requests from applications last 15 minutes and work once. Go back to the application and sign in again.", "")
		return
	}
	if err != nil {
		internal(w, r, err)
		return
	}
	idp, err := h.provider(r.Context())
	if err != nil {
		internal(w, r, err)
		return
	}
	req := &saml.IdpAuthnRequest{IDP: idp, HTTPRequest: r, RequestBuffer: pending.Request, RelayState: pending.RelayState, Now: pending.CreatedAt}
	if err := req.Validate(); err != nil {
		unreadable(w, err)
		return
	}
	req.Now = time.Now()
	h.respond(w, r, req)
}

func (h *handler) launch(w http.ResponseWriter, r *http.Request) {
	app, err := h.st.SAMLApplication(r.Context(), r.PathValue("appId"))
	if errors.Is(err, store.ErrNotFound) || err == nil && len(app.RedirectURIs) == 0 {
		page(w, http.StatusNotFound, "Halo can't open this application", "It may have been deleted, or it has no ACS URL yet. Open it from its own address, or ask your administrator.", "")
		return
	}
	if err != nil {
		internal(w, r, err)
		return
	}
	idp, err := h.provider(r.Context())
	if err != nil {
		internal(w, r, err)
		return
	}
	sp := descriptor(app)
	h.respond(w, r, &saml.IdpAuthnRequest{IDP: idp, HTTPRequest: r, Now: time.Now(), ServiceProviderMetadata: sp, SPSSODescriptor: &sp.SPSSODescriptors[0],
		ACSEndpoint: &sp.SPSSODescriptors[0].AssertionConsumerServices[0]})
}

func (h *handler) respond(w http.ResponseWriter, r *http.Request, req *saml.IdpAuthnRequest) {
	f := &flow{handler: h}
	session := f.GetSession(w, r, req)
	if session == nil {
		return
	}
	if err := f.MakeAssertion(req, session); err != nil {
		internal(w, r, err)
		return
	}
	form, err := req.PostBinding()
	if err == nil {
		err = h.st.RecordSignIn(r.Context(), f.event(r, "success", ""))
	}
	if err != nil {
		internal(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = pages.ExecuteTemplate(w, "form", form)
}

func (h *handler) signIn(w http.ResponseWriter, r *http.Request, req *saml.IdpAuthnRequest) {
	next := r.URL.RequestURI()
	if req.RequestBuffer != nil {
		requestID, err := h.st.CreateSAMLRequest(r.Context(), store.SAMLRequest{Request: req.RequestBuffer, RelayState: req.RelayState, CreatedAt: req.Now})
		if err != nil {
			internal(w, r, err)
			return
		}
		next = "/saml/resume?id=" + requestID
	}
	redirectToSignIn(w, r, next)
}

type flow struct {
	*handler
	app      store.Application
	user     store.User
	session  store.Session
	settings store.SAMLSettings
}

func (f *flow) GetSession(w http.ResponseWriter, r *http.Request, req *saml.IdpAuthnRequest) *saml.Session {
	ctx := r.Context()
	var signedIn bool
	if f.session, signedIn = httpx.CurrentSession(r); !signedIn {
		f.signIn(w, r, req)
		return nil
	}
	f.user, _ = httpx.CurrentUser(r)
	var err error
	if f.app, err = f.st.GetApplicationByClientID(ctx, req.ServiceProviderMetadata.EntityID); err != nil {
		internal(w, r, err)
		return nil
	}
	if f.app.Status != "active" {
		f.refuse(w, r, "The application is disabled.", f.app.Name+" is turned off", "Your administrator turned off sign-in to "+f.app.Name+". Contact them if you still need it.", "")
		return nil
	}
	if !slices.Contains(f.user.AppIDs, f.app.ID) {
		f.refuse(w, r, "User is not assigned to this application.", "You don't have access to "+f.app.Name, "Ask your administrator to assign "+f.app.Name+" to you, then open it again.", "")
		return nil
	}
	decision, err := f.policy.Evaluate(ctx, access.Input{User: f.user, App: &f.app, Method: f.session.Method, IP: httpx.ClientIP(r), UserAgent: r.UserAgent(), DeviceToken: httpx.DeviceToken(r), Session: &f.session})
	if err != nil {
		internal(w, r, err)
		return nil
	}
	reason := decision.Reason
	if decision.Policy != "" {
		reason = "Policy “" + decision.Policy + "”: " + decision.Reason
	}
	strong := f.session.Method == "passkey" || f.session.Method == "security-key"
	switch {
	case decision.Effect == access.Block:
		f.refuse(w, r, reason, "Your organization's access policy blocked this sign-in", "If you think this is wrong, contact your administrator and mention the policy below.", reason)
		return nil
	case decision.Effect == access.RequirePhishingResistant && !strong:
		f.refuse(w, r, reason, f.app.Name+" needs a passkey or security key", "Sign out of Halo, sign in again with a passkey or security key, then open "+f.app.Name+" again.", reason)
		return nil
	}
	if f.settings, err = f.st.SAMLSettings(ctx, f.app.ID); err != nil {
		internal(w, r, err)
		return nil
	}
	attributes, err := f.attributes(ctx)
	if err != nil {
		internal(w, r, err)
		return nil
	}
	nameID := f.user.Email
	if f.settings.NameIDFormat == "persistent" {
		nameID = f.user.ID
	}
	return &saml.Session{ID: f.session.ID, CreateTime: f.session.CreatedAt, ExpireTime: f.session.ExpiresAt, Index: f.session.ID,
		NameID: nameID, NameIDFormat: nameIDFormats[f.settings.NameIDFormat], CustomAttributes: attributes}
}

func (f *flow) MakeAssertion(req *saml.IdpAuthnRequest, session *saml.Session) error {
	if err := (saml.DefaultAssertionMaker{}).MakeAssertion(req, session); err != nil {
		return err
	}
	ip := httpx.ClientIP(req.HTTPRequest)
	req.Assertion.Subject.SubjectConfirmations[0].SubjectConfirmationData.Address = ip
	req.Assertion.AuthnStatements[0].SubjectLocality.Address = ip
	if len(session.CustomAttributes) == 0 {
		req.Assertion.AttributeStatements = nil
	}
	if err := req.MakeResponse(); err != nil {
		return err
	}
	if !f.settings.SignResponse {
		req.ResponseEl.RemoveChild(req.ResponseEl.SelectElement("Signature"))
	}
	return nil
}

func (f *flow) attributes(ctx context.Context) ([]saml.Attribute, error) {
	all, err := f.st.ListGroupsRaw(ctx)
	if err != nil {
		return nil, err
	}
	var groups []string
	for _, g := range all {
		if slices.Contains(f.user.GroupIDs, g.ID) {
			groups = append(groups, g.Name)
		}
	}
	given, family, _ := strings.Cut(strings.TrimSpace(f.user.Name), " ")
	names := f.settings.Attributes
	var out []saml.Attribute
	for _, a := range []struct {
		name   string
		values []string
	}{
		{names.Email, []string{f.user.Email}},
		{names.Name, []string{f.user.Name}},
		{names.GivenName, []string{given}},
		{names.FamilyName, []string{strings.TrimSpace(family)}},
		{names.Groups, groups},
	} {
		values := slices.DeleteFunc(a.values, func(v string) bool { return v == "" })
		if a.name == "" || len(values) == 0 {
			continue
		}
		attr := saml.Attribute{Name: a.name, NameFormat: "urn:oasis:names:tc:SAML:2.0:attrname-format:basic"}
		if strings.Contains(a.name, ":") {
			attr.NameFormat = "urn:oasis:names:tc:SAML:2.0:attrname-format:uri"
		}
		for _, v := range values {
			attr.Values = append(attr.Values, saml.AttributeValue{Type: "xs:string", Value: v})
		}
		out = append(out, attr)
	}
	return out, nil
}

func (f *flow) refuse(w http.ResponseWriter, r *http.Request, reason, title, message, detail string) {
	if err := f.st.RecordSignIn(r.Context(), f.event(r, "failure", reason)); err != nil {
		internal(w, r, err)
		return
	}
	page(w, http.StatusForbidden, title, message, detail)
}

func (f *flow) event(r *http.Request, result, reason string) store.SignInEvent {
	_, system, browser := store.ParseUserAgent(r.UserAgent())
	return store.SignInEvent{UserID: &f.user.ID, Email: f.user.Email, AppID: &f.app.ID, Result: result, Method: f.session.Method, IP: httpx.ClientIP(r), Device: browser + " · " + system, Reason: reason}
}

func redirectToSignIn(w http.ResponseWriter, r *http.Request, next string) {
	http.Redirect(w, r, "/sign-in?next="+url.QueryEscape(next), http.StatusSeeOther)
}

func unreadable(w http.ResponseWriter, err error) {
	detail := err.Error()
	if strings.HasPrefix(detail, "cannot find assertion consumer service") {
		detail = "The ACS URL in the request is not one of the ACS URLs registered for this application in Halo."
	}
	page(w, http.StatusBadRequest, "Halo couldn't read this sign-in request", "Go back to the application and sign in again. If this keeps happening, ask your administrator to compare the application's SAML settings with Halo.", detail)
}

func internal(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "saml request failed", "path", r.URL.Path, "error", err)
	page(w, http.StatusInternalServerError, "Halo couldn't finish signing you in", "Try again in a minute. If it keeps failing, contact your administrator.", "")
}

func page(w http.ResponseWriter, status int, title, message, detail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = pages.ExecuteTemplate(w, "page", struct{ Title, Message, Detail string }{title, message, detail})
}

var pages = template.Must(template.New("").Parse(`{{define "head"}}<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>{{.}} · Halo</title>
<style>
:root{color-scheme:dark}
body{margin:0;min-height:100vh;box-sizing:border-box;display:grid;place-items:center;padding:64px 20px;background:#111111;color:#a8a29b;font:15px/1.6 "Inter Variable",Inter,-apple-system,"Segoe UI",Roboto,Helvetica,Arial,sans-serif}
main,form{display:flex;flex-direction:column;align-items:center;gap:24px;max-width:448px;text-align:center}
.logo{display:inline-flex;align-items:center;gap:12px;color:#ffffff;font:700 22px/1.26 "Archivo Variable",Archivo,"Helvetica Neue",Arial,sans-serif;letter-spacing:-0.01em}
.logo svg{width:32px;height:32px}
.copy{display:flex;flex-direction:column;gap:8px}
h1{margin:0;color:#ffffff;font:700 30px/1.18 "Archivo Variable",Archivo,"Helvetica Neue",Arial,sans-serif;letter-spacing:-0.015em}
p{margin:0}
code{display:block;padding:8px 12px;border:1px solid #2a2724;border-radius:8px;background:#0c0b0a;color:#d3d0cc;font:12px/1.5 "JetBrains Mono",Menlo,Consolas,monospace;overflow-wrap:anywhere}
a,button{display:inline-flex;align-items:center;height:40px;padding:0 24px;border:0;border-radius:8px;background:#fa7e26;color:#111111;font-family:inherit;font-size:15px;font-weight:600;line-height:1.2;text-decoration:none;cursor:pointer;transition:background-color 160ms cubic-bezier(0.2,0.8,0.2,1)}
a:hover,button:hover{background:#ffa05a}
a:focus-visible,button:focus-visible{outline:2px solid #fa7e26;outline-offset:2px}
</style>
</head>
<body>
{{end}}{{define "logo"}}<span class="logo"><svg viewBox="0 0 32 32" aria-hidden="true"><defs><linearGradient id="halo" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#F7AD31"/><stop offset="0.48" stop-color="#FA7E26"/><stop offset="1" stop-color="#FC4F1B"/></linearGradient></defs><path d="M18 29.351A13.5 13.5 0 1 0 14 29.351L14 23.228A7.5 7.5 0 1 1 18 23.228Z" fill="url(#halo)"/></svg>Halo</span>{{end}}{{define "page"}}{{template "head" .Title}}<main>
{{template "logo"}}
<div class="copy">
<h1>{{.Title}}</h1>
<p>{{.Message}}</p>
</div>
{{if .Detail}}<code>{{.Detail}}</code>
{{end}}<a href="/account">Go to your account</a>
</main>
</body>
</html>
{{end}}{{define "form"}}{{template "head" "Signing in"}}<form method="post" action="{{.URL}}">
{{template "logo"}}
<p>Signing you in…</p>
<input type="hidden" name="SAMLResponse" value="{{.SAMLResponse}}">
{{if .RelayState}}<input type="hidden" name="RelayState" value="{{.RelayState}}">
{{end}}<noscript><button type="submit">Continue</button></noscript>
</form>
<script>document.forms[0].submit()</script>
</body>
</html>
{{end}}`))
