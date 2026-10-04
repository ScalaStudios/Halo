package sshca_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"halo/internal/httpx"
	"halo/internal/sshca"
	"halo/internal/store"
	"halo/internal/testdb"
)

func TestCertificates(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	rule := `user.department == "Infrastructure"`
	infra, err := st.CreateGroup(ctx, store.NewGroup{Name: "Infrastructure", Kind: "dynamic", Rule: &rule})
	must(err)
	admins, err := st.CreateGroup(ctx, store.NewGroup{Name: "Cluster admins", Kind: "assigned"})
	must(err)
	security, err := st.CreateUser(ctx, store.NewUser{Email: "sam@example.com", Name: "Sam", Status: "active", Roles: []string{"security_admin"}})
	must(err)
	auditor, err := st.CreateUser(ctx, store.NewUser{Email: "ann@example.com", Name: "Ann", Status: "active", Roles: []string{"auditor"}})
	must(err)
	ops, err := st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada", Status: "active", Department: "Infrastructure", GroupIDs: []string{admins.ID}})
	must(err)
	other, err := st.CreateUser(ctx, store.NewUser{Email: "bo@example.com", Name: "Bo", Status: "active", Department: "Design"})
	must(err)
	session := map[string]string{}
	for _, u := range []store.User{security, auditor, ops, other} {
		_, token, err := st.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: "passkey"})
		must(err)
		session[u.ID] = token
	}

	public, _ := url.Parse("http://halo.test")
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	mux := http.NewServeMux()
	must(sshca.Register(mux, httpx.Deps{Store: st, Auth: authn}))
	handler := authn.Resolve(authn.SameOrigin(mux))
	call := func(method, path, as string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var reader io.Reader
		if body != nil {
			data, _ := json.Marshal(body)
			reader = bytes.NewReader(data)
		}
		req := httptest.NewRequest(method, path, reader)
		if as != "" {
			req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: session[as]})
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	expect := func(rec *httptest.ResponseRecorder, status int) {
		t.Helper()
		if rec.Code != status {
			t.Fatalf("want %d, got %d: %s", status, rec.Code, rec.Body.String())
		}
	}

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	must(err)
	sshKey, err := ssh.NewPublicKey(pub)
	must(err)
	authorized := string(ssh.MarshalAuthorizedKey(sshKey))

	expect(call("POST", "/api/v1/me/ssh/certificates", "", map[string]string{"publicKey": authorized}), 401)
	expect(call("POST", "/api/v1/me/ssh/certificates", ops.ID, map[string]string{"publicKey": authorized}), 403)

	expect(call("POST", "/api/v1/ssh/principal-mappings", auditor.ID, map[string]any{"groupId": infra.ID, "principals": []string{"ops"}}), 403)
	expect(call("POST", "/api/v1/ssh/principal-mappings", security.ID, map[string]any{"groupId": infra.ID, "principals": []string{"Ops Team"}}), 422)
	expect(call("POST", "/api/v1/ssh/principal-mappings", security.ID, map[string]any{"groupId": infra.ID, "principals": []string{"ops", "deploy"}}), 201)
	expect(call("POST", "/api/v1/ssh/principal-mappings", security.ID, map[string]any{"groupId": infra.ID, "principals": []string{"web"}}), 409)
	rec := call("POST", "/api/v1/ssh/principal-mappings", security.ID, map[string]any{"groupId": admins.ID, "principals": []string{"web"}})
	expect(rec, 201)
	var mapping store.SSHPrincipalMapping
	must(json.Unmarshal(rec.Body.Bytes(), &mapping))
	expect(call("PUT", "/api/v1/ssh/principal-mappings/"+mapping.ID, security.ID, map[string]any{"groupId": admins.ID, "principals": []string{"root", "ops"}}), 200)

	expect(call("PUT", "/api/v1/ssh/authority", security.ID, map[string]int{"certificateLifetime": 90000}), 422)
	expect(call("PUT", "/api/v1/ssh/authority", auditor.ID, map[string]int{"certificateLifetime": 3600}), 403)
	expect(call("PUT", "/api/v1/ssh/authority", security.ID, map[string]int{"certificateLifetime": 3600}), 200)

	rec = call("GET", "/api/v1/ssh/ca.pub", "", nil)
	expect(rec, 200)
	caKey, _, _, _, err := ssh.ParseAuthorizedKey(rec.Body.Bytes())
	must(err)

	expect(call("POST", "/api/v1/me/ssh/certificates", other.ID, map[string]string{"publicKey": authorized}), 403)
	expect(call("POST", "/api/v1/me/ssh/certificates", ops.ID, map[string]string{"publicKey": "not a key"}), 422)
	weak, err := rsa.GenerateKey(rand.Reader, 1024)
	must(err)
	weakKey, err := ssh.NewPublicKey(&weak.PublicKey)
	must(err)
	expect(call("POST", "/api/v1/me/ssh/certificates", ops.ID, map[string]string{"publicKey": string(ssh.MarshalAuthorizedKey(weakKey))}), 422)

	rec = call("POST", "/api/v1/me/ssh/certificates", ops.ID, map[string]string{"publicKey": authorized})
	expect(rec, 201)
	var issued struct {
		Certificate string   `json:"certificate"`
		Serial      uint64   `json:"serial"`
		KeyID       string   `json:"keyId"`
		Principals  []string `json:"principals"`
	}
	must(json.Unmarshal(rec.Body.Bytes(), &issued))
	parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(issued.Certificate))
	must(err)
	cert := parsed.(*ssh.Certificate)
	checker := ssh.CertChecker{IsUserAuthority: func(auth ssh.PublicKey) bool { return bytes.Equal(auth.Marshal(), caKey.Marshal()) }}
	must(checker.CheckCert("root", cert))
	if err := checker.CheckCert("admin", cert); err == nil {
		t.Fatal("certificate must not be valid for an unmapped principal")
	}
	lifetime := time.Unix(int64(cert.ValidBefore), 0).Sub(time.Now())
	_, pty := cert.Permissions.Extensions["permit-pty"]
	if cert.CertType != ssh.UserCert || !bytes.Equal(cert.Key.Marshal(), sshKey.Marshal()) || !slices.Equal(cert.ValidPrincipals, []string{"deploy", "ops", "root"}) ||
		cert.KeyId != fmt.Sprintf("halo:%s:%d", ops.ID, cert.Serial) || issued.KeyID != cert.KeyId || cert.Serial != issued.Serial ||
		lifetime < 59*time.Minute || lifetime > time.Hour || !pty || len(cert.Permissions.Extensions) != 5 {
		t.Fatalf("certificate: %+v (lifetime %s)", cert, lifetime)
	}
	expect(call("POST", "/api/v1/me/ssh/certificates", ops.ID, map[string]string{"publicKey": issued.Certificate}), 422)

	rec = call("GET", "/api/v1/ssh/certificates", auditor.ID, nil)
	expect(rec, 200)
	var listed []store.SSHCertificate
	must(json.Unmarshal(rec.Body.Bytes(), &listed))
	if len(listed) != 1 || listed[0].UserName != "Ada" || listed[0].Fingerprint != ssh.FingerprintSHA256(sshKey) || listed[0].Serial != issued.Serial {
		t.Fatalf("issued certificates: %+v", listed)
	}
	expect(call("GET", "/api/v1/ssh/certificates", ops.ID, nil), 403)
	expect(call("DELETE", "/api/v1/ssh/principal-mappings/"+mapping.ID, security.ID, nil), 204)
	expect(call("DELETE", "/api/v1/ssh/principal-mappings/"+mapping.ID, security.ID, nil), 404)
}
