package kmstest

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"halo/internal/config"
	"halo/internal/id"
)

var azureName = regexp.MustCompile(`^[0-9a-zA-Z-]+$`)

type Azure struct {
	*httptest.Server
	HSM           bool
	wrapping      *rsa.PrivateKey
	mu            sync.Mutex
	tokens        map[string]bool
	keys          map[string]crypto.Signer
	exportable    map[string]bool
	authorization string
}

func NewAzure(t testing.TB) *Azure {
	t.Helper()
	wrapping, _ := rsa.GenerateKey(rand.Reader, 2048)
	a := &Azure{wrapping: wrapping, tokens: map[string]bool{}, keys: map[string]crypto.Signer{}, exportable: map[string]bool{}}
	a.Server = httptest.NewServer(http.HandlerFunc(a.serve))
	t.Cleanup(a.Close)
	return a
}

func (a *Azure) Config() config.KMS {
	return config.KMS{Azure: config.AzureKeyVault{URL: a.URL, HSM: a.HSM, TenantID: "acme-tenant", ClientID: "halo-app", ClientSecret: "halo-client-secret", AuthorityHost: a.URL}, WrappingKey: "halo-secret-key/0123456789abcdef0123456789abcdef", Signing: true}
}

func (a *Azure) ManagedIdentityConfig() config.KMS {
	return config.KMS{Azure: config.AzureKeyVault{URL: a.URL, MetadataEndpoint: a.URL}, WrappingKey: "halo-secret-key/0123456789abcdef0123456789abcdef", Signing: true}
}

func (a *Azure) AppServiceConfig() config.KMS {
	return config.KMS{Azure: config.AzureKeyVault{URL: a.URL, IdentityEndpoint: a.URL + "/msi/token", IdentityHeader: "identity-header"}, WrappingKey: "halo-secret-key/0123456789abcdef0123456789abcdef", Signing: true}
}

func (a *Azure) WorkloadIdentityConfig(t testing.TB) config.KMS {
	file := filepath.Join(t.TempDir(), "azure-identity-token")
	if err := os.WriteFile(file, []byte("federated-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := a.Config()
	cfg.Azure.ClientSecret, cfg.Azure.FederatedTokenFile = "", file
	return cfg
}

func (a *Azure) Wrap(plain []byte) string {
	sealed, _ := rsa.EncryptOAEP(sha256.New(), rand.Reader, &a.wrapping.PublicKey, plain, nil)
	return base64.StdEncoding.EncodeToString(sealed)
}

func (a *Azure) Has(kid string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.keys[strings.TrimPrefix(kid, a.URL+"/keys")] != nil
}

func (a *Azure) AllowExport(kid string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.exportable[strings.TrimPrefix(kid, a.URL+"/keys")] = true
}

func (a *Azure) Authorization() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.authorization
}

func (a *Azure) serve(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch {
	case r.URL.Path == "/acme-tenant/oauth2/v2.0/token":
		secret := r.PostFormValue("client_secret") == "halo-client-secret" || r.PostFormValue("client_assertion_type") == "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" && r.PostFormValue("client_assertion") == "federated-token"
		if r.PostFormValue("grant_type") != "client_credentials" || r.PostFormValue("client_id") != "halo-app" || !strings.HasSuffix(r.PostFormValue("scope"), "/.default") || !secret {
			azureFail(w, http.StatusUnauthorized, "invalid_client")
			return
		}
		a.grant(w, 3599)
		return
	case r.URL.Path == "/metadata/identity/oauth2/token":
		if r.Header.Get("Metadata") != "true" || r.URL.Query().Get("api-version") == "" || r.URL.Query().Get("resource") == "" {
			azureFail(w, http.StatusBadRequest, "invalid_request")
			return
		}
		a.grant(w, "3599")
		return
	case r.URL.Path == "/msi/token":
		if r.Header.Get("X-IDENTITY-HEADER") != "identity-header" || r.URL.Query().Get("api-version") != "2019-08-01" || r.URL.Query().Get("resource") == "" {
			azureFail(w, http.StatusBadRequest, "invalid_request")
			return
		}
		token := "eyAzure." + id.New("token")
		a.tokens[token] = true
		azureReply(w, map[string]any{"access_token": token, "expires_on": strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10), "token_type": "Bearer"})
		return
	}
	token, bearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !bearer || !a.tokens[token] {
		azureFail(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if r.URL.Query().Get("api-version") != "7.4" {
		azureFail(w, http.StatusBadRequest, "The api-version query parameter is required")
		return
	}
	a.authorization = r.Header.Get("Authorization")
	var in struct {
		Kty, Crv, Alg, Value string
		KeySize              int `json:"key_size"`
	}
	if r.Method == http.MethodPost && json.NewDecoder(r.Body).Decode(&in) != nil {
		azureFail(w, http.StatusBadRequest, "BadParameter")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/keys"), "/")
	kid := strings.Join(parts[:min(len(parts), 3)], "/")
	key := a.keys[kid]
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/keys":
		azureReply(w, map[string]any{"value": []any{}})
	case len(parts) == 3 && parts[2] == "create" && r.Method == http.MethodPost:
		if !azureName.MatchString(parts[1]) {
			azureFail(w, http.StatusBadRequest, "The key name is invalid")
			return
		}
		kind := map[string]string{"RSA": "rsa-2048", "EC": "ecdsa-p256"}[strings.TrimSuffix(in.Kty, "-HSM")]
		if kind == "" || strings.HasSuffix(in.Kty, "-HSM") != a.HSM || kind == "ecdsa-p256" && in.Crv != "P-256" || kind == "rsa-2048" && in.KeySize != 2048 {
			azureFail(w, http.StatusBadRequest, "The key type is not supported")
			return
		}
		version := make([]byte, 16)
		_, _ = rand.Read(version)
		kid = "/" + parts[1] + "/" + hex.EncodeToString(version)
		a.keys[kid] = generate(kind)
		azureReply(w, a.bundle(kid))
	case len(parts) == 4 && kid == "/halo-secret-key/0123456789abcdef0123456789abcdef" && parts[3] == "unwrapkey":
		sealed, _ := base64.RawURLEncoding.DecodeString(in.Value)
		plain, err := rsa.DecryptOAEP(sha256.New(), nil, a.wrapping, sealed, nil)
		if in.Alg != "RSA-OAEP-256" || err != nil {
			azureFail(w, http.StatusBadRequest, "Unwrap failed")
			return
		}
		azureReply(w, map[string]string{"kid": a.URL + "/keys/halo-secret-key/0123456789abcdef0123456789abcdef", "value": base64.RawURLEncoding.EncodeToString(plain)})
	case key == nil:
		azureFail(w, http.StatusNotFound, "A key with (name/id) "+r.URL.Path+" was not found in this key vault")
	case len(parts) == 3 && r.Method == http.MethodGet:
		azureReply(w, a.bundle(kid))
	case len(parts) == 4 && parts[3] == "sign":
		signature, problem := azureSign(key, in.Alg, in.Value)
		if problem != "" {
			azureFail(w, http.StatusBadRequest, problem)
			return
		}
		azureReply(w, map[string]string{"kid": a.URL + "/keys" + kid, "value": base64.RawURLEncoding.EncodeToString(signature)})
	default:
		azureFail(w, http.StatusBadRequest, "BadParameter")
	}
}

func (a *Azure) bundle(kid string) map[string]any {
	jwk := map[string]any{"kid": a.URL + "/keys" + kid, "key_ops": []string{"sign", "verify"}}
	switch public := a.keys[kid].Public().(type) {
	case *rsa.PublicKey:
		jwk["kty"], jwk["n"], jwk["e"] = "RSA", base64.RawURLEncoding.EncodeToString(public.N.Bytes()), base64.RawURLEncoding.EncodeToString(big.NewInt(int64(public.E)).Bytes())
	case *ecdsa.PublicKey:
		point, _ := public.Bytes()
		jwk["kty"], jwk["crv"], jwk["x"], jwk["y"] = "EC", "P-256", base64.RawURLEncoding.EncodeToString(point[1:33]), base64.RawURLEncoding.EncodeToString(point[33:])
	}
	if a.HSM {
		jwk["kty"] = jwk["kty"].(string) + "-HSM"
	}
	return map[string]any{"key": jwk, "attributes": map[string]any{"enabled": true, "exportable": a.exportable[kid], "recoveryLevel": "Recoverable+Purgeable"}}
}

func (a *Azure) grant(w http.ResponseWriter, expires any) {
	token := "eyAzure." + id.New("token")
	a.tokens[token] = true
	azureReply(w, map[string]any{"access_token": token, "expires_in": expires, "token_type": "Bearer"})
}

func azureSign(key crypto.Signer, alg, value string) ([]byte, string) {
	digest, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(digest) != sha256.Size {
		return nil, "The value is not a SHA-256 digest"
	}
	switch k := key.(type) {
	case *rsa.PrivateKey:
		switch alg {
		case "RS256":
			signature, err := rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA256, digest)
			return signature, errorText(err)
		case "PS256":
			signature, err := rsa.SignPSS(rand.Reader, k, crypto.SHA256, digest, nil)
			return signature, errorText(err)
		}
	case *ecdsa.PrivateKey:
		if alg == "ES256" && k.Curve == elliptic.P256() {
			r, s, err := ecdsa.Sign(rand.Reader, k, digest)
			if err != nil {
				return nil, err.Error()
			}
			return append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...), ""
		}
	}
	return nil, "Key and signing algorithm are incompatible"
}

func errorText(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

func azureReply(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func azureFail(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": http.StatusText(status), "message": message}})
}
