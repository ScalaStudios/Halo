package kmstest

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"halo/internal/config"
)

var hashes = map[string]crypto.Hash{"sha2-256": crypto.SHA256, "sha2-384": crypto.SHA384, "sha2-512": crypto.SHA512}

type Vault struct {
	*httptest.Server
	Token      string
	mu         sync.Mutex
	keys       map[string]crypto.Signer
	exportable map[string]bool
	aead       *envelope
	ttl        time.Duration
	expires    time.Time
	renewals   int
}

func New(t testing.TB) *Vault {
	t.Helper()
	v := &Vault{Token: "test-token", keys: map[string]crypto.Signer{}, exportable: map[string]bool{}, aead: newEnvelope()}
	v.Server = httptest.NewServer(http.HandlerFunc(v.serve))
	t.Cleanup(v.Close)
	return v
}

func (v *Vault) Config() config.KMS {
	return config.KMS{Vault: config.Vault{Address: v.URL, Token: v.Token, Mount: "transit"}, WrappingKey: "halo-secret-key", Signing: true}
}

func (v *Vault) Wrap(plain []byte) string {
	return "vault:v1:" + v.aead.seal(plain)
}

func (v *Vault) Has(name string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.keys[name] != nil
}

func (v *Vault) ExpireToken(ttl time.Duration) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.ttl, v.expires = ttl, time.Now().Add(ttl)
}

func (v *Vault) Renewals() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.renewals
}

func (v *Vault) AllowExport(name string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.exportable[name] = true
}

func (v *Vault) serve(w http.ResponseWriter, r *http.Request) {
	v.mu.Lock()
	expired := v.ttl > 0 && time.Now().After(v.expires)
	ttl := 0
	if v.ttl > 0 {
		ttl = max(int(time.Until(v.expires).Round(time.Second).Seconds()), 0)
	}
	v.mu.Unlock()
	if r.Header.Get("X-Vault-Token") != v.Token || expired {
		fail(w, http.StatusForbidden, "permission denied")
		return
	}
	var in struct {
		Type, Input, Ciphertext string
		Prehashed               bool
		SignatureAlgorithm      string `json:"signature_algorithm"`
		MarshalingAlgorithm     string `json:"marshaling_algorithm"`
		KeyVersion              int    `json:"key_version"`
	}
	if r.Method == http.MethodPost && json.NewDecoder(r.Body).Decode(&in) != nil {
		fail(w, http.StatusBadRequest, "failed to parse JSON input")
		return
	}
	path := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/"), "/")
	if r.Method == http.MethodGet && strings.Join(path, "/") == "auth/token/lookup-self" {
		reply(w, map[string]any{"id": v.Token, "ttl": ttl, "renewable": ttl > 0})
		return
	}
	if r.Method == http.MethodPost && strings.Join(path, "/") == "auth/token/renew-self" {
		v.mu.Lock()
		v.expires = time.Now().Add(v.ttl)
		v.renewals++
		lease := int(v.ttl.Seconds())
		v.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"auth": map[string]any{"lease_duration": lease, "renewable": true}})
		return
	}
	if len(path) < 3 || path[0] != "transit" {
		fail(w, http.StatusNotFound, "no handler for route")
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	name, key := path[2], v.keys[path[2]]
	switch {
	case path[1] == "keys" && len(path) == 3 && r.Method == http.MethodPost:
		if key == nil {
			if key = generate(in.Type); key == nil {
				fail(w, http.StatusBadRequest, "unknown key type "+in.Type)
				return
			}
			v.keys[name] = key
		}
		reply(w, map[string]any{"name": name})
	case path[1] == "keys" && len(path) == 3 && r.Method == http.MethodGet && key != nil:
		reply(w, map[string]any{"type": kind(key), "exportable": v.exportable[name], "allow_plaintext_backup": false, "latest_version": 1,
			"keys": map[string]any{"1": map[string]any{"public_key": public(key)}}})
	case path[1] == "sign" && len(path) <= 4 && r.Method == http.MethodPost && key != nil:
		signature, problem := sign(key, path, in.Input, in.Prehashed, in.SignatureAlgorithm, in.MarshalingAlgorithm, in.KeyVersion)
		if problem != "" {
			fail(w, http.StatusBadRequest, problem)
			return
		}
		reply(w, map[string]any{"signature": "vault:v1:" + base64.StdEncoding.EncodeToString(signature), "key_version": 1})
	case path[1] == "decrypt" && len(path) == 3 && r.Method == http.MethodPost && name == "halo-secret-key":
		plain, err := v.aead.open(strings.TrimPrefix(in.Ciphertext, "vault:v1:"))
		if err != nil || !strings.HasPrefix(in.Ciphertext, "vault:v1:") {
			fail(w, http.StatusBadRequest, "invalid ciphertext")
			return
		}
		reply(w, map[string]any{"plaintext": base64.StdEncoding.EncodeToString(plain)})
	default:
		fail(w, http.StatusBadRequest, "signing key not found")
	}
}

func sign(key crypto.Signer, path []string, input string, prehashed bool, scheme, marshaling string, version int) ([]byte, string) {
	data, err := base64.StdEncoding.DecodeString(input)
	if err != nil || version > 1 {
		return nil, "invalid request"
	}
	if k, ok := key.(ed25519.PrivateKey); ok {
		if prehashed {
			return nil, "only Pure Ed25519 signatures are supported"
		}
		return ed25519.Sign(k, data), ""
	}
	name := "sha2-256"
	if len(path) == 4 {
		name = path[3]
	}
	hash, ok := hashes[name]
	if !ok {
		return nil, "unsupported hash algorithm " + name
	}
	digest := data
	if !prehashed {
		h := hash.New()
		h.Write(data)
		digest = h.Sum(nil)
	}
	if len(digest) != hash.Size() {
		return nil, "input is not a digest of the hash algorithm"
	}
	var signature []byte
	switch k := key.(type) {
	case *rsa.PrivateKey:
		switch scheme {
		case "", "pss":
			signature, err = rsa.SignPSS(rand.Reader, k, hash, digest, nil)
		case "pkcs1v15":
			signature, err = rsa.SignPKCS1v15(rand.Reader, k, hash, digest)
		default:
			return nil, "unsupported signature algorithm " + scheme
		}
	case *ecdsa.PrivateKey:
		if marshaling != "" && marshaling != "asn1" {
			return nil, "unsupported marshaling algorithm " + marshaling
		}
		signature, err = ecdsa.SignASN1(rand.Reader, k, digest)
	}
	if err != nil {
		return nil, err.Error()
	}
	return signature, ""
}

func generate(kind string) crypto.Signer {
	switch kind {
	case "rsa-2048":
		k, _ := rsa.GenerateKey(rand.Reader, 2048)
		return k
	case "ecdsa-p256":
		k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		return k
	case "ed25519":
		_, k, _ := ed25519.GenerateKey(rand.Reader)
		return k
	}
	return nil
}

func kind(key crypto.Signer) string {
	switch key.(type) {
	case *rsa.PrivateKey:
		return "rsa-2048"
	case *ecdsa.PrivateKey:
		return "ecdsa-p256"
	}
	return "ed25519"
}

func public(key crypto.Signer) string {
	if k, ok := key.Public().(ed25519.PublicKey); ok {
		return base64.StdEncoding.EncodeToString(k)
	}
	der, _ := x509.MarshalPKIXPublicKey(key.Public())
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

type envelope struct{ cipher.AEAD }

func newEnvelope() *envelope {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	block, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(block)
	return &envelope{aead}
}

func (e *envelope) seal(plain []byte) string {
	nonce := make([]byte, e.NonceSize())
	_, _ = rand.Read(nonce)
	return base64.StdEncoding.EncodeToString(e.Seal(nonce, nonce, plain, nil))
}

func (e *envelope) open(sealed string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil || len(raw) < e.NonceSize() {
		return nil, errors.New("invalid ciphertext")
	}
	return e.Open(nil, raw[:e.NonceSize()], raw[e.NonceSize():], nil)
}

func reply(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func fail(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"errors": []string{message}})
}
