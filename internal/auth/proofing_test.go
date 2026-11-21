package auth_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"

	"halo/internal/auth"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/proofing"
	"halo/internal/secret"
	"halo/internal/store"
	"halo/internal/testdb"
)

const (
	birthDate      = "1815-12-10"
	documentNumber = "X9876543"
)

type fakeVerifier struct {
	mu       sync.Mutex
	srv      *httptest.Server
	statuses map[string]string
	created  int
	requests []*http.Request
	bodies   []string
}

func newFakeVerifier(t *testing.T, stripe bool) *fakeVerifier {
	f := &fakeVerifier{statuses: map[string]string{}}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests, f.bodies = append(f.requests, r), append(f.bodies, string(body))
		personal := map[string]any{"name": "Ada Lovelace", "dateOfBirth": birthDate, "documentNumber": documentNumber, "verified_outputs": map[string]any{"dob": birthDate, "id_number": documentNumber}}
		if r.Method == http.MethodPost {
			f.created++
			id := "ses_" + strconv.Itoa(f.created)
			if stripe {
				id = "vs_" + strconv.Itoa(f.created)
			}
			f.statuses[id] = map[bool]string{true: "requires_input", false: "pending"}[stripe]
			personal["id"], personal["url"], personal["status"] = id, "https://verify.example.com/start/"+id, f.statuses[id]
		} else {
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			personal["id"], personal["status"] = id, f.statuses[id]
		}
		json.NewEncoder(w).Encode(personal)
	}))
	t.Cleanup(f.srv.Close)
	client, stripeURL := proofing.Client, proofing.StripeURL
	proofing.Client, proofing.StripeURL = f.srv.Client(), f.srv.URL+"/"
	t.Cleanup(func() { proofing.Client, proofing.StripeURL = client, stripeURL })
	return f
}

func (f *fakeVerifier) set(id, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses[id] = status
}

func newProofingEnv(t *testing.T) *env {
	st := testdb.New(t)
	public, _ := url.Parse("http://localhost:3200")
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	mux := http.NewServeMux()
	d := httpx.Deps{Config: config.Config{PublicURL: public, Dev: true}, Store: st, Auth: authn}
	if err := auth.Register(mux, d); err != nil {
		t.Fatal(err)
	}
	if err := proofing.Register(mux, d); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(authn.Resolve(mux))
	t.Cleanup(srv.Close)
	return &env{t: t, st: st, srv: srv, rp: virtualwebauthn.RelyingParty{ID: "localhost", Name: "Halo", Origin: "http://localhost:3200"}}
}

func signature(key string, body []byte, at time.Time) string {
	stamp := strconv.FormatInt(at.Unix(), 10)
	return "t=" + stamp + ",v1=" + hex.EncodeToString(secret.MAC([]byte(key), stamp+"."+string(body)))
}

func (e *env) deliver(header, value string, body []byte) int {
	e.t.Helper()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/identity-proofing/webhook", bytes.NewReader(body))
	if value != "" {
		req.Header.Set(header, value)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

type proofInfo struct {
	IdentityProof struct {
		Required bool
		Status   string
		Provider string
	}
}

func (e *env) results(action string) int {
	e.t.Helper()
	events, err := e.st.ListAudit(context.Background(), 1000)
	if err != nil {
		e.t.Fatal(err)
	}
	n := 0
	for _, ev := range events {
		if ev.Action == action {
			n++
		}
	}
	return n
}

func (e *env) rows(query string, args ...any) []string {
	e.t.Helper()
	var out []string
	_, err := e.st.ExportRows(context.Background(), store.ExportSection{Name: "rows", Table: "users", Query: query}, func(row json.RawMessage) error {
		out = append(out, string(row))
		return nil
	}, args...)
	if err != nil {
		e.t.Fatal(err)
	}
	return out
}

func TestIdentityCheckBeforeEnrollment(t *testing.T) {
	e := newProofingEnv(t)
	ctx := context.Background()
	u, err := e.st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace"})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := e.st.CreateEnrollmentToken(ctx, u.ID, "invite", nil)
	if err != nil {
		t.Fatal(err)
	}
	c := client()
	var info proofInfo
	if status := e.do(c, "GET", "/api/v1/auth/enroll?token="+token, nil, &info); status != http.StatusOK || info.IdentityProof.Required {
		t.Fatalf("requirement off: %d %+v", status, info)
	}

	var started started
	if status := e.do(c, "POST", "/api/v1/auth/enroll/begin", map[string]any{"token": token}, &started); status != http.StatusOK {
		t.Fatalf("begin: %d", status)
	}
	e.org(func(s *store.Settings) { s.IdentityProofingInvites = true })
	if e.do(c, "GET", "/api/v1/auth/enroll?token="+token, nil, &info); !info.IdentityProof.Required || info.IdentityProof.Status != "" || info.IdentityProof.Provider != "" {
		t.Fatalf("requirement on: %+v", info)
	}
	opts, err := virtualwebauthn.ParseAttestationOptions(string(started.Options))
	if err != nil {
		t.Fatal(err)
	}
	d := newDevice()
	finish := map[string]any{"token": token, "ceremony": started.Ceremony, "credential": json.RawMessage(virtualwebauthn.CreateAttestationResponse(e.rp, d.auth, d.cred, *opts))}
	e.expect(c, "POST", "/api/v1/auth/enroll/finish", finish, http.StatusForbidden, "ERR_IDENTITY_CHECK_REQUIRED")
	if got, err := e.st.GetUser(ctx, u.ID); err != nil || got.Status != "invited" || len(got.Methods) != 0 {
		t.Fatalf("a refused enrollment changed the account: %+v %v", got, err)
	}
	e.expect(c, "POST", "/api/v1/auth/enroll/begin", map[string]any{"token": token}, http.StatusForbidden, "ERR_IDENTITY_CHECK_REQUIRED")
	e.expect(c, "POST", "/api/v1/auth/enroll/identity-proof", map[string]any{"token": token}, http.StatusConflict, "ERR_NO_IDENTITY_PROVIDER")

	f := newFakeVerifier(t, false)
	e.org(func(s *store.Settings) {
		s.IdentityProofingProvider, s.IdentityProofingName, s.IdentityProofingEndpoint = "generic", "Acme Verify", f.srv.URL+"/sessions"
	})
	if err := e.st.SetProofingSecrets(ctx, "acme-api-key", "acme-hook-secret"); err != nil {
		t.Fatal(err)
	}
	var link struct{ URL string }
	if status := e.do(c, "POST", "/api/v1/auth/enroll/identity-proof", map[string]any{"token": token}, &link); status != http.StatusOK || link.URL != "https://verify.example.com/start/ses_1" {
		t.Fatalf("start: %d %+v", status, link)
	}
	if got := f.requests[0]; got.Header.Get("Authorization") != "Bearer acme-api-key" || got.URL.Path != "/sessions" || strings.Contains(f.bodies[0], token) || !strings.Contains(f.bodies[0], `"returnUrl":"http://localhost:3200/enroll?proofing=done"`) {
		t.Fatalf("create call: %s %s %s", got.Header.Get("Authorization"), got.URL.Path, f.bodies[0])
	}

	f.set("ses_1", "failed")
	if e.do(c, "GET", "/api/v1/auth/enroll?token="+token, nil, &info); info.IdentityProof.Status != "failed" || info.IdentityProof.Provider != "Acme Verify" {
		t.Fatalf("failed check after polling: %+v", info)
	}
	e.expect(c, "POST", "/api/v1/auth/enroll/begin", map[string]any{"token": token}, http.StatusForbidden, "ERR_IDENTITY_CHECK_REQUIRED")
	if got, _ := e.st.GetUser(ctx, u.ID); got.Status != "invited" {
		t.Fatalf("a failed check must keep the account inactive: %s", got.Status)
	}

	if status := e.do(c, "POST", "/api/v1/auth/enroll/identity-proof", map[string]any{"token": token}, &link); status != http.StatusOK || !strings.HasSuffix(link.URL, "/ses_2") {
		t.Fatalf("second start: %d %+v", status, link)
	}
	event := []byte(`{"id":"evt_1","session":"ses_2","status":"verified","name":"Ada Lovelace","dateOfBirth":"` + birthDate + `","documentNumber":"` + documentNumber + `"}`)
	now := time.Now()
	for _, bad := range []string{"", signature("wrong-secret", event, now), signature("acme-hook-secret", event, now.Add(-10*time.Minute)), signature("acme-hook-secret", []byte(`{"id":"evt_1"}`), now)} {
		if status := e.deliver("Halo-Signature", bad, event); status != http.StatusUnauthorized {
			t.Fatalf("webhook with signature %q: %d", bad, status)
		}
	}
	if status := e.deliver("Stripe-Signature", signature("acme-hook-secret", event, now), event); status != http.StatusUnauthorized {
		t.Fatalf("a generic provider must sign with Halo-Signature: %d", status)
	}
	valid := signature("acme-hook-secret", event, now)
	if status := e.deliver("Halo-Signature", valid, event); status != http.StatusOK {
		t.Fatalf("signed webhook: %d", status)
	}
	if status := e.deliver("Halo-Signature", valid, event); status != http.StatusOK {
		t.Fatalf("replayed webhook: %d", status)
	}
	if status := e.deliver("Halo-Signature", signature("acme-hook-secret", event, now.Add(time.Second)), event); status != http.StatusOK {
		t.Fatalf("redelivered webhook: %d", status)
	}
	failed := []byte(`{"id":"evt_2","session":"ses_2","status":"failed"}`)
	if status := e.deliver("Halo-Signature", signature("acme-hook-secret", failed, now), failed); status != http.StatusOK {
		t.Fatalf("late failure: %d", status)
	}
	if n := e.results("user.identity_proof.result"); n != 2 {
		t.Fatalf("one result per check, not per delivery: %d", n)
	}
	if e.do(c, "GET", "/api/v1/auth/enroll?token="+token, nil, &info); info.IdentityProof.Status != "verified" {
		t.Fatalf("verified by webhook: %+v", info)
	}

	columns := e.rows(`select column_name from information_schema.columns where table_name = 'identity_proofs' order by column_name`)
	want := []string{"checked_by", "completed_at", "created_at", "document", "id", "method", "note", "provider", "reference", "sign_up_id", "status", "user_id"}
	for i := range columns {
		var c struct{ ColumnName string }
		_ = json.Unmarshal([]byte(columns[i]), &c)
		columns[i] = c.ColumnName
	}
	if !slices.Equal(columns, want) {
		t.Fatalf("identity_proofs columns: %v", columns)
	}
	stored := e.rows(`select to_jsonb(p) as row from identity_proofs p`)
	audit := e.rows(`select to_jsonb(a) as row from audit_events a`)
	for _, row := range append(stored, audit...) {
		for _, personal := range []string{birthDate, documentNumber, "acme-api-key", "acme-hook-secret"} {
			if strings.Contains(row, personal) {
				t.Fatalf("Halo kept %q from the provider: %s", personal, row)
			}
		}
	}
	if len(stored) != 2 {
		t.Fatalf("stored checks: %v", stored)
	}

	if status := e.register(c, "/api/v1/auth/enroll", map[string]any{"token": token}, map[string]any{"token": token}, d); status != http.StatusOK {
		t.Fatalf("enroll after the check: %d", status)
	}
	if me, _ := e.me(c); me.Status != "active" {
		t.Fatalf("after enrolling: %+v", me)
	}
}

func TestIdentityCheckGroupsAndSignUps(t *testing.T) {
	e, box := newMagicEnv(t, nil)
	ctx := context.Background()
	finance, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Finance", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	e.org(func(s *store.Settings) {
		s.IdentityProofingInvites, s.IdentityProofingSignUps, s.IdentityProofingGroupIDs = true, true, []string{finance.ID}
		s.SignUp, s.SignUpDomains = "open", []string{"example.com"}
	})
	e.enrolledUser("outside@example.com", virtualwebauthn.AuthenticatorOptions{})

	u, err := e.st.CreateUser(ctx, store.NewUser{Email: "fin@example.com", Name: "Fin", GroupIDs: []string{finance.ID}})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := e.st.CreateEnrollmentToken(ctx, u.ID, "invite", nil)
	if err != nil {
		t.Fatal(err)
	}
	e.expect(client(), "POST", "/api/v1/auth/enroll/begin", map[string]any{"token": token}, http.StatusForbidden, "ERR_IDENTITY_CHECK_REQUIRED")

	if status, code := e.signUp("grace@example.com", "Grace Hopper", ""); status != http.StatusAccepted {
		t.Fatalf("sign up: %d %s", status, code)
	}
	_, signUpToken := box.match(t, signUpPattern)
	var info proofInfo
	if e.do(client(), "GET", "/api/v1/auth/sign-up?token="+signUpToken, nil, &info); info.IdentityProof.Required {
		t.Fatalf("sign-ups outside the listed groups: %+v", info)
	}
	e.org(func(s *store.Settings) { s.SignUpGroupIDs = []string{finance.ID} })
	if e.do(client(), "GET", "/api/v1/auth/sign-up?token="+signUpToken, nil, &info); !info.IdentityProof.Required {
		t.Fatalf("sign-ups into a listed group: %+v", info)
	}
	c := client()
	var out struct{ Pending, IdentityCheck bool }
	var s started
	e.do(c, "POST", "/api/v1/auth/sign-up/begin", map[string]any{"token": signUpToken}, &s)
	opts, err := virtualwebauthn.ParseAttestationOptions(string(s.Options))
	if err != nil {
		t.Fatal(err)
	}
	d := newDevice()
	finish := map[string]any{"token": signUpToken, "ceremony": s.Ceremony, "credential": json.RawMessage(virtualwebauthn.CreateAttestationResponse(e.rp, d.auth, d.cred, *opts))}
	if status := e.do(c, "POST", "/api/v1/auth/sign-up/finish", finish, &out); status != http.StatusOK || !out.Pending || !out.IdentityCheck {
		t.Fatalf("open sign-up without a check: %d %+v", status, out)
	}
	if _, err := e.st.GetUserByEmail(ctx, "grace@example.com"); err == nil {
		t.Fatal("an open sign-up without an identity check became an account")
	}
	queue, err := e.st.ListSignUps(ctx)
	if err != nil || len(queue) != 1 {
		t.Fatalf("waiting sign-ups: %+v %v", queue, err)
	}
	if _, err := e.st.ActivateSignUp(ctx, queue[0].ID, []string{finance.ID}, true); err != store.ErrProofRequired {
		t.Fatalf("activation without a check: %v", err)
	}
	if _, err := e.st.CreateIdentityProof(ctx, store.NewIdentityProof{SignUpID: queue[0].ID, Method: "provider", Provider: "generic", Reference: "ses_9", Status: "failed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.ActivateSignUp(ctx, queue[0].ID, []string{finance.ID}, true); err != store.ErrProofRequired {
		t.Fatalf("activation after a failed check: %v", err)
	}
	if _, err := e.st.CreateIdentityProof(ctx, store.NewIdentityProof{SignUpID: queue[0].ID, Method: "in-person", Status: "verified", Document: "passport", Note: "Front desk"}); err != nil {
		t.Fatal(err)
	}
	activated, err := e.st.ActivateSignUp(ctx, queue[0].ID, []string{finance.ID}, true)
	if err != nil || activated.Status != "active" {
		t.Fatalf("activation after a check: %+v %v", activated, err)
	}
	if proofs, err := e.st.ListIdentityProofs(ctx, activated.ID); err != nil || len(proofs) != 2 {
		t.Fatalf("checks after activation: %+v %v", proofs, err)
	}
}

func TestStripeIdentityCheck(t *testing.T) {
	e := newProofingEnv(t)
	ctx := context.Background()
	f := newFakeVerifier(t, true)
	e.org(func(s *store.Settings) { s.IdentityProofingInvites, s.IdentityProofingProvider = true, "stripe" })
	if err := e.st.SetProofingSecrets(ctx, "sk_test_123", "whsec_test"); err != nil {
		t.Fatal(err)
	}
	u, err := e.st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace"})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := e.st.CreateEnrollmentToken(ctx, u.ID, "invite", nil)
	if err != nil {
		t.Fatal(err)
	}
	c := client()
	var link struct{ URL string }
	if status := e.do(c, "POST", "/api/v1/auth/enroll/identity-proof", map[string]any{"token": token}, &link); status != http.StatusOK || link.URL != "https://verify.example.com/start/vs_1" {
		t.Fatalf("start: %d %+v", status, link)
	}
	form, _ := url.ParseQuery(f.bodies[0])
	if got := f.requests[0]; got.URL.Path != "/v1/identity/verification_sessions" || got.Header.Get("Authorization") != "Bearer sk_test_123" || form.Get("type") != "document" || form.Get("return_url") != "http://localhost:3200/enroll?proofing=done" {
		t.Fatalf("Stripe call: %s %s %v", got.URL.Path, got.Header.Get("Authorization"), form)
	}
	var info proofInfo
	if e.do(c, "GET", "/api/v1/auth/enroll?token="+token, nil, &info); info.IdentityProof.Status != "pending" || info.IdentityProof.Provider != "Stripe Identity" {
		t.Fatalf("waiting for Stripe: %+v", info)
	}
	event := []byte(`{"id":"evt_1","type":"identity.verification_session.verified","data":{"object":{"id":"vs_1","status":"verified","verified_outputs":{"dob":"` + birthDate + `","id_number":"` + documentNumber + `"}}}}`)
	if status := e.deliver("Halo-Signature", signature("whsec_test", event, time.Now()), event); status != http.StatusUnauthorized {
		t.Fatalf("Stripe signs with Stripe-Signature: %d", status)
	}
	if status := e.deliver("Stripe-Signature", signature("whsec_test", event, time.Now())+",v1=00", event); status != http.StatusOK {
		t.Fatalf("Stripe webhook: %d", status)
	}
	if e.do(c, "GET", "/api/v1/auth/enroll?token="+token, nil, &info); info.IdentityProof.Status != "verified" {
		t.Fatalf("verified by Stripe: %+v", info)
	}
	for _, row := range e.rows(`select to_jsonb(p) as row from identity_proofs p`) {
		if strings.Contains(row, birthDate) || strings.Contains(row, documentNumber) {
			t.Fatalf("Halo kept Stripe's verified outputs: %s", row)
		}
	}
}
