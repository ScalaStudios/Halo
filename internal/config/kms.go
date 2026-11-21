package config

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

type KMS struct {
	Vault       Vault
	AWS         AWSKMS
	GCP         GCPKMS
	Azure       AzureKeyVault
	PKCS11      PKCS11
	WrappingKey string
	Signing     bool
}

type Vault struct {
	Address   string
	Token     string
	Namespace string
	Mount     string
}

type AWSKMS struct {
	Region           string
	Endpoint         string
	AccessKeyID      string
	SecretAccessKey  string
	SessionToken     string
	MetadataEndpoint string
}

type GCPKMS struct {
	KeyRing         string
	Endpoint        string
	HSM             bool
	CredentialsFile string
	MetadataHost    string
}

type AzureKeyVault struct {
	URL                string
	HSM                bool
	TenantID           string
	ClientID           string
	ClientSecret       string
	FederatedTokenFile string
	AuthorityHost      string
	IdentityEndpoint   string
	IdentityHeader     string
	MetadataEndpoint   string
}

type PKCS11 struct {
	Module     string
	TokenLabel string
	PIN        string
}

const (
	NameVault    = "Vault Transit"
	NameAWS      = "AWS KMS"
	NameGCP      = "Google Cloud KMS"
	NameAzure    = "Azure Key Vault"
	NamePKCS11   = "PKCS #11"
	vaultPrefix  = "HALO_VAULT_"
	awsPrefix    = "HALO_AWS_KMS_"
	gcpPrefix    = "HALO_GCP_KMS_"
	azurePrefix  = "HALO_AZURE_KEY_VAULT_"
	pkcs11Prefix = "HALO_PKCS11_"
)

var (
	awsRegion  = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-[0-9]+$`)
	gcpKeyRing = regexp.MustCompile(`^projects/([a-z0-9-]+)/locations/([a-z0-9-]+)/keyRings/([A-Za-z0-9_-]+)$`)
	gcpKeyName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	azureKey   = regexp.MustCompile(`^[0-9A-Za-z-]+/[0-9a-f]{32}$`)
	providers  = []struct{ setting, prefix string }{
		{"HALO_VAULT_ADDR", vaultPrefix},
		{"HALO_AWS_KMS_REGION", awsPrefix},
		{"HALO_GCP_KMS_KEY_RING", gcpPrefix},
		{"HALO_AZURE_KEY_VAULT_URL", azurePrefix},
		{"HALO_PKCS11_MODULE", pkcs11Prefix},
	}
)

func (k KMS) Name() string {
	switch {
	case k.Vault.Address != "":
		return NameVault
	case k.AWS.Region != "":
		return NameAWS
	case k.GCP.KeyRing != "":
		return NameGCP
	case k.Azure.URL != "":
		return NameAzure
	case k.PKCS11.Module != "":
		return NamePKCS11
	}
	return ""
}

func (k KMS) WrapCommand() string {
	oaep := "openssl rand 32 | openssl pkeyutl -encrypt -pubin -inkey %s -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha256 -pkeyopt rsa_mgf1_md:sha256 | base64 -w0"
	switch k.Name() {
	case NameAWS:
		return "aws kms encrypt --region " + k.AWS.Region + " --key-id " + k.WrappingKey + " --plaintext fileb://<(openssl rand 32) --query CiphertextBlob --output text"
	case NameGCP:
		ring := gcpKeyRing.FindStringSubmatch(k.GCP.KeyRing)
		if ring == nil {
			ring = []string{"", "<project>", "<location>", "<key ring>"}
		}
		return "openssl rand 32 | gcloud kms encrypt --project " + ring[1] + " --location " + ring[2] + " --keyring " + ring[3] + " --key " + k.WrappingKey + " --plaintext-file - --ciphertext-file - | base64 -w0"
	case NameAzure:
		return "az keyvault key download --id " + k.Azure.URL + "/keys/" + k.WrappingKey + " --file halo-wrapping-key.pem && " + fmt.Sprintf(oaep, "halo-wrapping-key.pem")
	case NamePKCS11:
		return fmt.Sprintf(oaep, "<public key of "+k.WrappingKey+">.pem")
	}
	return "vault write -field=ciphertext " + k.Vault.Mount + "/encrypt/" + k.WrappingKey + ` plaintext="$(openssl rand -base64 32)"`
}

func (k KMS) Wrapped(value string) bool {
	if k.Name() == NameVault {
		return strings.HasPrefix(value, "vault:v")
	}
	_, err := base64.StdEncoding.DecodeString(value)
	return value != "" && err == nil
}

func loadKMS(dev bool, problems []string) (KMS, []string) {
	var chosen []string
	prefix := ""
	for _, p := range providers {
		if os.Getenv(p.setting) != "" {
			chosen, prefix = append(chosen, p.setting), p.prefix
			continue
		}
		if os.Getenv(p.prefix+"WRAPPING_KEY") != "" || os.Getenv(p.prefix+"SIGNING") != "" {
			problems = append(problems, p.setting+" is required when "+p.prefix+"WRAPPING_KEY or "+p.prefix+"SIGNING is set")
		}
	}
	if len(chosen) > 1 {
		problems = append(problems, "Set only one of "+strings.Join(chosen, ", ")+": Halo keeps its keys in one key manager")
	}
	cfg := KMS{WrappingKey: os.Getenv(prefix + "WRAPPING_KEY"), Signing: prefix != "" && os.Getenv(prefix+"SIGNING") == "1"}
	switch prefix {
	case vaultPrefix:
		cfg.Vault = Vault{
			Address:   strings.TrimRight(os.Getenv("HALO_VAULT_ADDR"), "/"),
			Token:     os.Getenv("HALO_VAULT_TOKEN"),
			Namespace: os.Getenv("HALO_VAULT_NAMESPACE"),
			Mount:     strings.Trim(os.Getenv("HALO_VAULT_TRANSIT_MOUNT"), "/"),
		}
		if cfg.Vault.Mount == "" {
			cfg.Vault.Mount = "transit"
		}
		if !secureEndpoint(cfg.Vault.Address, dev) {
			problems = append(problems, "HALO_VAULT_ADDR must be the https address of Vault, such as https://vault.example.com:8200. Plain http is accepted only to a Vault Agent on the loopback address, such as http://127.0.0.1:8100, or with HALO_DEV=1")
		}
	case awsPrefix:
		cfg.AWS = AWSKMS{
			Region:           os.Getenv("HALO_AWS_KMS_REGION"),
			Endpoint:         strings.TrimRight(os.Getenv("HALO_AWS_KMS_ENDPOINT"), "/"),
			AccessKeyID:      os.Getenv("AWS_ACCESS_KEY_ID"),
			SecretAccessKey:  os.Getenv("AWS_SECRET_ACCESS_KEY"),
			SessionToken:     os.Getenv("AWS_SESSION_TOKEN"),
			MetadataEndpoint: strings.TrimRight(os.Getenv("AWS_EC2_METADATA_SERVICE_ENDPOINT"), "/"),
		}
		if !awsRegion.MatchString(cfg.AWS.Region) {
			problems = append(problems, "HALO_AWS_KMS_REGION must be an AWS Region such as eu-central-1")
		}
		if cfg.AWS.Endpoint != "" && !secureEndpoint(cfg.AWS.Endpoint, dev) {
			problems = append(problems, "HALO_AWS_KMS_ENDPOINT must be an https address without a path, such as https://kms-fips.us-east-1.amazonaws.com")
		}
		if (cfg.AWS.AccessKeyID == "") != (cfg.AWS.SecretAccessKey == "") {
			problems = append(problems, "Set both AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY, or neither to use the instance role")
		}
	case gcpPrefix:
		cfg.GCP = GCPKMS{
			KeyRing:         strings.Trim(os.Getenv("HALO_GCP_KMS_KEY_RING"), "/"),
			Endpoint:        strings.TrimRight(os.Getenv("HALO_GCP_KMS_ENDPOINT"), "/"),
			HSM:             os.Getenv("HALO_GCP_KMS_HSM") == "1",
			CredentialsFile: os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
			MetadataHost:    os.Getenv("GCE_METADATA_HOST"),
		}
		if !gcpKeyRing.MatchString(cfg.GCP.KeyRing) {
			problems = append(problems, "HALO_GCP_KMS_KEY_RING must be the resource name of a key ring, such as projects/acme/locations/europe-west1/keyRings/halo")
		}
		if cfg.GCP.Endpoint != "" && !secureEndpoint(cfg.GCP.Endpoint, dev) {
			problems = append(problems, "HALO_GCP_KMS_ENDPOINT must be an https address without a path, such as https://cloudkms.googleapis.com")
		}
		if cfg.WrappingKey != "" && !gcpKeyName.MatchString(cfg.WrappingKey) {
			problems = append(problems, "HALO_GCP_KMS_WRAPPING_KEY must be the name of an encryption key in HALO_GCP_KMS_KEY_RING, such as halo-secret-key")
		}
	case azurePrefix:
		cfg.Azure = AzureKeyVault{
			URL:                strings.TrimRight(os.Getenv("HALO_AZURE_KEY_VAULT_URL"), "/"),
			HSM:                os.Getenv("HALO_AZURE_KEY_VAULT_HSM") == "1",
			TenantID:           os.Getenv("AZURE_TENANT_ID"),
			ClientID:           os.Getenv("AZURE_CLIENT_ID"),
			ClientSecret:       os.Getenv("AZURE_CLIENT_SECRET"),
			FederatedTokenFile: os.Getenv("AZURE_FEDERATED_TOKEN_FILE"),
			AuthorityHost:      strings.TrimRight(os.Getenv("AZURE_AUTHORITY_HOST"), "/"),
			IdentityEndpoint:   os.Getenv("IDENTITY_ENDPOINT"),
			IdentityHeader:     os.Getenv("IDENTITY_HEADER"),
		}
		if !secureEndpoint(cfg.Azure.URL, dev) {
			problems = append(problems, "HALO_AZURE_KEY_VAULT_URL must be the https address of the key vault, such as https://acme-halo.vault.azure.net")
		}
		if cfg.Azure.AuthorityHost != "" && !secureEndpoint(cfg.Azure.AuthorityHost, dev) {
			problems = append(problems, "AZURE_AUTHORITY_HOST must be an https address without a path, such as https://login.microsoftonline.com")
		}
		if (cfg.Azure.ClientSecret != "" || cfg.Azure.FederatedTokenFile != "") && (cfg.Azure.TenantID == "" || cfg.Azure.ClientID == "") {
			problems = append(problems, "AZURE_TENANT_ID and AZURE_CLIENT_ID are required with AZURE_CLIENT_SECRET or AZURE_FEDERATED_TOKEN_FILE")
		}
		if cfg.WrappingKey != "" && !azureKey.MatchString(cfg.WrappingKey) {
			problems = append(problems, "HALO_AZURE_KEY_VAULT_WRAPPING_KEY must be the name of an RSA key and its version, such as halo-secret-key/0123456789abcdef0123456789abcdef")
		}
	case pkcs11Prefix:
		cfg.PKCS11 = PKCS11{Module: os.Getenv("HALO_PKCS11_MODULE"), TokenLabel: os.Getenv("HALO_PKCS11_TOKEN_LABEL"), PIN: os.Getenv("HALO_PKCS11_PIN")}
		if cfg.PKCS11.TokenLabel == "" || cfg.PKCS11.PIN == "" {
			problems = append(problems, "HALO_PKCS11_TOKEN_LABEL and HALO_PKCS11_PIN are required with HALO_PKCS11_MODULE")
		}
	}
	return cfg, problems
}

func secureEndpoint(raw string, dev bool) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Host != "" && u.Path == "" && u.RawQuery == "" && (u.Scheme == "https" || u.Scheme == "http" && (dev || Loopback(u.Hostname())))
}
