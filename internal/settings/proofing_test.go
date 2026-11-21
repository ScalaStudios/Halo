package settings

import (
	"context"
	"strings"
	"testing"
)

func TestIdentityProofingSettings(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	security := e.person("sam@example.com", "security_admin")
	for _, body := range []map[string]any{
		{"identityProofingProvider": "acme"},
		{"identityProofingProvider": "generic", "identityProofingName": "Acme", "identityProofingEndpoint": "https://verify.example.com/sessions"},
		{"identityProofingProvider": "generic", "identityProofingName": "Acme", "identityProofingEndpoint": "http://verify.example.com/sessions", "identityProofingApiKey": "key"},
		{"identityProofingProvider": "generic", "identityProofingName": "", "identityProofingEndpoint": "https://verify.example.com/sessions", "identityProofingApiKey": "key"},
		{"identityProofingProvider": "stripe", "identityProofingApiKey": "pk_live_123"},
		{"identityProofingProvider": "stripe", "identityProofingApiKey": "sk_live_123", "identityProofingWebhookSecret": "secret"},
		{"identityProofingGroupIds": []string{"grp_missing"}},
	} {
		e.expect(e.call(security, "PATCH", "/api/v1/settings", body), 422)
	}
	rec := e.expect(e.call(security, "PATCH", "/api/v1/settings", map[string]any{"identityProofingInvites": true, "identityProofingProvider": "stripe", "identityProofingName": "Ignored",
		"identityProofingApiKey": "sk_live_123", "identityProofingWebhookSecret": "whsec_abc"}), 200)
	if body := rec.Body.String(); strings.Contains(body, "sk_live_123") || strings.Contains(body, "whsec_abc") || !strings.Contains(body, `"identityProofingKeySet":true`) || !strings.Contains(body, `"identityProofingWebhookSecretSet":true`) {
		t.Fatalf("response: %s", body)
	}
	got := decode[settingsResponse](t, rec)
	if !got.Settings.IdentityProofingInvites || got.Settings.IdentityProofingProvider != "stripe" || got.Settings.IdentityProofingName != "" {
		t.Fatalf("saved: %+v", got.Settings)
	}
	if key, hook, err := e.st.ProofingSecrets(ctx); err != nil || key != "sk_live_123" || hook != "whsec_abc" {
		t.Fatalf("sealed secrets: %q %q %v", key, hook, err)
	}
	e.expect(e.call(security, "PATCH", "/api/v1/settings", map[string]any{"identityProofingSignUps": true}), 200)
	if key, _, _ := e.st.ProofingSecrets(ctx); key != "sk_live_123" {
		t.Fatalf("saving without a key must keep the stored one: %q", key)
	}
	audit, _ := e.st.ListAudit(ctx, 10)
	for _, a := range audit {
		for _, c := range a.Changes {
			if (c.Field == "identityProofingApiKey" || c.Field == "identityProofingWebhookSecret") && !c.Redacted {
				t.Fatalf("secret in the audit log: %+v", c)
			}
		}
		if strings.Contains(a.Summary, "sk_live_123") {
			t.Fatalf("secret in the audit summary: %s", a.Summary)
		}
	}
}
