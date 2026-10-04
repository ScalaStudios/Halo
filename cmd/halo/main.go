package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"halo/internal/cli"
	"halo/internal/config"
	"halo/internal/db"
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
	case "login", "logout", "whoami", "ssh-cert":
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
	return fn(ctx, cfg, st)
}

func serve(ctx context.Context) error {
	return withStore(ctx, func(ctx context.Context, cfg config.Config, st *store.Store) error {
		app, err := server.New(cfg, st)
		if err != nil {
			return err
		}
		go app.Jobs.Run(ctx)
		srv := &http.Server{Addr: cfg.Listen, Handler: app.Handler, ReadHeaderTimeout: 10 * time.Second}
		errs := make(chan error, 1)
		go func() {
			slog.Info("halo listening", "addr", cfg.Listen, "issuer", cfg.Issuer())
			errs <- srv.ListenAndServe()
		}()
		select {
		case err := <-errs:
			return err
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
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
		users, err := st.ListUsers(ctx)
		if err != nil {
			return err
		}
		for _, u := range users {
			if u.HasRole("global_admin") {
				return fmt.Errorf("%s is already a global administrator; invite more people from the console instead", u.Email)
			}
		}
		user, err := st.CreateUser(ctx, store.NewUser{Email: *email, Name: *name, Status: "invited", Roles: []string{"global_admin"}})
		if err != nil {
			return err
		}
		token, expires, err := st.CreateEnrollmentToken(ctx, user.ID, "invite", nil)
		if err != nil {
			return err
		}
		if err := st.RecordAudit(ctx, store.AuditEvent{Action: "user.bootstrap", Summary: "Created the first global administrator", TargetType: "user", TargetID: user.ID, TargetLabel: user.Name}); err != nil {
			return err
		}
		fmt.Printf("Created %s <%s> as global administrator.\n\nOpen this link to set up a passkey. It works once and expires %s:\n\n  %s%s\n\n",
			user.Name, user.Email, expires.Format("2 January 2006 15:04 MST"), cfg.Issuer(), store.EnrollPath(token))
		return nil
	})
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

func rotateSecretKey(ctx context.Context) error {
	next, err := base64.StdEncoding.DecodeString(os.Getenv("HALO_NEW_SECRET_KEY"))
	if err != nil || len(next) != 32 {
		return errors.New("set HALO_NEW_SECRET_KEY to the new key: 32 random bytes, base64 encoded. Generate one with: openssl rand -base64 32")
	}
	sealer, err := secret.NewSealer(next)
	if err != nil {
		return err
	}
	return withStore(ctx, func(ctx context.Context, cfg config.Config, st *store.Store) error {
		if bytes.Equal(next, cfg.SecretKey) {
			return errors.New("HALO_NEW_SECRET_KEY is the same as HALO_SECRET_KEY. Generate a new key with: openssl rand -base64 32")
		}
		rotation, err := st.RotateSecretKey(ctx, sealer)
		if err != nil {
			return err
		}
		fmt.Println("Re-encrypted every stored secret with the new key in one transaction:")
		for _, c := range rotation.Resealed {
			fmt.Printf("  %-40s %d\n", c.Name, c.Rows)
		}
		fmt.Println("\nRecovery codes are stored as HMACs keyed from HALO_SECRET_KEY, so they cannot be re-derived.")
		if len(rotation.RecoveryCodeUsers) == 0 {
			fmt.Println("Nobody had recovery codes, so nothing else needs regenerating.")
		} else {
			people := fmt.Sprintf("%d people", len(rotation.RecoveryCodeUsers))
			if len(rotation.RecoveryCodeUsers) == 1 {
				people = "1 person"
			}
			fmt.Printf("Halo removed the recovery codes of %s. Ask them to generate new ones on their account's Security page:\n", people)
			for _, email := range rotation.RecoveryCodeUsers {
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
