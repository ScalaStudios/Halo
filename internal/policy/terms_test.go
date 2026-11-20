package policy_test

import (
	"context"
	"errors"
	"testing"

	"halo/internal/access"
	"halo/internal/policy"
	"halo/internal/store"
	"halo/internal/testdb"
)

func TestTermsStayPendingUntilTheCurrentVersionIsAccepted(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	eng := policy.NewEngine(st)
	ada := member(t, st, "ada@example.com")
	aup, err := st.SaveTerms(ctx, store.Terms{Name: "Acceptable use", Text: "Use company systems for work."})
	if err != nil {
		t.Fatal(err)
	}
	require := rule(t, st, "Require passkeys", "enforce", access.RequirePhishingResistant, everyone)
	terms, err := st.CreateAccessPolicy(ctx, store.AccessPolicy{Name: "Accept the AUP", Enabled: true, Mode: "enforce", Effect: access.RequireTerms, TermsID: &aup.ID, Conditions: everyone})
	if err != nil {
		t.Fatal(err)
	}
	pending := func(method string) []store.Terms {
		t.Helper()
		return evaluate(t, eng, ada, method, "198.51.100.7", "").Terms
	}

	d := evaluate(t, eng, ada, "passkey", "198.51.100.7", "")
	want(t, d, access.RequirePhishingResistant, require.Name)
	if len(d.Terms) != 1 || d.Terms[0].ID != aup.ID || d.Terms[0].Version != 1 {
		t.Fatalf("a passkey policy must not hide the terms: %+v", d.Terms)
	}
	if err := st.AcceptTerms(ctx, ada.ID, d.Terms); err != nil {
		t.Fatal(err)
	}
	if got := pending("passkey"); len(got) != 0 {
		t.Fatalf("accepted terms are still pending: %+v", got)
	}

	if renamed, err := st.SaveTerms(ctx, store.Terms{ID: aup.ID, Name: "Acceptable use policy", Text: aup.Text}); err != nil || renamed.Version != 1 {
		t.Fatalf("renaming must keep the version: %+v %v", renamed, err)
	}
	if got := pending("passkey"); len(got) != 0 {
		t.Fatalf("a rename asked again: %+v", got)
	}
	updated, err := st.SaveTerms(ctx, store.Terms{ID: aup.ID, Name: "Acceptable use policy", Text: "Use company systems for work only."})
	if err != nil || updated.Version != 2 {
		t.Fatalf("new text must publish version 2: %+v %v", updated, err)
	}
	if got := pending("passkey"); len(got) != 1 || got[0].Version != 2 {
		t.Fatalf("version 2 must be asked again: %+v", got)
	}
	if _, err := st.SaveTerms(ctx, store.Terms{ID: "tou_missing", Name: "Gone", Text: "x"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("saving a missing document must not create it: %v", err)
	}

	terms.Mode = "report"
	if _, err := st.UpdateAccessPolicy(ctx, terms); err != nil {
		t.Fatal(err)
	}
	if got := pending("passkey"); len(got) != 0 {
		t.Fatalf("a report-only policy must not ask: %+v", got)
	}
	activity, err := st.PolicyActivitySince(ctx, d.Terms[0].CreatedAt.Add(-1))
	if err != nil || activity[terms.ID].Stopped != 1 {
		t.Fatalf("report-only activity: %+v %v", activity, err)
	}
}

func TestTermsAPI(t *testing.T) {
	st, h := server(t)
	ctx := context.Background()
	_, security := admin(t, st, "sec@example.com", "security_admin")
	_, auditor := admin(t, st, "audrey@example.com", "auditor")
	ada, person := admin(t, st, "ada@example.com")

	expect(t, call(t, h, auditor, "GET", "/api/v1/terms", nil), 200)
	expect(t, call(t, h, person, "GET", "/api/v1/terms", nil), 403)
	expect(t, call(t, h, auditor, "POST", "/api/v1/terms", map[string]any{"name": "AUP", "text": "Be kind."}), 403)
	expect(t, call(t, h, security, "POST", "/api/v1/terms", map[string]any{"name": "AUP"}), 422)
	expect(t, call(t, h, security, "POST", "/api/v1/terms", map[string]any{"name": "AUP", "url": "javascript:alert(1)"}), 422)
	aup := decode[store.Terms](t, call(t, h, security, "POST", "/api/v1/terms", map[string]any{"name": "AUP", "url": "https://example.com/aup"}))
	if aup.Version != 1 || aup.URL != "https://example.com/aup" {
		t.Fatalf("created terms: %+v", aup)
	}
	expect(t, call(t, h, security, "POST", "/api/v1/terms", map[string]any{"name": "aup", "text": "Be kind."}), 409)

	missing := body("Accept the AUP", "enforce", access.RequireTerms, everyone)
	expect(t, call(t, h, security, "POST", "/api/v1/policies", missing), 422)
	missing["termsId"] = "tou_missing"
	expect(t, call(t, h, security, "POST", "/api/v1/policies", missing), 422)
	missing["termsId"] = aup.ID
	created := decode[store.AccessPolicy](t, call(t, h, security, "POST", "/api/v1/policies", missing))
	if created.TermsID == nil || *created.TermsID != aup.ID {
		t.Fatalf("policy terms: %+v", created)
	}
	blocking := body("Block", "report", access.Block, everyone)
	blocking["termsId"] = aup.ID
	if p := decode[store.AccessPolicy](t, call(t, h, security, "POST", "/api/v1/policies", blocking)); p.TermsID != nil {
		t.Fatalf("only a terms policy keeps a terms id: %+v", p)
	}

	updated := decode[store.Terms](t, call(t, h, security, "PUT", "/api/v1/terms/"+aup.ID, map[string]any{"name": "AUP", "url": "https://example.com/aup-2"}))
	audit, _ := st.ListAudit(ctx, 1)
	if updated.Version != 2 || audit[0].Action != "terms.update" || audit[0].Summary != "Published version 2" {
		t.Fatalf("new version: %+v %+v", updated, audit[0])
	}
	expect(t, call(t, h, security, "DELETE", "/api/v1/terms/"+aup.ID, nil), 409)

	type mine struct {
		Terms []struct {
			ID         string
			Version    int
			AcceptedAt *string
		}
		Accepted []store.TermsAcceptance
	}
	got := decode[mine](t, call(t, h, person, "GET", "/api/v1/me/terms", nil))
	if len(got.Terms) != 1 || got.Terms[0].ID != aup.ID || got.Terms[0].AcceptedAt != nil || len(got.Accepted) != 0 {
		t.Fatalf("terms before accepting: %+v", got)
	}
	expect(t, call(t, h, person, "POST", "/api/v1/me/terms/"+aup.ID+"/accept", map[string]any{"version": 1}), 409)
	expect(t, call(t, h, person, "POST", "/api/v1/me/terms/"+aup.ID+"/accept", map[string]any{"version": 2}), 204)
	got = decode[mine](t, call(t, h, person, "GET", "/api/v1/me/terms", nil))
	if len(got.Terms) != 1 || got.Terms[0].AcceptedAt == nil || len(got.Accepted) != 1 || got.Accepted[0].Version != 2 {
		t.Fatalf("terms after accepting: %+v", got)
	}

	created.Conditions = store.PolicyConditions{AllUsers: true, ExcludeUserIDs: []string{ada.ID}, AllApps: true}
	if _, err := st.UpdateAccessPolicy(ctx, created); err != nil {
		t.Fatal(err)
	}
	if got := decode[mine](t, call(t, h, person, "GET", "/api/v1/me/terms", nil)); len(got.Terms) != 0 || len(got.Accepted) != 1 {
		t.Fatalf("excluded person still sees the terms as required: %+v", got)
	}
	expect(t, call(t, h, "", "GET", "/api/v1/me/terms", nil), 401)
}
