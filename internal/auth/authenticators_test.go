package auth_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"
	"github.com/go-webauthn/webauthn/metadata"
	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/google/uuid"

	"halo/internal/auth"
	"halo/internal/store"
)

type attester struct {
	aaguid uuid.UUID
	root   *x509.Certificate
	cert   *x509.Certificate
	key    *ecdsa.PrivateKey
}

func newAttester(t *testing.T, aaguid uuid.UUID) attester {
	t.Helper()
	a := attester{aaguid: aaguid}
	rootKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	a.key, _ = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	validity := func(c *x509.Certificate) *x509.Certificate {
		c.NotBefore, c.NotAfter, c.BasicConstraintsValid = time.Now().Add(-time.Hour), time.Now().Add(time.Hour), true
		return c
	}
	rootTemplate := validity(&x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Halo Test Root"}, IsCA: true, KeyUsage: x509.KeyUsageCertSign})
	der, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	a.root, _ = x509.ParseCertificate(der)
	extension, _ := asn1.Marshal(a.aaguid[:])
	template := validity(&x509.Certificate{
		SerialNumber:    big.NewInt(2),
		Subject:         pkix.Name{Country: []string{"US"}, Organization: []string{"Halo Test"}, OrganizationalUnit: []string{"Authenticator Attestation"}, CommonName: "Halo Test Key"},
		ExtraExtensions: []pkix.Extension{{Id: asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 45724, 1, 1, 4}, Value: extension}},
	})
	if der, err = x509.CreateCertificate(rand.Reader, template, a.root, &a.key.PublicKey, rootKey); err != nil {
		t.Fatal(err)
	}
	a.cert, _ = x509.ParseCertificate(der)
	return a
}

func (a attester) entry() metadata.Entry {
	return metadata.Entry{AaGUID: a.aaguid, MetadataStatement: metadata.Statement{
		AaGUID:                      a.aaguid,
		AttestationTypes:            metadata.AuthenticatorAttestationTypes{metadata.BasicFull},
		AttestationRootCertificates: []*x509.Certificate{a.root},
	}}
}

func (a attester) device(opts virtualwebauthn.AuthenticatorOptions) *device {
	d := &device{auth: virtualwebauthn.NewAuthenticatorWithOptions(opts), cred: virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)}
	d.auth.Aaguid = a.aaguid
	return d
}

func (a attester) sign(t *testing.T, response string) string {
	t.Helper()
	var outer map[string]json.RawMessage
	var inner map[string]any
	if err := json.Unmarshal([]byte(response), &outer); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(outer["response"], &inner); err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(inner["attestationObject"].(string))
	clientData, _ := base64.RawURLEncoding.DecodeString(inner["clientDataJSON"].(string))
	var object struct {
		AuthData []byte `cbor:"authData"`
	}
	if err := webauthncbor.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	clientHash := sha256.Sum256(clientData)
	digest := sha256.Sum256(append(object.AuthData, clientHash[:]...))
	sig, _ := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	signed, err := webauthncbor.Marshal(map[string]any{
		"fmt":      "packed",
		"authData": object.AuthData,
		"attStmt":  map[string]any{"alg": -7, "sig": sig, "x5c": [][]byte{a.cert.Raw}},
	})
	if err != nil {
		t.Fatal(err)
	}
	inner["attestationObject"] = base64.RawURLEncoding.EncodeToString(signed)
	outer["response"], _ = json.Marshal(inner)
	out, _ := json.Marshal(outer)
	return string(out)
}

func trusting(t *testing.T, attesters ...attester) {
	t.Helper()
	previous := *auth.FetchMetadata
	entries := []metadata.Entry{}
	for _, a := range attesters {
		entries = append(entries, a.entry())
	}
	*auth.FetchMetadata = func() (*metadata.Metadata, error) {
		return &metadata.Metadata{Parsed: metadata.Parsed{Entries: entries}}, nil
	}
	t.Cleanup(func() { *auth.FetchMetadata = previous })
}

type addResult struct {
	status      int
	code        string
	message     string
	attestation string
}

func (e *env) addKey(c *http.Client, d *device, sign func(string) string) addResult {
	e.t.Helper()
	return e.registerKey(c, "/api/v1/me/passkeys", map[string]any{"kind": "passkey"}, map[string]any{"label": "Key"}, d, sign)
}

func (e *env) registerKey(c *http.Client, prefix string, begin, finish map[string]any, d *device, sign func(string) string) addResult {
	e.t.Helper()
	var s started
	if status := e.do(c, "POST", prefix+"/begin", begin, &s); status != http.StatusOK {
		return addResult{status: status}
	}
	var creation struct {
		PublicKey struct {
			Attestation string `json:"attestation"`
		} `json:"publicKey"`
	}
	_ = json.Unmarshal(s.Options, &creation)
	opts, err := virtualwebauthn.ParseAttestationOptions(string(s.Options))
	if err != nil {
		e.t.Fatal(err)
	}
	d.cred = virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	response := virtualwebauthn.CreateAttestationResponse(e.rp, d.auth, d.cred, *opts)
	if sign != nil {
		response = sign(response)
	}
	var out struct {
		Error struct{ Code, Message string } `json:"error"`
	}
	finish["ceremony"], finish["credential"] = s.Ceremony, json.RawMessage(response)
	status := e.do(c, "POST", prefix+"/finish", finish, &out)
	return addResult{status: status, code: out.Error.Code, message: out.Error.Message, attestation: creation.PublicKey.Attestation}
}

func (e *env) group(name string, members ...store.User) store.Group {
	e.t.Helper()
	ctx := context.Background()
	g, err := e.st.CreateGroup(ctx, store.NewGroup{Name: name, Kind: "assigned"})
	if err != nil {
		e.t.Fatal(err)
	}
	for _, u := range members {
		if err := e.st.AddGroupMember(ctx, g.ID, u.ID); err != nil {
			e.t.Fatal(err)
		}
	}
	return g
}

func refused(t *testing.T, r addResult, group string) {
	t.Helper()
	if r.status != http.StatusForbidden || r.code != "ERR_AUTHENTICATOR_NOT_ALLOWED" || !strings.HasPrefix(r.message, "Members of "+group+" ") {
		t.Fatalf("want a refusal naming %s, got %+v", group, r)
	}
}

func added(t *testing.T, r addResult) {
	t.Helper()
	if r.status != http.StatusOK {
		t.Fatalf("want the key added, got %+v", r)
	}
}

func TestDeviceBoundRule(t *testing.T) {
	e := newEnv(t)
	ada, c, _ := e.enrolledUser("ada@example.com", virtualwebauthn.AuthenticatorOptions{})
	grace, other, _ := e.enrolledUser("grace@example.com", virtualwebauthn.AuthenticatorOptions{})
	synced := func() *device {
		return &device{auth: virtualwebauthn.NewAuthenticatorWithOptions(virtualwebauthn.AuthenticatorOptions{BackupEligible: true, BackupState: true})}
	}
	added(t, e.addKey(c, synced(), nil))

	admins := e.group("Administrators", ada)
	e.group("Staff", grace)
	e.org(func(s *store.Settings) {
		s.AuthenticatorRules = []store.AuthenticatorRule{{GroupID: admins.ID, DeviceBound: true}}
	})
	r := e.addKey(c, synced(), nil)
	refused(t, r, "Administrators")
	if r.attestation == "direct" {
		t.Fatal("a device-bound rule asked for attestation")
	}
	added(t, e.addKey(c, &device{auth: virtualwebauthn.NewAuthenticator()}, nil))
	added(t, e.addKey(other, synced(), nil))
}

func TestAttestationRule(t *testing.T) {
	e := newEnv(t)
	ada, c, _ := e.enrolledUser("ada@example.com", virtualwebauthn.AuthenticatorOptions{})
	approved, unlisted, unknown := newAttester(t, uuid.New()), newAttester(t, uuid.New()), newAttester(t, uuid.New())
	counterfeit := newAttester(t, approved.aaguid)
	admins := e.group("Administrators", ada)
	e.org(func(s *store.Settings) {
		s.AuthenticatorRules = []store.AuthenticatorRule{{GroupID: admins.ID, Attestation: true, AAGUIDs: []string{approved.aaguid.String()}}}
	})

	previous := *auth.FetchMetadata
	*auth.FetchMetadata = func() (*metadata.Metadata, error) { return nil, errors.New("offline") }
	r := e.addKey(c, approved.device(virtualwebauthn.AuthenticatorOptions{}), func(s string) string { return approved.sign(t, s) })
	*auth.FetchMetadata = previous
	if r.status != http.StatusServiceUnavailable || r.code != "ERR_METADATA_UNAVAILABLE" {
		t.Fatalf("an unreachable metadata service: %+v", r)
	}

	trusting(t, approved, unlisted)
	r = e.addKey(c, approved.device(virtualwebauthn.AuthenticatorOptions{}), nil)
	refused(t, r, "Administrators")
	if r.attestation != "direct" {
		t.Fatalf("registration asked for %q attestation, want direct", r.attestation)
	}
	refused(t, e.addKey(c, unknown.device(virtualwebauthn.AuthenticatorOptions{}), func(s string) string { return unknown.sign(t, s) }), "Administrators")
	refused(t, e.addKey(c, counterfeit.device(virtualwebauthn.AuthenticatorOptions{}), func(s string) string { return counterfeit.sign(t, s) }), "Administrators")
	r = e.addKey(c, unlisted.device(virtualwebauthn.AuthenticatorOptions{}), func(s string) string { return unlisted.sign(t, s) })
	refused(t, r, "Administrators")
	if !strings.Contains(r.message, "approved key models") {
		t.Fatalf("an unlisted model: %s", r.message)
	}
	added(t, e.addKey(c, approved.device(virtualwebauthn.AuthenticatorOptions{}), func(s string) string { return approved.sign(t, s) }))
}

func TestRulesFromEveryGroupApply(t *testing.T) {
	e := newEnv(t)
	ada, c, _ := e.enrolledUser("ada@example.com", virtualwebauthn.AuthenticatorOptions{})
	key := newAttester(t, uuid.New())
	trusting(t, key)
	admins, finance := e.group("Administrators", ada), e.group("Finance", ada)
	e.org(func(s *store.Settings) {
		s.AuthenticatorRules = []store.AuthenticatorRule{
			{GroupID: admins.ID, DeviceBound: true},
			{GroupID: finance.ID, Attestation: true, AAGUIDs: []string{key.aaguid.String()}},
		}
	})
	sign := func(s string) string { return key.sign(t, s) }
	refused(t, e.addKey(c, key.device(virtualwebauthn.AuthenticatorOptions{BackupEligible: true}), sign), "Administrators")
	refused(t, e.addKey(c, key.device(virtualwebauthn.AuthenticatorOptions{}), nil), "Finance")
	added(t, e.addKey(c, key.device(virtualwebauthn.AuthenticatorOptions{}), sign))
}

func TestKeyMinimum(t *testing.T) {
	e := newEnv(t)
	ada, c, _ := e.enrolledUser("ada@example.com", virtualwebauthn.AuthenticatorOptions{})
	admins := e.group("Administrators", ada)
	e.org(func(s *store.Settings) {
		s.AuthenticatorRules = []store.AuthenticatorRule{{GroupID: admins.ID, DeviceBound: true, Minimum: 2}}
	})
	rules := func() map[string]any {
		var out map[string]any
		if status := e.do(c, "GET", "/api/v1/me/passkey-rules", nil, &out); status != http.StatusOK {
			t.Fatalf("passkey rules: %d", status)
		}
		return out
	}
	if got := rules(); got["minimum"] != 2.0 || got["keys"] != 1.0 || got["deviceBound"] != true || got["attestation"] != false {
		t.Fatalf("passkey rules: %v", got)
	}
	added(t, e.addKey(c, &device{auth: virtualwebauthn.NewAuthenticator()}, nil))
	me, _ := e.me(c)
	e.expect(c, "DELETE", "/api/v1/me/methods/"+me.Methods[0].ID, nil, http.StatusConflict, "ERR_KEY_MINIMUM")
	added(t, e.addKey(c, &device{auth: virtualwebauthn.NewAuthenticator()}, nil))
	if got := rules(); got["keys"] != 3.0 {
		t.Fatalf("passkey rules after a third key: %v", got)
	}
	if status := e.do(c, "DELETE", "/api/v1/me/methods/"+me.Methods[0].ID, nil, nil); status != http.StatusNoContent {
		t.Fatalf("removing a key above the minimum: %d", status)
	}
}

func TestRulesApplyToSetupLinksAndSignUps(t *testing.T) {
	e, box := newMagicEnv(t, nil)
	ctx := context.Background()
	ada, err := e.st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace"})
	if err != nil {
		t.Fatal(err)
	}
	admins, members := e.group("Administrators", ada), e.group("Members")
	e.org(func(s *store.Settings) {
		s.AuthenticatorRules = []store.AuthenticatorRule{{GroupID: admins.ID, DeviceBound: true}, {GroupID: members.ID, DeviceBound: true}}
		s.SignUp, s.SignUpDomains, s.SignUpGroupIDs = "open", []string{"example.com"}, []string{members.ID}
	})
	synced := virtualwebauthn.AuthenticatorOptions{BackupEligible: true, BackupState: true}

	token, _, err := e.st.CreateEnrollmentToken(ctx, ada.ID, "invite", nil)
	if err != nil {
		t.Fatal(err)
	}
	link := map[string]any{"token": token}
	refused(t, e.registerKey(client(), "/api/v1/auth/enroll", link, map[string]any{"token": token}, &device{auth: virtualwebauthn.NewAuthenticatorWithOptions(synced)}, nil), "Administrators")
	added(t, e.registerKey(client(), "/api/v1/auth/enroll", link, map[string]any{"token": token}, &device{auth: virtualwebauthn.NewAuthenticator()}, nil))

	if status, code := e.signUp("grace@example.com", "Grace Hopper", ""); status != http.StatusAccepted {
		t.Fatalf("sign up: %d %s", status, code)
	}
	_, token = box.match(t, signUpPattern)
	link = map[string]any{"token": token}
	refused(t, e.registerKey(client(), "/api/v1/auth/sign-up", link, map[string]any{"token": token}, &device{auth: virtualwebauthn.NewAuthenticatorWithOptions(synced)}, nil), "Members")
	added(t, e.registerKey(client(), "/api/v1/auth/sign-up", link, map[string]any{"token": token}, &device{auth: virtualwebauthn.NewAuthenticator()}, nil))
}
