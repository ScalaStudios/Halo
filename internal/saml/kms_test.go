package saml_test

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/crewjam/saml"
	dsig "github.com/russellhaering/goxmldsig"

	"halo/internal/access"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/kms"
	"halo/internal/kms/kmstest"
	halosaml "halo/internal/saml"
	"halo/internal/store"
	"halo/internal/testdb"
)

func TestExternalSAMLKey(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	fake := kmstest.New(t)
	st.Keys = kms.FromConfig(fake.Config())
	group, err := st.CreateGroup(ctx, store.NewGroup{Name: "Support", Kind: "assigned"})
	must(err)
	ada, err := st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active", GroupIDs: []string{group.ID}})
	must(err)
	const entityID = "https://help.example.com/saml/metadata"
	acsURL, _ := url.Parse("https://help.example.com/saml/acs")
	_, err = st.CreateApplication(ctx, store.NewApplication{Name: "Helpdesk", Protocol: "saml", Type: "web", ClientID: entityID, RedirectURIs: []string{acsURL.String()}, GroupIDs: []string{group.ID}})
	must(err)
	issuer, _ := url.Parse("https://halo.example.com")
	authn := &httpx.Auth{Store: st, Public: issuer, Dev: true}
	idp, err := halosaml.New(httpx.Deps{Config: config.Config{PublicURL: issuer, Dev: true}, Store: st, Auth: authn, Policy: &fakePolicy{decision: access.Decision{Effect: access.Allow}}})
	must(err)
	_, session, err := st.CreateSession(ctx, store.NewSession{UserID: ada.ID, Method: "passkey"})
	must(err)
	call := func(target string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("GET", target, nil)
		req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: session})
		rec := httptest.NewRecorder()
		authn.Resolve(idp).ServeHTTP(rec, req)
		return rec
	}

	var meta saml.EntityDescriptor
	must(xml.Unmarshal(call("/saml/metadata").Body.Bytes(), &meta))
	key, err := st.ActiveSAMLKey(ctx)
	if err != nil || key.PrivateKey != nil || !fake.Has(key.ExternalKey) {
		t.Fatalf("the SAML key must live only in Vault: %+v %v", key, err)
	}
	if meta.IDPSSODescriptors[0].KeyDescriptors[0].KeyInfo.X509Data.X509Certificates[0].Data != base64.StdEncoding.EncodeToString(key.Certificate) {
		t.Fatal("the metadata must publish the certificate of the Vault key")
	}

	sp := &saml.ServiceProvider{EntityID: entityID, AcsURL: *acsURL, IDPMetadata: &meta}
	request, err := sp.MakeAuthenticationRequest(sp.GetSSOBindingLocation(saml.HTTPRedirectBinding), saml.HTTPRedirectBinding, saml.HTTPPostBinding)
	must(err)
	redirect, err := request.Redirect("", sp)
	must(err)
	rec := call(redirect.String())
	raw, err := base64.StdEncoding.DecodeString(fields(rec.Body.String())["SAMLResponse"])
	must(err)
	a, err := sp.ParseXMLResponse(raw, []string{request.ID}, *acsURL)
	if err != nil || a.Subject.NameID.Value != ada.Email {
		t.Fatalf("the response signed in Vault must verify with the published certificate: %v", err)
	}
}

func TestSignedAuthnRequests(t *testing.T) {
	for name, keys := range map[string]func(*testing.T) *kms.Keys{
		"local": func(*testing.T) *kms.Keys { return nil },
		"vault": func(t *testing.T) *kms.Keys { return kms.FromConfig(kmstest.New(t).Config()) },
		"aws":   func(t *testing.T) *kms.Keys { return kms.FromConfig(kmstest.NewAWS(t).Config()) },
	} {
		t.Run(name, func(t *testing.T) {
			st := testdb.New(t)
			st.Keys = keys(t)
			ctx := context.Background()
			issuer, _ := url.Parse("https://halo.example.com")
			cfg := config.Config{PublicURL: issuer, Dev: true}
			sso := func(binding string) string {
				return `<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.example.com"><IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol"><SingleSignOnService Binding="` + binding + `" Location="https://idp.example.com/sso?tenant=acme"/></IDPSSODescriptor></EntityDescriptor>`
			}
			p, err := st.SaveIdentityProvider(ctx, store.IdentityProvider{Kind: "saml", Name: "Upstream", Scopes: []string{}, AllowedDomains: []string{}, JITGroupIDs: []string{}, SAMLMetadata: sso(saml.HTTPRedirectBinding), Enabled: true}, "")
			if err != nil {
				t.Fatal(err)
			}
			redirect, err := halosaml.Begin(ctx, httptest.NewRecorder(), st, cfg, p, "state", func(string) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			key, err := st.ActiveSAMLKey(ctx)
			if err != nil {
				t.Fatal(err)
			}
			cert, _ := x509.ParseCertificate(key.Certificate)
			u, _ := url.Parse(redirect)
			query, encoded, _ := strings.Cut(u.RawQuery, "&Signature=")
			signed, prefixed := strings.CutPrefix(query, "tenant=acme&")
			signature, _ := url.QueryUnescape(encoded)
			raw, _ := base64.StdEncoding.DecodeString(signature)
			digest := sha256.Sum256([]byte(signed))
			if !prefixed || !strings.HasPrefix(signed, "SAMLRequest=") || !strings.HasSuffix(signed, "&RelayState=state&SigAlg="+url.QueryEscape("http://www.w3.org/2001/04/xmldsig-more#rsa-sha256")) ||
				rsa.VerifyPKCS1v15(cert.PublicKey.(*rsa.PublicKey), crypto.SHA256, digest[:], raw) != nil {
				t.Fatalf("the redirect binding must carry a signature that verifies with the SAML certificate: %s", redirect)
			}

			p.SAMLMetadata = sso(saml.HTTPPostBinding)
			rec := httptest.NewRecorder()
			if _, err := halosaml.Begin(ctx, rec, st, cfg, p, "state", func(string) error { return nil }); err != nil {
				t.Fatal(err)
			}
			request, _ := base64.StdEncoding.DecodeString(fields(rec.Body.String())["SAMLRequest"])
			doc := etree.NewDocument()
			if err := doc.ReadFromBytes(request); err != nil {
				t.Fatal(err)
			}
			validation := dsig.NewDefaultValidationContext(&dsig.MemoryX509CertificateStore{Roots: []*x509.Certificate{cert}})
			if _, err := validation.Validate(doc.Root()); err != nil {
				t.Fatalf("the POST binding must carry an enveloped signature by the SAML key: %v", err)
			}

			idp, err := halosaml.New(httpx.Deps{Config: cfg, Store: st, Auth: &httpx.Auth{Store: st, Public: issuer, Dev: true}, Policy: &fakePolicy{}})
			if err != nil {
				t.Fatal(err)
			}
			metadata := httptest.NewRecorder()
			idp.ServeHTTP(metadata, httptest.NewRequest("GET", "/saml/sp/"+p.ID+"/metadata", nil))
			var md saml.EntityDescriptor
			if err := xml.Unmarshal(metadata.Body.Bytes(), &md); err != nil {
				t.Fatal(err)
			}
			descriptor := md.SPSSODescriptors[0]
			if descriptor.AuthnRequestsSigned == nil || !*descriptor.AuthnRequestsSigned || len(descriptor.KeyDescriptors) != 1 || descriptor.KeyDescriptors[0].Use != "signing" ||
				descriptor.KeyDescriptors[0].KeyInfo.X509Data.X509Certificates[0].Data != base64.StdEncoding.EncodeToString(cert.Raw) {
				t.Fatalf("the service provider metadata must say requests are signed and list only the signing certificate: %s", metadata.Body)
			}
		})
	}
}
