package hostd

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Config struct {
	Server   string
	Signer   ssh.Signer
	Client   *http.Client
	StateDir string
	NSSDir   string
	Sudoers  string
	Etc      string
	Visudo   string
	Interval time.Duration
}

func (c Config) cachePath() string {
	return filepath.Join(c.StateDir, "cache.json")
}

func (c Config) Fetch(ctx context.Context) (Directory, error) {
	var d Directory
	if u, err := url.Parse(c.Server); err != nil || u.Scheme != "https" || u.Host == "" {
		return d, fmt.Errorf("the Halo server address must start with https://, got %q", c.Server)
	}
	now := time.Now().Unix()
	sig, err := c.Signer.Sign(rand.Reader, DirectoryMessage(now))
	if err != nil {
		return d, err
	}
	body, err := json.Marshal(map[string]any{"publicKey": string(ssh.MarshalAuthorizedKey(c.Signer.PublicKey())), "timestamp": now, "signature": base64.StdEncoding.EncodeToString(ssh.Marshal(sig))})
	if err != nil {
		return d, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.Server, "/")+"/api/v1/ssh/hosts/directory", bytes.NewReader(body))
	if err != nil {
		return d, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "halo-hostd")
	resp, err := c.Client.Do(req)
	if err != nil {
		return d, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return d, err
	}
	if resp.StatusCode != http.StatusOK {
		var problem struct {
			Error struct{ Code, Message string } `json:"error"`
		}
		_ = json.Unmarshal(data, &problem)
		return d, fmt.Errorf("halo answered %d %s: %s", resp.StatusCode, problem.Error.Code, problem.Error.Message)
	}
	return d, json.Unmarshal(data, &d)
}

func (c Config) Load() (Directory, error) {
	var d Directory
	data, err := os.ReadFile(c.cachePath())
	if err != nil {
		return d, err
	}
	return d, json.Unmarshal(data, &d)
}

func (c Config) Save(d Directory) error {
	if err := os.MkdirAll(c.StateDir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return writeFile(c.cachePath(), data, 0o600)
}

func (c Config) Apply(ctx context.Context, d Directory) error {
	localUsers, err := readAccounts(filepath.Join(c.Etc, "passwd"))
	if err != nil {
		return err
	}
	localGroups, err := readAccounts(filepath.Join(c.Etc, "group"))
	if err != nil {
		return err
	}
	f := render(d, localUsers, localGroups)
	if len(f.skipped) > 0 {
		slog.Warn("skipped entries that are invalid or clash with local accounts", "names", strings.Join(f.skipped, ", "))
	}
	if err := os.MkdirAll(c.NSSDir, 0o755); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(c.NSSDir, "passwd"), f.passwd, 0o644); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(c.NSSDir, "group"), f.group, 0o644); err != nil {
		return err
	}
	return c.installSudoers(ctx, f.sudoers)
}

func (c Config) installSudoers(ctx context.Context, rules []byte) error {
	if len(rules) == 0 {
		return removeFile(c.Sudoers)
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.Sudoers), "."+filepath.Base(c.Sudoers)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := fill(tmp, rules, 0o440); err != nil {
		return err
	}
	if out, err := exec.CommandContext(ctx, c.Visudo, "-c", "-q", "-f", tmp.Name()).CombinedOutput(); err != nil {
		return errors.Join(fmt.Errorf("visudo rejected the generated sudo rules, so none are installed: %v %s", err, bytes.TrimSpace(out)), removeFile(c.Sudoers))
	}
	return os.Rename(tmp.Name(), c.Sudoers)
}

func (c Config) Refresh(ctx context.Context) error {
	d, err := c.Fetch(ctx)
	if err != nil {
		return err
	}
	if err := c.Save(d); err != nil {
		return err
	}
	return c.Apply(ctx, d)
}

func (c Config) Run(ctx context.Context, reload <-chan os.Signal) error {
	if d, err := c.Load(); err == nil {
		if err := c.Apply(ctx, d); err != nil {
			slog.Error("could not apply the cached directory", "error", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		slog.Error("could not read the cached directory", "error", err)
	}
	for {
		if err := c.Refresh(ctx); err != nil && ctx.Err() == nil {
			slog.Error("refresh failed, keeping the cached directory", "error", err)
		} else if err == nil {
			slog.Info("directory refreshed")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-reload:
		case <-time.After(c.Interval):
		}
	}
}

func Principals(nssDir, etc, user string, out io.Writer) error {
	if !NamePattern.MatchString(user) {
		return nil
	}
	local, err := readAccounts(filepath.Join(etc, "passwd"))
	if err != nil || local.names[user] {
		return err
	}
	path := filepath.Join(nssDir, "passwd")
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s is writable by other users, so its accounts are not trusted", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) != 7 || fields[0] != user {
			continue
		}
		uid, err := strconv.Atoi(fields[2])
		if err != nil || uid < MinID {
			return nil
		}
		_, err = fmt.Fprintln(out, Principal(uid))
		return err
	}
	return scanner.Err()
}

func writeFile(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := fill(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func fill(f *os.File, data []byte, mode os.FileMode) error {
	err := f.Chmod(mode)
	if err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	return errors.Join(err, f.Close())
}

func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
