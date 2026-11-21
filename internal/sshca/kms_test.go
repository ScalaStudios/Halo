package sshca_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"halo/internal/httpx"
	"halo/internal/kms"
	"halo/internal/kms/kmstest"
	"halo/internal/sshca"
	"halo/internal/store"
	"halo/internal/testdb"
)

func TestExternalAuthority(t *testing.T) {
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
	group, err := st.CreateGroup(ctx, store.NewGroup{Name: "Ops", Kind: "assigned"})
	must(err)
	ops, err := st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada", Status: "active", GroupIDs: []string{group.ID}})
	must(err)
	_, err = st.SaveSSHPrincipalMapping(ctx, store.SSHPrincipalMapping{GroupID: group.ID, Principals: []string{"ops"}})
	must(err)
	_, session, err := st.CreateSession(ctx, store.NewSession{UserID: ops.ID, Method: "passkey"})
	must(err)
	public, _ := url.Parse("http://halo.test")
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	mux := http.NewServeMux()
	must(sshca.Register(mux, httpx.Deps{Store: st, Auth: authn}))
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: session})
		rec := httptest.NewRecorder()
		authn.Resolve(authn.SameOrigin(mux)).ServeHTTP(rec, req)
		return rec
	}

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	must(err)
	userKey, err := ssh.NewPublicKey(pub)
	must(err)
	body, _ := json.Marshal(map[string]string{"publicKey": string(ssh.MarshalAuthorizedKey(userKey))})
	rec := call("POST", "/api/v1/me/ssh/certificates", string(body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("issue: %d %s", rec.Code, rec.Body)
	}
	var issued struct{ Certificate string }
	must(json.Unmarshal(rec.Body.Bytes(), &issued))
	parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(issued.Certificate))
	must(err)

	authority, err := st.GetSSHAuthority(ctx, false)
	must(err)
	signer, err := st.Keys.Signer(ctx, authority.ExternalKey)
	must(err)
	caKey, err := ssh.NewPublicKey(signer.Public())
	must(err)
	published, _, _, _, err := ssh.ParseAuthorizedKey(call("GET", "/api/v1/ssh/ca.pub", "").Body.Bytes())
	must(err)
	if authority.Seed != nil || !bytes.Equal(published.Marshal(), caKey.Marshal()) {
		t.Fatal("the certificate authority key must live only in Vault and be the published one")
	}
	checker := ssh.CertChecker{IsUserAuthority: func(auth ssh.PublicKey) bool { return bytes.Equal(auth.Marshal(), caKey.Marshal()) }}
	must(checker.CheckCert("ops", parsed.(*ssh.Certificate)))

	host, err := st.CreateSSHHost(ctx, []string{"web1.example.com"})
	must(err)
	_, token, err := st.IssueSSHHostToken(ctx, host.ID)
	must(err)
	hostPub, _, err := ed25519.GenerateKey(rand.Reader)
	must(err)
	hostKey, err := ssh.NewPublicKey(hostPub)
	must(err)
	body, _ = json.Marshal(map[string]string{"token": token, "publicKey": string(ssh.MarshalAuthorizedKey(hostKey))})
	rec = call("POST", "/api/v1/ssh/hosts/enroll", string(body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll: %d %s", rec.Code, rec.Body)
	}
	must(json.Unmarshal(rec.Body.Bytes(), &issued))
	parsed, _, _, _, err = ssh.ParseAuthorizedKey([]byte(issued.Certificate))
	must(err)
	hostAuthority, err := st.GetSSHAuthority(ctx, true)
	must(err)
	if hostAuthority.Seed != nil || hostAuthority.ExternalKey == "" || hostAuthority.ExternalKey == authority.ExternalKey {
		t.Fatalf("the host authority must be its own key in Vault: %+v", hostAuthority)
	}
	hostSigner, err := st.Keys.Signer(ctx, hostAuthority.ExternalKey)
	must(err)
	hostCA, err := ssh.NewPublicKey(hostSigner.Public())
	must(err)
	hostChecker := ssh.CertChecker{IsHostAuthority: func(auth ssh.PublicKey, _ string) bool { return bytes.Equal(auth.Marshal(), hostCA.Marshal()) }}
	must(hostChecker.CheckHostKey("web1.example.com:22", nil, parsed))
}
