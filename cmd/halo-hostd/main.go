package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"

	"halo/internal/hostd"
)

const usage = `Halo host daemon

Usage:
  halo-hostd run --server URL [--key K]   Keep this host's Linux accounts and sudo rules in step with Halo
  halo-hostd refresh --server URL [--key K]
                                          Fetch the directory once, write the files and exit
  halo-hostd principals USER              Print the SSH certificate principal for USER, for sshd's AuthorizedPrincipalsCommand

Send SIGHUP (systemctl reload halo-hostd) to refresh at once.
`

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "halo-hostd:", err)
		os.Exit(1)
	}
}

func run(command string, args []string) error {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	server := flags.String("server", "", "address of your Halo server, for example https://auth.example.com")
	keyPath := flags.String("key", "/etc/ssh/ssh_host_ed25519_key", "host private key enrolled with halo ssh-host enroll")
	state := flags.String("state", "/var/lib/halo-hostd", "directory for the offline cache")
	nss := flags.String("nss", "/var/lib/extrausers", "directory libnss-extrausers reads passwd and group from")
	sudoers := flags.String("sudoers", "/etc/sudoers.d/halo", "sudoers file for the sudo rules")
	etc := flags.String("etc", "/etc", "directory with the local passwd and group files, whose accounts Halo never overrides")
	visudo := flags.String("visudo", "/usr/sbin/visudo", "visudo binary that validates the sudo rules")
	interval := flags.Duration("interval", 5*time.Minute, "time between refreshes")
	if err := flags.Parse(args); err != nil {
		return err
	}
	switch command {
	case "principals":
		if flags.NArg() != 1 {
			return fmt.Errorf("pass the account name, for example halo-hostd principals %%u")
		}
		return hostd.Principals(*nss, *etc, flags.Arg(0), os.Stdout)
	case "run", "refresh":
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if *interval < time.Minute {
		return fmt.Errorf("refresh at most once a minute")
	}
	raw, err := os.ReadFile(*keyPath)
	if err != nil {
		return err
	}
	signer, err := ssh.ParsePrivateKey(raw)
	if err != nil {
		return fmt.Errorf("%s is not an unencrypted OpenSSH private key. Pass the enrolled host key, for example --key /etc/ssh/ssh_host_ed25519_key", *keyPath)
	}
	c := hostd.Config{Server: *server, Signer: signer, Client: &http.Client{Timeout: 30 * time.Second}, StateDir: *state, NSSDir: *nss, Sudoers: *sudoers, Etc: *etc, Visudo: *visudo, Interval: *interval}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if command == "refresh" {
		return c.Refresh(ctx)
	}
	reload := make(chan os.Signal, 1)
	signal.Notify(reload, syscall.SIGHUP)
	return c.Run(ctx, reload)
}
