package saml

import (
	"cmp"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/beevik/etree"
	"github.com/crewjam/saml"

	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/store"
)

var metadataClient = httpx.GuardedClient()

type Profile struct {
	Subject string
	Email   string
	Name    string
	Claims  map[string]any
}

func serviceProvider(ctx context.Context, st *store.Store, cfg config.Config, p store.IdentityProvider) (*saml.ServiceProvider, error) {
	var idp saml.EntityDescriptor
	if err := xml.Unmarshal([]byte(p.SAMLMetadata), &idp); err != nil {
		return nil, err
	}
	key, cert, err := signingKey(ctx, st, cfg.Hostname())
	if err != nil {
		return nil, err
	}
	base := cfg.PublicURL.JoinPath("saml", "sp", p.ID)
	metadata := base.JoinPath("metadata")
	entityID := metadata.String()
	return &saml.ServiceProvider{
		EntityID:          entityID,
		Key:               key,
		Certificate:       cert,
		MetadataURL:       *metadata,
		AcsURL:            *base.JoinPath("acs"),
		IDPMetadata:       &idp,
		SignatureMethod:   rsaSHA256,
		AuthnNameIDFormat: saml.UnspecifiedNameIDFormat,
		ValidateAudienceRestriction: func(a *saml.Assertion) error {
			if len(a.Conditions.AudienceRestrictions) == 0 {
				return errors.New("the assertion has no audience restriction")
			}
			for _, restriction := range a.Conditions.AudienceRestrictions {
				if restriction.Audience.Value != entityID {
					return fmt.Errorf("the assertion is for %q, not %q", restriction.Audience.Value, entityID)
				}
			}
			return nil
		},
	}, nil
}

func singleSignOn(md *saml.EntityDescriptor) (string, string) {
	sp := saml.ServiceProvider{IDPMetadata: md}
	for _, binding := range []string{saml.HTTPRedirectBinding, saml.HTTPPostBinding} {
		if location := sp.GetSSOBindingLocation(binding); location != "" {
			return binding, location
		}
	}
	return "", ""
}

func Begin(ctx context.Context, w http.ResponseWriter, st *store.Store, cfg config.Config, p store.IdentityProvider, relayState string, save func(requestID string) error) (string, error) {
	sp, err := serviceProvider(ctx, st, cfg, p)
	if err != nil {
		return "", err
	}
	binding, location := singleSignOn(sp.IDPMetadata)
	req, err := sp.MakeAuthenticationRequest(location, binding, saml.HTTPPostBinding)
	if err != nil {
		return "", err
	}
	if err := save(req.ID); err != nil {
		return "", err
	}
	if binding == saml.HTTPRedirectBinding {
		u, err := req.Redirect(relayState, sp)
		if err != nil {
			return "", err
		}
		return u.String(), nil
	}
	doc := etree.NewDocument()
	doc.SetRoot(req.Element())
	raw, err := doc.WriteToBytes()
	if err != nil {
		return "", err
	}
	post(w, location, map[string]string{"SAMLRequest": base64.StdEncoding.EncodeToString(raw), "RelayState": relayState})
	return "", nil
}

func SignIn(ctx context.Context, st *store.Store, cfg config.Config, p store.IdentityProvider, response, requestID string) (Profile, error) {
	if requestID == "" {
		return Profile{}, errors.New("the sign-in has no SAML request to answer")
	}
	sp, err := serviceProvider(ctx, st, cfg, p)
	if err != nil {
		return Profile{}, err
	}
	raw, err := base64.StdEncoding.DecodeString(response)
	if err != nil {
		return Profile{}, err
	}
	a, err := sp.ParseXMLResponse(raw, []string{requestID}, sp.AcsURL)
	if invalid := (*saml.InvalidResponseError)(nil); errors.As(err, &invalid) {
		err = invalid.PrivateErr
	}
	if err != nil {
		return Profile{}, err
	}
	if a.Subject.NameID == nil || len(a.Subject.SubjectConfirmations) == 0 {
		return Profile{}, errors.New("the assertion has no NameID or no subject confirmation")
	}
	if err := st.UseSAMLAssertion(ctx, p.ID, a.ID, a.Conditions.NotOnOrAfter.Add(saml.MaxClockSkew)); errors.Is(err, store.ErrConflict) {
		return Profile{}, fmt.Errorf("assertion %s was already used", a.ID)
	} else if err != nil {
		return Profile{}, err
	}
	claims := map[string]any{}
	for _, statement := range a.AttributeStatements {
		for _, attr := range statement.Attributes {
			values := []any{}
			for _, v := range attr.Values {
				values = append(values, v.Value)
			}
			var claim any = values
			if len(values) == 1 {
				claim = values[0]
			}
			for _, key := range []string{attr.Name, attr.FriendlyName} {
				if _, taken := claims[key]; key != "" && !taken {
					claims[key] = claim
				}
			}
		}
	}
	text := func(key string) string {
		value, _ := claims[key].(string)
		return strings.TrimSpace(value)
	}
	email := strings.TrimSpace(a.Subject.NameID.Value)
	if p.EmailAttribute != "" {
		email = text(p.EmailAttribute)
	}
	if !strings.Contains(email, "@") {
		email = ""
	}
	subject := a.Subject.NameID.Value
	if a.Subject.NameID.Format == string(saml.TransientNameIDFormat) {
		subject = email
	}
	if subject == "" {
		return Profile{}, errors.New("the assertion names no lasting subject")
	}
	name := cmp.Or(text("displayName"), text("http://schemas.microsoft.com/identity/claims/displayname"), text("cn"), strings.TrimSpace(text("givenName")+" "+text("sn")))
	return Profile{Subject: subject, Email: email, Name: name, Claims: claims}, nil
}

func IdentityProviderEntityID(raw string, dev bool) (string, error) {
	var md saml.EntityDescriptor
	if err := xml.Unmarshal([]byte(raw), &md); err != nil || md.EntityID == "" || len(md.IDPSSODescriptors) == 0 {
		return "", errors.New("Halo couldn't read the metadata. Use the identity provider's <EntityDescriptor> document, which has an IDPSSODescriptor.")
	}
	if _, location := singleSignOn(&md); !httpx.ValidIssuer(strings.SplitN(location, "?", 2)[0], dev) {
		return "", errors.New("The metadata needs a SingleSignOnService with the HTTP-Redirect or HTTP-POST binding at an https:// address.")
	}
	signing := false
	for _, d := range md.IDPSSODescriptors {
		for _, k := range d.KeyDescriptors {
			for _, c := range k.KeyInfo.X509Data.X509Certificates {
				cert, err := parseCertificate(c.Data)
				signing = signing || (k.Use == "" || k.Use == "signing") && err == nil && time.Now().Before(cert.NotAfter)
			}
		}
	}
	if !signing {
		return "", errors.New("The metadata needs a signing certificate that has not expired. Ask the identity provider for its current metadata.")
	}
	return md.EntityID, nil
}

func ManualMetadata(entityID, location, certificate string) (string, error) {
	if entityID == "" {
		return "", errors.New("Enter the identity provider's entity ID, from its SAML settings or metadata.")
	}
	cert, err := parseCertificate(certificate)
	if err != nil {
		return "", errors.New("Halo couldn't read the signing certificate. Paste the identity provider's X.509 certificate in PEM format, starting with -----BEGIN CERTIFICATE-----.")
	}
	key := saml.KeyDescriptor{Use: "signing", KeyInfo: saml.KeyInfo{X509Data: saml.X509Data{X509Certificates: []saml.X509Certificate{{Data: base64.StdEncoding.EncodeToString(cert.Raw)}}}}}
	md := saml.EntityDescriptor{EntityID: entityID, IDPSSODescriptors: []saml.IDPSSODescriptor{{
		SSODescriptor:        saml.SSODescriptor{RoleDescriptor: saml.RoleDescriptor{ProtocolSupportEnumeration: "urn:oasis:names:tc:SAML:2.0:protocol", KeyDescriptors: []saml.KeyDescriptor{key}}},
		SingleSignOnServices: []saml.Endpoint{{Binding: saml.HTTPRedirectBinding, Location: location}},
	}}}
	out, err := xml.MarshalIndent(md, "", "  ")
	return string(out), err
}

func FetchMetadata(ctx context.Context, address string) (string, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", errors.New("Enter the metadata URL starting with https://.")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	res, err := metadataClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Halo couldn't download the metadata: %v. Check the URL, or paste the metadata instead.", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("The metadata URL answered %s. Check the URL, or paste the metadata instead.", res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	return string(body), err
}

func (h *handler) serviceProviderMetadata(w http.ResponseWriter, r *http.Request) {
	p, err := h.st.GetIdentityProvider(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) || err == nil && p.Kind != "saml" {
		page(w, http.StatusNotFound, "This SAML identity provider doesn't exist", "Copy the metadata URL again from Identity providers in the Halo console.", "")
		return
	}
	if err != nil {
		internal(w, r, err)
		return
	}
	sp, err := serviceProvider(r.Context(), h.st, h.cfg, p)
	if err != nil {
		internal(w, r, err)
		return
	}
	md := sp.Metadata()
	descriptor := &md.SPSSODescriptors[0]
	descriptor.AssertionConsumerServices = descriptor.AssertionConsumerServices[:1]
	descriptor.KeyDescriptors = slices.DeleteFunc(descriptor.KeyDescriptors, func(k saml.KeyDescriptor) bool { return k.Use == "encryption" })
	body, err := xml.MarshalIndent(md, "", "  ")
	if err != nil {
		internal(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	_, _ = w.Write(body)
}

func (h *handler) assertionConsumer(w http.ResponseWriter, r *http.Request) {
	post(w, "/api/v1/auth/federated/"+url.PathEscape(r.PathValue("id"))+"/acs", map[string]string{"SAMLResponse": r.PostFormValue("SAMLResponse"), "RelayState": r.PostFormValue("RelayState")})
}
