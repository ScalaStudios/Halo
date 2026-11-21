package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"halo/internal/oidc"
	"halo/internal/sshca"
)

const scopes = "openid profile email groups offline_access"

var (
	httpClient = &http.Client{Timeout: 30 * time.Second}
	userAgent  = "Halo CLI (" + runtime.GOOS + "/" + runtime.GOARCH + ")"
	errSignIn  = errors.New("you are not signed in to Halo. Run halo login --server https://auth.example.com first")
)

type credentials struct {
	Server       string    `json:"server"`
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

type remoteError struct {
	Status  int
	Code    string
	Message string
}

func (e *remoteError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("Halo answered %d %s", e.Status, e.Code)
}

func Run(ctx context.Context, command string, args []string, out io.Writer) error {
	switch command {
	case "login":
		return login(ctx, args, out)
	case "logout":
		return logout(ctx, out)
	case "whoami":
		return whoami(ctx, out)
	case "ssh-cert":
		return sshCert(ctx, args, out)
	case "ssh-host":
		return sshHost(ctx, args, out)
	}
	return fmt.Errorf("unknown command %q", command)
}

func credentialsPath() (string, error) {
	dir, err := os.UserConfigDir()
	return filepath.Join(dir, "halo", "credentials.json"), err
}

func load() (credentials, error) {
	var c credentials
	path, err := credentialsPath()
	if err != nil {
		return c, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, errSignIn
	}
	if err != nil {
		return c, err
	}
	return c, json.Unmarshal(data, &c)
}

func save(c credentials) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func send(req *http.Request, dst any) error {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach Halo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 400 {
		if dst == nil {
			return nil
		}
		return json.NewDecoder(resp.Body).Decode(dst)
	}
	var body struct {
		Error       json.RawMessage `json:"error"`
		Description string          `json:"error_description"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	e := &remoteError{Status: resp.StatusCode, Message: body.Description}
	var api struct{ Code, Message string }
	if json.Unmarshal(body.Error, &api) == nil && api.Code != "" {
		e.Code, e.Message = api.Code, api.Message
	} else {
		_ = json.Unmarshal(body.Error, &e.Code)
	}
	return e
}

func postForm(ctx context.Context, endpoint string, form url.Values, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return send(req, dst)
}

func code(err error) string {
	var e *remoteError
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func login(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("login", flag.ContinueOnError)
	server := flags.String("server", "", "address of your Halo server, for example https://auth.example.com")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *server == "" {
		if c, err := load(); err == nil {
			*server = c.Server
		}
	}
	base, err := serverURL(*server, "halo login")
	if err != nil {
		return err
	}
	var device struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
		URI        string `json:"verification_uri_complete"`
		Interval   int    `json:"interval"`
	}
	if err := postForm(ctx, base+"/oauth2/device_authorization", url.Values{"client_id": {oidc.CLIClientID}, "scope": {scopes}}, &device); err != nil {
		return err
	}
	fmt.Fprintf(out, "Open %s\nand check that the page shows the code %s.\n\nWaiting for you to approve the sign-in…\n", device.URI, device.UserCode)
	interval := time.Duration(max(device.Interval, 1)) * time.Second
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
		var t tokens
		err := postForm(ctx, base+"/oauth2/token", url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "device_code": {device.DeviceCode}, "client_id": {oidc.CLIClientID}}, &t)
		switch code(err) {
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
			continue
		case "access_denied":
			return errors.New("the sign-in was denied in the browser")
		case "expired_token":
			return errors.New("the code expired before anyone approved it. Run halo login again")
		}
		if err != nil {
			return err
		}
		c := credentials{Server: base, AccessToken: t.AccessToken, RefreshToken: t.RefreshToken, ExpiresAt: time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)}
		if err := save(c); err != nil {
			return err
		}
		info, err := userinfo(ctx, c)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Signed in to %s as %s <%s>.\n", base, info.Name, info.Email)
		return nil
	}
}

func serverURL(raw, command string) (string, error) {
	base := strings.TrimRight(raw, "/")
	if u, err := url.Parse(base); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", fmt.Errorf("pass the address of your Halo server, for example: %s --server https://auth.example.com", command)
	}
	return base, nil
}

func fresh(ctx context.Context) (credentials, error) {
	c, err := load()
	if err != nil || time.Now().Add(30*time.Second).Before(c.ExpiresAt) {
		return c, err
	}
	if c.RefreshToken == "" {
		return c, errors.New("your Halo sign-in has expired. Run halo login again")
	}
	var t tokens
	err = postForm(ctx, c.Server+"/oauth2/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {c.RefreshToken}, "client_id": {oidc.CLIClientID}}, &t)
	if code(err) == "invalid_grant" {
		return c, errors.New("your Halo sign-in has expired or was revoked. Run halo login again")
	}
	if err != nil {
		return c, err
	}
	c.AccessToken, c.ExpiresAt = t.AccessToken, time.Now().Add(time.Duration(t.ExpiresIn)*time.Second)
	if t.RefreshToken != "" {
		c.RefreshToken = t.RefreshToken
	}
	return c, save(c)
}

type info struct {
	Subject string   `json:"sub"`
	Name    string   `json:"name"`
	Email   string   `json:"email"`
	Groups  []string `json:"groups"`
}

func authorized(ctx context.Context, c credentials, method, path string, body any, dst any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Server+path, reader)
	if err != nil {
		return err
	}
	if c.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return send(req, dst)
}

func userinfo(ctx context.Context, c credentials) (info, error) {
	var i info
	return i, authorized(ctx, c, http.MethodGet, "/oauth2/userinfo", nil, &i)
}

func whoami(ctx context.Context, out io.Writer) error {
	c, err := fresh(ctx)
	if err != nil {
		return err
	}
	i, err := userinfo(ctx, c)
	if err != nil {
		return err
	}
	groups := strings.Join(i.Groups, ", ")
	if groups == "" {
		groups = "none"
	}
	fmt.Fprintf(out, "%s <%s>\nUser ID  %s\nGroups   %s\nServer   %s\n", i.Name, i.Email, i.Subject, groups, c.Server)
	return nil
}

func sshCert(ctx context.Context, args []string, out io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("ssh-cert", flag.ContinueOnError)
	keyPath := flags.String("key", filepath.Join(home, ".ssh", "id_ed25519.pub"), "public key to certify")
	outPath := flags.String("out", "", "where to write the certificate (default: the key path ending in -cert.pub, which ssh loads automatically)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.HasPrefix(*keyPath, "~/") {
		*keyPath = filepath.Join(home, (*keyPath)[2:])
	}
	if *outPath == "" {
		*outPath = strings.TrimSuffix(*keyPath, ".pub") + "-cert.pub"
	}
	raw, err := os.ReadFile(*keyPath)
	if err != nil {
		return err
	}
	if _, _, _, _, err := ssh.ParseAuthorizedKey(raw); err != nil {
		return fmt.Errorf("%s is not an OpenSSH public key. Pass the .pub file, for example --key ~/.ssh/id_ed25519.pub", *keyPath)
	}
	c, err := fresh(ctx)
	if err != nil {
		return err
	}
	var issued certificate
	if err := authorized(ctx, c, http.MethodPost, "/api/v1/me/ssh/certificates", map[string]string{"publicKey": string(raw)}, &issued); err != nil {
		return err
	}
	if err := os.WriteFile(*outPath, []byte(issued.Certificate+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "Wrote certificate %d to %s.\nLog in as %s until %s.\n", issued.Serial, *outPath, strings.Join(issued.Principals, ", "), issued.ValidBefore.Local().Format("2 January 15:04 MST"))
	if issued.KnownHosts == "" {
		return nil
	}
	path := filepath.Join(home, ".ssh", "known_hosts")
	changed, err := trustHosts(path, issued.KnownHosts)
	if changed {
		fmt.Fprintf(out, "Added Halo's host certificate authority to %s, so ssh trusts hosts Halo signed for %s.\n", path, strings.Fields(issued.KnownHosts)[1])
	}
	return err
}

type certificate struct {
	Certificate string    `json:"certificate"`
	Serial      uint64    `json:"serial"`
	Principals  []string  `json:"principals"`
	ValidBefore time.Time `json:"validBefore"`
	KnownHosts  string    `json:"knownHosts"`
}

func trustHosts(path, line string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	ca := strings.Fields(line)
	if len(ca) != 4 || ca[0] != "@cert-authority" {
		return false, fmt.Errorf("halo sent a known_hosts line this version of the CLI does not understand: %q", line)
	}
	kept := []string{}
	for _, l := range strings.Split(string(data), "\n") {
		if f := strings.Fields(l); len(f) < 4 || f[0] != "@cert-authority" || f[3] != ca[3] {
			kept = append(kept, l)
		}
	}
	next := strings.TrimRight(strings.Join(kept, "\n"), "\n")
	if next != "" {
		next += "\n"
	}
	next += line + "\n"
	if next == string(data) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, []byte(next), 0o644)
}

func sshHost(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "enroll" && args[0] != "renew" {
		return errors.New("run halo ssh-host enroll --server URL --token T on a new host, or halo ssh-host renew --server URL to renew its certificate")
	}
	flags := flag.NewFlagSet("ssh-host "+args[0], flag.ContinueOnError)
	server := flags.String("server", "", "address of your Halo server, for example https://auth.example.com")
	token := flags.String("token", "", "enrollment token from the Infrastructure page of the Halo console (enroll only)")
	keyPath := flags.String("key", "/etc/ssh/ssh_host_ed25519_key", "private host key to certify; the certificate is written next to it, ending in -cert.pub")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	base, err := serverURL(*server, "halo ssh-host "+args[0])
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(*keyPath)
	if err != nil {
		return err
	}
	signer, err := ssh.ParsePrivateKey(raw)
	if err != nil {
		return fmt.Errorf("%s is not an unencrypted OpenSSH private key. Pass the host key itself, for example --key /etc/ssh/ssh_host_ed25519_key", *keyPath)
	}
	publicKey := string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
	var issued certificate
	if args[0] == "enroll" {
		if *token == "" {
			return errors.New("pass the enrollment token from the Halo console with --token")
		}
		err = authorized(ctx, credentials{Server: base}, http.MethodPost, "/api/v1/ssh/hosts/enroll", map[string]string{"token": *token, "publicKey": publicKey}, &issued)
	} else {
		now := time.Now().Unix()
		sig, signErr := signer.Sign(rand.Reader, sshca.RenewalMessage(now))
		if signErr != nil {
			return signErr
		}
		body := map[string]any{"publicKey": publicKey, "timestamp": now, "signature": base64.StdEncoding.EncodeToString(ssh.Marshal(sig))}
		err = authorized(ctx, credentials{Server: base}, http.MethodPost, "/api/v1/ssh/hosts/renew", body, &issued)
	}
	if err != nil {
		return err
	}
	certPath := *keyPath + "-cert.pub"
	if err := os.WriteFile(certPath, []byte(issued.Certificate+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "Wrote host certificate %d to %s for %s, valid until %s.\n", issued.Serial, certPath, strings.Join(issued.Principals, ", "), issued.ValidBefore.Local().Format("2 January 2006 15:04 MST"))
	if args[0] == "enroll" {
		fmt.Fprintf(out, "Add HostCertificate %s to /etc/ssh/sshd_config if it is not there yet, then reload sshd.\n", certPath)
	}
	return nil
}

func logout(ctx context.Context, out io.Writer) error {
	c, err := load()
	if errors.Is(err, errSignIn) {
		fmt.Fprintln(out, "You are not signed in to Halo.")
		return nil
	}
	if err != nil {
		return err
	}
	if c.RefreshToken != "" {
		if err := postForm(ctx, c.Server+"/oauth2/revoke", url.Values{"token": {c.RefreshToken}, "token_type_hint": {"refresh_token"}, "client_id": {oidc.CLIClientID}}, nil); err != nil {
			fmt.Fprintf(out, "Halo could not revoke the sign-in (%v); removing it from this computer anyway.\n", err)
		}
	}
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	fmt.Fprintf(out, "Signed out of %s.\n", c.Server)
	return nil
}
