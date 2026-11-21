package oidc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"

	"halo/internal/kms"
	"halo/internal/kms/kmstest"
	halooidc "halo/internal/oidc"
	"halo/internal/store"
)

func TestExternalSigningKeys(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	fake := kmstest.New(t)
	h.st.Keys = kms.FromConfig(fake.Config())
	_, sessionToken := h.session(h.user, "passkey")
	signIn := func(alg string) result {
		t.Helper()
		h.app.TokenPolicy.IDTokenAlg = alg
		must(h.st.UpdateApplication(ctx, h.app))
		code := h.silent(sessionToken, url.Values{"scope": {"openid"}}).Query().Get("code")
		return h.post("/oauth2/token", url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callback}, "code_verifier": {pkceSecret}}, h.app.ClientID, h.secret)
	}
	published := func() *jose.JSONWebKeySet {
		t.Helper()
		res, err := http.Get(h.srv.URL + "/oauth2/keys")
		must(err)
		defer res.Body.Close()
		var set jose.JSONWebKeySet
		must(json.NewDecoder(res.Body).Decode(&set))
		return &set
	}
	verify := func(r result, want jose.SignatureAlgorithm) string {
		t.Helper()
		token, _ := r.body["id_token"].(string)
		jws, err := jose.ParseSigned(token, []jose.SignatureAlgorithm{want})
		if r.status != http.StatusOK || err != nil {
			t.Fatalf("%s sign in: %d %v %v", want, r.status, r.body, err)
		}
		kid := jws.Signatures[0].Header.KeyID
		keys := published().Key(kid)
		if len(keys) != 1 || keys[0].Algorithm != string(want) {
			t.Fatalf("%s key %s is not published: %+v", want, kid, keys)
		}
		if _, err := jws.Verify(keys[0].Key); err != nil {
			t.Fatalf("%s ID token does not verify with the published Vault key: %v", want, err)
		}
		return kid
	}

	first := verify(signIn("RS256"), jose.RS256)
	verify(signIn("ES256"), jose.ES256)
	rows, err := h.st.ListSigningKeys(ctx)
	must(err)
	if len(rows) != 2 {
		t.Fatalf("one key per algorithm: %+v", rows)
	}
	for _, row := range rows {
		if row.PrivateKey != nil || !fake.Has(row.ExternalKey) {
			t.Fatalf("key %s must live only in Vault, as %q", row.ID, row.ExternalKey)
		}
	}

	rotated, err := halooidc.CreateSigningKey(ctx, h.st, "RS256")
	must(err)
	if rotated.ExternalKey == "" || !fake.Has(rotated.ExternalKey) {
		t.Fatalf("rotation must create the new key in Vault: %+v", rotated)
	}
	if kid := verify(signIn("RS256"), jose.RS256); kid != rotated.ID || len(published().Key(first)) != 1 {
		t.Fatalf("after rotation, tokens use %s and the previous key %s stays published", kid, first)
	}

	receiver, err := h.st.CreateApplication(ctx, store.NewApplication{Name: "SIEM", Protocol: "oauth", Type: "service"})
	must(err)
	_, receiverSecret, err := h.st.AddClientSecret(ctx, receiver.ID, "Test", time.Hour)
	must(err)
	must(h.st.SaveSignalReceiver(ctx, receiver.ID, "iss_sub"))
	receiverToken, _ := h.post("/oauth2/token", url.Values{"grant_type": {"client_credentials"}}, receiver.ClientID, receiverSecret).body["access_token"].(string)
	ssf := func(path string, body any) *http.Response {
		t.Helper()
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, h.srv.URL+path, bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+receiverToken)
		res, err := http.DefaultClient.Do(req)
		must(err)
		t.Cleanup(func() { res.Body.Close() })
		return res
	}
	var stream struct {
		ID string `json:"stream_id"`
	}
	must(json.NewDecoder(ssf("/oauth2/ssf/stream", map[string]any{"events_requested": []string{"https://schemas.openid.net/secevent/caep/event-type/session-revoked"}}).Body).Decode(&stream))
	if res := ssf("/oauth2/ssf/verify", map[string]any{"stream_id": stream.ID, "state": "vault"}); res.StatusCode != http.StatusNoContent {
		t.Fatalf("verify stream %q: %d", stream.ID, res.StatusCode)
	}
	var polled struct {
		Sets map[string]string `json:"sets"`
	}
	must(json.NewDecoder(ssf("/oauth2/ssf/poll/"+stream.ID, map[string]any{"returnImmediately": true}).Body).Decode(&polled))
	if len(polled.Sets) != 1 {
		t.Fatalf("polled SETs: %v", polled.Sets)
	}
	for _, set := range polled.Sets {
		jws, err := jose.ParseSigned(set, []jose.SignatureAlgorithm{jose.RS256})
		must(err)
		keys := published().Key(jws.Signatures[0].Header.KeyID)
		if len(keys) != 1 {
			t.Fatalf("SET key %s is not published", jws.Signatures[0].Header.KeyID)
		}
		if _, err := jws.Verify(keys[0].Key); err != nil {
			t.Fatalf("SET does not verify with the published Vault key: %v", err)
		}
	}

	fake.Close()
	for _, alg := range []string{"ES256", "EdDSA"} {
		if r := signIn(alg); r.status != http.StatusInternalServerError || r.body["id_token"] != nil || strings.Contains(fmt.Sprint(r.body), fake.URL) {
			t.Fatalf("with Vault down, %s signing must fail without naming Vault to the client: %d %v", alg, r.status, r.body)
		}
	}
	if _, err := halooidc.CreateSigningKey(ctx, h.st, "EdDSA"); err == nil {
		t.Fatal("with Vault down, creating a key must fail")
	}
	after, err := h.st.ListSigningKeys(ctx)
	must(err)
	if len(after) != 3 || slices.ContainsFunc(after, func(k store.SigningKey) bool { return k.ExternalKey == "" }) {
		t.Fatalf("Halo must never fall back to a local key: %+v", after)
	}
	h.st.Keys = kms.FromConfig(fake.Config())
	if err := h.st.CheckExternalKeys(ctx); err == nil {
		t.Fatal("startup must fail while Vault is unreachable")
	}
	h.st.Keys = nil
	if err := h.st.CheckExternalKeys(ctx); err == nil {
		t.Fatal("startup must fail when keys are in Vault but Vault is not configured")
	}
}
