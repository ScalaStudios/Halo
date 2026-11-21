package kms

import (
	"cmp"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"halo/internal/config"
)

var (
	awsKeySpecs = map[string]string{"RS256": "RSA_2048", "ES256": "ECC_NIST_P256", "EdDSA": "ECC_NIST_EDWARDS25519"}
	awsHashes   = map[crypto.Hash]string{crypto.SHA256: "256", crypto.SHA384: "384", crypto.SHA512: "512"}
	awsKeyARN   = regexp.MustCompile(`^arn:aws[a-z-]*:kms:[a-z0-9-]+:[0-9]{12}:key/[A-Za-z0-9-]+$`)
)

type AWS struct {
	cfg         config.AWSKMS
	endpoint    string
	wrappingKey string
	credentials *cached[awsCredentials]
}

type awsCredentials struct {
	AccessKeyID     string `json:"AccessKeyId"`
	SecretAccessKey string
	Token           string
	Code            string
	Expiration      time.Time
}

func NewAWS(cfg config.KMS) *AWS {
	a := &AWS{cfg: cfg.AWS, endpoint: cfg.AWS.Endpoint, wrappingKey: cfg.WrappingKey}
	if a.endpoint == "" {
		a.endpoint = "https://kms." + cfg.AWS.Region + ".amazonaws.com"
		if strings.HasPrefix(cfg.AWS.Region, "cn-") {
			a.endpoint += ".cn"
		}
	}
	a.credentials = &cached[awsCredentials]{fetch: a.fetchCredentials}
	return a
}

func (a *AWS) UnwrapKey(ctx context.Context, wrapped string) ([]byte, error) {
	var out struct{ Plaintext []byte }
	err := a.call(ctx, "decrypt", "Decrypt", map[string]string{"CiphertextBlob": wrapped, "KeyId": a.wrappingKey}, &out)
	return out.Plaintext, err
}

func (a *AWS) CreateKey(ctx context.Context, algorithm string) (string, error) {
	spec, ok := awsKeySpecs[algorithm]
	if !ok {
		return "", fmt.Errorf("AWS KMS has no key spec for %s", algorithm)
	}
	var out struct{ KeyMetadata struct{ Arn string } }
	in := map[string]any{"KeySpec": spec, "KeyUsage": "SIGN_VERIFY", "Description": "Halo signing key", "Tags": []map[string]string{{"TagKey": "halo", "TagValue": "signing-key"}}}
	if err := a.call(ctx, "create", "CreateKey", in, &out); err != nil {
		return "", err
	}
	if !awsKeyARN.MatchString(out.KeyMetadata.Arn) {
		return "", fmt.Errorf("AWS KMS returned %q, which is not a key ARN", out.KeyMetadata.Arn)
	}
	return out.KeyMetadata.Arn, nil
}

func (a *AWS) Signer(ctx context.Context, ref string) (crypto.Signer, error) {
	if !awsKeyARN.MatchString(ref) {
		return nil, fmt.Errorf("%q is not an AWS KMS key ARN", ref)
	}
	var described struct {
		KeyMetadata struct{ KeyUsage, KeyState, Origin string }
	}
	if err := a.call(ctx, "read", "DescribeKey", map[string]string{"KeyId": ref}, &described); err != nil {
		return nil, err
	}
	if m := described.KeyMetadata; m.KeyUsage != "SIGN_VERIFY" || m.Origin == "EXTERNAL" || m.KeyState != "Enabled" {
		return nil, fmt.Errorf("AWS KMS key %s is %s with usage %s and origin %s, so Halo won't sign with it. Halo signs only with enabled SIGN_VERIFY keys whose key material AWS KMS created", ref, m.KeyState, m.KeyUsage, m.Origin)
	}
	var out struct{ PublicKey []byte }
	if err := a.call(ctx, "read", "GetPublicKey", map[string]string{"KeyId": ref}, &out); err != nil {
		return nil, err
	}
	public, err := signingPublicKey(out.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("AWS KMS key %s: %w", ref, err)
	}
	return &signer{public: public, sign: func(digest []byte, hash crypto.Hash) ([]byte, error) {
		return a.sign(ref, public, digest, hash)
	}}, nil
}

func (a *AWS) Ping(ctx context.Context) error {
	return a.call(ctx, "ping", "ListKeys", map[string]int{"Limit": 1}, nil)
}

func (a *AWS) sign(ref string, public crypto.PublicKey, digest []byte, hash crypto.Hash) ([]byte, error) {
	in := map[string]any{"KeyId": ref, "Message": digest, "MessageType": "DIGEST"}
	suffix, ok := awsHashes[hash]
	switch public.(type) {
	case *rsa.PublicKey:
		in["SigningAlgorithm"] = "RSASSA_PKCS1_V1_5_SHA_" + suffix
	case *ecdsa.PublicKey:
		in["SigningAlgorithm"] = "ECDSA_SHA_" + suffix
	case ed25519.PublicKey:
		if len(digest) > 4096 {
			return nil, fmt.Errorf("AWS KMS signs at most 4096 bytes with Ed25519, and this message is %d bytes. Use RS256 or ES256 for it", len(digest))
		}
		in["SigningAlgorithm"], in["MessageType"], ok = "ED25519_SHA_512", "RAW", true
	}
	if !ok {
		return nil, fmt.Errorf("AWS KMS can't sign a %s digest", hash)
	}
	var out struct{ Signature []byte }
	if err := a.call(context.Background(), "sign", "Sign", in, &out); err != nil {
		return nil, err
	}
	if len(out.Signature) == 0 {
		return nil, errors.New("aws kms sign: the response holds no signature")
	}
	return out.Signature, nil
}

func (a *AWS) call(ctx context.Context, operation, action string, in, out any) error {
	return observe(operation, "aws kms", func() error {
		credentials, err := a.credentials.get(ctx)
		if err != nil {
			return err
		}
		req, body, err := newRequest(ctx, http.MethodPost, a.endpoint+"/", in)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-amz-json-1.1")
		req.Header.Set("X-Amz-Target", "TrentService."+action)
		signV4(req, body, credentials, a.cfg.Region, "kms", time.Now())
		return send(req, out)
	})
}

func (a *AWS) fetchCredentials(ctx context.Context) (awsCredentials, time.Time, error) {
	if a.cfg.AccessKeyID != "" {
		return awsCredentials{AccessKeyID: a.cfg.AccessKeyID, SecretAccessKey: a.cfg.SecretAccessKey, Token: a.cfg.SessionToken}, time.Now().Add(time.Hour), nil
	}
	base := cmp.Or(a.cfg.MetadataEndpoint, "http://169.254.169.254") + "/latest/"
	var token, role string
	var out awsCredentials
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, base+"api/token", nil)
	if err != nil {
		return out, time.Time{}, err
	}
	req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")
	if err := send(req, &token); err != nil {
		return out, time.Time{}, fmt.Errorf("the instance metadata service: %w", err)
	}
	get := func(path string, dest any) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"meta-data/iam/security-credentials/"+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-aws-ec2-metadata-token", token)
		return send(req, dest)
	}
	if err := get("", &role); err != nil {
		return out, time.Time{}, fmt.Errorf("the instance role: %w", err)
	}
	role, _, _ = strings.Cut(strings.TrimSpace(role), "\n")
	if role == "" || strings.ContainsAny(role, "/?#") {
		return out, time.Time{}, errors.New("the instance has no IAM role. Attach one, or set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY")
	}
	if err := get(role, &out); err != nil {
		return out, time.Time{}, fmt.Errorf("the instance role %s: %w", role, err)
	}
	if out.Code != "Success" || out.AccessKeyID == "" || out.SecretAccessKey == "" {
		return out, time.Time{}, fmt.Errorf("the instance metadata service returned no credentials for role %s: %s", role, out.Code)
	}
	return out, out.Expiration, nil
}

func signV4(req *http.Request, body []byte, credentials awsCredentials, region, service string, now time.Time) {
	stamp := now.UTC().Format("20060102T150405Z")
	req.Header.Set("X-Amz-Date", stamp)
	if credentials.Token != "" {
		req.Header.Set("X-Amz-Security-Token", credentials.Token)
	}
	names := []string{"host"}
	for name := range req.Header {
		names = append(names, strings.ToLower(name))
	}
	slices.Sort(names)
	var headers strings.Builder
	for _, name := range names {
		value := req.Host
		if name != "host" {
			value = strings.TrimSpace(req.Header.Get(name))
		}
		headers.WriteString(name + ":" + value + "\n")
	}
	signed := strings.Join(names, ";")
	payload := sha256.Sum256(body)
	canonical := sha256.Sum256([]byte(strings.Join([]string{req.Method, cmp.Or(req.URL.EscapedPath(), "/"), strings.ReplaceAll(req.URL.Query().Encode(), "+", "%20"), headers.String(), signed, hex.EncodeToString(payload[:])}, "\n")))
	scope := stamp[:8] + "/" + region + "/" + service + "/aws4_request"
	key := []byte("AWS4" + credentials.SecretAccessKey)
	for _, part := range []string{stamp[:8], region, service, "aws4_request"} {
		key = hmacSHA256(key, part)
	}
	signature := hex.EncodeToString(hmacSHA256(key, "AWS4-HMAC-SHA256\n"+stamp+"\n"+scope+"\n"+hex.EncodeToString(canonical[:])))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+credentials.AccessKeyID+"/"+scope+", SignedHeaders="+signed+", Signature="+signature)
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}
