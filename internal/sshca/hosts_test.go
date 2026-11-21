package sshca_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"halo/internal/hostd"
	"halo/internal/httpx"
	"halo/internal/sshca"
	"halo/internal/store"
	"halo/internal/testdb"
)

func TestHostDirectory(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	ops, err := st.CreateGroup(ctx, store.NewGroup{Name: "Ops", Kind: "assigned"})
	must(err)
	devs, err := st.CreateGroup(ctx, store.NewGroup{Name: "Developers", Kind: "assigned"})
	must(err)
	security, err := st.CreateUser(ctx, store.NewUser{Email: "sam@example.com", Name: "Sam", Status: "active", Roles: []string{"security_admin"}})
	must(err)
	auditor, err := st.CreateUser(ctx, store.NewUser{Email: "ann@example.com", Name: "Ann", Status: "active", Roles: []string{"auditor"}})
	must(err)
	ada, err := st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active", GroupIDs: []string{ops.ID, devs.ID}})
	must(err)
	bo, err := st.CreateUser(ctx, store.NewUser{Email: "bo@example.com", Name: "Bo", Status: "active", GroupIDs: []string{devs.ID}})
	must(err)
	_, err = st.CreateUser(ctx, store.NewUser{Email: "cy@example.com", Name: "Cy", Status: "suspended", GroupIDs: []string{ops.ID}})
	must(err)
	dee, err := st.CreateUser(ctx, store.NewUser{Email: "dee@example.com", Name: "Dee", Status: "active"})
	must(err)
	session := map[string]string{}
	for _, u := range []store.User{security, auditor, ada, bo, dee} {
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

	expect(call("PUT", "/api/v1/ssh/posix", auditor.ID, map[string]any{"uidMin": 100000, "uidMax": 199999, "gidMin": 200000, "gidMax": 299999, "loginShell": "/bin/bash"}), 403)
	expect(call("PUT", "/api/v1/ssh/posix", security.ID, map[string]any{"uidMin": 1000, "uidMax": 199999, "gidMin": 200000, "gidMax": 299999, "loginShell": "/bin/bash"}), 422)
	expect(call("PUT", "/api/v1/ssh/posix", security.ID, map[string]any{"uidMin": 100000, "uidMax": 250000, "gidMin": 200000, "gidMax": 299999, "loginShell": "/bin/bash"}), 422)
	expect(call("PUT", "/api/v1/ssh/posix", security.ID, map[string]any{"uidMin": 100000, "uidMax": 199999, "gidMin": 200000, "gidMax": 1 << 32, "loginShell": "/bin/bash"}), 422)
	expect(call("PUT", "/api/v1/ssh/posix", security.ID, map[string]any{"uidMin": 100000, "uidMax": 199999, "gidMin": 200000, "gidMax": 299999, "loginShell": "bash"}), 422)
	expect(call("PUT", "/api/v1/ssh/posix", security.ID, map[string]any{"uidMin": 100000, "uidMax": 199999, "gidMin": 200000, "gidMax": 299999, "loginShell": "/bin/zsh"}), 200)
	expect(call("GET", "/api/v1/ssh/posix", auditor.ID, nil), 200)

	production := map[string]any{"name": "Production", "rules": []map[string]any{{"groupId": ops.ID, "sudo": []string{"ALL"}}, {"groupId": devs.ID, "sudo": []string{"/usr/bin/journalctl", "/usr/bin/systemctl  restart nginx"}}}}
	expect(call("POST", "/api/v1/ssh/host-groups", auditor.ID, production), 403)
	for _, bad := range []string{"rm -rf /", "/bin/ls *", "/bin/sh, ALL", "/usr/bin/env A=b", "/bin/true\n#1 ALL=(ALL) ALL"} {
		expect(call("POST", "/api/v1/ssh/host-groups", security.ID, map[string]any{"name": "Bad", "rules": []map[string]any{{"groupId": ops.ID, "sudo": []string{bad}}}}), 422)
	}
	expect(call("POST", "/api/v1/ssh/host-groups", security.ID, map[string]any{"name": "Bad", "rules": []map[string]any{{"groupId": "grp_missing"}}}), 422)
	rec := call("POST", "/api/v1/ssh/host-groups", security.ID, production)
	expect(rec, 201)
	var group store.HostGroup
	must(json.Unmarshal(rec.Body.Bytes(), &group))
	if len(group.Rules) != 2 || !slices.Equal(group.Rules[0].Sudo, []string{"/usr/bin/journalctl", "/usr/bin/systemctl restart nginx"}) {
		t.Fatalf("rules: %+v", group.Rules)
	}
	expect(call("POST", "/api/v1/ssh/host-groups", security.ID, map[string]any{"name": "production"}), 409)
	staging := call("POST", "/api/v1/ssh/host-groups", security.ID, map[string]any{"name": "Staging"})
	expect(staging, 201)

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	hostKey, err := ssh.NewPublicKey(pub)
	must(err)
	signer, err := ssh.NewSignerFromKey(priv)
	must(err)
	host, err := st.CreateSSHHost(ctx, []string{"web1.example.com"})
	must(err)
	must(st.CertifySSHHost(ctx, host.ID, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(hostKey))), ssh.FingerprintSHA256(hostKey), hostKey.Type(), 1, time.Now().Add(time.Hour)))
	signed := func(message []byte, at time.Time) map[string]any {
		sig, err := signer.Sign(rand.Reader, message)
		must(err)
		return map[string]any{"publicKey": string(ssh.MarshalAuthorizedKey(hostKey)), "timestamp": at.Unix(), "signature": base64.StdEncoding.EncodeToString(ssh.Marshal(sig))}
	}
	directory := func() hostd.Directory {
		t.Helper()
		rec := call("POST", "/api/v1/ssh/hosts/directory", "", signed(hostd.DirectoryMessage(time.Now().Unix()), time.Now()))
		expect(rec, 200)
		var d hostd.Directory
		must(json.Unmarshal(rec.Body.Bytes(), &d))
		return d
	}
	if d := directory(); len(d.Users)+len(d.Groups) != 0 {
		t.Fatalf("a host outside every host group got %+v", d)
	}

	stale := time.Now().Add(-10 * time.Minute)
	expect(call("POST", "/api/v1/ssh/hosts/directory", "", signed(hostd.DirectoryMessage(stale.Unix()), stale)), 401)
	expect(call("POST", "/api/v1/ssh/hosts/directory", "", signed(sshca.RenewalMessage(time.Now().Unix()), time.Now())), 401)
	strangerPub, strangerPriv, _ := ed25519.GenerateKey(rand.Reader)
	strangerKey, _ := ssh.NewPublicKey(strangerPub)
	strangerSigner, _ := ssh.NewSignerFromKey(strangerPriv)
	sig, _ := strangerSigner.Sign(rand.Reader, hostd.DirectoryMessage(time.Now().Unix()))
	expect(call("POST", "/api/v1/ssh/hosts/directory", "", map[string]any{"publicKey": string(ssh.MarshalAuthorizedKey(strangerKey)), "timestamp": time.Now().Unix(), "signature": base64.StdEncoding.EncodeToString(ssh.Marshal(sig))}), 403)

	expect(call("PUT", "/api/v1/ssh/hosts/"+host.ID+"/host-group", auditor.ID, map[string]any{"hostGroupId": group.ID}), 403)
	expect(call("PUT", "/api/v1/ssh/hosts/"+host.ID+"/host-group", security.ID, map[string]any{"hostGroupId": "hgr_missing"}), 422)
	expect(call("PUT", "/api/v1/ssh/hosts/"+host.ID+"/host-group", security.ID, map[string]any{"hostGroupId": group.ID}), 200)

	ada, _ = st.GetUser(ctx, ada.ID)
	bo, _ = st.GetUser(ctx, bo.ID)
	opsGroup, _ := st.GetGroup(ctx, ops.ID)
	d := directory()
	want := []hostd.User{
		{Name: "ada", UID: ada.POSIX.UIDNumber, GID: ada.POSIX.UIDNumber, Gecos: "Ada Lovelace", Home: "/home/ada", Shell: "/bin/bash", Sudo: []string{"ALL"}},
		{Name: "bo", UID: bo.POSIX.UIDNumber, GID: bo.POSIX.UIDNumber, Gecos: "Bo", Home: "/home/bo", Shell: "/bin/bash", Sudo: []string{"/usr/bin/journalctl", "/usr/bin/systemctl restart nginx"}},
	}
	got, _ := json.Marshal(d.Users)
	wanted, _ := json.Marshal(want)
	if !bytes.Equal(got, wanted) {
		t.Fatalf("users:\n%s\nwant:\n%s", got, wanted)
	}
	if len(d.Groups) != 2 || d.Groups[0].Name != "ops" || d.Groups[0].GID != opsGroup.POSIX.GIDNumber || !slices.Equal(d.Groups[0].Members, []string{"ada"}) || !slices.Equal(d.Groups[1].Members, []string{"ada", "bo"}) {
		t.Fatalf("groups: %+v", d.Groups)
	}
	for _, u := range d.Users {
		if u.Name == "cy" || u.Name == "dee" || u.Name == "sam" || u.Name == "ann" {
			t.Fatalf("%s is not allowed on this host", u.Name)
		}
	}

	key, _, _ := ed25519.GenerateKey(rand.Reader)
	personKey, _ := ssh.NewPublicKey(key)
	certificate := func(as string) []string {
		t.Helper()
		rec := call("POST", "/api/v1/me/ssh/certificates", as, map[string]string{"publicKey": string(ssh.MarshalAuthorizedKey(personKey))})
		expect(rec, 201)
		var issued struct{ Principals []string }
		must(json.Unmarshal(rec.Body.Bytes(), &issued))
		return issued.Principals
	}
	if p := certificate(ada.ID); !slices.Equal(p, []string{hostd.Principal(ada.POSIX.UIDNumber)}) {
		t.Fatalf("ada's certificate names %v", p)
	}
	expect(call("POST", "/api/v1/me/ssh/certificates", dee.ID, map[string]string{"publicKey": string(ssh.MarshalAuthorizedKey(personKey))}), 403)

	srv := httptest.NewTLSServer(handler)
	defer srv.Close()
	root := t.TempDir()
	daemon := hostd.Config{Server: srv.URL, Signer: signer, Client: srv.Client(), StateDir: filepath.Join(root, "state"), NSSDir: filepath.Join(root, "extrausers"), Sudoers: filepath.Join(root, "halo"), Etc: filepath.Join(root, "etc"), Visudo: "/usr/sbin/visudo"}
	if _, err := os.Stat(daemon.Visudo); err != nil {
		daemon.Visudo = "/bin/true"
	}
	must(daemon.Refresh(ctx))
	passwd, err := os.ReadFile(filepath.Join(daemon.NSSDir, "passwd"))
	must(err)
	if !strings.Contains(string(passwd), "ada:*:") || !strings.Contains(string(passwd), "bo:*:") {
		t.Fatalf("passwd: %s", passwd)
	}
	sudoers, err := os.ReadFile(daemon.Sudoers)
	must(err)
	if !strings.Contains(string(sudoers), "#"+strconv.Itoa(ada.POSIX.UIDNumber)+" ALL=(root) NOPASSWD: ALL") {
		t.Fatalf("sudoers: %s", sudoers)
	}
	var out bytes.Buffer
	must(hostd.Principals(daemon.NSSDir, daemon.Etc, "ada", &out))
	if out.String() != hostd.Principal(ada.POSIX.UIDNumber)+"\n" {
		t.Fatalf("principals for ada: %q", out.String())
	}

	must(st.SetUserStatus(ctx, ada.ID, "suspended"))
	must(daemon.Refresh(ctx))
	passwd, err = os.ReadFile(filepath.Join(daemon.NSSDir, "passwd"))
	must(err)
	if strings.Contains(string(passwd), "ada:") {
		t.Fatalf("a suspended person still resolves: %s", passwd)
	}
	out.Reset()
	must(hostd.Principals(daemon.NSSDir, daemon.Etc, "ada", &out))
	if out.Len() != 0 {
		t.Fatalf("a suspended person still gets a principal: %q", out.String())
	}
	if sudoers, err := os.ReadFile(daemon.Sudoers); err == nil && strings.Contains(string(sudoers), "NOPASSWD: ALL") {
		t.Fatalf("a suspended person kept sudo: %s", sudoers)
	}

	expect(call("DELETE", "/api/v1/ssh/host-groups/"+group.ID, security.ID, nil), 204)
	if d := directory(); len(d.Users) != 0 {
		t.Fatalf("a deleted host group still lets people in: %+v", d.Users)
	}
	_, err = st.RevokeSSHHost(ctx, host.ID)
	must(err)
	expect(call("POST", "/api/v1/ssh/hosts/directory", "", signed(hostd.DirectoryMessage(time.Now().Unix()), time.Now())), 403)
}
