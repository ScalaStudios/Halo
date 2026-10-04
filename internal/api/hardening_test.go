package api_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"halo/internal/api"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/mail/mailtest"
	"halo/internal/store"
	"halo/internal/testdb"
)

type signingKeys struct {
	OIDC []store.SigningKeyInfo `json:"oidc"`
	SAML []struct {
		ID          string     `json:"id"`
		Fingerprint string     `json:"fingerprint"`
		NotAfter    time.Time  `json:"notAfter"`
		RetiredAt   *time.Time `json:"retiredAt"`
	} `json:"saml"`
}

func TestSigningKeyEndpoints(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, global := person(t, st, "gia@example.com", "global_admin")
	_, security := person(t, st, "sec@example.com", "security_admin")
	_, auditor := person(t, st, "aud@example.com", "auditor")
	first, err := st.CreateSigningKey(ctx, "RS256", []byte("unused"))
	if err != nil {
		t.Fatal(err)
	}

	listed := decode[signingKeys](t, call(t, h, auditor, "GET", "/api/v1/signing-keys", nil))
	if len(listed.OIDC) != 1 || listed.OIDC[0].ID != first.ID || !listed.OIDC[0].Active || len(listed.SAML) != 0 {
		t.Fatalf("initial keys: %+v", listed)
	}
	expect(t, call(t, h, security, "POST", "/api/v1/signing-keys/rotate", nil), 403)
	expect(t, call(t, h, security, "POST", "/api/v1/saml/certificate/rotate", nil), 403)

	rec := call(t, h, global, "POST", "/api/v1/signing-keys/rotate", nil)
	expect(t, rec, 200)
	rotated := decode[signingKeys](t, rec)
	if len(rotated.OIDC) != 2 || rotated.OIDC[1].ID != first.ID || !rotated.OIDC[0].Active || rotated.OIDC[0].Algorithm != "RS256" || rotated.OIDC[1].Active || rotated.OIDC[1].RetiresAt == nil {
		t.Fatalf("after rotation: %+v", rotated.OIDC)
	}
	if audit, _ := st.ListAudit(ctx, 1); audit[0].Action != "signing_key.rotate" || audit[0].TargetID != rotated.OIDC[0].ID {
		t.Fatalf("rotation audit: %+v", audit[0])
	}

	expect(t, call(t, h, global, "POST", "/api/v1/saml/certificate/rotate", nil), 200)
	rec = call(t, h, global, "POST", "/api/v1/saml/certificate/rotate", nil)
	expect(t, rec, 200)
	certs := decode[signingKeys](t, rec).SAML
	if len(certs) != 2 || certs[0].RetiredAt != nil || certs[1].RetiredAt == nil || len(strings.Split(certs[0].Fingerprint, ":")) != 32 || certs[0].NotAfter.Before(time.Now().AddDate(9, 0, 0)) {
		t.Fatalf("SAML certificates: %+v", certs)
	}
	if active, err := st.ActiveSAMLKey(ctx); err != nil || active.ID != certs[0].ID {
		t.Fatalf("active SAML key: %+v %v", active.ID, err)
	}
	expect(t, call(t, h, global, "DELETE", "/api/v1/saml/certificates/"+certs[0].ID, nil), 404)
	expect(t, call(t, h, security, "DELETE", "/api/v1/saml/certificates/"+certs[1].ID, nil), 403)
	expect(t, call(t, h, global, "DELETE", "/api/v1/saml/certificates/"+certs[1].ID, nil), 204)
	if left := decode[signingKeys](t, call(t, h, auditor, "GET", "/api/v1/signing-keys", nil)).SAML; len(left) != 1 || left[0].ID != certs[0].ID {
		t.Fatalf("after removing the old certificate: %+v", left)
	}
	if audit, _ := st.ListAudit(ctx, 2); audit[0].Action != "saml_certificate.delete" || audit[1].Action != "saml_certificate.rotate" {
		t.Fatalf("SAML audit: %+v", audit)
	}
}

func TestGroupDeletionGuard(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	admin, token := person(t, st, "uma@example.com", "user_admin", "global_admin")
	group, err := st.CreateGroup(ctx, store.NewGroup{Name: "Contractors", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddGroupMember(ctx, group.ID, admin.ID); err != nil {
		t.Fatal(err)
	}
	provider, err := st.SaveIdentityProvider(ctx, store.IdentityProvider{Kind: "oidc", Name: "Acme SSO", Issuer: "https://idp.example.com", ClientID: "halo", Scopes: []string{}, AllowedDomains: []string{"example.com"}, JIT: true, JITGroupIDs: []string{group.ID}}, "")
	if err != nil {
		t.Fatal(err)
	}
	rec := call(t, h, token, "DELETE", "/api/v1/groups/"+group.ID, nil)
	expect(t, rec, 409)
	if !strings.Contains(rec.Body.String(), "the identity provider Acme SSO") {
		t.Fatalf("conflict must name the provider: %s", rec.Body)
	}
	provider.JITGroupIDs = []string{}
	if _, err := st.SaveIdentityProvider(ctx, provider, ""); err != nil {
		t.Fatal(err)
	}

	ruleID, err := st.SaveLifecycleRule(ctx, "", store.LifecycleRuleInput{Name: "Contractor leavers", Trigger: "leaver", Actions: []store.LifecycleAction{{Type: "revoke-sessions"}, {Type: "remove-from-group", GroupID: group.ID}}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	rec = call(t, h, token, "DELETE", "/api/v1/groups/"+group.ID, nil)
	expect(t, rec, 409)
	if !strings.Contains(rec.Body.String(), "the lifecycle rule Contractor leavers") {
		t.Fatalf("conflict must name the lifecycle rule: %s", rec.Body)
	}
	if err := st.DeleteLifecycleRule(ctx, ruleID); err != nil {
		t.Fatal(err)
	}

	reviewID, err := st.CreateAccessReview(ctx, store.NewAccessReview{Name: "Q3 contractors", GroupID: group.ID, ReviewerIDs: []string{admin.ID}, DueAt: time.Now().Add(time.Hour), CreatedBy: admin.ID})
	if err != nil {
		t.Fatal(err)
	}
	rec = call(t, h, token, "DELETE", "/api/v1/groups/"+group.ID, nil)
	expect(t, rec, 409)
	if body := rec.Body.String(); !strings.Contains(body, "the access review Q3 contractors") || !strings.Contains(body, "complete its open access reviews") {
		t.Fatalf("conflict must name the review: %s", body)
	}
	if _, err := st.CompleteAccessReview(ctx, reviewID, admin.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	expect(t, call(t, h, token, "DELETE", "/api/v1/groups/"+group.ID, nil), 204)
	review, err := st.GetAccessReview(ctx, reviewID)
	if err != nil || review.Group.Name != "Contractors" || review.Status != "completed" || len(review.Items) != 1 {
		t.Fatalf("completed review must survive with the group name: %+v %v", review, err)
	}
	if _, err := st.CompleteAccessReview(ctx, reviewID, admin.ID, time.Now()); err != store.ErrConflict {
		t.Fatalf("completing again: %v", err)
	}
}

func TestUserListQueryCount(t *testing.T) {
	st, queries := testdb.NewCounted(t)
	public, _ := url.Parse("http://localhost:3200")
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	mux := http.NewServeMux()
	if err := api.Register(mux, httpx.Deps{Config: config.Config{PublicURL: public, Dev: true}, Store: st, Auth: authn}); err != nil {
		t.Fatal(err)
	}
	h := authn.Resolve(authn.SameOrigin(mux))
	ctx := context.Background()
	_, token := person(t, st, "uma@example.com", "user_admin")
	rule := `user.department == "Engineering"`
	dynamic, err := st.CreateGroup(ctx, store.NewGroup{Name: "Engineering", Kind: "dynamic", Rule: &rule})
	if err != nil {
		t.Fatal(err)
	}
	assigned, err := st.CreateGroup(ctx, store.NewGroup{Name: "Ops", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateApplication(ctx, store.NewApplication{Name: "Wiki", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://wiki.example.com/cb"}, GroupIDs: []string{dynamic.ID, assigned.ID}}); err != nil {
		t.Fatal(err)
	}
	count := func(people int) int64 {
		t.Helper()
		for i := range people {
			u, err := st.CreateUser(ctx, store.NewUser{Email: fmt.Sprintf("p%d-%d@example.com", people, i), Name: "P", Department: "Engineering", Status: "active", Roles: []string{"auditor"}, GroupIDs: []string{assigned.ID}})
			if err != nil {
				t.Fatal(err)
			}
			if err := st.ReplaceRecoveryCodes(ctx, u.ID, [][]byte{st.RecoveryCodeHash(u.ID)}); err != nil {
				t.Fatal(err)
			}
		}
		start := queries.Load()
		expect(t, call(t, h, token, "GET", "/api/v1/users", nil), 200)
		return queries.Load() - start
	}
	const limit = 15
	small, large := count(2), count(40)
	if small != large || large > limit {
		t.Fatalf("GET /api/v1/users ran %d queries with 3 people and %d with 43; want the same number, at most %d", small, large, limit)
	}
}

func TestEmailChangeClearsVerification(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, token := person(t, st, "uma@example.com", "user_admin")
	target, _ := person(t, st, "sam@example.com")
	if err := st.MarkEmailVerified(ctx, target.ID); err != nil {
		t.Fatal(err)
	}
	if u := decode[store.User](t, call(t, h, token, "PATCH", "/api/v1/users/"+target.ID, map[string]any{"email": "SAM@example.com", "title": "Engineer"})); !u.EmailVerified {
		t.Fatalf("a change of letter case keeps the address verified: %+v", u)
	}
	if u := decode[store.User](t, call(t, h, token, "PATCH", "/api/v1/users/"+target.ID, map[string]any{"email": "sam.lee@example.com"})); u.EmailVerified || u.Email != "sam.lee@example.com" {
		t.Fatalf("a new address is not verified: %+v", u)
	}
}

func TestEmailedLinksAreRecorded(t *testing.T) {
	server := mailtest.Start(t, 0)
	host, port, _ := net.SplitHostPort(server.Addr)
	st := testdb.New(t)
	public, _ := url.Parse("http://localhost:3200")
	cfg := config.Config{PublicURL: public, Dev: true, SMTP: config.SMTP{Host: host, Port: port, From: "Halo <halo@example.com>"}}
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	mux := http.NewServeMux()
	if err := api.Register(mux, httpx.Deps{Config: cfg, Store: st, Auth: authn}); err != nil {
		t.Fatal(err)
	}
	h := authn.Resolve(authn.SameOrigin(mux))
	_, admin := person(t, st, "uma@example.com", "user_admin")
	emailed := func(h http.Handler, email string) bool {
		t.Helper()
		rec := call(t, h, admin, "POST", "/api/v1/users", map[string]any{"email": email, "name": "Sam Lee"})
		expect(t, rec, 201)
		link := decode[linkResponse](t, rec)
		_, recorded, err := st.ConsumeEnrollmentToken(context.Background(), link.EnrollURL[strings.Index(link.EnrollURL, "token=")+6:])
		if err != nil || recorded != link.Emailed {
			t.Fatalf("emailed %v, recorded %v: %v", link.Emailed, recorded, err)
		}
		return recorded
	}
	if !emailed(h, "sam@example.com") {
		t.Fatal("a delivered invitation must be recorded as emailed")
	}
	plain := http.NewServeMux()
	if err := api.Register(plain, httpx.Deps{Config: config.Config{PublicURL: public, Dev: true}, Store: st, Auth: authn}); err != nil {
		t.Fatal(err)
	}
	if emailed(authn.Resolve(authn.SameOrigin(plain)), "kim@example.com") {
		t.Fatal("an invitation that was not delivered must not be recorded as emailed")
	}
}
