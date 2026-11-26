package config

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Config struct {
	Organization   string
	DatabaseURL    string
	SMTP           SMTP
	S3             S3
	LogExport      LogExport
	LDAP           LDAP
	TrustedProxies []string
	CertHeader     string
	PublicURL      *url.URL
	Listen         string
	MetricsListen  string
	RADIUSListen   string
	MetricsToken   string
	SecretKey      []byte
	KMS            KMS
	WrappedKey     string
	Dev            bool
	NoCustomCSS    bool
	OutboundAllow  []netip.Prefix
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

type S3 struct {
	Endpoint        *url.URL
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
}

func (s S3) Configured() bool {
	return s.Endpoint != nil && s.Endpoint.Host != "" && s.Bucket != ""
}

func (s S3) Client() (*minio.Client, error) {
	return minio.New(s.Endpoint.Host, &minio.Options{
		Creds:        credentials.NewStaticV4(s.AccessKeyID, s.SecretAccessKey, ""),
		Secure:       s.Endpoint.Scheme == "https",
		Region:       s.Region,
		BucketLookup: minio.BucketLookupPath,
	})
}

type LDAP struct {
	Listen   string
	TLS      string
	CertFile string
	KeyFile  string
}

type LogExport struct {
	Protocol     string
	Endpoint     string
	Headers      string
	AllowPrivate bool
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

	cfg.MetricsListen = os.Getenv("HALO_METRICS_LISTEN")
	cfg.MetricsToken = os.Getenv("HALO_METRICS_TOKEN")
	if cfg.MetricsListen != "" && cfg.MetricsListen == cfg.Listen {
		problems = append(problems, "HALO_METRICS_LISTEN must differ from HALO_LISTEN, for example :9090, so metrics stay off the public port")
	}

	cfg.RADIUSListen = os.Getenv("HALO_RADIUS_LISTEN")

	cfg.LDAP = LDAP{Listen: os.Getenv("HALO_LDAP_LISTEN"), TLS: os.Getenv("HALO_LDAP_TLS"), CertFile: os.Getenv("HALO_LDAP_CERT_FILE"), KeyFile: os.Getenv("HALO_LDAP_KEY_FILE")}
	if cfg.LDAP.Listen != "" {
		if cfg.LDAP.TLS == "" {
			cfg.LDAP.TLS = "ldaps"
		}
		switch {
		case cfg.LDAP.Listen == cfg.Listen || cfg.LDAP.Listen == cfg.MetricsListen:
			problems = append(problems, "HALO_LDAP_LISTEN must differ from HALO_LISTEN and HALO_METRICS_LISTEN, for example :636")
		case cfg.LDAP.TLS != "ldaps" && cfg.LDAP.TLS != "starttls" && cfg.LDAP.TLS != "none":
			problems = append(problems, "HALO_LDAP_TLS must be ldaps (the default), starttls, or none to accept passwords without TLS")
		case cfg.LDAP.TLS != "none" && (cfg.LDAP.CertFile == "" || cfg.LDAP.KeyFile == ""):
			problems = append(problems, "HALO_LDAP_CERT_FILE and HALO_LDAP_KEY_FILE are required for LDAPS and StartTLS: PEM files with the certificate chain and its private key")
		}
	}

	cfg.Dev = os.Getenv("HALO_DEV") == "1"
	cfg.NoCustomCSS = os.Getenv("HALO_DISABLE_CUSTOM_CSS") == "1"
	for _, item := range strings.FieldsFunc(os.Getenv("HALO_OUTBOUND_ALLOW"), func(r rune) bool { return r == ',' || r == ' ' }) {
		prefix, err := netip.ParsePrefix(item)
		if err != nil {
			problems = append(problems, "HALO_OUTBOUND_ALLOW must list address ranges separated by commas, for example 10.20.0.0/16,192.168.5.10/32; "+item+" is not one")
			continue
		}
		cfg.OutboundAllow = append(cfg.OutboundAllow, prefix.Masked())
	}

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

	cfg.LogExport = LogExport{
		Protocol:     os.Getenv("HALO_LOG_EXPORT_PROTOCOL"),
		Endpoint:     os.Getenv("HALO_LOG_EXPORT_ENDPOINT"),
		Headers:      os.Getenv("HALO_LOG_EXPORT_HEADERS"),
		AllowPrivate: os.Getenv("HALO_LOG_EXPORT_ALLOW_PRIVATE") == "1",
	}

	s3 := map[string]string{}
	for _, name := range []string{"HALO_S3_ENDPOINT", "HALO_S3_BUCKET", "HALO_S3_ACCESS_KEY_ID", "HALO_S3_SECRET_ACCESS_KEY"} {
		s3[name] = os.Getenv(name)
	}
	if s3["HALO_S3_ENDPOINT"] != "" || s3["HALO_S3_BUCKET"] != "" {
		for name, value := range s3 {
			if value == "" {
				problems = append(problems, name+" is required when storing profile pictures in an S3-compatible bucket")
			}
		}
		endpoint := strings.TrimRight(s3["HALO_S3_ENDPOINT"], "/")
		u, err := url.Parse(endpoint)
		if endpoint == "" || err != nil || u.Scheme != "https" && u.Scheme != "http" || u.Host == "" || u.Path != "" && u.Path != "/" {
			problems = append(problems, "HALO_S3_ENDPOINT must be the endpoint address without the bucket, such as https://<account>.r2.cloudflarestorage.com, https://s3.us-west-004.backblazeb2.com, https://s3.us-east-1.idrivee2.com or http://minio:9000")
		} else if s3["HALO_S3_BUCKET"] != "" && s3["HALO_S3_ACCESS_KEY_ID"] != "" && s3["HALO_S3_SECRET_ACCESS_KEY"] != "" {
			region := os.Getenv("HALO_S3_REGION")
			if region == "" {
				region = "auto"
			}
			u.Path = ""
			cfg.S3 = S3{Endpoint: u, Region: region, Bucket: s3["HALO_S3_BUCKET"], AccessKeyID: s3["HALO_S3_ACCESS_KEY_ID"], SecretAccessKey: s3["HALO_S3_SECRET_ACCESS_KEY"]}
		}
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

	cfg.CertHeader = strings.TrimSpace(os.Getenv("HALO_CLIENT_CERT_HEADER"))

	cfg.KMS, problems = loadKMS(cfg.Dev, problems)
	if cfg.KMS.WrappingKey != "" {
		cfg.WrappedKey = os.Getenv("HALO_SECRET_KEY")
		if !cfg.KMS.Wrapped(cfg.WrappedKey) {
			problems = append(problems, "HALO_SECRET_KEY must hold the key wrapped by "+cfg.KMS.Name()+", because a wrapping key is set. Wrap a key with: "+cfg.KMS.WrapCommand())
		}
	} else {
		key, err := base64.StdEncoding.DecodeString(os.Getenv("HALO_SECRET_KEY"))
		if err != nil || len(key) != 32 {
			problems = append(problems, "HALO_SECRET_KEY must be 32 random bytes, base64 encoded. Generate one with: openssl rand -base64 32")
		}
		cfg.SecretKey = key
	}

	if cfg.PublicURL != nil && cfg.PublicURL.Scheme != "https" && !cfg.Dev {
		problems = append(problems, "HALO_PUBLIC_URL must use https outside development. Set HALO_DEV=1 for local http")
	}

	if len(problems) > 0 {
		return cfg, fmt.Errorf("configuration is incomplete:\n  %s", strings.Join(problems, "\n  "))
	}
	return cfg, nil
}

func Loopback(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || ip != nil && ip.IsLoopback()
}

func (c Config) Issuer() string {
	return c.PublicURL.String()
}

func (c Config) Hostname() string {
	return c.PublicURL.Hostname()
}
