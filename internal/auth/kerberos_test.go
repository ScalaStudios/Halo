package auth_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	krbclient "github.com/jcmturner/gokrb5/v8/client"
	krbconfig "github.com/jcmturner/gokrb5/v8/config"
	"github.com/jcmturner/gokrb5/v8/iana/chksumtype"
	"github.com/jcmturner/gokrb5/v8/iana/etypeID"
	"github.com/jcmturner/gokrb5/v8/iana/nametype"
	"github.com/jcmturner/gokrb5/v8/keytab"
	"github.com/jcmturner/gokrb5/v8/messages"
	"github.com/jcmturner/gokrb5/v8/spnego"
	"github.com/jcmturner/gokrb5/v8/types"

	"halo/internal/auth"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/policy"
	"halo/internal/store"
	"halo/internal/testdb"
)

const serviceRealm = "CORP.EXAMPLE.COM"

func serviceKeytab(t *testing.T, password string, principals ...string) *keytab.Keytab {
	t.Helper()
	kt := keytab.New()
	for _, p := range principals {
		if err := kt.AddEntry(p, serviceRealm, password, time.Now(), 3, etypeID.AES256_CTS_HMAC_SHA1_96); err != nil {
			t.Fatal(err)
		}
	}
	return kt
}

type ticket struct {
	client, realm, service, keyFor string
	keys                           *keytab.Keytab
	end, ctime                     time.Time
}

func (k ticket) token(t *testing.T) string {
	t.Helper()
	now := time.Now().UTC()
	if k.end.IsZero() {
		k.end = now.Add(10 * time.Hour)
	}
	if k.ctime.IsZero() {
		k.ctime = now
	}
	start := k.end.Add(-10 * time.Hour)
	cname := types.NewPrincipalName(nametype.KRB_NT_PRINCIPAL, k.client)
	tkt, session, err := messages.NewTicket(cname, k.realm, types.NewPrincipalName(nametype.KRB_NT_SRV_INST, k.keyFor), serviceRealm, types.NewKrbFlags(), k.keys, etypeID.AES256_CTS_HMAC_SHA1_96, 3, start, start, k.end, k.end)
	if err != nil {
		t.Fatal(err)
	}
	tkt.SName = types.NewPrincipalName(nametype.KRB_NT_SRV_INST, k.service)
	authenticator, err := types.NewAuthenticator(k.realm, cname)
	if err != nil {
		t.Fatal(err)
	}
	checksum := make([]byte, 24)
	binary.LittleEndian.PutUint32(checksum, 16)
	authenticator.CTime, authenticator.Cusec = k.ctime.Truncate(time.Second), k.ctime.Nanosecond()/1000
	authenticator.Cksum = types.Checksum{CksumType: chksumtype.GSSAPI, Checksum: checksum}
	req, err := messages.NewAPReq(tkt, session, authenticator)
	if err != nil {
		t.Fatal(err)
	}
	cl := krbclient.NewWithPassword(k.client, k.realm, "unused", krbconfig.New())
	init, err := spnego.NewNegTokenInitKRB5(cl, tkt, session)
	if err != nil {
		t.Fatal(err)
	}
	mech, err := spnego.NewKRB5TokenAPREQ(cl, tkt, session, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	mech.APReq = req
	if init.MechTokenBytes, err = mech.Marshal(); err != nil {
		t.Fatal(err)
	}
	raw, err := (&spnego.SPNEGOToken{Init: true, NegTokenInit: init}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func kerberosServer(t *testing.T, st *store.Store) *env {
	public, _ := url.Parse("http://localhost:3200")
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	mux := http.NewServeMux()
	if err := auth.Register(mux, httpx.Deps{Config: config.Config{PublicURL: public, Dev: true}, Store: st, Auth: authn, Policy: policy.NewEngine(st)}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(authn.Resolve(mux))
	t.Cleanup(srv.Close)
	return &env{t: t, st: st, srv: srv}
}

type negotiated struct {
	status    int
	code      string
	challenge string
	redirect  string
}

func (e *env) negotiate(token string, body map[string]any) negotiated {
	e.t.Helper()
	payload, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/auth/kerberos", bytes.NewReader(payload))
	req.Header.Set("User-Agent", firefox)
	if token != "" {
		req.Header.Set("Authorization", "Negotiate "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	var out struct {
		Redirect string
		Error    struct{ Code string }
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return negotiated{res.StatusCode, out.Error.Code, res.Header.Get("WWW-Authenticate"), out.Redirect}
}

func (e *env) staff(role string) *http.Client {
	e.t.Helper()
	ctx := context.Background()
	u, err := e.st.CreateUser(ctx, store.NewUser{Email: role + "@example.com", Name: role, Status: "active", Roles: []string{role}})
	if err != nil {
		e.t.Fatal(err)
	}
	_, token, err := e.st.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: "passkey"})
	if err != nil {
		e.t.Fatal(err)
	}
	c := client()
	base, _ := url.Parse(e.srv.URL)
	c.Jar.SetCookies(base, []*http.Cookie{{Name: httpx.SessionCookie, Value: token}})
	return c
}

func encodeKeytab(t *testing.T, kt *keytab.Keytab) string {
	t.Helper()
	raw, err := kt.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func lastReason(t *testing.T, st *store.Store) string {
	t.Helper()
	events, err := st.ListSignIns(context.Background(), store.SignInFilter{})
	if err != nil || len(events) == 0 {
		t.Fatalf("sign-in log: %v %v", events, err)
	}
	return events[0].Reason
}

func TestKerberosSource(t *testing.T) {
	e := kerberosServer(t, testdb.New(t))
	ctx := context.Background()
	admin, auditor := e.staff("security_admin"), e.staff("auditor")
	keys := serviceKeytab(t, "service-secret", "HTTP/auth.example.com", "HTTP/other.example.com")
	body := func(edit func(map[string]any)) map[string]any {
		b := map[string]any{"servicePrincipal": "http/Auth.Example.com@corp.example.com", "realms": []map[string]string{{"realm": "corp.example.com", "domain": "Example.com"}}, "keytab": encodeKeytab(t, keys)}
		if edit != nil {
			edit(b)
		}
		return b
	}
	risky, err := e.st.SaveNetworkZone(ctx, store.NetworkZone{Name: "Tor", Kind: "risky", CIDRs: []string{"185.220.101.0/24"}})
	if err != nil {
		t.Fatal(err)
	}

	var empty json.RawMessage
	if status := e.do(auditor, "GET", "/api/v1/kerberos", nil, &empty); status != http.StatusOK || string(empty) != "null" {
		t.Fatalf("before setup: %d %s", status, empty)
	}
	e.expect(auditor, "PUT", "/api/v1/kerberos", body(nil), http.StatusForbidden, "ERR_FORBIDDEN")
	for _, edit := range []func(map[string]any){
		func(b map[string]any) { b["servicePrincipal"] = "host/auth.example.com@CORP.EXAMPLE.COM" },
		func(b map[string]any) { b["servicePrincipal"] = "HTTP/auth.example.com" },
		func(b map[string]any) { b["realms"] = []map[string]string{} },
		func(b map[string]any) {
			b["realms"] = []map[string]string{{"realm": "CORP.EXAMPLE.COM", "domain": "@example.com"}}
		},
		func(b map[string]any) { b["keytab"] = base64.StdEncoding.EncodeToString([]byte("not a keytab")) },
		func(b map[string]any) { b["keytab"] = encodeKeytab(t, serviceKeytab(t, "x", "HTTP/wiki.example.com")) },
		func(b map[string]any) { b["keytab"] = "" },
		func(b map[string]any) { b["zoneIds"] = []string{risky.ID} },
	} {
		e.expect(admin, "PUT", "/api/v1/kerberos", body(edit), http.StatusUnprocessableEntity, "ERR_INVALID")
	}

	var saved struct {
		ServicePrincipal string
		Realms           []store.KerberosRealm
		Keys             []struct {
			Version uint32
			Type    string
		}
	}
	if status := e.do(admin, "PUT", "/api/v1/kerberos", body(nil), &saved); status != http.StatusOK || saved.ServicePrincipal != "HTTP/auth.example.com@CORP.EXAMPLE.COM" ||
		len(saved.Realms) != 1 || saved.Realms[0] != (store.KerberosRealm{Realm: "CORP.EXAMPLE.COM", Domain: "example.com"}) || len(saved.Keys) != 1 || saved.Keys[0].Version != 3 || saved.Keys[0].Type != "aes256-cts-hmac-sha1-96" {
		t.Fatalf("save: %d %+v", status, saved)
	}
	if status := e.do(admin, "PUT", "/api/v1/kerberos", body(func(b map[string]any) { delete(b, "keytab") }), &saved); status != http.StatusOK || len(saved.Keys) != 1 {
		t.Fatalf("save without a new keytab: %d %+v", status, saved)
	}
	var listed json.RawMessage
	if status := e.do(auditor, "GET", "/api/v1/kerberos", nil, &listed); status != http.StatusOK || strings.Contains(string(listed), "keytab") {
		t.Fatalf("read: %d %s", status, listed)
	}
	if status := e.do(admin, "DELETE", "/api/v1/kerberos", nil, nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	e.expect(admin, "DELETE", "/api/v1/kerberos", nil, http.StatusNotFound, "ERR_NOT_FOUND")
	events, err := e.st.ListAudit(ctx, 5)
	if err != nil || auditActions(events) != "kerberos.delete,kerberos.update,kerberos.update" {
		t.Fatalf("audit: %s %v", auditActions(events), err)
	}
}

func TestKerberosSignIn(t *testing.T) {
	st := testdb.New(t)
	e, other := kerberosServer(t, st), kerberosServer(t, st)
	ctx := context.Background()
	group, err := st.CreateGroup(ctx, store.NewGroup{Name: "Staff", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	ada, err := st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active", GroupIDs: []string{group.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser(ctx, store.NewUser{Email: "grace@example.com", Name: "Grace Hopper", Status: "suspended"}); err != nil {
		t.Fatal(err)
	}
	keys := serviceKeytab(t, "service-secret", "HTTP/auth.example.com", "HTTP/other.example.com")
	admin := e.staff("security_admin")
	trusted, err := st.SaveNetworkZone(ctx, store.NetworkZone{Name: "Office", Kind: "trusted", CIDRs: []string{"127.0.0.0/8", "::1/128"}})
	if err != nil {
		t.Fatal(err)
	}
	configure := func(zones ...string) {
		t.Helper()
		body := map[string]any{"servicePrincipal": "HTTP/auth.example.com@CORP.EXAMPLE.COM", "realms": []map[string]string{{"realm": "CORP.EXAMPLE.COM", "domain": "example.com"}}, "zoneIds": zones, "keytab": encodeKeytab(t, keys)}
		if status := e.do(admin, "PUT", "/api/v1/kerberos", body, nil); status != http.StatusOK {
			t.Fatalf("configure: %d", status)
		}
	}
	valid := func(client string) ticket {
		return ticket{client: client, realm: serviceRealm, service: "HTTP/auth.example.com", keyFor: "HTTP/auth.example.com", keys: keys}
	}
	expectRejected := func(name string, got negotiated, code, reason string) {
		t.Helper()
		if got.status != http.StatusForbidden || got.code != code || got.challenge != "" {
			t.Fatalf("%s: %+v", name, got)
		}
		if logged := lastReason(t, st); !strings.Contains(logged, reason) {
			t.Fatalf("%s: logged %q, want %q", name, logged, reason)
		}
	}

	if got := e.negotiate(valid("ada").token(t), map[string]any{}); got.status != http.StatusNotFound || got.code != "ERR_KERBEROS_UNAVAILABLE" || got.challenge != "" {
		t.Fatalf("before setup: %+v", got)
	}
	configure()
	if got := e.negotiate(valid("ada").token(t), map[string]any{}); got.status != http.StatusNotFound || got.code != "ERR_KERBEROS_UNAVAILABLE" {
		t.Fatalf("method off: %+v", got)
	}
	if err := st.SetMethodEnabled(ctx, "kerberos", true); err != nil {
		t.Fatal(err)
	}

	if got := e.negotiate("", map[string]any{}); got.status != http.StatusUnauthorized || got.code != "ERR_KERBEROS_NO_TICKET" || got.challenge != "Negotiate" {
		t.Fatalf("challenge: %+v", got)
	}
	if got := e.negotiate("", map[string]any{"automatic": true}); got.status != http.StatusNoContent || got.challenge != "" {
		t.Fatalf("automatic outside a zone: %+v", got)
	}
	if events, _ := st.ListSignIns(ctx, store.SignInFilter{}); len(events) != 0 {
		t.Fatalf("challenges are not sign-in attempts: %+v", events)
	}

	first := valid("Ada").token(t)
	if got := e.negotiate(first, map[string]any{}); got.status != http.StatusOK || got.redirect != "/account" {
		t.Fatalf("valid ticket: %+v", got)
	}
	if linked, err := st.KerberosPrincipalUser(ctx, "ada@CORP.EXAMPLE.COM"); err != nil || linked != ada.ID {
		t.Fatalf("link: %q %v", linked, err)
	}
	expectRejected("replayed authenticator", other.negotiate(first, map[string]any{}), "ERR_KERBEROS_REJECTED", "replay")
	if got := other.negotiate(valid("ada").token(t), map[string]any{}); got.status != http.StatusOK {
		t.Fatalf("second sign-in: %+v", got)
	}
	audit, err := st.ListAudit(ctx, 10)
	if err != nil || !strings.HasPrefix(auditActions(audit), "user.kerberos.link,kerberos.update") || strings.Count(auditActions(audit), "user.kerberos.link") != 1 {
		t.Fatalf("audit: %s %v", auditActions(audit), err)
	}

	wrongKey := valid("ada")
	wrongKey.service, wrongKey.keyFor = "HTTP/other.example.com", "HTTP/other.example.com"
	expectRejected("ticket for another service", e.negotiate(wrongKey.token(t), map[string]any{}), "ERR_KERBEROS_REJECTED", "not Halo's service principal HTTP/auth.example.com@CORP.EXAMPLE.COM")
	renamed := valid("ada")
	renamed.service = "HTTP/other.example.com"
	expectRejected("ticket renamed to another service", e.negotiate(renamed.token(t), map[string]any{}), "ERR_KERBEROS_REJECTED", "not Halo's service principal")
	renamed.service = "HTTP/auth\xff\x00.example.com"
	expectRejected("ticket renamed to bytes that aren't text", e.negotiate(renamed.token(t), map[string]any{}), "ERR_KERBEROS_REJECTED", "The ticket is for HTTP/auth??.example.com@CORP.EXAMPLE.COM")
	stale := valid("ada")
	stale.keys = serviceKeytab(t, "old-secret", "HTTP/auth.example.com")
	expectRejected("ticket from an old key", e.negotiate(stale.token(t), map[string]any{}), "ERR_KERBEROS_REJECTED", "couldn't decrypt the ticket")
	expired := valid("ada")
	expired.end = time.Now().Add(-10 * time.Minute)
	expectRejected("expired ticket", e.negotiate(expired.token(t), map[string]any{}), "ERR_KERBEROS_REJECTED", "ticket expired")
	skewed := valid("ada")
	skewed.ctime = time.Now().Add(-10 * time.Minute)
	expectRejected("skewed clock", e.negotiate(skewed.token(t), map[string]any{}), "ERR_KERBEROS_REJECTED", "more than 5 minutes")
	expectRejected("unmapped principal", e.negotiate(valid("nobody").token(t), map[string]any{}), "ERR_KERBEROS_REJECTED", "No person's email address is nobody@example.com")
	if _, err := st.CreateUser(ctx, store.NewUser{Email: "root@example.com", Name: "Root", Status: "active", Roles: []string{"global_admin"}}); err != nil {
		t.Fatal(err)
	}
	expectRejected("administrator matched by email", e.negotiate(valid("root").token(t), map[string]any{}), "ERR_KERBEROS_REJECTED", "Root holds or is eligible for an administrator role")
	foreign := valid("ada")
	foreign.realm = "PARTNER.EXAMPLE.NET"
	expectRejected("unmapped realm", e.negotiate(foreign.token(t), map[string]any{}), "ERR_KERBEROS_REJECTED", "PARTNER.EXAMPLE.NET isn't one of the realms")
	expectRejected("machine principal", e.negotiate(valid("host/pc1.corp.example.com").token(t), map[string]any{}), "ERR_KERBEROS_REJECTED", "isn't a person's principal")
	expectRejected("ntlm", e.negotiate(base64.StdEncoding.EncodeToString([]byte("NTLMSSP\x00\x01\x00\x00\x00")), map[string]any{}), "ERR_KERBEROS_REJECTED", "NTLM")
	expectRejected("suspended person", e.negotiate(valid("grace").token(t), map[string]any{}), "ERR_ACCOUNT_SUSPENDED", "The account is suspended.")
	if _, err := st.KerberosPrincipalUser(ctx, "grace@CORP.EXAMPLE.COM"); err == nil {
		t.Fatal("a refused sign-in must not link the principal")
	}

	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Wiki", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://wiki.example.com/cb"}, GroupIDs: []string{group.ID}})
	if err != nil {
		t.Fatal(err)
	}
	request := func(id string) map[string]any {
		t.Helper()
		if _, err := st.CreateAuthRequest(ctx, store.AuthRequest{ID: id, ClientID: app.ClientID, RedirectURI: app.RedirectURIs[0], Scopes: []string{"openid"}, ResponseType: "code"}); err != nil {
			t.Fatal(err)
		}
		return map[string]any{"authRequest": id}
	}
	if got := e.negotiate(valid("ada").token(t), request("arq_wiki")); got.status != http.StatusOK || got.redirect != store.AuthorizeCallbackPath("arq_wiki") {
		t.Fatalf("application sign-in: %+v", got)
	}
	app.KerberosSignIn = false
	if err := st.UpdateApplication(ctx, app); err != nil {
		t.Fatal(err)
	}
	expectRejected("application with Windows sign-in off", e.negotiate(valid("ada").token(t), request("arq_off")), "ERR_BLOCKED_BY_POLICY", "Windows sign-in is turned off for Wiki.")
	browser := client()
	signIn, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/auth/kerberos", strings.NewReader("{}"))
	signIn.Header.Set("Authorization", "Negotiate "+valid("ada").token(t))
	if res, err := browser.Do(signIn); err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("sign in to Halo with Windows: %v %v", res, err)
	} else {
		res.Body.Close()
	}
	e.expect(browser, "POST", "/api/v1/auth/continue", request("arq_sso"), http.StatusForbidden, "ERR_BLOCKED_BY_POLICY")
	if logged := lastReason(t, st); logged != "Windows sign-in is turned off for Wiki." {
		t.Fatalf("single sign-on from a Windows session: logged %q", logged)
	}

	configure(trusted.ID)
	if got := e.negotiate("", map[string]any{"automatic": true}); got.status != http.StatusUnauthorized || got.challenge != "Negotiate" {
		t.Fatalf("automatic inside a trusted zone: %+v", got)
	}
	if got := e.negotiate("", map[string]any{"automatic": true, "authRequest": "arq_off"}); got.status != http.StatusNoContent {
		t.Fatalf("automatic for an application with Windows sign-in off: %+v", got)
	}

	for range 5 {
		if err := st.RecordSignIn(ctx, store.SignInEvent{Result: "failure", Method: "totp", UserID: &ada.ID, Email: ada.Email, IP: "127.0.0.1", Reason: "The code did not match."}); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.negotiate(valid("ada").token(t), map[string]any{}); got.status != http.StatusTooManyRequests || got.code != "ERR_TOO_MANY_ATTEMPTS" {
		t.Fatalf("locked out: %+v", got)
	}

	moved, err := st.GetUser(ctx, ada.ID)
	if err != nil {
		t.Fatal(err)
	}
	moved.Email = "ada.lovelace@example.com"
	if err := st.UpdateUserProfile(ctx, moved); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Byron", Status: "active", GroupIDs: []string{group.ID}}); err != nil {
		t.Fatal(err)
	}
	expectRejected("reissued account name", other.negotiate(valid("ada").token(t), map[string]any{}), "ERR_KERBEROS_REJECTED", "is linked to ada.lovelace@example.com but its realm now maps it to ada@example.com")
}
