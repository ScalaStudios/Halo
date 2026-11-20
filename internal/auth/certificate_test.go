package auth_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/cryptobyte"
	cbasn1 "golang.org/x/crypto/cryptobyte/asn1"
	"golang.org/x/crypto/ocsp"

	"halo/internal/access"
	"halo/internal/httpx"
	"halo/internal/store"
)

type authority struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  string
}

var serials atomic.Int64

func newAuthority(t *testing.T, name string, parent ...authority) authority {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(serials.Add(1)), Subject: pkix.Name{CommonName: name}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature}
	issuer, signer := tmpl, key
	if len(parent) > 0 {
		issuer, signer = parent[0].cert, parent[0].key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, issuer, &key.PublicKey, signer)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	return authority{cert, key, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))}
}

func (ca authority) issue(t *testing.T, edit func(*x509.Certificate)) *x509.Certificate {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(serials.Add(1)), Subject: pkix.Name{CommonName: "Ada Lovelace"}, EmailAddresses: []string{"ada@example.com"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	if edit != nil {
		edit(tmpl)
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	return cert
}

func rfc9440(c *x509.Certificate) string {
	return ":" + base64.StdEncoding.EncodeToString(c.Raw) + ":"
}

func principalNameSAN(upn string) pkix.Extension {
	var b cryptobyte.Builder
	b.AddASN1(cbasn1.SEQUENCE, func(b *cryptobyte.Builder) {
		b.AddASN1(cbasn1.Tag(0).ContextSpecific().Constructed(), func(b *cryptobyte.Builder) {
			b.AddASN1ObjectIdentifier(asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 20, 2, 3})
			b.AddASN1(cbasn1.Tag(0).ContextSpecific().Constructed(), func(b *cryptobyte.Builder) {
				b.AddASN1(cbasn1.UTF8String, func(b *cryptobyte.Builder) { b.AddBytes([]byte(upn)) })
			})
		})
	})
	return pkix.Extension{Id: asn1.ObjectIdentifier{2, 5, 29, 17}, Value: b.BytesOrPanic()}
}

func TestCertificateSignIn(t *testing.T) {
	var methods []string
	e, _ := newMagicEnv(t, policyFunc(func(in access.Input) access.Decision {
		methods = append(methods, in.Method)
		return access.Decision{Effect: access.Allow}
	}))
	ctx := context.Background()
	if _, err := e.st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.CreateUser(ctx, store.NewUser{Email: "grace@example.com", Name: "Grace Hopper", Status: "invited"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.CreateServiceAccount(ctx, store.NewServiceAccount{Name: "Robot"}); err != nil {
		t.Fatal(err)
	}
	corporate, smartCards, stranger, root := newAuthority(t, "Corporate"), newAuthority(t, "Smart cards"), newAuthority(t, "Stranger"), newAuthority(t, "Root")
	issuing := newAuthority(t, "Issuing", root)
	for _, ca := range []struct {
		authority
		name, mapping string
	}{{corporate, "Corporate", "email"}, {smartCards, "Smart cards", "upn"}, {issuing, "Issuing", "email"}} {
		if _, err := e.st.CreateCertificateAuthority(ctx, store.CertificateAuthority{Name: ca.name, Mapping: ca.mapping, Certificates: ca.pem}); err != nil {
			t.Fatal(err)
		}
	}

	revokedSerial := big.NewInt(-1)
	crl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		list, _ := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{Number: big.NewInt(1), ThisUpdate: time.Now().Add(-time.Minute), NextUpdate: time.Now().Add(time.Hour),
			RevokedCertificateEntries: []x509.RevocationListEntry{{SerialNumber: revokedSerial, RevocationTime: time.Now()}}}, corporate.cert, corporate.key)
		w.Write(list)
	}))
	t.Cleanup(crl.Close)
	responder := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		req, err := ocsp.ParseRequest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		status := ocsp.Good
		if req.SerialNumber.Cmp(revokedSerial) == 0 {
			status = ocsp.Revoked
		}
		res, _ := ocsp.CreateResponse(corporate.cert, corporate.cert, ocsp.Response{Status: status, SerialNumber: req.SerialNumber, ThisUpdate: time.Now().Add(-time.Minute), NextUpdate: time.Now().Add(time.Hour), RevokedAt: time.Now()}, corporate.key)
		w.Write(res)
	}))
	t.Cleanup(responder.Close)

	signIn := func(headers ...string) (*http.Client, int, string) {
		t.Helper()
		c := client()
		req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/auth/certificate", strings.NewReader(`{}`))
		req.Header.Set("User-Agent", firefox)
		for _, header := range headers {
			req.Header.Add("Client-Cert", header)
		}
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out struct {
			Redirect string `json:"redirect"`
			apiError
		}
		_ = json.NewDecoder(res.Body).Decode(&out)
		if res.StatusCode == http.StatusOK && out.Redirect != "/account" {
			t.Fatalf("redirect: %q", out.Redirect)
		}
		return c, res.StatusCode, out.Error.Code
	}
	refused := func(status int, code string, headers ...string) {
		t.Helper()
		if _, got, gotCode := signIn(headers...); got != status || gotCode != code {
			t.Fatalf("want %d %s, got %d %s", status, code, got, gotCode)
		}
	}
	valid := corporate.issue(t, nil)

	httpx.TrustProxies(nil)
	refused(http.StatusUnauthorized, "ERR_CERTIFICATE_MISSING", rfc9440(valid))
	httpx.TrustProxies([]string{"127.0.0.1/32", "::1/128"})
	t.Cleanup(func() { httpx.TrustProxies(nil) })

	unconfigured := newEnv(t)
	req, _ := http.NewRequest("POST", unconfigured.srv.URL+"/api/v1/auth/certificate", strings.NewReader(`{}`))
	req.Header.Set("Client-Cert", rfc9440(valid))
	res, err := client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if events, _ := unconfigured.st.ListSignIns(ctx, store.SignInFilter{}); res.StatusCode != http.StatusUnauthorized || len(events) != 1 || !strings.HasPrefix(events[0].Reason, "HALO_CLIENT_CERT_HEADER isn't set") {
		t.Fatalf("without HALO_CLIENT_CERT_HEADER: %d %+v", res.StatusCode, events)
	}

	refused(http.StatusUnauthorized, "ERR_CERTIFICATE_MISSING")
	refused(http.StatusUnauthorized, "ERR_CERTIFICATE_MISSING", "::")
	refused(http.StatusUnauthorized, "ERR_CERTIFICATE_REJECTED", ":bm90IGEgY2VydGlmaWNhdGU=:")
	refused(http.StatusUnauthorized, "ERR_CERTIFICATE_REJECTED", rfc9440(stranger.issue(t, nil)))
	refused(http.StatusUnauthorized, "ERR_CERTIFICATE_REJECTED", rfc9440(corporate.issue(t, func(c *x509.Certificate) { c.NotAfter = time.Now().Add(-time.Minute) })))
	refused(http.StatusUnauthorized, "ERR_CERTIFICATE_REJECTED", rfc9440(corporate.issue(t, func(c *x509.Certificate) { c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth} })))
	refused(http.StatusUnauthorized, "ERR_CERTIFICATE_REJECTED", rfc9440(corporate.issue(t, func(c *x509.Certificate) { c.EmailAddresses = []string{"nobody@example.com"} })))
	refused(http.StatusUnauthorized, "ERR_CERTIFICATE_REJECTED", rfc9440(corporate.issue(t, func(c *x509.Certificate) { c.EmailAddresses = nil })))
	refused(http.StatusUnauthorized, "ERR_CERTIFICATE_REJECTED", rfc9440(corporate.issue(t, func(c *x509.Certificate) { c.EmailAddresses = []string{"robot@service.halo.invalid"} })))
	refused(http.StatusUnauthorized, "ERR_CERTIFICATE_MISSING", rfc9440(stranger.issue(t, nil)), rfc9440(valid))
	refused(http.StatusUnauthorized, "ERR_CERTIFICATE_REJECTED", rfc9440(root.issue(t, nil)))
	if len(methods) != 0 {
		t.Fatalf("refused certificates reached the policy engine: %v", methods)
	}

	c, status, _ := signIn(rfc9440(valid))
	if _, me := e.me(c); status != http.StatusOK || me != http.StatusOK {
		t.Fatalf("sign in with a certificate: %d, then /me %d", status, me)
	}
	escaped := url.PathEscape(string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: valid.Raw})))
	if _, status, code := signIn(escaped); status != http.StatusOK {
		t.Fatalf("URL-escaped PEM from nginx: %d %s", status, code)
	}
	upn := smartCards.issue(t, func(c *x509.Certificate) {
		c.EmailAddresses = nil
		c.ExtraExtensions = []pkix.Extension{principalNameSAN("ADA@example.com")}
	})
	if _, status, code := signIn(rfc9440(upn)); status != http.StatusOK {
		t.Fatalf("user principal name: %d %s", status, code)
	}
	if _, status, code := signIn(rfc9440(issuing.issue(t, nil))); status != http.StatusOK {
		t.Fatalf("issuing authority under a root: %d %s", status, code)
	}
	if _, status, code := signIn(rfc9440(issuing.issue(t, func(c *x509.Certificate) { c.EmailAddresses = []string{"grace@example.com"} }))); status != http.StatusOK {
		t.Fatalf("invited person: %d %s", status, code)
	}
	if grace, err := e.st.GetUserByEmail(ctx, "grace@example.com"); err != nil || grace.Status != "active" {
		t.Fatalf("invited person after signing in: %+v %v", grace.Status, err)
	}

	withCRL := func(c *x509.Certificate) { c.CRLDistributionPoints = []string{"ldap:///CN=Corporate", crl.URL} }
	withOCSP := func(c *x509.Certificate) { c.OCSPServer = []string{responder.URL} }
	for name, edit := range map[string]func(*x509.Certificate){"CRL": withCRL, "OCSP": withOCSP} {
		if _, status, code := signIn(rfc9440(corporate.issue(t, edit))); status != http.StatusOK {
			t.Fatalf("%s good: %d %s", name, status, code)
		}
		revoked := corporate.issue(t, edit)
		revokedSerial.Set(revoked.SerialNumber)
		refused(http.StatusUnauthorized, "ERR_CERTIFICATE_REJECTED", rfc9440(revoked))
	}
	refused(http.StatusServiceUnavailable, "ERR_REVOCATION_UNAVAILABLE", rfc9440(corporate.issue(t, func(c *x509.Certificate) { c.CRLDistributionPoints = []string{crl.URL + "/missing"} })))
	refused(http.StatusServiceUnavailable, "ERR_REVOCATION_UNAVAILABLE", rfc9440(corporate.issue(t, func(c *x509.Certificate) { c.OCSPServer = []string{crl.URL} })))
	refused(http.StatusServiceUnavailable, "ERR_REVOCATION_UNAVAILABLE", rfc9440(corporate.issue(t, func(c *x509.Certificate) { c.CRLDistributionPoints = []string{"ldap:///CN=Corporate"} })))

	events, err := e.st.ListSignIns(ctx, store.SignInFilter{})
	if err != nil {
		t.Fatal(err)
	}
	reasons := []string{}
	for _, ev := range events {
		if ev.Method != "certificate" {
			t.Fatalf("sign-in method: %+v", ev)
		}
		reasons = append(reasons, ev.Result+": "+ev.Reason)
	}
	log := strings.Join(reasons, "\n")
	for _, want := range []string{
		"failure: The browser sent no client certificate.",
		"failure: The client certificate could not be read.",
		"failure: No trusted certificate authority issued the certificate.",
		"failure: The certificate is valid from",
		"failure: No person's email address matches the certificate's email address.",
		"failure: The certificate has no email address to match a person.",
		"failure: “Corporate” revoked the certificate.",
		"failure: The certificate's revocation status could not be checked.",
		"success: ",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("sign-in log is missing %q:\n%s", want, log)
		}
	}
	if strings.Join(methods, ",") != strings.TrimSuffix(strings.Repeat("certificate,", 7), ",") {
		t.Fatalf("policy inputs: %v", methods)
	}
}

func TestCertificateAuthorities(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	session := func(role string) *http.Client {
		u, err := e.st.CreateUser(ctx, store.NewUser{Email: role + "@example.com", Name: role, Status: "active", Roles: []string{role}})
		if err != nil {
			t.Fatal(err)
		}
		_, token, err := e.st.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: "passkey"})
		if err != nil {
			t.Fatal(err)
		}
		c := client()
		base, _ := url.Parse(e.srv.URL)
		c.Jar.SetCookies(base, []*http.Cookie{{Name: httpx.SessionCookie, Value: token}})
		return c
	}
	admin, auditor := session("security_admin"), session("auditor")
	ca := newAuthority(t, "Corporate")
	leaf := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.issue(t, nil).Raw})
	body := func(name, mapping, certificates string) map[string]any {
		return map[string]any{"name": name, "mapping": mapping, "certificates": certificates}
	}

	e.expect(auditor, "POST", "/api/v1/certificate-authorities", body("Corporate", "email", ca.pem), http.StatusForbidden, "ERR_FORBIDDEN")
	e.expect(admin, "POST", "/api/v1/certificate-authorities", body("Corporate", "email", "not a certificate"), http.StatusUnprocessableEntity, "ERR_INVALID")
	e.expect(admin, "POST", "/api/v1/certificate-authorities", body("Corporate", "email", string(leaf)), http.StatusUnprocessableEntity, "ERR_INVALID")
	e.expect(admin, "POST", "/api/v1/certificate-authorities", body("Corporate", "serial", ca.pem), http.StatusUnprocessableEntity, "ERR_INVALID")
	e.expect(admin, "POST", "/api/v1/certificate-authorities", body(" ", "email", ca.pem), http.StatusUnprocessableEntity, "ERR_INVALID")

	var created struct {
		ID           string
		Name         string
		Mapping      string
		Certificates []struct{ Subject string }
	}
	if status := e.do(admin, "POST", "/api/v1/certificate-authorities", body(" Corporate ", "upn", "Corporate root\n"+ca.pem), &created); status != http.StatusCreated || created.Name != "Corporate" || created.Mapping != "upn" || len(created.Certificates) != 1 || created.Certificates[0].Subject != "CN=Corporate" {
		t.Fatalf("create: %d %+v", status, created)
	}
	e.expect(admin, "POST", "/api/v1/certificate-authorities", body("corporate", "email", ca.pem), http.StatusConflict, "ERR_CONFLICT")
	var listed []json.RawMessage
	if status := e.do(auditor, "GET", "/api/v1/certificate-authorities", nil, &listed); status != http.StatusOK || len(listed) != 1 || strings.Contains(string(listed[0]), "BEGIN CERTIFICATE") {
		t.Fatalf("list: %d %s", status, listed)
	}
	if status := e.do(admin, "DELETE", "/api/v1/certificate-authorities/"+created.ID, nil, nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	e.expect(admin, "DELETE", "/api/v1/certificate-authorities/"+created.ID, nil, http.StatusNotFound, "ERR_NOT_FOUND")
	audit, err := e.st.ListAudit(ctx, 5)
	if err != nil || auditActions(audit) != "certificate_authority.delete,certificate_authority.create" {
		t.Fatalf("audit: %s %v", auditActions(audit), err)
	}
}
