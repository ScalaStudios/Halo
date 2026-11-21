package hostd

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

var sample = Directory{
	Users: []User{
		{Name: "ada", UID: 1000001, GID: 1000001, Gecos: "Ada: Lovelace,\nroot", Home: "/home/ada", Shell: "/bin/bash", Sudo: []string{"ALL"}},
		{Name: "bo", UID: 1000002, GID: 1000002, Gecos: "Bo", Home: "/home/bo", Shell: "/bin/zsh", Sudo: []string{"/usr/bin/journalctl", "/usr/bin/systemctl restart nginx", "/bin/sh, ALL", "/bin/ls *", "/usr/bin/journalctl"}},
		{Name: "luna", UID: 1000003, GID: 1000003, Home: "/home/luna", Shell: "/bin/bash"},
		{Name: "evil", UID: 0, GID: 0, Home: "/root", Shell: "/bin/bash"},
		{Name: "x:0:0::/:/bin/sh\nroot2", UID: 1000004, GID: 1000004, Home: "/home/x", Shell: "/bin/bash"},
		{Name: "cy", UID: 1000005, GID: 1000005, Home: "/home/cy\n", Shell: "/bin/bash"},
		{Name: "dup", UID: 1000001, GID: 1000001, Home: "/home/dup", Shell: "/bin/bash"},
		{Name: "local", UID: 1001, GID: 1001, Home: "/home/local", Shell: "/bin/bash"},
		{Name: "taken", UID: 1000006, GID: 1000006, Home: "/home/taken", Shell: "/bin/bash"},
	},
	Groups: []Group{
		{Name: "ops", GID: 2000001, Members: []string{"ada", "bo", "evil", "ghost"}},
		{Name: "sudo", GID: 2000002, Members: []string{"ada"}},
		{Name: "ada", GID: 2000003, Members: []string{"ada"}},
		{Name: "clash", GID: 1000002, Members: []string{"bo"}},
	},
}

func config(t *testing.T) Config {
	t.Helper()
	root := t.TempDir()
	c := Config{StateDir: filepath.Join(root, "state"), NSSDir: filepath.Join(root, "extrausers"), Sudoers: filepath.Join(root, "sudoers.d", "halo"), Etc: filepath.Join(root, "etc"), Visudo: "/usr/sbin/visudo", Interval: time.Hour}
	for _, dir := range []string{c.Etc, filepath.Dir(c.Sudoers)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(c.Etc, "passwd"), "root:x:0:0:root:/root:/bin/bash\nluna:x:1000:1000::/home/luna:/bin/bash\n")
	write(t, filepath.Join(c.Etc, "group"), "root:x:0:\nsudo:x:27:luna\ntaken:x:1000006:\n")
	return c
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestRender(t *testing.T) {
	c := config(t)
	users, err := readAccounts(filepath.Join(c.Etc, "passwd"))
	if err != nil {
		t.Fatal(err)
	}
	groups, err := readAccounts(filepath.Join(c.Etc, "group"))
	if err != nil {
		t.Fatal(err)
	}
	f := render(sample, users, groups)
	if want := "ada:*:1000001:1000001:Ada Lovelaceroot:/home/ada:/bin/bash\nbo:*:1000002:1000002:Bo:/home/bo:/bin/zsh\n"; string(f.passwd) != want {
		t.Fatalf("passwd:\n%s\nwant:\n%s", f.passwd, want)
	}
	if want := "ada:x:1000001:\nbo:x:1000002:\nops:x:2000001:ada,bo\n"; string(f.group) != want {
		t.Fatalf("group:\n%s\nwant:\n%s", f.group, want)
	}
	if want := "#1000001 ALL=(root) NOPASSWD: ALL\n#1000002 ALL=(root) NOPASSWD: /usr/bin/journalctl, /usr/bin/systemctl restart nginx\n"; string(f.sudoers) != want {
		t.Fatalf("sudoers:\n%s\nwant:\n%s", f.sudoers, want)
	}
	if want := "luna evil x:0:0::/:/bin/sh\nroot2 cy dup local taken sudo ada clash"; strings.Join(f.skipped, " ") != want {
		t.Fatalf("skipped %q, want %q", strings.Join(f.skipped, " "), want)
	}
}

func TestSudoersAreValidatedBeforeInstall(t *testing.T) {
	if _, err := os.Stat("/usr/sbin/visudo"); err != nil {
		t.Skip("visudo is not installed")
	}
	c := config(t)
	ctx := context.Background()
	if err := c.Apply(ctx, sample); err != nil {
		t.Fatal(err)
	}
	if got := read(t, c.Sudoers); !strings.HasPrefix(got, "#1000001 ALL=(root) NOPASSWD: ALL\n") {
		t.Fatalf("sudoers: %s", got)
	}
	if mode(t, c.Sudoers) != 0o440 || mode(t, filepath.Join(c.NSSDir, "passwd")) != 0o644 || mode(t, filepath.Join(c.NSSDir, "group")) != 0o644 {
		t.Fatal("wrong file modes")
	}
	if out, err := exec.Command("/usr/sbin/visudo", "-c", "-f", c.Sudoers).CombinedOutput(); err != nil {
		t.Fatalf("visudo: %v %s", err, out)
	}
	if out, err := exec.Command("cvtsudoers", "-f", "json", c.Sudoers).Output(); err == nil && !bytes.Contains(out, []byte(`"userid": 1000002`)) {
		t.Fatalf("sudo reads the rules differently: %s", out)
	}

	c.Visudo = filepath.Join(t.TempDir(), "refuse")
	write(t, c.Visudo, "#!/bin/sh\nexit 1\n")
	if err := os.Chmod(c.Visudo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := c.Apply(ctx, sample); err == nil {
		t.Fatal("installed sudo rules visudo rejected")
	}
	if _, err := os.Stat(c.Sudoers); !os.IsNotExist(err) {
		t.Fatal("kept the old sudo rules after visudo rejected the new ones")
	}
	entries, _ := os.ReadDir(filepath.Dir(c.Sudoers))
	if len(entries) != 0 {
		t.Fatalf("left %d files in sudoers.d", len(entries))
	}

	c.Visudo = "/usr/sbin/visudo"
	if err := c.Apply(ctx, Directory{Users: []User{{Name: "ada", UID: 1000001, GID: 1000001, Home: "/home/ada", Shell: "/bin/bash"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c.Sudoers); !os.IsNotExist(err) {
		t.Fatal("kept sudo rules after the last one was removed")
	}
}

func TestPrincipals(t *testing.T) {
	c := config(t)
	c.Visudo = "/bin/true"
	if err := c.Apply(context.Background(), sample); err != nil {
		t.Fatal(err)
	}
	for user, want := range map[string]string{"ada": "halo:1000001\n", "bo": "halo:1000002\n", "luna": "", "root": "", "ghost": "", "../ada": "", "": ""} {
		var out bytes.Buffer
		if err := Principals(c.NSSDir, c.Etc, user, &out); err != nil || out.String() != want {
			t.Fatalf("%q: got %q, %v; want %q", user, out.String(), err, want)
		}
	}
	write(t, filepath.Join(c.Etc, "passwd"), "ada:x:1002:1002::/home/ada:/bin/bash\n")
	var out bytes.Buffer
	if err := Principals(c.NSSDir, c.Etc, "ada", &out); err != nil || out.Len() != 0 {
		t.Fatalf("a local account named ada got the Halo principal: %q %v", out.String(), err)
	}
	if err := os.Chmod(filepath.Join(c.NSSDir, "passwd"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := Principals(c.NSSDir, c.Etc, "bo", &out); err == nil || out.Len() != 0 {
		t.Fatal("trusted a world-writable passwd file")
	}
}

func TestCacheKeepsHostWorkingOffline(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	directory := sample
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			PublicKey string `json:"publicKey"`
			Timestamp int64  `json:"timestamp"`
		}
		if r.URL.Path != "/api/v1/ssh/hosts/directory" || json.NewDecoder(r.Body).Decode(&in) != nil || !strings.HasPrefix(in.PublicKey, "ssh-ed25519 ") {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(directory)
	}))
	c := config(t)
	c.Visudo, c.Signer, c.Client, c.Server = "/bin/true", signer, srv.Client(), srv.URL

	insecure := c
	insecure.Server = strings.Replace(srv.URL, "https://", "http://", 1)
	if err := insecure.Refresh(context.Background()); err == nil || !strings.Contains(err.Error(), "https://") {
		t.Fatalf("fetched over plain http: %v", err)
	}
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if mode(t, filepath.Join(c.StateDir, "cache.json")) != 0o600 || mode(t, c.StateDir) != 0o700 {
		t.Fatal("the cache is readable by other users")
	}
	srv.Close()
	if err := c.Refresh(context.Background()); err == nil {
		t.Fatal("refreshed from a stopped server")
	}
	if got := read(t, filepath.Join(c.NSSDir, "passwd")); !strings.Contains(got, "ada:*:1000001") {
		t.Fatalf("a failed refresh changed the accounts: %s", got)
	}

	if err := os.Remove(filepath.Join(c.NSSDir, "passwd")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, nil) }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if data, err := os.ReadFile(filepath.Join(c.NSSDir, "passwd")); err == nil && strings.Contains(string(data), "bo:*:1000002") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the daemon didn't restore the accounts from its cache while Halo was unreachable")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
