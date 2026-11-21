package server

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
	"github.com/crewjam/saml"

	"halo/internal/httpx"
	halosaml "halo/internal/saml"
	"halo/internal/store"
)

type spDescriptor struct{ md *saml.EntityDescriptor }

func (d spDescriptor) GetServiceProvider(*http.Request, string) (*saml.EntityDescriptor, error) {
	return d.md, nil
}

var (
	samlField  = regexp.MustCompile(`name="(\w+)" value="([^"]*)"`)
	samlAction = regexp.MustCompile(`action="([^"]*)"`)
)

func newSAMLIdP(t *testing.T) *saml.IdentityProvider {
	t.Helper()
	der, certDER, err := halosaml.NewKey("idp.example.test")
	if err != nil {
		t.Fatal(err)
	}
	key, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatal(err)
	}
	base, _ := url.Parse("https://idp.example.test")
	return &saml.IdentityProvider{Key: key, Certificate: cert, MetadataURL: *base.JoinPath("metadata"), SSOURL: *base.JoinPath("sso"), SignatureMethod: "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"}
}

type samlFlow struct {
	req      *saml.IdpAuthnRequest
	cookies  []*http.Cookie
	location string
}

func (e *env) samlStart(idp *saml.IdentityProvider, providerID string, session saml.Session) samlFlow {
	e.t.Helper()
	rec := e.call("", "GET", "/api/v1/auth/federated/"+providerID+"/start", nil)
	location := rec.Header().Get("Location")
	if rec.Code != http.StatusFound || !strings.HasPrefix(location, idp.SSOURL.String()+"?") {
		e.t.Fatalf("start: %d %s", rec.Code, location)
	}
	req, err := saml.NewIdpAuthnRequest(idp, httptest.NewRequest("GET", location, nil))
	if err == nil {
		err = req.Validate()
	}
	if err == nil {
		err = saml.DefaultAssertionMaker{}.MakeAssertion(req, &session)
	}
	if err != nil {
		e.t.Fatal(err)
	}
	return samlFlow{req: req, cookies: rec.Result().Cookies(), location: location}
}

func (f samlFlow) assertion(t *testing.T, sign bool) *etree.Element {
	t.Helper()
	if !sign {
		return f.req.Assertion.Element()
	}
	if err := f.req.MakeAssertionEl(); err != nil {
		t.Fatal(err)
	}
	return f.req.AssertionEl
}

func (f samlFlow) response(t *testing.T, assertions ...*etree.Element) string {
	t.Helper()
	resp := saml.Response{Destination: f.req.ACSEndpoint.Location, ID: "id-response-" + f.req.Request.ID, InResponseTo: f.req.Request.ID, IssueInstant: time.Now(), Version: "2.0",
		Issuer: &saml.Issuer{Value: f.req.IDP.MetadataURL.String()}, Status: saml.Status{StatusCode: saml.StatusCode{Value: saml.StatusSuccess}}}
	el := resp.Element()
	for _, a := range assertions {
		el.AddChild(a.Copy())
	}
	doc := etree.NewDocument()
	doc.SetRoot(el)
	raw, err := doc.WriteToBytes()
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func (e *env) samlPost(providerID string, f samlFlow, response string) (string, string) {
	e.t.Helper()
	form := url.Values{"SAMLResponse": {response}, "RelayState": {f.req.RelayState}}
	relay := httptest.NewRequest("POST", "/saml/sp/"+providerID+"/acs", strings.NewReader(form.Encode()))
	relay.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	relay.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, relay)
	action := samlAction.FindStringSubmatch(rec.Body.String())
	if rec.Code != http.StatusOK || len(rec.Result().Cookies()) != 0 || action == nil {
		e.t.Fatalf("relay: %d %v %s", rec.Code, rec.Result().Cookies(), rec.Body)
	}
	fields := url.Values{}
	for _, m := range samlField.FindAllStringSubmatch(rec.Body.String(), -1) {
		fields.Set(m[1], html.UnescapeString(m[2]))
	}
	if fields.Get("SAMLResponse") != response || fields.Get("RelayState") != f.req.RelayState {
		e.t.Fatalf("relay fields: %v", fields)
	}
	acs := httptest.NewRequest("POST", html.UnescapeString(action[1]), strings.NewReader(fields.Encode()))
	acs.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	acs.Header.Set("Sec-Fetch-Site", "same-origin")
	for _, c := range f.cookies {
		acs.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	e.h.ServeHTTP(rec, acs)
	session := ""
	for _, c := range rec.Result().Cookies() {
		if c.Name == httpx.SessionCookie {
			session = c.Value
		}
	}
	return rec.Header().Get("Location"), session
}

func (e *env) samlUser(session string) store.User {
	e.t.Helper()
	var u store.User
	if err := json.Unmarshal(e.expect(session, "GET", "/api/v1/me", nil, http.StatusOK, "").Body.Bytes(), &u); err != nil {
		e.t.Fatal(err)
	}
	return u
}

func TestSAMLIdentityProviderSignIn(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	idp := newSAMLIdP(t)
	metadata, err := xml.Marshal(idp.Metadata())
	if err != nil {
		t.Fatal(err)
	}
	_, admin, _ := e.person("root@example.com", "passkey", false, "global_admin")
	ada, _, _ := e.person("ada@example.com", "passkey", false)

	var p store.IdentityProvider
	created := e.expect(admin, "POST", "/api/v1/identity-providers", map[string]any{"kind": "saml", "name": "University", "metadataXml": string(metadata), "emailAttribute": "mail",
		"enabled": true, "showOnSignIn": true, "allowedDomains": []string{"example.com"}, "clientId": "ignored", "scopes": []string{"openid"}}, http.StatusCreated, "")
	if err := json.Unmarshal(created.Body.Bytes(), &p); err != nil || p.Kind != "saml" || p.Issuer != idp.MetadataURL.String() || p.ClientID != "" || len(p.Scopes) != 0 || p.EmailAttribute != "mail" {
		t.Fatalf("created: %+v %v", p, err)
	}
	certificate := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: idp.Certificate.Raw}))
	var manual store.IdentityProvider
	_ = json.Unmarshal(e.expect(admin, "POST", "/api/v1/identity-providers", map[string]any{"kind": "saml", "name": "Partner", "issuer": "https://partner.example.test",
		"ssoUrl": "https://partner.example.test/sso?tenant=1", "certificate": certificate}, http.StatusCreated, "").Body.Bytes(), &manual)
	if manual.Issuer != "https://partner.example.test" || !strings.Contains(manual.SAMLMetadata, "https://partner.example.test/sso?tenant=1") {
		t.Fatalf("manual: %+v", manual)
	}
	if rec := e.expect(admin, "POST", "/api/v1/identity-providers", map[string]any{"kind": "saml", "name": "Loopback", "metadataUrl": "https://127.0.0.1:9/metadata"}, http.StatusUnprocessableEntity, "ERR_INVALID"); !strings.Contains(rec.Body.String(), "only calls public addresses") {
		t.Fatalf("metadata from a loopback address: %s", rec.Body)
	}
	e.expect(admin, "POST", "/api/v1/identity-providers", map[string]any{"kind": "saml", "name": "Empty"}, http.StatusUnprocessableEntity, "ERR_INVALID")

	var sp saml.EntityDescriptor
	if err := xml.Unmarshal(e.expect("", "GET", "/saml/sp/"+p.ID+"/metadata", nil, http.StatusOK, "").Body.Bytes(), &sp); err != nil {
		t.Fatal(err)
	}
	descriptor := sp.SPSSODescriptors[0]
	if sp.EntityID != "http://localhost:3200/saml/sp/"+p.ID+"/metadata" || len(descriptor.AssertionConsumerServices) != 1 || descriptor.AssertionConsumerServices[0].Binding != saml.HTTPPostBinding ||
		descriptor.AssertionConsumerServices[0].Location != "http://localhost:3200/saml/sp/"+p.ID+"/acs" || len(descriptor.KeyDescriptors) != 1 || descriptor.KeyDescriptors[0].Use != "signing" || !*descriptor.AuthnRequestsSigned {
		t.Fatalf("service provider metadata: %+v", sp)
	}
	idp.ServiceProviderProvider = spDescriptor{&sp}

	adaSession := saml.Session{ID: "s1", NameID: "ada-1", NameIDFormat: string(saml.PersistentNameIDFormat), UserEmail: "ada@example.com", UserGivenName: "Ada", UserSurname: "Lovelace"}
	f := e.samlStart(idp, p.ID, adaSession)
	signed, _ := url.Parse(f.location)
	query := signed.RawQuery[:strings.Index(signed.RawQuery, "&Signature=")]
	signature, _ := base64.StdEncoding.DecodeString(signed.Query().Get("Signature"))
	spDER, _ := base64.StdEncoding.DecodeString(descriptor.KeyDescriptors[0].KeyInfo.X509Data.X509Certificates[0].Data)
	spCert, err := x509.ParseCertificate(spDER)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(query))
	if signed.Query().Get("SigAlg") != idp.SignatureMethod || rsa.VerifyPKCS1v15(spCert.PublicKey.(*rsa.PublicKey), crypto.SHA256, digest[:], signature) != nil {
		t.Fatalf("AuthnRequest is not signed with the published key: %s", f.location)
	}
	assertion := f.assertion(t, true)
	response := f.response(t, assertion)
	location, session := e.samlPost(p.ID, f, response)
	if location != "/account" || e.samlUser(session).ID != ada.ID {
		t.Fatalf("valid response: %s", location)
	}
	if location, _ := e.samlPost(p.ID, f, response); location != "/sign-in?error=ERR_FEDERATION_STATE" {
		t.Fatalf("replayed response: %s", location)
	}
	reused := e.samlStart(idp, p.ID, adaSession)
	reused.req.Assertion.ID = f.req.Assertion.ID
	if location, _ := e.samlPost(p.ID, reused, reused.response(t, reused.assertion(t, true))); location != "/sign-in?error=ERR_FEDERATION_FAILED" {
		t.Fatalf("replayed assertion: %s", location)
	}

	attacker := newSAMLIdP(t)
	for name, change := range map[string]func(*samlFlow){
		"wrong audience": func(f *samlFlow) {
			f.req.Assertion.Conditions.AudienceRestrictions[0].Audience.Value = "https://other.example.test/metadata"
		},
		"no audience":   func(f *samlFlow) { f.req.Assertion.Conditions.AudienceRestrictions = nil },
		"expired":       func(f *samlFlow) { f.req.Assertion.Conditions.NotOnOrAfter = time.Now().Add(-10 * time.Minute) },
		"not yet valid": func(f *samlFlow) { f.req.Assertion.Conditions.NotBefore = time.Now().Add(10 * time.Minute) },
		"wrong recipient": func(f *samlFlow) {
			f.req.Assertion.Subject.SubjectConfirmations[0].SubjectConfirmationData.Recipient = "https://other.example.test/acs"
		},
		"no subject confirmation": func(f *samlFlow) { f.req.Assertion.Subject.SubjectConfirmations = nil },
		"untrusted signer":        func(f *samlFlow) { f.req.IDP = attacker },
		"wrong InResponseTo": func(f *samlFlow) {
			f.req.Assertion.Subject.SubjectConfirmations[0].SubjectConfirmationData.InResponseTo = "id-other"
		},
		"unsolicited": func(f *samlFlow) { f.req.Request.ID = "" },
	} {
		f := e.samlStart(idp, p.ID, adaSession)
		change(&f)
		if location, _ := e.samlPost(p.ID, f, f.response(t, f.assertion(t, true))); location != "/sign-in?error=ERR_FEDERATION_FAILED" {
			t.Fatalf("%s: %s", name, location)
		}
	}

	f = e.samlStart(idp, p.ID, adaSession)
	if location, _ := e.samlPost(p.ID, f, f.response(t, f.assertion(t, false))); location != "/sign-in?error=ERR_FEDERATION_FAILED" {
		t.Fatalf("unsigned: %s", location)
	}

	f = e.samlStart(idp, p.ID, saml.Session{ID: "s2", NameID: "grace-1", NameIDFormat: string(saml.PersistentNameIDFormat), UserEmail: "grace@example.com"})
	forged := f.assertion(t, true).Copy()
	forged.FindElement("./Subject/NameID").SetText("ada-1")
	if location, _ := e.samlPost(p.ID, f, f.response(t, forged)); location != "/sign-in?error=ERR_FEDERATION_FAILED" {
		t.Fatalf("forged assertion with the original signature: %s", location)
	}

	f = e.samlStart(idp, p.ID, adaSession)
	injected := f.req.Assertion.Element()
	injected.CreateAttr("ID", "id-injected")
	injected.FindElement("./Subject/NameID").SetText("root-1")
	for _, value := range injected.FindElements("./AttributeStatement/Attribute[@FriendlyName='mail']/AttributeValue") {
		value.SetText("root@example.com")
	}
	location, session = e.samlPost(p.ID, f, f.response(t, injected, f.assertion(t, true)))
	if location != "/account" || e.samlUser(session).ID != ada.ID {
		t.Fatalf("injected unsigned assertion: %s", location)
	}

	f = e.samlStart(idp, p.ID, adaSession)
	f.req.AssertionEl = f.req.Assertion.Element()
	if err := f.req.MakeResponse(); err != nil {
		t.Fatal(err)
	}
	doc := etree.NewDocument()
	doc.SetRoot(f.req.ResponseEl)
	signedResponse, err := doc.WriteToString()
	if err != nil {
		t.Fatal(err)
	}
	location, session = e.samlPost(p.ID, f, base64.StdEncoding.EncodeToString([]byte(signedResponse)))
	if location != "/account" || e.samlUser(session).ID != ada.ID {
		t.Fatalf("signed response with an unsigned assertion: %s", location)
	}

	postOnly := idp.Metadata()
	postOnly.IDPSSODescriptors[0].SingleSignOnServices = slices.DeleteFunc(postOnly.IDPSSODescriptors[0].SingleSignOnServices, func(s saml.Endpoint) bool { return s.Binding != saml.HTTPPostBinding })
	postMetadata, _ := xml.Marshal(postOnly)
	var q store.IdentityProvider
	_ = json.Unmarshal(e.expect(admin, "POST", "/api/v1/identity-providers", map[string]any{"kind": "saml", "name": "Post only", "metadataXml": string(postMetadata), "enabled": true, "emailAttribute": "mail"}, http.StatusCreated, "").Body.Bytes(), &q)
	var qsp saml.EntityDescriptor
	if err := xml.Unmarshal(e.expect("", "GET", "/saml/sp/"+q.ID+"/metadata", nil, http.StatusOK, "").Body.Bytes(), &qsp); err != nil {
		t.Fatal(err)
	}
	idp.ServiceProviderProvider = spDescriptor{&qsp}
	start := e.call("", "GET", "/api/v1/auth/federated/"+q.ID+"/start", nil)
	action := samlAction.FindStringSubmatch(start.Body.String())
	fields := url.Values{}
	for _, m := range samlField.FindAllStringSubmatch(start.Body.String(), -1) {
		fields.Set(m[1], html.UnescapeString(m[2]))
	}
	request, _ := base64.StdEncoding.DecodeString(fields.Get("SAMLRequest"))
	if start.Code != http.StatusOK || action == nil || html.UnescapeString(action[1]) != idp.SSOURL.String() || fields.Get("RelayState") == "" ||
		!strings.Contains(string(request), "SignatureValue") || !strings.Contains(string(request), descriptor.KeyDescriptors[0].KeyInfo.X509Data.X509Certificates[0].Data) {
		t.Fatalf("HTTP-POST AuthnRequest: %d %s", start.Code, start.Body)
	}
	posted := httptest.NewRequest("POST", idp.SSOURL.String(), strings.NewReader(fields.Encode()))
	posted.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req, err := saml.NewIdpAuthnRequest(idp, posted)
	if err == nil {
		err = req.Validate()
	}
	if err == nil {
		err = saml.DefaultAssertionMaker{}.MakeAssertion(req, &adaSession)
	}
	if err != nil {
		t.Fatal(err)
	}
	f = samlFlow{req: req, cookies: start.Result().Cookies()}
	location, session = e.samlPost(q.ID, f, f.response(t, f.assertion(t, true)))
	if location != "/account" || e.samlUser(session).ID != ada.ID {
		t.Fatalf("sign-in after an HTTP-POST AuthnRequest: %s", location)
	}
	idp.ServiceProviderProvider = spDescriptor{&sp}

	p.JIT = true
	if _, err := e.st.SaveIdentityProvider(ctx, p, ""); err != nil {
		t.Fatal(err)
	}
	f = e.samlStart(idp, p.ID, saml.Session{ID: "s3", NameID: "grace-1", NameIDFormat: string(saml.PersistentNameIDFormat), UserEmail: "grace@example.com", UserGivenName: "Grace", UserSurname: "Hopper"})
	location, session = e.samlPost(p.ID, f, f.response(t, f.assertion(t, true)))
	if grace := e.samlUser(session); location != "/account" || grace.Email != "grace@example.com" || grace.Name != "Grace Hopper" || grace.Source != "University" {
		t.Fatalf("created on first sign-in: %s %+v", location, grace)
	}
}

func TestSAMLSignInMapsAttributes(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	idp := newSAMLIdP(t)
	metadata, err := xml.Marshal(idp.Metadata())
	if err != nil {
		t.Fatal(err)
	}
	group := func(name string) store.Group {
		t.Helper()
		g, err := e.st.CreateGroup(ctx, store.NewGroup{Name: name, Kind: "assigned"})
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	staff, alumni, club := group("Staff"), group("Alumni"), group("Book club")
	ada, _, _ := e.person("ada@example.com", "passkey", false)
	for _, g := range []string{alumni.ID, club.ID} {
		if err := e.st.AddGroupMember(ctx, g, ada.ID); err != nil {
			t.Fatal(err)
		}
	}
	_, security, _ := e.person("sec@example.com", "passkey", false, "security_admin")
	scoped := []string{}
	for _, role := range []string{"user_admin", "helpdesk_admin"} {
		u, token, _ := e.person(strings.ReplaceAll(role, "_", "-")+"@example.com", "passkey", false)
		if err := e.st.SetUserRoles(ctx, u.ID, []string{role}, map[string][]string{role: {staff.ID}}); err != nil {
			t.Fatal(err)
		}
		scoped = append(scoped, token)
	}

	body := map[string]any{"kind": "saml", "name": "University", "metadataXml": string(metadata), "emailAttribute": "mail", "enabled": true, "allowedDomains": []string{"example.com"},
		"mapping": map[string]any{
			"profile":    []map[string]string{{"claim": "department", "field": "department"}},
			"groupClaim": "eduPersonAffiliation",
			"groups":     []map[string]string{{"value": "staff", "groupId": staff.ID}, {"value": "alum", "groupId": alumni.ID}},
		}}
	for _, token := range scoped {
		e.expect(token, "POST", "/api/v1/identity-providers", body, http.StatusForbidden, "")
	}
	var p store.IdentityProvider
	if err := json.Unmarshal(e.expect(security, "POST", "/api/v1/identity-providers", body, http.StatusCreated, "").Body.Bytes(), &p); err != nil ||
		p.Kind != "saml" || len(p.Mapping.Profile) != 1 || p.Mapping.GroupClaim != "eduPersonAffiliation" || len(p.Mapping.Groups) != 2 {
		t.Fatalf("created with mapping: %+v %v", p.Mapping, err)
	}
	for _, token := range scoped {
		e.expect(token, "PUT", "/api/v1/identity-providers/"+p.ID, body, http.StatusForbidden, "")
	}

	var sp saml.EntityDescriptor
	if err := xml.Unmarshal(e.expect("", "GET", "/saml/sp/"+p.ID+"/metadata", nil, http.StatusOK, "").Body.Bytes(), &sp); err != nil {
		t.Fatal(err)
	}
	idp.ServiceProviderProvider = spDescriptor{&sp}
	f := e.samlStart(idp, p.ID, saml.Session{ID: "s1", NameID: "ada-1", NameIDFormat: string(saml.PersistentNameIDFormat), UserEmail: "ada@example.com", Groups: []string{"staff", "student"},
		CustomAttributes: []saml.Attribute{{Name: "department", NameFormat: "urn:oasis:names:tc:SAML:2.0:attrname-format:basic", Values: []saml.AttributeValue{{Type: "xs:string", Value: "Physics"}}}}})
	if location, _ := e.samlPost(p.ID, f, f.response(t, f.assertion(t, true))); location != "/account" {
		t.Fatalf("sign-in: %s", location)
	}
	got, err := e.st.GetUser(ctx, ada.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Department != "Physics" || !slices.Contains(got.GroupIDs, staff.ID) || slices.Contains(got.GroupIDs, alumni.ID) || !slices.Contains(got.GroupIDs, club.ID) {
		t.Fatalf("mapped from attributes: %q %v", got.Department, got.GroupIDs)
	}
}
