package api_test

import (
	"context"
	"crypto"
	"crypto/x509"
	"net/http"
	"strings"
	"testing"

	"halo/internal/kms"
	"halo/internal/kms/kmstest"
	"halo/internal/store"
)

func TestExternalKeyRotation(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	fake := kmstest.New(t)
	st.Keys = kms.FromConfig(fake.Config())
	_, global := person(t, st, "gia@example.com", "global_admin")
	local, err := st.CreateSigningKey(ctx, "RS256", []byte("unused"), "")
	if err != nil {
		t.Fatal(err)
	}

	rec := call(t, h, global, "POST", "/api/v1/signing-keys/rotate", nil)
	expect(t, rec, 200)
	oidc := decode[signingKeys](t, rec).OIDC
	if len(oidc) != 2 || oidc[0].ExternalKey == nil || !fake.Has(*oidc[0].ExternalKey) || oidc[1].ID != local.ID || oidc[1].ExternalKey != nil {
		t.Fatalf("rotation must create the new key in Vault and keep the local one until it retires: %+v", oidc)
	}

	rec = call(t, h, global, "POST", "/api/v1/saml/certificate/rotate", nil)
	expect(t, rec, 200)
	saml := decode[struct {
		SAML []struct {
			ExternalKey *string `json:"externalKey"`
		} `json:"saml"`
	}](t, rec).SAML
	active, err := st.ActiveSAMLKey(ctx)
	if err != nil || len(saml) != 1 || saml[0].ExternalKey == nil || *saml[0].ExternalKey != active.ExternalKey || active.PrivateKey != nil {
		t.Fatalf("the SAML key must live in Vault: %+v %+v %v", saml, active, err)
	}
	cert, err := x509.ParseCertificate(active.Certificate)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := st.Keys.Signer(ctx, active.ExternalKey)
	if err != nil || cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature) != nil || !signer.Public().(interface{ Equal(crypto.PublicKey) bool }).Equal(cert.PublicKey) {
		t.Fatalf("the SAML certificate must be self-signed by the Vault key: %v", err)
	}
}

func TestAlgorithmsTheKeyManagerCantSign(t *testing.T) {
	st, h := registrationStack(t, "open")
	ctx := context.Background()
	st.Keys = kms.FromConfig(kmstest.NewAzure(t).Config())
	_, admin := person(t, st, "gia@example.com", "global_admin")
	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://grafana.example.com/login"}})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/applications/" + app.ID
	rec := call(t, h, admin, "PATCH", path, map[string]any{"tokenPolicy": map[string]any{"idTokenAlg": "EdDSA"}})
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "Azure Key Vault can't sign with EdDSA. Choose RS256 or ES256.") {
		t.Fatalf("EdDSA ID tokens must be refused: %d %s", rec.Code, rec.Body)
	}
	expect(t, call(t, h, admin, "PATCH", path, map[string]any{"tokenPolicy": map[string]any{"idTokenAlg": "ES256"}}), 200)
	oauthError(t, oauth(t, h, http.MethodPost, "/oauth2/register", "", map[string]any{"redirect_uris": []string{toolCallback}, "id_token_signed_response_alg": "EdDSA"}), http.StatusBadRequest, "invalid_client_metadata")
	if err := st.CheckExternalKeys(ctx); err != nil {
		t.Fatal(err)
	}

	st.Keys = nil
	expect(t, call(t, h, admin, "PATCH", path, map[string]any{"tokenPolicy": map[string]any{"idTokenAlg": "EdDSA"}}), 200)
	st.Keys = kms.FromConfig(kmstest.NewAzure(t).Config())
	if err := st.CheckExternalKeys(ctx); err == nil || !strings.Contains(err.Error(), "signs tokens with EdDSA") {
		t.Fatalf("Halo must not start while an application signs with an algorithm the key manager can't: %v", err)
	}
}
