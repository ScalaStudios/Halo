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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"halo/internal/config"
)

var hashes = map[string]crypto.Hash{"sha2-256": crypto.SHA256, "sha2-384": crypto.SHA384, "sha2-512": crypto.SHA512}

type Vault struct {
	*httptest.Server
	Token      string
	mu         sync.Mutex
	keys       map[string]crypto.Signer
	exportable map[string]bool
	aead       cipher.AEAD
}

func New(t testing.TB) *Vault {
	t.Helper()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	block, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(block)
	v := &Vault{Token: "test-token", keys: map[string]crypto.Signer{}, exportable: map[string]bool{}, aead: aead}
	v.Server = httptest.NewServer(http.HandlerFunc(v.serve))
	t.Cleanup(v.Close)
	return v
}

func (v *Vault) Config() config.Vault {
	return config.Vault{Address: v.URL, Token: v.Token, Mount: "transit", WrappingKey: "halo-secret-key", Signing: true}
}

func (v *Vault) Wrap(plain []byte) string {
	nonce := make([]byte, v.aead.NonceSize())
	_, _ = rand.Read(nonce)
	return "vault:v1:" + base64.StdEncoding.EncodeToString(v.aead.Seal(nonce, nonce, plain, nil))
}

func (v *Vault) Has(name string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.keys[name] != nil
}

func (v *Vault) AllowExport(name string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.exportable[name] = true
}

func (v *Vault) serve(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Vault-Token") != v.Token {
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
		reply(w, map[string]any{"id": v.Token})
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
		sealed, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(in.Ciphertext, "vault:v1:"))
		size := v.aead.NonceSize()
		if err != nil || len(sealed) < size || !strings.HasPrefix(in.Ciphertext, "vault:v1:") {
			fail(w, http.StatusBadRequest, "invalid ciphertext")
			return
		}
		plain, err := v.aead.Open(nil, sealed[:size], sealed[size:], nil)
		if err != nil {
			fail(w, http.StatusBadRequest, "cipher: message authentication failed")
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

func reply(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func fail(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"errors": []string{message}})
}
