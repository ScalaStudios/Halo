package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"

	"halo/internal/config"
	"halo/internal/ldapserver"
	"halo/internal/secret"
	"halo/internal/store"
)

type ldapEnv struct {
	*env
	addr  string
	roots *x509.CertPool
	admin string
}

type ldapApp struct {
	store.Application
	LDAP ldapserver.Connection `json:"ldap"`
}

func setupLDAP(t *testing.T, mode string) *ldapEnv {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalECPrivateKey(key)
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	e := setup(t, func(c *config.Config) {
		c.LDAP = config.LDAP{Listen: "127.0.0.1:636", TLS: mode, CertFile: certFile, KeyFile: keyFile}
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go e.app.LDAP.Serve(listener)
	t.Cleanup(func() { e.app.LDAP.Close() })
	if err := e.st.SetMethodEnabled(context.Background(), "password", true); err != nil {
		t.Fatal(err)
	}
	_, admin, _ := e.person("admin@example.com", "passkey", false, "global_admin")
	return &ldapEnv{env: e, addr: listener.Addr().String(), roots: roots, admin: admin}
}

func (l *ldapEnv) dial() *ldap.Conn {
	l.t.Helper()
	c, err := ldap.DialURL("ldaps://"+l.addr, ldap.DialWithTLSConfig(&tls.Config{RootCAs: l.roots, ServerName: "localhost"}))
	if err != nil {
		l.t.Fatal(err)
	}
	l.t.Cleanup(func() { c.Close() })
	return c
}

func (l *ldapEnv) application(name string, groups ...string) (ldapApp, string) {
	l.t.Helper()
	rec := l.expect(l.admin, "POST", "/api/v1/applications", map[string]any{"name": name, "protocol": "ldap", "groupIds": groups}, http.StatusCreated, "")
	var out struct {
		Application ldapApp `json:"application"`
		Secret      string  `json:"clientSecret"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Secret == "" || out.Application.LDAP.BindDN != "cn=bind,ou="+out.Application.ID+",dc=localhost" {
		l.t.Fatalf("an LDAP application gets a bind password and its DNs: %v %s", err, rec.Body)
	}
	return out.Application, out.Secret
}

func (l *ldapEnv) member(email, password string, groups ...string) store.User {
	l.t.Helper()
	ctx := context.Background()
	u, _, _ := l.person(email, "passkey", false)
	if password != "" {
		if err := l.st.SetPassword(ctx, u.ID, secret.HashPassword(password), false); err != nil {
			l.t.Fatal(err)
		}
	}
	for _, g := range groups {
		if err := l.st.AddGroupMember(ctx, g, u.ID); err != nil {
			l.t.Fatal(err)
		}
	}
	return u
}

func (l *ldapEnv) group(name string) store.Group {
	l.t.Helper()
	g, err := l.st.CreateGroup(context.Background(), store.NewGroup{Name: name, Kind: "assigned"})
	if err != nil {
		l.t.Fatal(err)
	}
	return g
}

func code(err error) uint16 {
	var e *ldap.Error
	if errors.As(err, &e) {
		return e.ResultCode
	}
	if err != nil {
		return 0xffff
	}
	return ldap.LDAPResultSuccess
}

func dns(r *ldap.SearchResult) []string {
	out := []string{}
	if r != nil {
		for _, e := range r.Entries {
			out = append(out, e.DN)
		}
	}
	return out
}

func search(c *ldap.Conn, base, filter string, size int) (*ldap.SearchResult, error) {
	return c.Search(ldap.NewSearchRequest(base, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, size, 0, false, filter, nil, nil))
}

func TestLDAPApplicationSeesOnlyAssignedPeople(t *testing.T) {
	l := setupLDAP(t, "ldaps")
	staff, other := l.group("Staff, Ops"), l.group("Other")
	app, password := l.application("NAS", staff.ID)
	l.member("ada@example.com", "correct horse battery", staff.ID)
	carolUser := l.member("carol@example.com", "", staff.ID)
	l.member("bob@example.com", "bob's password", other.ID)
	second, secondPassword := l.application("Printer", other.ID)
	people := "ou=people," + app.LDAP.BaseDN
	ada, carol := "uid=ada@example.com,"+people, "uid=carol@example.com,"+people
	staffDN := "cn=Staff\\, Ops,ou=groups," + app.LDAP.BaseDN

	c := l.dial()
	if err := c.Bind(app.LDAP.BindDN, password); err != nil {
		t.Fatalf("the application's bind DN and password must bind: %v", err)
	}
	all, err := search(c, app.LDAP.BaseDN, "(objectClass=*)", 0)
	if err != nil || !slices.Equal(dns(all), []string{app.LDAP.BaseDN, people, "ou=groups," + app.LDAP.BaseDN, ada, carol, staffDN}) {
		t.Fatalf("the directory must hold the assigned people and groups only: %v %q", err, dns(all))
	}
	for _, e := range all.Entries {
		if e.DN == ada && (e.GetAttributeValue("mail") != "ada@example.com" || e.GetAttributeValue("cn") != "ada" || !slices.Equal(e.GetAttributeValues("memberOf"), []string{staffDN}) || e.GetAttributeValue("userPassword") != "") {
			t.Fatalf("person entry: %+v", e.Attributes)
		}
		if e.DN == staffDN && (!slices.Equal(e.GetAttributeValues("member"), []string{ada, carol}) || !slices.Contains(e.GetAttributeValues("objectClass"), "groupOfNames")) {
			t.Fatalf("group entry: %+v", e.Attributes)
		}
	}
	for filter, want := range map[string][]string{
		"(uid=" + ldap.EscapeFilter("*") + ")":                                        {},
		"(uid=" + ldap.EscapeFilter("*)(uid=*") + ")":                                 {},
		"(|(uid=bob@example.com)(mail=ada@example.com))":                              {ada},
		"(&(objectClass=inetOrgPerson)(memberOf=" + ldap.EscapeFilter(staffDN) + "))": {ada, carol},
		"(&(objectClass=person)(!(uid=ada*)))":                                        {carol},
		"(mail=*@EXAMPLE.com)":                                                        {ada, carol},
		"(&(uid=*)(unknownAttribute=1))":                                              {},
		"(!(unknownAttribute=1))":                                                     {},
	} {
		got, err := search(c, people, filter, 0)
		if err != nil || !slices.Equal(dns(got), want) {
			t.Fatalf("%s: %v %q, want %q", filter, err, dns(got), want)
		}
	}
	if _, err := search(c, second.LDAP.BaseDN, "(objectClass=*)", 0); code(err) != ldap.LDAPResultNoSuchObject {
		t.Fatalf("another application's directory must not exist for this one: %v", err)
	}
	limited, err := search(c, people, "(uid=*)", 1)
	if code(err) != ldap.LDAPResultSizeLimitExceeded || len(limited.Entries) != 1 {
		t.Fatalf("the size limit must cut the result: %v %q", err, dns(limited))
	}
	paged, err := c.SearchWithPaging(ldap.NewSearchRequest(people, ldap.ScopeSingleLevel, ldap.NeverDerefAliases, 0, 0, false, "(objectClass=*)", []string{"uid"}, nil), 1)
	if err != nil || !slices.Equal(dns(paged), []string{ada, carol}) || paged.Entries[0].GetAttributeValue("mail") != "" {
		t.Fatalf("paging must return every entry one page at a time with only the requested attributes: %v %q", err, dns(paged))
	}

	root, err := c.Search(ldap.NewSearchRequest("", ldap.ScopeBaseObject, ldap.NeverDerefAliases, 0, 0, false, "(objectClass=*)", []string{"namingContexts", "supportedControl"}, nil))
	if err != nil || len(root.Entries) != 1 || root.Entries[0].GetAttributeValue("namingContexts") != app.LDAP.BaseDN || root.Entries[0].GetAttributeValue("supportedControl") != ldap.ControlTypePaging {
		t.Fatalf("the root DSE names this application's base DN: %v %+v", err, root)
	}
	critical := ldap.NewSearchRequest(people, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false, "(uid=*)", nil, []ldap.Control{ldap.NewControlString("1.2.3.4", true, "")})
	if _, err := c.Search(critical); code(err) != ldap.LDAPResultUnavailableCriticalExtension {
		t.Fatalf("an unknown critical control must end the search: %v", err)
	}
	if err := l.st.SetUserStatus(context.Background(), carolUser.ID, "suspended"); err != nil {
		t.Fatal(err)
	}
	if after, err := search(c, app.LDAP.BaseDN, "(|(uid=carol@example.com)(member=*))", 0); err != nil || len(after.Entries) != 1 || !slices.Equal(after.Entries[0].GetAttributeValues("member"), []string{ada}) {
		t.Fatalf("a suspended person must leave the directory and its groups: %v %q", err, dns(after))
	}

	writes := map[string]error{
		"modify":    c.Modify(&ldap.ModifyRequest{DN: ada, Changes: []ldap.Change{{Operation: ldap.ReplaceAttribute, Modification: ldap.PartialAttribute{Type: "mail", Vals: []string{"x@example.com"}}}}}),
		"add":       c.Add(&ldap.AddRequest{DN: "uid=eve@example.com," + people, Attributes: []ldap.Attribute{{Type: "objectClass", Vals: []string{"inetOrgPerson"}}}}),
		"delete":    c.Del(&ldap.DelRequest{DN: ada}),
		"modify DN": c.ModifyDN(&ldap.ModifyDNRequest{DN: ada, NewRDN: "uid=eve@example.com", DeleteOldRDN: true}),
		"password modify": func() error {
			_, err := c.PasswordModify(&ldap.PasswordModifyRequest{UserIdentity: ada, NewPassword: "x"})
			return err
		}(),
	}
	_, compareErr := c.Compare(ada, "mail", "ada@example.com")
	writes["compare"] = compareErr
	for name, err := range writes {
		if code(err) != ldap.LDAPResultUnwillingToPerform {
			t.Fatalf("%s must be refused: %v", name, err)
		}
	}

	if err := c.Bind(second.LDAP.BindDN, password); code(err) != ldap.LDAPResultInvalidCredentials {
		t.Fatalf("one application's password must not bind another: %v", err)
	}
	if _, err := search(c, app.LDAP.BaseDN, "(objectClass=*)", 0); code(err) != ldap.LDAPResultInsufficientAccessRights {
		t.Fatalf("a failed bind must drop the earlier identity: %v", err)
	}
	if err := c.Bind(second.LDAP.BindDN, secondPassword); err != nil {
		t.Fatal(err)
	}
	if printer, err := search(c, "ou=people,"+second.LDAP.BaseDN, "(uid=*)", 0); err != nil || len(printer.Entries) != 1 || printer.Entries[0].GetAttributeValue("uid") != "bob@example.com" {
		t.Fatalf("each application sees its own people: %v %q", err, dns(printer))
	}
	if err := c.UnauthenticatedBind(""); code(err) != ldap.LDAPResultInappropriateAuthentication {
		t.Fatalf("anonymous bind must fail: %v", err)
	}
	if err := c.UnauthenticatedBind(app.LDAP.BindDN); code(err) != ldap.LDAPResultUnwillingToPerform {
		t.Fatalf("a bind without a password must fail: %v", err)
	}
	if _, err := search(c, app.LDAP.BaseDN, "(objectClass=*)", 0); code(err) != ldap.LDAPResultInsufficientAccessRights {
		t.Fatalf("an anonymous connection must not search: %v", err)
	}

	rotated := l.expect(l.admin, "POST", "/api/v1/applications/"+app.ID+"/secrets", nil, http.StatusCreated, "")
	var fresh struct {
		Secret string `json:"clientSecret"`
	}
	json.Unmarshal(rotated.Body.Bytes(), &fresh)
	if err := c.Bind(app.LDAP.BindDN, password); code(err) != ldap.LDAPResultInvalidCredentials {
		t.Fatalf("a rotated bind password must stop working: %v", err)
	}
	if err := c.Bind(app.LDAP.BindDN, fresh.Secret); err != nil {
		t.Fatalf("the new bind password must work: %v", err)
	}
	l.expect(l.admin, "POST", "/api/v1/applications/"+app.ID+"/disable", nil, http.StatusOK, "")
	if _, err := search(c, app.LDAP.BaseDN, "(objectClass=*)", 0); code(err) != ldap.LDAPResultInsufficientAccessRights {
		t.Fatalf("a disabled application must stop searching on a bound connection: %v", err)
	}
	if err := c.Bind(app.LDAP.BindDN, fresh.Secret); code(err) != ldap.LDAPResultInvalidCredentials {
		t.Fatalf("a disabled application must not bind: %v", err)
	}
}

func TestLDAPPersonBind(t *testing.T) {
	l := setupLDAP(t, "ldaps")
	ctx := context.Background()
	staff, other := l.group("Staff"), l.group("Other")
	app, _ := l.application("NAS", staff.ID)
	ada := l.member("ada@example.com", "correct horse battery", staff.ID)
	l.member("carol@example.com", "", staff.ID)
	l.member("bob@example.com", "bob's password", other.ID)
	dave := l.member("dave@example.com", "dave's password", staff.ID)
	erin := l.member("erin@example.com", "erin's password", staff.ID)
	dn := func(email string) string { return "uid=" + email + ",ou=people," + app.LDAP.BaseDN }
	settings, _ := l.st.Settings(ctx)
	settings.PasswordSecondFactor = false
	if err := l.st.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	l.st.ForgetSettings()

	c := l.dial()
	if err := c.Bind(dn("ADA@example.com"), "correct horse battery"); err != nil {
		t.Fatalf("an assigned person must bind with their password: %v", err)
	}
	own, err := search(c, app.LDAP.BaseDN, "(objectClass=*)", 0)
	if err != nil || slices.Contains(dns(own), dn("carol@example.com")) || !slices.Contains(dns(own), dn("ada@example.com")) || slices.ContainsFunc(dns(own), func(d string) bool { return strings.HasPrefix(d, "cn=") }) {
		t.Fatalf("a person sees only their own entry: %v %q", err, dns(own))
	}
	reasons := map[string]string{"ada@example.com": "The password did not match.", "bob@example.com": "User is not assigned to this application.", "carol@example.com": "The account has no password.", "nobody@example.com": "No account uses this email address."}
	for email, password := range map[string]string{"ada@example.com": "wrong", "bob@example.com": "bob's password", "carol@example.com": "anything", "nobody@example.com": "anything"} {
		if err := c.Bind(dn(email), password); code(err) != ldap.LDAPResultInvalidCredentials {
			t.Fatalf("%s must not bind: %v", email, err)
		}
	}
	events, err := l.st.ListSignIns(ctx, store.SignInFilter{AppID: app.ID})
	if err != nil || len(events) != 5 || events[4].Result != "success" || *events[4].UserID != ada.ID || events[4].Method != "password" {
		t.Fatalf("every person bind must be in the sign-in log: %v %+v", err, events)
	}
	for _, e := range events[:4] {
		if e.Result != "failure" || e.Reason != reasons[e.Email] {
			t.Fatalf("refused bind for %s: %+v", e.Email, e)
		}
	}

	if err := c.Bind(dn("erin@example.com"), "erin's password"); err != nil {
		t.Fatal(err)
	}
	if err := l.st.SetUserStatus(ctx, erin.ID, "suspended"); err != nil {
		t.Fatal(err)
	}
	if err := c.Bind(dn("erin@example.com"), "erin's password"); code(err) != ldap.LDAPResultInvalidCredentials {
		t.Fatalf("a suspended person must not bind: %v", err)
	}
	if last, _ := l.st.ListSignIns(ctx, store.SignInFilter{UserID: erin.ID, Limit: 1}); len(last) != 1 || last[0].Reason != "The account is suspended." {
		t.Fatalf("the suspension must be logged: %+v", last)
	}

	for range 5 {
		if err := c.Bind(dn("dave@example.com"), "guess"); code(err) != ldap.LDAPResultInvalidCredentials {
			t.Fatal(err)
		}
	}
	if err := c.Bind(dn("dave@example.com"), "dave's password"); code(err) != ldap.LDAPResultInvalidCredentials {
		t.Fatalf("a locked account must not bind, even with the right password: %v", err)
	}
	locked, _ := l.st.ListSignIns(ctx, store.SignInFilter{UserID: dave.ID, Limit: 1})
	if len(locked) != 1 || locked[0].Reason != "Too many failed attempts." {
		t.Fatalf("the lockout must be logged: %+v", locked)
	}

	if _, err := l.st.CreateAccessPolicy(ctx, store.AccessPolicy{Name: "No NAS for Ada", Enabled: true, Mode: "enforce", Effect: "block", Conditions: store.PolicyConditions{UserIDs: []string{ada.ID}, AppIDs: []string{app.ID}}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Bind(dn("ada@example.com"), "correct horse battery"); code(err) != ldap.LDAPResultInvalidCredentials {
		t.Fatalf("access policies must apply to binds: %v", err)
	}
	if blocked, _ := l.st.ListSignIns(ctx, store.SignInFilter{UserID: ada.ID, Limit: 1}); len(blocked) != 1 || !strings.HasPrefix(blocked[0].Reason, "Policy “No NAS for Ada”") {
		t.Fatalf("the policy refusal must be logged: %+v", blocked)
	}
}

func TestLDAPPersonBindNeedsPasswordsWithoutSecondFactor(t *testing.T) {
	l := setupLDAP(t, "ldaps")
	ctx := context.Background()
	staff := l.group("Staff")
	app, _ := l.application("NAS", staff.ID)
	ada := l.member("ada@example.com", "correct horse battery", staff.ID)
	dn := "uid=ada@example.com,ou=people," + app.LDAP.BaseDN
	c := l.dial()
	refused := func(why string) {
		t.Helper()
		err := c.Bind(dn, "correct horse battery")
		last, _ := l.st.ListSignIns(ctx, store.SignInFilter{UserID: ada.ID, Limit: 1})
		if code(err) != ldap.LDAPResultInvalidCredentials || len(last) != 1 || last[0].Reason != why {
			t.Fatalf("want refusal %q: %v %+v", why, err, last)
		}
	}
	refused("Passwords need an authenticator app code as well, which an LDAP bind can't carry.")
	settings, _ := l.st.Settings(ctx)
	settings.PasswordSecondFactor = false
	if err := l.st.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	l.st.ForgetSettings()
	if err := c.Bind(dn, "correct horse battery"); err != nil {
		t.Fatalf("without the second factor the password binds: %v", err)
	}
	if err := l.st.SetMethodEnabled(ctx, "password", false); err != nil {
		t.Fatal(err)
	}
	refused("Sign-in with passwords is turned off for this organization.")
}

func TestLDAPStartTLSIsRequiredBeforeBind(t *testing.T) {
	l := setupLDAP(t, "starttls")
	staff := l.group("Staff")
	app, password := l.application("NAS", staff.ID)
	c, err := ldap.DialURL("ldap://" + l.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Bind(app.LDAP.BindDN, password); code(err) != ldap.LDAPResultConfidentialityRequired {
		t.Fatalf("a bind without TLS must be refused before the password is checked: %v", err)
	}
	if err := c.StartTLS(&tls.Config{RootCAs: l.roots, ServerName: "localhost"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Bind(app.LDAP.BindDN, password); err != nil {
		t.Fatalf("a bind after StartTLS must work: %v", err)
	}
	if app.LDAP.URL != "ldap://localhost:636" {
		t.Fatalf("connection URL: %q", app.LDAP.URL)
	}
}

func TestLDAPServerTurnedOffClosesConnections(t *testing.T) {
	l := setupLDAP(t, "ldaps")
	staff := l.group("Staff")
	app, password := l.application("NAS", staff.ID)
	open := l.dial()
	if err := open.Bind(app.LDAP.BindDN, password); err != nil {
		t.Fatal(err)
	}
	l.expect(l.admin, "PATCH", "/api/v1/settings", map[string]any{"featuresOff": []string{"ldap"}}, http.StatusConflict, "ERR_FEATURE_IN_USE")
	l.expect(l.admin, "PATCH", "/api/v1/settings", map[string]any{"featuresOff": []string{"ldap"}, "confirmFeatures": true}, http.StatusOK, "")
	if _, err := search(open, app.LDAP.BaseDN, "(objectClass=*)", 0); err == nil {
		t.Fatal("a connection opened before the LDAP server was turned off must stop answering")
	}
	c, err := ldap.DialURL("ldaps://"+l.addr, ldap.DialWithTLSConfig(&tls.Config{RootCAs: l.roots, ServerName: "localhost"}))
	if err == nil {
		defer c.Close()
		err = c.Bind(app.LDAP.BindDN, password)
	}
	if err == nil {
		t.Fatal("a new connection must not bind while the LDAP server is off")
	}
	l.expect(l.admin, "PATCH", "/api/v1/settings", map[string]any{"featuresOff": []string{}}, http.StatusOK, "")
	if err := l.dial().Bind(app.LDAP.BindDN, password); err != nil {
		t.Fatalf("binds must work again once the LDAP server is back on: %v", err)
	}
}
