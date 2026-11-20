package api_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"halo/internal/posture"
)

func TestRecommendations(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, auditor := person(t, st, "aud@example.com", "auditor")
	security, token := person(t, st, "sec@example.com", "security_admin")
	if err := st.SetMethodEnabled(ctx, "password", true); err != nil {
		t.Fatal(err)
	}

	pick := func(checks []posture.Check, id string) posture.Check {
		t.Helper()
		i := slices.IndexFunc(checks, func(c posture.Check) bool { return c.ID == id })
		if i < 0 {
			t.Fatalf("no %s in %+v", id, checks)
		}
		return checks[i]
	}
	checks := decode[[]posture.Check](t, call(t, h, auditor, "GET", "/api/v1/recommendations", nil))
	if len(checks) != 14 {
		t.Fatalf("checks: %+v", checks)
	}
	if c := pick(checks, "admin-sign-in"); c.Status != "fail" || !strings.Contains(c.Detail, "aud, sec") {
		t.Fatalf("admins without a passkey: %+v", c)
	}
	if c := pick(checks, "password-everyone"); c.Status != "attention" {
		t.Fatalf("password sign-in for everyone: %+v", c)
	}
	if c := pick(checks, "lockout"); c.Status != "pass" {
		t.Fatalf("default lockout: %+v", c)
	}

	path := "/api/v1/recommendations/password-everyone/dismissal"
	reason := map[string]string{"reason": "Contractors can't use passkeys until March"}
	expect(t, call(t, h, auditor, "PUT", path, reason), 403)
	expect(t, call(t, h, token, "PUT", "/api/v1/recommendations/no-such-check/dismissal", reason), 404)
	expect(t, call(t, h, token, "PUT", "/api/v1/recommendations/lockout/dismissal", reason), 422)
	expect(t, call(t, h, token, "PUT", path, map[string]string{"reason": "  "}), 422)

	rec := call(t, h, token, "PUT", path, reason)
	expect(t, rec, 200)
	c := pick(decode[[]posture.Check](t, rec), "password-everyone")
	if c.Status != "attention" || c.Dismissal == nil || c.Dismissal.Reason != reason["reason"] || c.Dismissal.By == nil || c.Dismissal.By.ID != security.ID {
		t.Fatalf("dismissed: %+v", c)
	}
	audit, _ := st.ListAudit(ctx, 1)
	if audit[0].Action != "recommendation.dismiss" || audit[0].TargetID != "password-everyone" || !strings.HasSuffix(audit[0].Summary, ": "+reason["reason"]) {
		t.Fatalf("audit: %+v", audit[0])
	}

	expect(t, call(t, h, auditor, "DELETE", path, nil), 403)
	rec = call(t, h, token, "DELETE", path, nil)
	expect(t, rec, 200)
	if c := pick(decode[[]posture.Check](t, rec), "password-everyone"); c.Dismissal != nil {
		t.Fatalf("restored: %+v", c)
	}
	audit, _ = st.ListAudit(ctx, 1)
	if audit[0].Action != "recommendation.restore" || audit[0].TargetID != "password-everyone" {
		t.Fatalf("audit: %+v", audit[0])
	}
	expect(t, call(t, h, token, "DELETE", path, nil), 404)
}
