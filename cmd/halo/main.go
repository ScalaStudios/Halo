package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"halo/internal/cli"
	"halo/internal/config"
	"halo/internal/db"
	"halo/internal/httpx"
	"halo/internal/kms"
	"halo/internal/secret"
	"halo/internal/server"
	"halo/internal/store"
)

const usage = `Halo identity server

Usage:
  halo serve                              Run migrations, then serve the API and OpenID Connect endpoints
  halo migrate                            Apply database migrations and exit
  halo bootstrap --email E --name N       Create the first global administrator and print their setup link
  halo seed-demo                          Load the Fernway Systems demo organization (development only)
  halo dev-session --email E              Create a session cookie for a user (development only)
  halo rotate-secret-key                  Re-encrypt stored secrets from HALO_SECRET_KEY to HALO_NEW_SECRET_KEY (stop Halo first)

Client commands:
  halo login --server URL                 Sign in to a Halo server from this computer with the device flow
  halo whoami                             Show who you are signed in as
  halo ssh-cert [--key K] [--out F]       Get a short-lived SSH certificate for a public key (default ~/.ssh/id_ed25519.pub)
                                          and trust hosts Halo signed in ~/.ssh/known_hosts
  halo ssh-host enroll --server URL --token T [--key K]
                                          Get a host certificate for this server's host key (default /etc/ssh/ssh_host_ed25519_key)
  halo ssh-host renew --server URL [--key K]
                                          Renew the host certificate, signing the request with the host key
  halo logout                             Revoke this computer's sign-in and delete the stored credentials

Configuration comes from environment variables: HALO_DATABASE_URL, HALO_PUBLIC_URL,
HALO_SECRET_KEY, HALO_LISTEN (default :8080) and HALO_DEV=1 for local http.
Client credentials are stored in $XDG_CONFIG_HOME/halo/credentials.json.
`

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch os.Args[1] {
	case "serve":
		err = serve(ctx)
	case "migrate":
		err = withStore(ctx, func(context.Context, config.Config, *store.Store) error { return nil })
	case "bootstrap":
		err = bootstrap(ctx, os.Args[2:])
	case "seed-demo":
		err = seedDemo(ctx)
	case "dev-session":
		err = devSession(ctx, os.Args[2:])
	case "rotate-secret-key":
		err = rotateSecretKey(ctx)
	case "login", "logout", "whoami", "ssh-cert", "ssh-host":
		err = cli.Run(ctx, os.Args[1], os.Args[2:], os.Stdout)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "halo:", err)
		os.Exit(1)
	}
}

func withStore(ctx context.Context, fn func(context.Context, config.Config, *store.Store) error) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	httpx.AllowOutbound(cfg.OutboundAllow)
	keys := kms.FromConfig(cfg.KMS)
	if cfg.WrappedKey != "" {
		if cfg.SecretKey, err = keys.SecretKey(ctx, cfg.WrappedKey); err != nil {
			return err
		}
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	applied, err := db.Migrate(ctx, pool)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	for _, version := range applied {
		slog.Info("applied migration", "version", version)
	}
	st, err := store.New(pool, cfg.SecretKey)
	if err != nil {
		return err
	}
	st.Keys = keys
	return fn(ctx, cfg, st)
}

func serve(ctx context.Context) error {
	return withStore(ctx, func(ctx context.Context, cfg config.Config, st *store.Store) error {
		if err := st.CheckExternalKeys(ctx); err != nil {
			return fmt.Errorf("Halo can't start without its external keys: %w", err)
		}
		go st.Keys.KeepRenewed(ctx)
		app, err := server.New(cfg, st)
		if err != nil {
			return err
		}
		running := make(chan struct{})
		go func() {
			defer close(running)
			app.Run(ctx)
		}()
		srv := &http.Server{Addr: cfg.Listen, Handler: app.Handler, ReadHeaderTimeout: 10 * time.Second}
		metricsSrv := &http.Server{Addr: cfg.MetricsListen, Handler: app.Metrics, ReadHeaderTimeout: 10 * time.Second}
		errs := make(chan error, 4)
		if cfg.RADIUSListen != "" {
			addr, err := net.ResolveUDPAddr("udp", cfg.RADIUSListen)
			if err != nil {
				return err
			}
			conn, err := net.ListenUDP("udp", addr)
			if err != nil {
				return err
			}
			go func() {
				slog.Info("radius listening", "addr", cfg.RADIUSListen)
				if err := app.RADIUS.Serve(ctx, conn); err != nil {
					errs <- err
				}
			}()
		}
		go func() {
			slog.Info("halo listening", "addr", cfg.Listen, "issuer", cfg.Issuer())
			errs <- srv.ListenAndServe()
		}()
		if app.Metrics != nil {
			go func() {
				slog.Info("metrics listening", "addr", cfg.MetricsListen)
				errs <- metricsSrv.ListenAndServe()
			}()
		}
		if app.LDAP != nil {
			go func() { errs <- app.LDAP.ListenAndServe() }()
		}
		select {
		case err := <-errs:
			return err
		case <-ctx.Done():
			if app.LDAP != nil {
				app.LDAP.Close()
			}
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			if err := metricsSrv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			select {
			case <-running:
			case <-shutdown.Done():
			}
			return nil
		}
	})
}

func bootstrap(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	email := fs.String("email", "", "email address of the first administrator")
	name := fs.String("name", "", "display name of the first administrator")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *email == "" || *name == "" {
		return errors.New("bootstrap needs --email and --name, for example: halo bootstrap --email you@example.com --name \"Sam Lee\"")
	}
	return withStore(ctx, func(ctx context.Context, cfg config.Config, st *store.Store) error {
		user, token, expires, err := bootstrapAdmin(ctx, st, *email, *name)
		if err != nil {
			return err
		}
		fmt.Printf("%s <%s> is the first global administrator.\n\nOpen this link to set up a passkey. It works once and expires %s:\n\n  %s%s\n\n",
			user.Name, user.Email, expires.Format("2 January 2006 15:04 MST"), cfg.Issuer(), store.EnrollPath(token))
		return nil
	})
}

func bootstrapAdmin(ctx context.Context, st *store.Store, email, name string) (store.User, string, time.Time, error) {
	users, err := st.ListUsers(ctx)
	if err != nil {
		return store.User{}, "", time.Time{}, err
	}
	var pending []store.User
	for _, u := range users {
		if !u.HasRole("global_admin") {
			continue
		}
		if u.Status != "invited" || len(u.Methods) > 0 {
			return store.User{}, "", time.Time{}, fmt.Errorf("%s is already a global administrator; invite more people from the console instead", u.Email)
		}
		pending = append(pending, u)
	}
	var user store.User
	for _, u := range pending {
		if !strings.EqualFold(u.Email, strings.TrimSpace(email)) {
			return store.User{}, "", time.Time{}, fmt.Errorf("%s is the first global administrator but has not set up a passkey yet. Run halo bootstrap --email %s --name %q to print a new setup link", u.Email, u.Email, u.Name)
		}
		user = u
	}
	var token string
	var expires time.Time
	err = st.Tx(ctx, func(tx *store.Store) error {
		var err error
		summary := "Issued a new setup link for the first global administrator"
		if user.ID == "" {
			summary = "Created the first global administrator"
			if user, err = tx.CreateUser(ctx, store.NewUser{Email: email, Name: name, Status: "invited", Roles: []string{"global_admin"}}); err != nil {
				return err
			}
		}
		if token, expires, err = tx.CreateEnrollmentToken(ctx, user.ID, "invite", nil); err != nil {
			return err
		}
		return tx.RecordAudit(ctx, store.AuditEvent{Action: "user.bootstrap", Summary: summary, TargetType: "user", TargetID: user.ID, TargetLabel: user.Name})
	})
	return user, token, expires, err
}

func devSession(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("dev-session", flag.ContinueOnError)
	email := fs.String("email", "", "email address of the user to sign in as")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return withStore(ctx, func(ctx context.Context, cfg config.Config, st *store.Store) error {
		if !cfg.Dev {
			return errors.New("dev-session only runs with HALO_DEV=1")
		}
		user, err := st.GetUserByEmail(ctx, *email)
		if err != nil {
			return fmt.Errorf("no user with email %q", *email)
		}
		_, token, err := st.CreateSession(ctx, store.NewSession{UserID: user.ID, Method: "passkey", IP: "127.0.0.1", Location: "Development", UserAgent: "halo dev-session"})
		if err != nil {
			return err
		}
		fmt.Printf("halo_session=%s\n", token)
		return nil
	})
}

func seedDemo(ctx context.Context) error {
	return withStore(ctx, func(ctx context.Context, cfg config.Config, st *store.Store) error {
		if !cfg.Dev {
			return errors.New("seed-demo only runs with HALO_DEV=1")
		}
		if err := st.SeedDemo(ctx); err != nil {
			return err
		}
		fmt.Println("Loaded the Fernway Systems demo organization.")
		return nil
	})
}

func newSecretKey(ctx context.Context, cfg config.Config, keys *kms.Keys) ([]byte, error) {
	value := os.Getenv("HALO_NEW_SECRET_KEY")
	if cfg.WrappedKey == "" {
		next, err := base64.StdEncoding.DecodeString(value)
		if err != nil || len(next) != 32 {
			return nil, errors.New("set HALO_NEW_SECRET_KEY to the new key: 32 random bytes, base64 encoded. Generate one with: openssl rand -base64 32")
		}
		return next, nil
	}
	if !cfg.KMS.Wrapped(value) {
		return nil, errors.New("set HALO_NEW_SECRET_KEY to a new key wrapped by " + cfg.KMS.Name() + ". Wrap one with: " + cfg.KMS.WrapCommand())
	}
	next, err := keys.SecretKey(ctx, value)
	if err != nil {
		return nil, fmt.Errorf("HALO_NEW_SECRET_KEY: %w", err)
	}
	return next, nil
}

func people(n int) string {
	if n == 1 {
		return "1 person"
	}
	return fmt.Sprintf("%d people", n)
}

func rotateSecretKey(ctx context.Context) error {
	return withStore(ctx, func(ctx context.Context, cfg config.Config, st *store.Store) error {
		next, err := newSecretKey(ctx, cfg, st.Keys)
		if err != nil {
			return err
		}
		if bytes.Equal(next, cfg.SecretKey) {
			generate := "openssl rand -base64 32"
			if cfg.WrappedKey != "" {
				generate = cfg.KMS.WrapCommand()
			}
			return errors.New("HALO_NEW_SECRET_KEY is the same key as HALO_SECRET_KEY. Generate a new key with: " + generate)
		}
		sealer, err := secret.NewSealer(next)
		if err != nil {
			return err
		}
		rotation, err := st.RotateSecretKey(ctx, sealer)
		if err != nil {
			return err
		}
		fmt.Println("Re-encrypted every stored secret with the new key in one transaction:")
		for _, c := range rotation.Resealed {
			fmt.Printf("  %-40s %d\n", c.Name, c.Rows)
		}
		fmt.Println("\nRecovery codes and temporary access passes are stored as HMACs keyed from HALO_SECRET_KEY, so they cannot be re-derived.")
		if len(rotation.RecoveryCodeUsers) == 0 && len(rotation.AccessPassUsers) == 0 {
			fmt.Println("Nobody had recovery codes or temporary access passes, so nothing else needs regenerating.")
		}
		if len(rotation.RecoveryCodeUsers) > 0 {
			fmt.Printf("Halo removed the recovery codes of %s. Ask them to generate new ones on their account's Security page:\n", people(len(rotation.RecoveryCodeUsers)))
			for _, email := range rotation.RecoveryCodeUsers {
				fmt.Println("  " + email)
			}
		}
		if len(rotation.AccessPassUsers) > 0 {
			fmt.Printf("Halo removed the temporary access passes of %s. Create new passes for them from the console:\n", people(len(rotation.AccessPassUsers)))
			for _, email := range rotation.AccessPassUsers {
				fmt.Println("  " + email)
			}
		}
		fmt.Print(`
Opaque access tokens and authorization codes that were not yet redeemed are encrypted with a key
derived from HALO_SECRET_KEY. They stop working once Halo runs with the new key: applications get
new access tokens with their refresh tokens, and people who were halfway through signing in start again.

Now change the deployment:
  1. Set HALO_SECRET_KEY to the value of HALO_NEW_SECRET_KEY wherever Halo reads its environment
     (deploy/.env with Docker Compose), and remove HALO_NEW_SECRET_KEY.
  2. Start Halo again, for example: docker compose up -d
  3. Store the new key in your password manager. Database backups taken before this rotation
     still need the old key, so keep the old key until those backups expire.
`)
		return nil
	})
}
