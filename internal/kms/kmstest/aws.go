package kmstest

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"halo/internal/config"
	"halo/internal/id"
)

var (
	awsAuthorization = regexp.MustCompile(`^AWS4-HMAC-SHA256 Credential=([^/]+)/([0-9]{8})/([a-z0-9-]+)/kms/aws4_request, SignedHeaders=([a-z0-9;-]+), Signature=([0-9a-f]{64})$`)
	awsSpecs         = map[string]string{"RSA_2048": "rsa-2048", "ECC_NIST_P256": "ecdsa-p256", "ECC_NIST_EDWARDS25519": "ed25519"}
	awsHashes        = map[string]crypto.Hash{"256": crypto.SHA256, "384": crypto.SHA384, "512": crypto.SHA512}
)

type AWS struct {
	*httptest.Server
	Metadata        *httptest.Server
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	mu              sync.Mutex
	keys            map[string]crypto.Signer
	imported        map[string]bool
	aead            *envelope
	sessionToken    string
	authorization   string
}

func NewAWS(t testing.TB) *AWS {
	t.Helper()
	a := &AWS{Region: "eu-central-1", AccessKeyID: "AKIDHALOTEST", SecretAccessKey: "halo-test-secret", keys: map[string]crypto.Signer{}, imported: map[string]bool{}, aead: newEnvelope()}
	a.Server = httptest.NewServer(http.HandlerFunc(a.serve))
	a.Metadata = httptest.NewServer(http.HandlerFunc(a.serveMetadata))
	t.Cleanup(a.Close)
	return a
}

func (a *AWS) Config() config.KMS {
	return config.KMS{AWS: config.AWSKMS{Region: a.Region, Endpoint: a.URL, AccessKeyID: a.AccessKeyID, SecretAccessKey: a.SecretAccessKey}, WrappingKey: "alias/halo-secret-key", Signing: true}
}

func (a *AWS) InstanceRoleConfig() config.KMS {
	cfg := a.Config()
	cfg.AWS.AccessKeyID, cfg.AWS.SecretAccessKey, cfg.AWS.MetadataEndpoint = "", "", a.Metadata.URL
	return cfg
}

func (a *AWS) Close() {
	a.Server.Close()
	a.Metadata.Close()
}

func (a *AWS) Wrap(plain []byte) string {
	return a.aead.seal(plain)
}

func (a *AWS) Has(ref string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.keys[ref] != nil
}

func (a *AWS) Import(ref string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.imported[ref] = true
}

func (a *AWS) Authorization() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.authorization
}

func (a *AWS) serveMetadata(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch {
	case r.Method == http.MethodPut && r.URL.Path == "/latest/api/token" && r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds") != "":
		_, _ = io.WriteString(w, "imds-token")
	case r.Header.Get("X-aws-ec2-metadata-token") != "imds-token":
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	case r.URL.Path == "/latest/meta-data/iam/security-credentials/":
		_, _ = io.WriteString(w, "halo-server\n")
	case r.URL.Path == "/latest/meta-data/iam/security-credentials/halo-server":
		a.sessionToken = "session-" + id.New("token")
		_ = json.NewEncoder(w).Encode(map[string]any{"Code": "Success", "Type": "AWS-HMAC", "AccessKeyId": "ASIAHALOROLE", "SecretAccessKey": "role-secret", "Token": a.sessionToken, "Expiration": time.Now().Add(6 * time.Hour).UTC().Format(time.RFC3339)})
	default:
		http.NotFound(w, r)
	}
}

func (a *AWS) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	a.mu.Lock()
	defer a.mu.Unlock()
	if problem := a.verify(r, body); problem != "" {
		awsFail(w, http.StatusBadRequest, "InvalidSignatureException", problem)
		return
	}
	a.authorization = r.Header.Get("Authorization")
	var in struct {
		KeyID            string `json:"KeyId"`
		KeySpec          string
		KeyUsage         string
		CiphertextBlob   []byte
		Message          []byte
		MessageType      string
		SigningAlgorithm string
	}
	if json.Unmarshal(body, &in) != nil {
		awsFail(w, http.StatusBadRequest, "SerializationException", "invalid JSON")
		return
	}
	key := a.keys[in.KeyID]
	switch action := strings.TrimPrefix(r.Header.Get("X-Amz-Target"), "TrentService."); {
	case action == "Decrypt" && in.KeyID == "alias/halo-secret-key":
		plain, err := a.aead.open(base64.StdEncoding.EncodeToString(in.CiphertextBlob))
		if err != nil {
			awsFail(w, http.StatusBadRequest, "InvalidCiphertextException", "")
			return
		}
		awsReply(w, map[string]any{"Plaintext": plain, "KeyId": "arn:aws:kms:" + a.Region + ":111122223333:key/wrapping"})
	case action == "CreateKey":
		kind, ok := awsSpecs[in.KeySpec]
		if !ok || in.KeyUsage != "SIGN_VERIFY" {
			awsFail(w, http.StatusBadRequest, "ValidationException", "unsupported key spec "+in.KeySpec)
			return
		}
		arn := "arn:aws:kms:" + a.Region + ":111122223333:key/" + strings.ToLower(strings.TrimPrefix(id.New("k"), "k_"))
		a.keys[arn] = generate(kind)
		awsReply(w, map[string]any{"KeyMetadata": map[string]any{"Arn": arn, "KeyId": arn, "KeySpec": in.KeySpec}})
	case key == nil:
		if action == "ListKeys" {
			awsReply(w, map[string]any{"Keys": []any{}})
			return
		}
		awsFail(w, http.StatusBadRequest, "NotFoundException", "key "+in.KeyID+" does not exist")
	case action == "DescribeKey":
		origin := "AWS_KMS"
		if a.imported[in.KeyID] {
			origin = "EXTERNAL"
		}
		awsReply(w, map[string]any{"KeyMetadata": map[string]any{"Arn": in.KeyID, "KeyState": "Enabled", "KeyUsage": "SIGN_VERIFY", "Origin": origin}})
	case action == "GetPublicKey":
		der, _ := x509.MarshalPKIXPublicKey(key.Public())
		awsReply(w, map[string]any{"KeyId": in.KeyID, "PublicKey": der, "KeyUsage": "SIGN_VERIFY"})
	case action == "Sign":
		signature, problem := awsSign(key, in.SigningAlgorithm, in.MessageType, in.Message)
		if problem != "" {
			awsFail(w, http.StatusBadRequest, "ValidationException", problem)
			return
		}
		awsReply(w, map[string]any{"KeyId": in.KeyID, "Signature": signature, "SigningAlgorithm": in.SigningAlgorithm})
	default:
		awsFail(w, http.StatusBadRequest, "UnknownOperationException", action)
	}
}

func (a *AWS) verify(r *http.Request, body []byte) string {
	match := awsAuthorization.FindStringSubmatch(r.Header.Get("Authorization"))
	if match == nil || r.Method != http.MethodPost || r.URL.Path != "/" || r.Header.Get("Content-Type") != "application/x-amz-json-1.1" {
		return "the request is not a signed AWS JSON request"
	}
	secret, token := a.SecretAccessKey, ""
	if match[1] == "ASIAHALOROLE" {
		secret, token = "role-secret", a.sessionToken
	} else if match[1] != a.AccessKeyID {
		return "unknown access key " + match[1]
	}
	stamp := r.Header.Get("X-Amz-Date")
	signed := strings.Split(match[4], ";")
	required := []string{"content-type", "host", "x-amz-date", "x-amz-target"}
	if token != "" {
		required = append(required, "x-amz-security-token")
	}
	if !strings.HasPrefix(stamp, match[2]) || match[3] != a.Region || r.Header.Get("X-Amz-Security-Token") != token || !slices.IsSorted(signed) || slices.ContainsFunc(required, func(h string) bool { return !slices.Contains(signed, h) }) {
		return "the credential scope, date, session token or signed headers do not match"
	}
	if when, err := time.Parse("20060102T150405Z", stamp); err != nil || time.Since(when).Abs() > 5*time.Minute {
		return "the signature is not current"
	}
	var headers strings.Builder
	for _, name := range signed {
		value := r.Header.Get(name)
		if name == "host" {
			value = r.Host
		}
		headers.WriteString(name + ":" + strings.TrimSpace(value) + "\n")
	}
	payload := sha256.Sum256(body)
	request := sha256.Sum256([]byte("POST\n/\n\n" + headers.String() + "\n" + match[4] + "\n" + hex.EncodeToString(payload[:])))
	key := []byte("AWS4" + secret)
	for _, part := range []string{match[2], a.Region, "kms", "aws4_request"} {
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(part))
		key = mac.Sum(nil)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("AWS4-HMAC-SHA256\n" + stamp + "\n" + match[2] + "/" + a.Region + "/kms/aws4_request\n" + hex.EncodeToString(request[:])))
	if !hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(match[5])) {
		return "The request signature we calculated does not match the signature you provided"
	}
	return ""
}

func awsSign(key crypto.Signer, algorithm, messageType string, message []byte) ([]byte, string) {
	if k, ok := key.(ed25519.PrivateKey); ok {
		if algorithm != "ED25519_SHA_512" || messageType != "RAW" || len(message) > 4096 {
			return nil, "ED25519_SHA_512 needs MessageType RAW and at most 4096 bytes"
		}
		return ed25519.Sign(k, message), ""
	}
	scheme, size, _ := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(algorithm, "RSASSA_"), "ECDSA_"), "SHA_")
	hash, ok := awsHashes[size]
	if !ok || messageType != "DIGEST" || len(message) != hash.Size() {
		return nil, "the message is not a " + algorithm + " digest"
	}
	var signature []byte
	var err error
	switch k := key.(type) {
	case *rsa.PrivateKey:
		switch scheme {
		case "PKCS1_V1_5_":
			signature, err = rsa.SignPKCS1v15(rand.Reader, k, hash, message)
		case "PSS_":
			signature, err = rsa.SignPSS(rand.Reader, k, hash, message, nil)
		default:
			return nil, "invalid signing algorithm " + algorithm + " for an RSA key"
		}
	case *ecdsa.PrivateKey:
		if !strings.HasPrefix(algorithm, "ECDSA_") || hash != crypto.SHA256 {
			return nil, "invalid signing algorithm " + algorithm + " for an ECC_NIST_P256 key"
		}
		signature, err = ecdsa.SignASN1(rand.Reader, k, message)
	}
	if err != nil {
		return nil, err.Error()
	}
	return signature, ""
}

func awsReply(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	_ = json.NewEncoder(w).Encode(data)
}

func awsFail(w http.ResponseWriter, status int, kind, message string) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": kind, "message": message})
}
