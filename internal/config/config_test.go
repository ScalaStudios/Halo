package config

import (
	"strings"
	"testing"
)

func TestVaultSettings(t *testing.T) {
	for _, c := range []struct {
		env     map[string]string
		problem string
	}{
		{map[string]string{"HALO_VAULT_ADDR": "http://vault.example.com:8200"}, "HALO_VAULT_ADDR must be the https address"},
		{map[string]string{"HALO_VAULT_ADDR": "https://vault.example.com:8200/v1"}, "HALO_VAULT_ADDR must be the https address"},
		{map[string]string{"HALO_VAULT_ADDR": "http://127.0.0.1:8100"}, ""},
		{map[string]string{"HALO_VAULT_ADDR": "https://vault.example.com:8200/", "HALO_VAULT_SIGNING": "1"}, ""},
		{map[string]string{"HALO_VAULT_SIGNING": "1"}, "HALO_VAULT_ADDR is required"},
		{map[string]string{"HALO_VAULT_ADDR": "https://vault.example.com", "HALO_VAULT_WRAPPING_KEY": "halo-secret-key"}, "starting with vault:v"},
		{map[string]string{"HALO_VAULT_ADDR": "https://vault.example.com", "HALO_VAULT_WRAPPING_KEY": "halo-secret-key", "HALO_SECRET_KEY": "vault:v1:c2VhbGVk"}, ""},
	} {
		env := map[string]string{"HALO_DATABASE_URL": "postgres://halo@localhost/halo", "HALO_PUBLIC_URL": "https://auth.example.com", "HALO_SECRET_KEY": "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
			"HALO_VAULT_ADDR": "", "HALO_VAULT_SIGNING": "", "HALO_VAULT_WRAPPING_KEY": "", "HALO_VAULT_TRANSIT_MOUNT": "", "HALO_DEV": ""}
		for k, v := range c.env {
			env[k] = v
		}
		for k, v := range env {
			t.Setenv(k, v)
		}
		cfg, err := Load()
		if c.problem == "" && err != nil || c.problem != "" && (err == nil || !strings.Contains(err.Error(), c.problem)) {
			t.Fatalf("%v: want %q, got %v", c.env, c.problem, err)
		}
		if err == nil && (cfg.Vault.Mount != "transit" || strings.HasSuffix(cfg.Vault.Address, "/")) {
			t.Fatalf("%v: %+v", c.env, cfg.Vault)
		}
		if c.env["HALO_VAULT_WRAPPING_KEY"] != "" && err == nil && (cfg.WrappedKey != "vault:v1:c2VhbGVk" || cfg.SecretKey != nil) {
			t.Fatalf("a wrapped key is unwrapped at startup, not decoded: %+v", cfg)
		}
	}
}
