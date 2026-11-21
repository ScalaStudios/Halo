package kmstest

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"halo/internal/config"
	"halo/internal/id"
)

const KeyRing = "projects/acme/locations/europe-west1/keyRings/halo"

var gcpKinds = map[string]string{"RSA_SIGN_PKCS1_2048_SHA256": "rsa-2048", "EC_SIGN_P256_SHA256": "ecdsa-p256", "EC_SIGN_ED25519": "ed25519"}

type GCP struct {
	*httptest.Server
	HSM           bool
	account       *rsa.PrivateKey
	file          string
	mu            sync.Mutex
	tokens        map[string]bool
	keys          map[string]*gcpKey
	aead          *envelope
	authorization string
}

type gcpKey struct {
	signer    crypto.Signer
	algorithm string
	reads     int
}

func NewGCP(t testing.TB) *GCP {
	t.Helper()
	account, _ := rsa.GenerateKey(rand.Reader, 2048)
	g := &GCP{account: account, tokens: map[string]bool{}, keys: map[string]*gcpKey{}, aead: newEnvelope(), file: filepath.Join(t.TempDir(), "service-account.json")}
	g.Server = httptest.NewServer(http.HandlerFunc(g.serve))
	t.Cleanup(g.Close)
	der, _ := x509.MarshalPKCS8PrivateKey(account)
	data, _ := json.Marshal(map[string]string{"type": "service_account", "client_email": "halo@acme.iam.gserviceaccount.com", "private_key_id": "key-1",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), "token_uri": g.URL + "/token"})
	if err := os.WriteFile(g.file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return g
}

func (g *GCP) Config() config.KMS {
	return config.KMS{GCP: config.GCPKMS{KeyRing: KeyRing, Endpoint: g.URL, HSM: g.HSM, CredentialsFile: g.file}, WrappingKey: "halo-secret-key", Signing: true}
}

func (g *GCP) MetadataConfig() config.KMS {
	cfg := g.Config()
	cfg.GCP.CredentialsFile, cfg.GCP.MetadataHost = "", strings.TrimPrefix(g.URL, "http://")
	return cfg
}

func (g *GCP) Wrap(plain []byte) string {
	return g.aead.seal(plain)
}

func (g *GCP) Has(ref string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.keys[strings.TrimSuffix(ref, "/cryptoKeyVersions/1")] != nil
}

func (g *GCP) Authorization() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.authorization
}

func (g *GCP) serve(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	switch {
	case r.URL.Path == "/token":
		g.issue(w, r)
		return
	case r.URL.Path == "/computeMetadata/v1/instance/service-accounts/default/token":
		if r.Header.Get("Metadata-Flavor") != "Google" {
			gcpFail(w, http.StatusForbidden, "Missing Metadata-Flavor:Google header")
			return
		}
		g.grant(w)
		return
	}
	token, bearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !bearer || !g.tokens[token] {
		gcpFail(w, http.StatusUnauthorized, "Request had invalid authentication credentials")
		return
	}
	g.authorization = r.Header.Get("Authorization")
	var in struct {
		Purpose         string
		VersionTemplate struct{ Algorithm, ProtectionLevel string }
		Ciphertext      []byte
		Data            []byte
		Digest          *struct{ SHA256 []byte }
	}
	if r.Method == http.MethodPost && json.NewDecoder(r.Body).Decode(&in) != nil {
		gcpFail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/")
	name, method, _ := strings.Cut(path, ":")
	key := g.keys[strings.TrimSuffix(strings.TrimSuffix(name, "/publicKey"), "/cryptoKeyVersions/1")]
	switch {
	case r.Method == http.MethodGet && path == KeyRing:
		gcpReply(w, map[string]any{"name": KeyRing})
	case r.Method == http.MethodPost && path == KeyRing+"/cryptoKeys" && r.URL.Query().Get("cryptoKeyId") != "":
		kind := gcpKinds[in.VersionTemplate.Algorithm]
		if in.Purpose != "ASYMMETRIC_SIGN" || kind == "" || in.VersionTemplate.ProtectionLevel == "HSM" && kind == "ed25519" {
			gcpFail(w, http.StatusBadRequest, "The specified algorithm is not supported at the requested protection level")
			return
		}
		g.keys[path+"/"+r.URL.Query().Get("cryptoKeyId")] = &gcpKey{signer: generate(kind), algorithm: in.VersionTemplate.Algorithm}
		gcpReply(w, map[string]any{"name": path + "/" + r.URL.Query().Get("cryptoKeyId"), "purpose": in.Purpose})
	case method == "decrypt" && name == KeyRing+"/cryptoKeys/halo-secret-key":
		plain, err := g.aead.open(base64.StdEncoding.EncodeToString(in.Ciphertext))
		if err != nil {
			gcpFail(w, http.StatusBadRequest, "Decryption failed: the ciphertext is invalid.")
			return
		}
		gcpReply(w, map[string]any{"plaintext": plain})
	case key == nil:
		gcpFail(w, http.StatusNotFound, name+" not found")
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/publicKey"):
		der, _ := x509.MarshalPKIXPublicKey(key.signer.Public())
		gcpReply(w, map[string]any{"pem": string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), "algorithm": key.algorithm})
	case r.Method == http.MethodGet:
		state := "ENABLED"
		if key.reads++; key.reads == 1 {
			state = "PENDING_GENERATION"
		}
		gcpReply(w, map[string]any{"name": path, "state": state, "algorithm": key.algorithm, "protectionLevel": "SOFTWARE"})
	case method == "asymmetricSign":
		signature, problem := gcpSign(key, in.Data, in.Digest)
		if problem != "" {
			gcpFail(w, http.StatusBadRequest, problem)
			return
		}
		gcpReply(w, map[string]any{"signature": signature, "name": name})
	default:
		gcpFail(w, http.StatusNotFound, "no such method")
	}
}

func (g *GCP) issue(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.PostFormValue("assertion"), ".")
	if r.PostFormValue("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || len(parts) != 3 {
		gcpFail(w, http.StatusBadRequest, "invalid_grant")
		return
	}
	signature, _ := base64.RawURLEncoding.DecodeString(parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	var header struct{ Alg, Kid string }
	var claims struct {
		Iss, Scope, Aud string
		Exp             int64
	}
	rawHeader, _ := base64.RawURLEncoding.DecodeString(parts[0])
	rawClaims, _ := base64.RawURLEncoding.DecodeString(parts[1])
	if rsa.VerifyPKCS1v15(&g.account.PublicKey, crypto.SHA256, digest[:], signature) != nil || json.Unmarshal(rawHeader, &header) != nil || json.Unmarshal(rawClaims, &claims) != nil ||
		header.Alg != "RS256" || header.Kid != "key-1" || claims.Iss != "halo@acme.iam.gserviceaccount.com" || claims.Aud != g.URL+"/token" || claims.Scope != "https://www.googleapis.com/auth/cloudkms" || claims.Exp < time.Now().Unix() {
		gcpFail(w, http.StatusBadRequest, "invalid_grant: Invalid JWT Signature.")
		return
	}
	g.grant(w)
}

func (g *GCP) grant(w http.ResponseWriter) {
	token := "ya29." + id.New("token")
	g.tokens[token] = true
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": token, "expires_in": 3599, "token_type": "Bearer"})
}

func gcpSign(key *gcpKey, data []byte, digest *struct{ SHA256 []byte }) ([]byte, string) {
	if k, ok := key.signer.(ed25519.PrivateKey); ok {
		if digest != nil {
			return nil, "EC_SIGN_ED25519 signs data, not a digest"
		}
		return ed25519.Sign(k, data), ""
	}
	if digest == nil || len(digest.SHA256) != sha256.Size || data != nil {
		return nil, "The request must contain a SHA-256 digest"
	}
	var signature []byte
	var err error
	switch k := key.signer.(type) {
	case *rsa.PrivateKey:
		signature, err = rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA256, digest.SHA256)
	case *ecdsa.PrivateKey:
		signature, err = ecdsa.SignASN1(rand.Reader, k, digest.SHA256)
	}
	if err != nil {
		return nil, err.Error()
	}
	return signature, ""
}

func gcpReply(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func gcpFail(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": status, "message": message, "status": http.StatusText(status)}})
}
