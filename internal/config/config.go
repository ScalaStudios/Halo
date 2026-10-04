package config

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	Organization   string
	DatabaseURL    string
	SMTP           SMTP
	TrustedProxies []string
	PublicURL      *url.URL
	Listen         string
	SecretKey      []byte
	Dev            bool
}

type SMTP struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

func (s SMTP) Configured() bool {
	return s.Host != ""
}

func Load() (Config, error) {
	var cfg Config
	var problems []string

	cfg.DatabaseURL = os.Getenv("HALO_DATABASE_URL")
	if cfg.DatabaseURL == "" {
		problems = append(problems, "HALO_DATABASE_URL is required, for example postgres://halo:halo@localhost:5436/halo")
	}

	public := os.Getenv("HALO_PUBLIC_URL")
	if public == "" {
		problems = append(problems, "HALO_PUBLIC_URL is required: the address people use to reach Halo, for example https://auth.example.com")
	} else if u, err := url.Parse(strings.TrimRight(public, "/")); err != nil || u.Scheme == "" || u.Host == "" {
		problems = append(problems, "HALO_PUBLIC_URL must be an absolute URL such as https://auth.example.com")
	} else {
		cfg.PublicURL = u
	}

	cfg.Organization = os.Getenv("HALO_ORGANIZATION")
	if cfg.Organization == "" {
		cfg.Organization = "Halo"
	}

	cfg.Listen = os.Getenv("HALO_LISTEN")
	if cfg.Listen == "" {
		cfg.Listen = ":8080"
	}

	cfg.Dev = os.Getenv("HALO_DEV") == "1"

	cfg.SMTP = SMTP{
		Host:     os.Getenv("HALO_SMTP_HOST"),
		Port:     os.Getenv("HALO_SMTP_PORT"),
		Username: os.Getenv("HALO_SMTP_USERNAME"),
		Password: os.Getenv("HALO_SMTP_PASSWORD"),
		From:     os.Getenv("HALO_SMTP_FROM"),
	}
	if cfg.SMTP.Port == "" {
		cfg.SMTP.Port = "587"
	}
	if cfg.SMTP.Configured() && cfg.SMTP.From == "" {
		problems = append(problems, "HALO_SMTP_FROM is required when HALO_SMTP_HOST is set, for example: Halo <halo@example.com>")
	}

	cfg.TrustedProxies = []string{"127.0.0.1/32", "::1/128"}
	if proxies := os.Getenv("HALO_TRUSTED_PROXIES"); proxies != "" {
		cfg.TrustedProxies = nil
		for _, cidr := range strings.Split(proxies, ",") {
			cidr = strings.TrimSpace(cidr)
			if _, _, err := net.ParseCIDR(cidr); err != nil {
				problems = append(problems, fmt.Sprintf("HALO_TRUSTED_PROXIES contains %q, which is not a CIDR range such as 10.0.0.0/8", cidr))
				continue
			}
			cfg.TrustedProxies = append(cfg.TrustedProxies, cidr)
		}
	}

	key, err := base64.StdEncoding.DecodeString(os.Getenv("HALO_SECRET_KEY"))
	if err != nil || len(key) != 32 {
		problems = append(problems, "HALO_SECRET_KEY must be 32 random bytes, base64 encoded. Generate one with: openssl rand -base64 32")
	}
	cfg.SecretKey = key

	if cfg.PublicURL != nil && cfg.PublicURL.Scheme != "https" && !cfg.Dev {
		problems = append(problems, "HALO_PUBLIC_URL must use https outside development. Set HALO_DEV=1 for local http")
	}

	if len(problems) > 0 {
		return cfg, fmt.Errorf("configuration is incomplete:\n  %s", strings.Join(problems, "\n  "))
	}
	return cfg, nil
}

func (c Config) Issuer() string {
	return c.PublicURL.String()
}

func (c Config) Hostname() string {
	return c.PublicURL.Hostname()
}
