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
	S3             S3
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
