package kms

import (
	"bytes"
	"net/http"
	"testing"
	"time"
)

func TestSignV4MatchesBotocore(t *testing.T) {
	at := time.Date(2026, 11, 21, 5, 31, 58, 0, time.UTC)
	for token, want := range map[string]string{
		"":              "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20261121/eu-central-1/kms/aws4_request, SignedHeaders=content-type;host;x-amz-date;x-amz-target, Signature=6a03f4a69e1aa5695b1b47ab5c27ed9738759175facf3ebd26f861ccc3029377",
		"session-token": "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20261121/eu-central-1/kms/aws4_request, SignedHeaders=content-type;host;x-amz-date;x-amz-security-token;x-amz-target, Signature=3b821f6ad35272c017b2aa487f7948bc03e51062865f8356d951934f98f4d764",
	} {
		body := []byte(`{"KeyId":"alias/halo-secret-key"}`)
		req, _ := http.NewRequest(http.MethodPost, "https://kms.eu-central-1.amazonaws.com/", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/x-amz-json-1.1")
		req.Header.Set("X-Amz-Target", "TrentService.Decrypt")
		signV4(req, body, awsCredentials{AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", Token: token}, "eu-central-1", "kms", at)
		if got := req.Header.Get("Authorization"); got != want || req.Header.Get("X-Amz-Date") != "20261121T053158Z" {
			t.Fatalf("token %q:\n got %s\nwant %s", token, got, want)
		}
	}
}
