package saml_test

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/crewjam/saml"

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
