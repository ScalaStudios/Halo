package cli

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/oidc"
	"halo/internal/sshca"
	"halo/internal/store"
	"halo/internal/testdb"
)

type output struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (o *output) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.Write(p)
}

func (o *output) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.String()
}

func TestLoginWhoamiCertificateLogout(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	group, err := st.CreateGroup(ctx, store.NewGroup{Name: "Infrastructure", Kind: "assigned"})
	must(err)
	user, err := st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active", GroupIDs: []string{group.ID}})
	must(err)
	_, session, err := st.CreateSession(ctx, store.NewSession{UserID: user.ID, Method: "passkey"})
	must(err)
	_, err = st.SaveSSHPrincipalMapping(ctx, store.SSHPrincipalMapping{GroupID: group.ID, Principals: []string{"ops"}})
	must(err)

	var handler http.Handler
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
	t.Cleanup(srv.Close)
	issuer, _ := url.Parse(srv.URL)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	authn := &httpx.Auth{Store: st, Public: issuer, Dev: true, AccessTokens: oidc.AccessTokenResolver(st)}
	deps := httpx.Deps{Config: config.Config{PublicURL: issuer, SecretKey: key, Dev: true}, Store: st, Auth: authn}
	provider, err := oidc.New(deps)
	must(err)
	api := http.NewServeMux()
	must(oidc.Register(api, deps))
	must(sshca.Register(api, deps))
	mux := http.NewServeMux()
	mux.Handle("/api/", authn.Resolve(authn.SameOrigin(api)))
	mux.Handle("/oauth2/", authn.Resolve(provider))
	handler = mux

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", t.TempDir())

	if err := Run(ctx, "whoami", nil, &output{}); err != errSignIn {
		t.Fatalf("whoami before login: %v", err)
	}

	out := &output{}
	done := make(chan error, 1)
	go func() { done <- Run(ctx, "login", []string{"--server", srv.URL + "/"}, out) }()
	pattern := regexp.MustCompile(`code ([B-Z]{4}-[B-Z]{4})`)
	var userCode string
	for deadline := time.Now().Add(5 * time.Second); userCode == "" && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if m := pattern.FindStringSubmatch(out.String()); m != nil {
			userCode = m[1]
		}
	}
	if userCode == "" {
		t.Fatalf("login printed no user code: %q", out.String())
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/device/"+userCode+"/approve", nil)
	must(err)
	req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: session})
	resp, err := http.DefaultClient.Do(req)
	must(err)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve: %d", resp.StatusCode)
	}
	select {
	case err := <-done:
		must(err)
	case <-time.After(20 * time.Second):
		t.Fatal("login did not finish after approval")
	}
	if !strings.Contains(out.String(), "Signed in to "+srv.URL+" as Ada Lovelace <ada@example.com>.") {
		t.Fatalf("login output: %q", out.String())
	}
	path := filepath.Join(dir, "halo", "credentials.json")
	info, err := os.Stat(path)
	must(err)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("credentials file mode %v, want 0600", info.Mode().Perm())
	}

	c, err := load()
	must(err)
	stale := c.AccessToken
	c.ExpiresAt = time.Now().Add(-time.Minute)
	must(save(c))
	out = &output{}
	must(Run(ctx, "whoami", nil, out))
	if !strings.Contains(out.String(), "Ada Lovelace <ada@example.com>") || !strings.Contains(out.String(), "Infrastructure") || !strings.Contains(out.String(), user.ID) {
		t.Fatalf("whoami: %q", out.String())
	}
	if c, err = load(); err != nil || c.AccessToken == stale || !c.ExpiresAt.After(time.Now()) {
		t.Fatalf("whoami must refresh an expired access token: %v %+v", err, c)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	must(err)
	sshKey, err := ssh.NewPublicKey(pub)
	must(err)
	keyPath := filepath.Join(t.TempDir(), "id_ed25519.pub")
	must(os.WriteFile(keyPath, ssh.MarshalAuthorizedKey(sshKey), 0o644))
	block, err := ssh.MarshalPrivateKey(priv, "")
	must(err)
	privatePath := strings.TrimSuffix(keyPath, ".pub")
	must(os.WriteFile(privatePath, pem.EncodeToMemory(block), 0o600))
	if err := Run(ctx, "ssh-cert", []string{"--key", privatePath}, &output{}); err == nil || !strings.Contains(err.Error(), "not an OpenSSH public key") {
		t.Fatalf("ssh-cert with a private key must refuse before contacting Halo: %v", err)
	}
	out = &output{}
	must(Run(ctx, "ssh-cert", []string{"--key", keyPath}, out))
	written, err := os.ReadFile(privatePath + "-cert.pub")
	must(err)
	parsed, _, _, _, err := ssh.ParseAuthorizedKey(written)
	must(err)
	if cert, ok := parsed.(*ssh.Certificate); !ok || cert.ValidPrincipals[0] != "ops" || !strings.Contains(out.String(), "Log in as ops until") {
		t.Fatalf("ssh-cert: %q %+v", out.String(), parsed)
	}

	out = &output{}
	must(Run(ctx, "logout", nil, out))
	if _, err := os.Stat(path); !os.IsNotExist(err) || !strings.Contains(out.String(), "Signed out of "+srv.URL) {
		t.Fatalf("logout: %v %q", err, out.String())
	}
	req, err = http.NewRequest(http.MethodPost, srv.URL+"/api/v1/me/ssh/certificates", strings.NewReader(`{"publicKey":"`+strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshKey)))+`"}`))
	must(err)
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	resp, err = http.DefaultClient.Do(req)
	must(err)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("access token after logout: %d", resp.StatusCode)
	}
}
