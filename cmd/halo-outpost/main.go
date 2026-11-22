package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"halo/internal/outpost"
)

const usage = `Halo outpost: reaches hosts on a private network for browser sessions started in Halo

Usage:
  halo-outpost --server URL [--token T] [--key-file F] [--guacd-port PORT]

The first run enrolls with the token from the Remote access page of the Halo console and
writes the outpost key to the key file. Later runs connect with that key and ignore the token.
To enroll again, create a new token in the console and delete the key file.

The outpost only connects out to Halo over HTTPS. It opens a TCP connection to a host only
when Halo asks for one, for a remote access application registered with this outpost.

Set --guacd-port to enable RDP and VNC through a Guacamole proxy listening on loopback.
Flags can also be set with HALO_URL, HALO_OUTPOST_TOKEN, HALO_OUTPOST_KEY_FILE and HALO_OUTPOST_GUACD_PORT.
`

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "halo-outpost:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("halo-outpost", flag.ContinueOnError)
	flags.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	server := flags.String("server", os.Getenv("HALO_URL"), "address of your Halo server, for example https://auth.example.com")
	token := flags.String("token", os.Getenv("HALO_OUTPOST_TOKEN"), "enrollment token from the Remote access page of the Halo console")
	keyFile := flags.String("key-file", cmp.Or(os.Getenv("HALO_OUTPOST_KEY_FILE"), "halo-outpost.key"), "file that holds the outpost key")
	defaultPort, err := strconv.Atoi(cmp.Or(os.Getenv("HALO_OUTPOST_GUACD_PORT"), "0"))
	if err != nil {
		return errors.New("HALO_OUTPOST_GUACD_PORT must be a port number")
	}
	guacdPort := flags.Int("guacd-port", defaultPort, "port of the local Guacamole proxy; 0 disables desktop access")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *guacdPort < 0 || *guacdPort > 65535 {
		return errors.New("pass a Guacamole proxy port between 0 and 65535")
	}
	base := strings.TrimRight(*server, "/")
	if u, err := url.Parse(base); err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("pass the HTTPS address of your Halo server, for example --server https://auth.example.com")
	}
	raw, err := os.ReadFile(*keyFile)
	key := strings.TrimSpace(string(raw))
	switch {
	case errors.Is(err, fs.ErrNotExist) && *token == "":
		return fmt.Errorf("%s does not exist yet. Pass the enrollment token from the Remote access page of the Halo console with --token", *keyFile)
	case errors.Is(err, fs.ErrNotExist):
		if key, err = outpost.Enroll(ctx, base, *token); err != nil {
			return fmt.Errorf("enrollment failed: %w", err)
		}
		if err := os.WriteFile(*keyFile, []byte(key+"\n"), 0o600); err != nil {
			return fmt.Errorf("enrolled, but could not save the key to %s: %w. Create a new token in the console and try again", *keyFile, err)
		}
		slog.Info("enrolled with Halo", "server", base, "key_file", *keyFile)
	case err != nil:
		return err
	}
	return (&outpost.Agent{Server: base, Key: key, GuacdPort: *guacdPort, Log: slog.Default()}).Run(ctx)
}
