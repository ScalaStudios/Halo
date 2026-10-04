package cli

import (
	"bytes"
	"context"
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
	base := strings.TrimRight(*server, "/")
	if u, err := url.Parse(base); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return errors.New("pass the address of your Halo server, for example: halo login --server https://auth.example.com")
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
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
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
	var issued struct {
		Certificate string    `json:"certificate"`
		Serial      uint64    `json:"serial"`
		Principals  []string  `json:"principals"`
		ValidBefore time.Time `json:"validBefore"`
	}
	if err := authorized(ctx, c, http.MethodPost, "/api/v1/me/ssh/certificates", map[string]string{"publicKey": string(raw)}, &issued); err != nil {
		return err
	}
	if err := os.WriteFile(*outPath, []byte(issued.Certificate+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "Wrote certificate %d to %s.\nLog in as %s until %s.\n", issued.Serial, *outPath, strings.Join(issued.Principals, ", "), issued.ValidBefore.Local().Format("2 January 15:04 MST"))
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
