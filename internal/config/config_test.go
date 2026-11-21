package config

import (
	"strings"
	"testing"
)

func TestKeyManagerSettings(t *testing.T) {
	for _, c := range []struct {
		env     map[string]string
		problem string
	}{
		{map[string]string{"HALO_VAULT_ADDR": "http://vault.example.com:8200"}, "HALO_VAULT_ADDR must be the https address"},
		{map[string]string{"HALO_VAULT_ADDR": "https://vault.example.com:8200/v1"}, "HALO_VAULT_ADDR must be the https address"},
		{map[string]string{"HALO_VAULT_ADDR": "http://127.0.0.1:8100"}, ""},
		{map[string]string{"HALO_VAULT_ADDR": "https://vault.example.com:8200/", "HALO_VAULT_SIGNING": "1"}, ""},
		{map[string]string{"HALO_VAULT_SIGNING": "1"}, "HALO_VAULT_ADDR is required"},
		{map[string]string{"HALO_VAULT_ADDR": "https://vault.example.com", "HALO_VAULT_WRAPPING_KEY": "halo-secret-key"}, "must hold the key wrapped by Vault Transit"},
		{map[string]string{"HALO_VAULT_ADDR": "https://vault.example.com", "HALO_VAULT_WRAPPING_KEY": "halo-secret-key", "HALO_SECRET_KEY": "vault:v1:c2VhbGVk"}, ""},
		{map[string]string{"HALO_VAULT_ADDR": "https://vault.example.com", "HALO_AWS_KMS_REGION": "eu-central-1"}, "Set only one of HALO_VAULT_ADDR, HALO_AWS_KMS_REGION"},
		{map[string]string{"HALO_AWS_KMS_SIGNING": "1"}, "HALO_AWS_KMS_REGION is required"},
		{map[string]string{"HALO_AWS_KMS_REGION": "eu-central-1.evil.example"}, "HALO_AWS_KMS_REGION must be an AWS Region"},
		{map[string]string{"HALO_AWS_KMS_REGION": "eu-central-1", "HALO_AWS_KMS_ENDPOINT": "http://kms.example.com"}, "HALO_AWS_KMS_ENDPOINT must be an https address"},
		{map[string]string{"HALO_AWS_KMS_REGION": "eu-central-1", "HALO_AWS_KMS_WRAPPING_KEY": "alias/halo", "HALO_SECRET_KEY": "not base64!"}, "must hold the key wrapped by AWS KMS"},
		{map[string]string{"HALO_AWS_KMS_REGION": "us-gov-west-1", "HALO_AWS_KMS_WRAPPING_KEY": "alias/halo", "HALO_SECRET_KEY": "vault:v1:c2VhbGVk"}, "must hold the key wrapped by AWS KMS"},
		{map[string]string{"HALO_GCP_KMS_KEY_RING": "projects/acme/locations/global/keyRings/halo", "HALO_GCP_KMS_SIGNING": "1"}, ""},
		{map[string]string{"HALO_GCP_KMS_KEY_RING": "projects/acme/keyRings/halo", "HALO_GCP_KMS_WRAPPING_KEY": "halo-secret-key", "HALO_SECRET_KEY": "not base64!"}, "HALO_GCP_KMS_KEY_RING must be the resource name"},
		{map[string]string{"HALO_GCP_KMS_KEY_RING": "projects/acme/locations/global/keyRings/halo", "HALO_GCP_KMS_WRAPPING_KEY": "halo-secret-key", "HALO_SECRET_KEY": "not base64!"}, "gcloud kms encrypt --project acme --location global --keyring halo --key halo-secret-key"},
		{map[string]string{"HALO_GCP_KMS_KEY_RING": "projects/acme/locations/global/keyRings/halo", "HALO_GCP_KMS_WRAPPING_KEY": "../x"}, "HALO_GCP_KMS_WRAPPING_KEY must be the name"},
		{map[string]string{"HALO_AZURE_KEY_VAULT_URL": "https://acme.vault.azure.net/", "HALO_AZURE_KEY_VAULT_WRAPPING_KEY": "halo-secret-key/0123456789abcdef0123456789abcdef", "HALO_SECRET_KEY": "c2VhbGVk"}, ""},
		{map[string]string{"HALO_AZURE_KEY_VAULT_URL": "https://acme.vault.azure.net/keys"}, "HALO_AZURE_KEY_VAULT_URL must be the https address"},
		{map[string]string{"HALO_AZURE_KEY_VAULT_URL": "https://acme.vault.azure.net", "HALO_AZURE_KEY_VAULT_WRAPPING_KEY": "halo-secret-key", "HALO_SECRET_KEY": "c2VhbGVk"}, "must be the name of an RSA key and its version"},
		{map[string]string{"HALO_AZURE_KEY_VAULT_URL": "https://acme.vault.azure.net", "AZURE_CLIENT_SECRET": "secret"}, "AZURE_TENANT_ID and AZURE_CLIENT_ID are required"},
		{map[string]string{"HALO_PKCS11_MODULE": "/usr/lib/softhsm/libsofthsm2.so"}, "HALO_PKCS11_TOKEN_LABEL and HALO_PKCS11_PIN are required"},
	} {
		env := map[string]string{"HALO_DATABASE_URL": "postgres://halo@localhost/halo", "HALO_PUBLIC_URL": "https://auth.example.com", "HALO_SECRET_KEY": "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=", "HALO_DEV": "",
			"AWS_ACCESS_KEY_ID": "", "AWS_SECRET_ACCESS_KEY": "", "AZURE_TENANT_ID": "", "AZURE_CLIENT_ID": "", "AZURE_CLIENT_SECRET": "", "AZURE_FEDERATED_TOKEN_FILE": "", "AZURE_AUTHORITY_HOST": "", "HALO_VAULT_TRANSIT_MOUNT": ""}
		for _, prefix := range []string{"HALO_VAULT_", "HALO_AWS_KMS_", "HALO_GCP_KMS_", "HALO_AZURE_KEY_VAULT_", "HALO_PKCS11_"} {
			for _, name := range []string{"ADDR", "REGION", "ENDPOINT", "KEY_RING", "URL", "MODULE", "TOKEN_LABEL", "PIN", "WRAPPING_KEY", "SIGNING"} {
				env[prefix+name] = ""
			}
		}
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
		if err == nil && c.env["HALO_VAULT_ADDR"] != "" && (cfg.KMS.Vault.Mount != "transit" || strings.HasSuffix(cfg.KMS.Vault.Address, "/") || cfg.KMS.Name() != NameVault) {
			t.Fatalf("%v: %+v", c.env, cfg.KMS)
		}
		if err == nil && c.env["HALO_AZURE_KEY_VAULT_URL"] != "" && (cfg.KMS.Azure.URL != "https://acme.vault.azure.net" || cfg.WrappedKey != "c2VhbGVk") {
			t.Fatalf("%v: %+v", c.env, cfg.KMS)
		}
		if c.env["HALO_VAULT_WRAPPING_KEY"] != "" && err == nil && (cfg.WrappedKey != "vault:v1:c2VhbGVk" || cfg.SecretKey != nil) {
			t.Fatalf("a wrapped key is unwrapped at startup, not decoded: %+v", cfg)
		}
	}
}
