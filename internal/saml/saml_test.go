package saml_test

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"html"
	"io"
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
	"github.com/crewjam/saml/xmlenc"

	"halo/internal/access"
	"halo/internal/config"
	"halo/internal/httpx"
	halosaml "halo/internal/saml"
	"halo/internal/store"
	"halo/internal/testdb"
)

type fakePolicy struct{ decision access.Decision }

func (p *fakePolicy) Evaluate(context.Context, access.Input) (access.Decision, error) {
	return p.decision, nil
}

type signatures struct {
	Response  *struct{} `xml:"http://www.w3.org/2000/09/xmldsig# Signature"`
	Assertion struct {
		Signature *struct{} `xml:"http://www.w3.org/2000/09/xmldsig# Signature"`
	} `xml:"urn:oasis:names:tc:SAML:2.0:assertion Assertion"`
}

var formField = regexp.MustCompile(`name="(SAMLRequest|SAMLResponse|RelayState)" value="([^"]*)"`)

func fields(body string) map[string]string {
	out := map[string]string{}
	for _, m := range formField.FindAllStringSubmatch(body, -1) {
		out[m[1]] = html.UnescapeString(m[2])
	}
	return out
}

func attribute(a *saml.Assertion, name string) []string {
	var values []string
	for _, statement := range a.AttributeStatements {
		for _, attr := range statement.Attributes {
			if attr.Name == name {
				for _, v := range attr.Values {
					values = append(values, v.Value)
				}
			}
		}
	}
	return values
}

type aes256GCM struct{}

func (aes256GCM) Algorithm() string { return "http://www.w3.org/2009/xmlenc11#aes256-gcm" }

func (aes256GCM) Decrypt(key any, el *etree.Element) ([]byte, error) {
	secret, err := xmlenc.Decrypt(key, el.FindElement("./KeyInfo/EncryptedKey"))
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(secret)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(el.FindElement("./CipherData/CipherValue").Text())
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], nil)
}

func keyPair(t *testing.T) (*rsa.PrivateKey, *x509.Certificate, string) {
	t.Helper()
	keyDER, certDER, err := halosaml.NewKey("sp.example.com")
	if err != nil {
		t.Fatal(err)
	}
	key, err := x509.ParsePKCS8PrivateKey(keyDER)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatal(err)
	}
	return key.(*rsa.PrivateKey), cert, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))
}

func TestIdentityProvider(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	group, err := st.CreateGroup(ctx, store.NewGroup{Name: "Support", Kind: "assigned"})
	must(err)
	ada, err := st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active", GroupIDs: []string{group.ID}})
	must(err)
	bob, err := st.CreateUser(ctx, store.NewUser{Email: "bob@example.com", Name: "Bob", Status: "active"})
	must(err)
	const entityID = "https://help.example.com/saml/metadata"
	acsURL, _ := url.Parse("https://help.example.com/saml/acs")
	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Helpdesk", Protocol: "saml", Type: "web", ClientID: entityID, RedirectURIs: []string{acsURL.String()}, GroupIDs: []string{group.ID}})
	must(err)

	issuer, _ := url.Parse("https://halo.example.com")
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	engine := &fakePolicy{decision: access.Decision{Effect: access.Allow}}
	authn := &httpx.Auth{Store: st, Public: issuer, Dev: true}
	idp, err := halosaml.New(httpx.Deps{Config: config.Config{PublicURL: issuer, SecretKey: key, Dev: true}, Store: st, Auth: authn, Policy: engine})
	must(err)
	handler := authn.Resolve(idp)

	call := func(method, target, token string, form url.Values) *httptest.ResponseRecorder {
		t.Helper()
		var body io.Reader
		if form != nil {
			body = strings.NewReader(form.Encode())
		}
		req := httptest.NewRequest(method, target, body)
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		if token != "" {
			req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: token})
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	session := func(userID, method string) string {
		t.Helper()
		_, token, err := st.CreateSession(ctx, store.NewSession{UserID: userID, Method: method})
		must(err)
		return token
	}
	lastEvent := func() store.SignInEvent {
		t.Helper()
		events, err := st.ListSignIns(ctx, store.SignInFilter{AppID: app.ID, Limit: 1})
		must(err)
		if len(events) == 0 {
			t.Fatal("no sign-in event was recorded")
		}
		return events[0]
	}

	metadata := call("GET", "/saml/metadata", "", nil)
	var meta saml.EntityDescriptor
	must(xml.Unmarshal(metadata.Body.Bytes(), &meta))
	if metadata.Code != http.StatusOK || meta.EntityID != "https://halo.example.com/saml/metadata" || len(meta.IDPSSODescriptors) != 1 ||
		meta.IDPSSODescriptors[0].SingleSignOnServices[0].Location != "https://halo.example.com/saml/sso" || meta.ValidUntil.Before(time.Now().AddDate(9, 0, 0)) {
		t.Fatalf("metadata: %d %s", metadata.Code, metadata.Body)
	}
	block, _ := pem.Decode(call("GET", "/saml/certificate", "", nil).Body.Bytes())
	if block == nil || base64.StdEncoding.EncodeToString(block.Bytes) != meta.IDPSSODescriptors[0].KeyDescriptors[0].KeyInfo.X509Data.X509Certificates[0].Data {
		t.Fatal("the downloadable certificate must be the one in the metadata")
	}

	sp := &saml.ServiceProvider{EntityID: entityID, AcsURL: *acsURL, IDPMetadata: &meta}
	assertion := func(rec *httptest.ResponseRecorder, sp *saml.ServiceProvider, requestIDs ...string) (*saml.Assertion, signatures, string) {
		t.Helper()
		form := fields(rec.Body.String())
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `action="`+acsURL.String()+`"`) || form["SAMLResponse"] == "" {
			t.Fatalf("want a form posting to the ACS URL, got %d: %s", rec.Code, rec.Body)
		}
		raw, err := base64.StdEncoding.DecodeString(form["SAMLResponse"])
		must(err)
		a, err := sp.ParseXMLResponse(raw, requestIDs, *acsURL)
		var invalid *saml.InvalidResponseError
		if errors.As(err, &invalid) {
			t.Fatalf("the service provider rejected the response: %v", invalid.PrivateErr)
		}
		must(err)
		var signed signatures
		must(xml.Unmarshal(raw, &signed))
		return a, signed, form["RelayState"]
	}

	request, err := sp.MakeAuthenticationRequest(sp.GetSSOBindingLocation(saml.HTTPRedirectBinding), saml.HTTPRedirectBinding, saml.HTTPPostBinding)
	must(err)
	redirect, err := request.Redirect("relay-1", sp)
	must(err)
	anonymous := call("GET", redirect.String(), "", nil)
	location, _ := url.Parse(anonymous.Header().Get("Location"))
	next := location.Query().Get("next")
	if anonymous.Code != http.StatusSeeOther || location.Path != "/sign-in" || !strings.HasPrefix(next, "/saml/resume?id=srq_") {
		t.Fatalf("without a session: want a redirect to /sign-in with next=/saml/resume, got %d %q", anonymous.Code, location)
	}
	if again := call("GET", next, "", nil); again.Code != http.StatusSeeOther || again.Header().Get("Location") != location.String() {
		t.Fatalf("resume without a session must go back to sign-in and keep the request: %d %q", again.Code, again.Header().Get("Location"))
	}

	passkey := session(ada.ID, "passkey")
	a, signed, relay := assertion(call("GET", next, passkey, nil), sp, request.ID)
	if a.Subject.NameID.Value != ada.Email || a.Subject.NameID.Format != "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress" || relay != "relay-1" {
		t.Fatalf("subject %+v, relay state %q", a.Subject.NameID, relay)
	}
	if !slices.Equal(attribute(a, "groups"), []string{"Support"}) || !slices.Equal(attribute(a, "email"), []string{ada.Email}) ||
		!slices.Equal(attribute(a, "given_name"), []string{"Ada"}) || !slices.Equal(attribute(a, "family_name"), []string{"Lovelace"}) {
		t.Fatalf("attributes: %+v", a.AttributeStatements)
	}
	if signed.Response != nil || signed.Assertion.Signature == nil {
		t.Fatal("by default only the assertion is signed")
	}
	if e := lastEvent(); e.Result != "success" || e.Method != "passkey" || *e.UserID != ada.ID {
		t.Fatalf("success event: %+v", e)
	}
	if used := call("GET", next, passkey, nil); used.Code != http.StatusNotFound || !strings.Contains(used.Body.String(), "no longer valid") {
		t.Fatalf("a used request must be refused: %d", used.Code)
	}
	expired, err := st.CreateSAMLRequest(ctx, store.SAMLRequest{Request: []byte("<x/>"), CreatedAt: time.Now().Add(-20 * time.Minute)})
	must(err)
	if old := call("GET", "/saml/resume?id="+expired, passkey, nil); old.Code != http.StatusNotFound {
		t.Fatalf("an expired request must be refused: %d", old.Code)
	}

	must(st.SetSAMLSettings(ctx, app.ID, entityID, []string{acsURL.String()}, store.SAMLSettings{NameIDFormat: "persistent", SignResponse: true, Encryption: "off", Attributes: store.SAMLAttributes{Email: "mail", Groups: "memberOf"}}))
	posted, err := sp.MakeAuthenticationRequest(sp.GetSSOBindingLocation(saml.HTTPPostBinding), saml.HTTPPostBinding, saml.HTTPPostBinding)
	must(err)
	form := url.Values{"SAMLRequest": {fields(string(posted.Post("relay-2")))["SAMLRequest"]}, "RelayState": {"relay-2"}}
	a, signed, relay = assertion(call("POST", "/saml/sso", passkey, form), sp, posted.ID)
	if a.Subject.NameID.Value != ada.ID || a.Subject.NameID.Format != "urn:oasis:names:tc:SAML:2.0:nameid-format:persistent" || relay != "relay-2" ||
		!slices.Equal(attribute(a, "memberOf"), []string{"Support"}) || !slices.Equal(attribute(a, "mail"), []string{ada.Email}) || attribute(a, "groups") != nil {
		t.Fatalf("post binding with custom settings: %+v %+v", a.Subject.NameID, a.AttributeStatements)
	}
	if signed.Response == nil || signed.Assertion.Signature == nil {
		t.Fatal("signResponse must sign the whole response")
	}

	ada.Attributes = map[string]any{"employeeNumber": 1000000, "teams": []string{"Platform", "Security"}, "contractor": false}
	must(st.SetUserAttributes(ctx, ada.ID, ada.Attributes))
	app.ClaimMappings = []store.Claim{{Name: "employeeNumber", Source: "attribute.employeeNumber"}, {Name: "teams", Source: "attribute.teams"}, {Name: "contractor", Source: "attribute.contractor"},
		{Name: "mail", Source: "user.name"}, {Name: "memberOf", Source: store.OmitClaim}, {Name: "costCentre", Source: "attribute.costCentre"}}
	must(st.UpdateApplication(ctx, app))
	a, _, _ = assertion(call("GET", redirect.String(), passkey, nil), sp, request.ID)
	if !slices.Equal(attribute(a, "employeeNumber"), []string{"1000000"}) || !slices.Equal(attribute(a, "teams"), []string{"Platform", "Security"}) || !slices.Equal(attribute(a, "contractor"), []string{"false"}) ||
		!slices.Equal(attribute(a, "mail"), []string{ada.Name}) || attribute(a, "memberOf") != nil || attribute(a, "costCentre") != nil {
		t.Fatalf("mapped attributes: %+v", a.AttributeStatements)
	}
	app.ClaimMappings = []store.Claim{{Name: "appRoles", Source: "user.roles"}}
	must(st.UpdateApplication(ctx, app))
	_, err = st.CreateAppRole(ctx, app.ID, store.AppRole{Key: "agent", Name: "Agent", GroupIDs: []string{group.ID}})
	must(err)
	_, err = st.CreateAppRole(ctx, app.ID, store.AppRole{Key: "supervisor", Name: "Supervisor", UserIDs: []string{ada.ID}})
	must(err)
	a, _, _ = assertion(call("GET", redirect.String(), passkey, nil), sp, request.ID)
	if !slices.Equal(attribute(a, "roles"), []string{"agent", "supervisor"}) || !slices.Equal(attribute(a, "appRoles"), []string{"agent", "supervisor"}) {
		t.Fatalf("role attributes: %+v", a.AttributeStatements)
	}
	groupsAsRoles := store.SAMLSettings{NameIDFormat: "persistent", SignResponse: true, Encryption: "off", Attributes: store.SAMLAttributes{Email: "mail", Groups: "roles"}}
	must(st.SetSAMLSettings(ctx, app.ID, entityID, []string{acsURL.String()}, groupsAsRoles))
	a, _, _ = assertion(call("GET", redirect.String(), passkey, nil), sp, request.ID)
	if !slices.Equal(attribute(a, "roles"), []string{"Support"}) {
		t.Fatalf("a groups attribute named roles must win over the default roles attribute: %+v", a.AttributeStatements)
	}
	groupsAsRoles.Attributes.Groups = "memberOf"
	must(st.SetSAMLSettings(ctx, app.ID, entityID, []string{acsURL.String()}, groupsAsRoles))
	app.ClaimMappings = []store.Claim{}
	must(st.UpdateApplication(ctx, app))

	xmlenc.RegisterDecrypter(aes256GCM{})
	spKey, spCert, spPEM := keyPair(t)
	otherKey, _, _ := keyPair(t)
	decrypting := *sp
	decrypting.Key, decrypting.Certificate = spKey, spCert
	wrongKey := decrypting
	wrongKey.Key = otherKey
	encrypted := store.DefaultSAMLSettings()
	encrypted.EncryptionCertificate = spPEM
	for _, method := range []string{"aes256-gcm", "aes256-cbc"} {
		encrypted.Encryption = method
		must(st.SetSAMLSettings(ctx, app.ID, entityID, []string{acsURL.String()}, encrypted))
		rec := call("GET", redirect.String(), passkey, nil)
		raw, err := base64.StdEncoding.DecodeString(fields(rec.Body.String())["SAMLResponse"])
		must(err)
		if !strings.Contains(string(raw), "<saml:EncryptedAssertion") || strings.Contains(string(raw), "<saml:Assertion") || strings.Contains(string(raw), ada.Email) {
			t.Fatalf("%s: the assertion must only be sent encrypted: %s", method, raw)
		}
		if _, err := wrongKey.ParseXMLResponse(raw, []string{request.ID}, *acsURL); err == nil {
			t.Fatalf("%s: a different service provider key must not decrypt the assertion", method)
		}
		a, _, _ := assertion(rec, &decrypting, request.ID)
		if a.Subject.NameID.Value != ada.Email || !slices.Equal(attribute(a, "groups"), []string{"Support"}) || !slices.Equal(attribute(a, "email"), []string{ada.Email}) {
			t.Fatalf("%s: decrypted assertion %+v %+v", method, a.Subject.NameID, a.AttributeStatements)
		}
	}
	encrypted.EncryptionCertificate = "not a certificate"
	must(st.SetSAMLSettings(ctx, app.ID, entityID, []string{acsURL.String()}, encrypted))
	if rec := call("GET", redirect.String(), passkey, nil); rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "SAMLResponse") {
		t.Fatalf("with encryption on and no usable certificate, sign-in must fail instead of sending the assertion: %d %s", rec.Code, rec.Body)
	}
	encrypted.Encryption, encrypted.EncryptionCertificate = "off", spPEM
	must(st.SetSAMLSettings(ctx, app.ID, entityID, []string{acsURL.String()}, encrypted))
	if a, signed, _ := assertion(call("GET", redirect.String(), passkey, nil), sp, request.ID); a.Subject.NameID.Value != ada.Email || signed.Assertion.Signature == nil {
		t.Fatal("with encryption off, the signed assertion is sent unencrypted even when a certificate is saved")
	}
	must(st.SetSAMLSettings(ctx, app.ID, entityID, []string{acsURL.String()}, store.DefaultSAMLSettings()))

	refused := func(rec *httptest.ResponseRecorder, text, reason, userID string) {
		t.Helper()
		if rec.Code != http.StatusForbidden || !strings.Contains(html.UnescapeString(rec.Body.String()), text) || strings.Contains(rec.Body.String(), "SAMLResponse") {
			t.Fatalf("want a 403 page saying %q, got %d: %s", text, rec.Code, rec.Body)
		}
		if e := lastEvent(); e.Result != "failure" || e.Reason != reason || *e.UserID != userID || *e.AppID != app.ID {
			t.Fatalf("failure event: %+v", e)
		}
	}
	refused(call("GET", redirect.String(), session(bob.ID, "passkey"), nil), "You don't have access to Helpdesk", "User is not assigned to this application.", bob.ID)
	must(st.SetApplicationStatus(ctx, app.ID, "disabled", false))
	refused(call("GET", redirect.String(), passkey, nil), "Helpdesk is turned off", "The application is disabled.", ada.ID)
	must(st.SetApplicationStatus(ctx, app.ID, "active", false))
	engine.decision = access.Decision{Effect: access.Block, Policy: "Block Tor", Reason: "Sign-ins from Tor exit nodes are blocked."}
	refused(call("GET", redirect.String(), passkey, nil), "access policy blocked", "Policy “Block Tor”: Sign-ins from Tor exit nodes are blocked.", ada.ID)
	engine.decision = access.Decision{Effect: access.RequirePhishingResistant, Policy: "Strong sign-in", Reason: "Helpdesk needs phishing-resistant sign-in."}
	refused(call("GET", redirect.String(), session(ada.ID, "totp"), nil), "Helpdesk needs a passkey or security key", "Policy “Strong sign-in”: Helpdesk needs phishing-resistant sign-in.", ada.ID)
	assertion(call("GET", redirect.String(), passkey, nil), sp, request.ID)
	engine.decision = access.Decision{Effect: access.RequireContext, Contexts: []store.AuthContext{{ID: "keys", Name: "Security keys", Methods: []string{"security-key"}}}}
	refused(call("GET", redirect.String(), passkey, nil), "Sign in again with a security key.", "The session does not meet the authentication context “Security keys”.", ada.ID)
	engine.decision = access.Decision{Effect: access.Allow, Terms: []store.Terms{{ID: "tou_1", Name: "Acceptable use", Version: 1}}}
	if rec := call("GET", redirect.String(), passkey, nil); rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/account/terms?next=%2Fsaml%2Fresume%3Fid%3D") {
		t.Fatalf("terms not accepted: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	engine.decision = access.Decision{Effect: access.Allow}

	stranger := &saml.ServiceProvider{EntityID: "https://unknown.example.com", AcsURL: *acsURL, IDPMetadata: &meta}
	unknown, err := stranger.MakeRedirectAuthenticationRequest("")
	must(err)
	if rec := call("GET", unknown.String(), passkey, nil); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "unknown.example.com") {
		t.Fatalf("unknown service provider: %d %s", rec.Code, rec.Body)
	}

	launch := call("GET", "/saml/launch/"+app.ID, "", nil)
	if launch.Code != http.StatusSeeOther || launch.Header().Get("Location") != "/sign-in?next="+url.QueryEscape("/saml/launch/"+app.ID) {
		t.Fatalf("launch without a session: %d %q", launch.Code, launch.Header().Get("Location"))
	}
	initiated := *sp
	initiated.AllowIDPInitiated = true
	if a, _, _ := assertion(call("GET", "/saml/launch/"+app.ID, passkey, nil), &initiated); a.Subject.NameID.Value != ada.Email {
		t.Fatalf("IdP-initiated subject: %+v", a.Subject.NameID)
	}
	if rec := call("GET", "/saml/launch/app_missing", passkey, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("launching a missing application: %d", rec.Code)
	}

	time.Sleep(1100 * time.Millisecond)
	limited, err := st.GetApplication(ctx, app.ID)
	must(err)
	limited.MaxAuthAge = 1
	must(st.UpdateApplication(ctx, limited))
	stale := call("GET", redirect.String(), passkey, nil)
	location, _ = url.Parse(stale.Header().Get("Location"))
	next = location.Query().Get("next")
	if stale.Code != http.StatusSeeOther || location.Path != "/sign-in" || !strings.HasPrefix(next, "/saml/resume?id=srq_") {
		t.Fatalf("a session past the sign-in frequency must sign in again: %d %q", stale.Code, location)
	}
	if a, _, _ := assertion(call("GET", next, session(ada.ID, "passkey"), nil), sp, request.ID); a.Subject.NameID.Value != ada.Email {
		t.Fatalf("after signing in again: %+v", a.Subject.NameID)
	}
}

func TestParseMetadata(t *testing.T) {
	acs, _ := url.Parse("https://wiki.example.com/saml/acs")
	raw, err := xml.Marshal((&saml.ServiceProvider{EntityID: "https://wiki.example.com", AcsURL: *acs}).Metadata())
	if err != nil {
		t.Fatal(err)
	}
	entityID, urls, certificate, err := halosaml.ParseMetadata(string(raw))
	if err != nil || entityID != "https://wiki.example.com" || !slices.Equal(urls, []string{acs.String()}) || certificate != "" {
		t.Fatalf("got %q %v %q %v", entityID, urls, certificate, err)
	}
	if _, _, _, err := halosaml.ParseMetadata("<html></html>"); err == nil {
		t.Fatal("non-metadata XML must be rejected")
	}

	key, cert, certPEM := keyPair(t)
	sp := (&saml.ServiceProvider{EntityID: "https://wiki.example.com", AcsURL: *acs, Key: key, Certificate: cert}).Metadata()
	own := sp.SPSSODescriptors[0].KeyDescriptors
	encryptionCertificate := func(descriptors ...saml.KeyDescriptor) string {
		t.Helper()
		md := *sp
		md.SPSSODescriptors = slices.Clone(sp.SPSSODescriptors)
		md.SPSSODescriptors[0].KeyDescriptors = descriptors
		raw, err := xml.Marshal(md)
		if err != nil {
			t.Fatal(err)
		}
		_, _, certificate, err := halosaml.ParseMetadata(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		return certificate
	}
	signed := func(use string, notAfter time.Time) (saml.KeyDescriptor, string) {
		template := &x509.Certificate{NotBefore: time.Now().AddDate(-2, 0, 0), NotAfter: notAfter}
		der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
		if err != nil {
			t.Fatal(err)
		}
		return saml.KeyDescriptor{Use: use, KeyInfo: saml.KeyInfo{X509Data: saml.X509Data{X509Certificates: []saml.X509Certificate{{Data: base64.StdEncoding.EncodeToString(der)}}}}},
			strings.TrimSpace(string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})))
	}
	if got := encryptionCertificate(own...); got != strings.TrimSpace(certPEM) {
		t.Fatalf("encryption certificate from metadata: %q", got)
	}
	expired, _ := signed("encryption", time.Now().AddDate(0, 0, -1))
	if got := encryptionCertificate(expired); got != "" {
		t.Fatalf("an expired encryption certificate must be skipped: %q", got)
	}
	rollover, rolloverPEM := signed("encryption", time.Now().AddDate(20, 0, 0))
	signing, _ := signed("signing", time.Now().AddDate(30, 0, 0))
	if got := encryptionCertificate(append(slices.Clone(own), expired, rollover, signing)...); got != rolloverPEM {
		t.Fatalf("during a rollover the encryption certificate that lasts longest must be used: %q", got)
	}
}

func TestSeedSAML(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if err := st.SeedDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.SeedSAML(ctx); err != nil {
		t.Fatal(err)
	}
	apps, err := st.ListApplications(ctx)
	if err != nil {
		t.Fatal(err)
	}
	guides := map[string]string{}
	for _, a := range apps {
		if a.Protocol == "saml" {
			guides[a.ClientID] = a.SetupGuide
			if _, err := st.SAMLSettings(ctx, a.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(guides) != 4 || guides["urn:amazon:webservices"] != "aws-iam-identity-center" || guides["https://slack.com"] != "slack" || guides["https://support.example.com/auth/saml/metadata"] != "generic-saml" {
		t.Fatalf("seeded SAML applications: %v", guides)
	}
}
