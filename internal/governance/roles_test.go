package governance

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"halo/internal/store"
)

func (e *env) audit(action string) []store.AuditEvent {
	e.t.Helper()
	events, err := e.st.ListAudit(context.Background(), 100)
	if err != nil {
		e.t.Fatal(err)
	}
	return slices.DeleteFunc(events, func(a store.AuditEvent) bool { return a.Action != action })
}

func TestRoleActivation(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	alex, token := e.person("alex@example.com")

	expect(t, e.call(token, "GET", "/api/v1/access-requests", nil), 403)
	activate := map[string]any{"role": "user_admin", "durationHours": 2, "justification": "Onboarding the new support team"}
	expect(t, e.call(token, "POST", "/api/v1/me/role-activations", activate), 403)

	if _, err := e.st.SetEligibleRoles(ctx, alex.ID, []string{"user_admin"}, nil); err != nil {
		t.Fatal(err)
	}
	expect(t, e.call(token, "GET", "/api/v1/access-requests", nil), 403)
	expect(t, e.call(token, "POST", "/api/v1/me/role-activations", map[string]any{"role": "user_admin", "durationHours": 9, "justification": "Too long"}), 422)
	expect(t, e.call(token, "POST", "/api/v1/me/role-activations", map[string]any{"role": "user_admin", "durationHours": 2, "justification": "  "}), 422)
	expect(t, e.call(token, "POST", "/api/v1/me/role-activations", map[string]any{"role": "global_admin", "durationHours": 1, "justification": "Not eligible"}), 403)

	rec := e.call(token, "POST", "/api/v1/me/role-activations", activate)
	expect(t, rec, 201)
	activation := decode[store.RoleActivation](t, rec)
	if until := time.Until(activation.ExpiresAt); until < 119*time.Minute || until > 2*time.Hour {
		t.Fatalf("activation ends in %s, want 2 hours", until)
	}
	expect(t, e.call(token, "GET", "/api/v1/access-requests", nil), 200)
	expect(t, e.call(token, "POST", "/api/v1/me/role-activations", activate), 409)

	u, err := e.st.GetUser(ctx, alex.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(u.Roles, []string{"User administrator"}) || len(u.Activations) != 1 || u.Activations[0].ID != activation.ID {
		t.Fatalf("roles %v, activations %+v", u.Roles, u.Activations)
	}
	if got := e.audit("role.activate"); len(got) != 1 || got[0].TargetID != alex.ID || !strings.Contains(got[0].Summary, "2 hours: Onboarding the new support team") {
		t.Fatalf("activation audit: %+v", got)
	}

	expect(t, e.call(token, "POST", "/api/v1/me/role-activations/"+activation.ID+"/deactivate", nil), 204)
	expect(t, e.call(token, "GET", "/api/v1/access-requests", nil), 403)
	expect(t, e.call(token, "POST", "/api/v1/me/role-activations/"+activation.ID+"/deactivate", nil), 409)
	if got := e.audit("role.deactivate"); len(got) != 1 || got[0].TargetID != alex.ID {
		t.Fatalf("deactivation audit: %+v", got)
	}
}

func TestExpiredActivationLosesAccessOnNextRequest(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	alex, token := e.person("alex@example.com")
	if _, err := e.st.SetEligibleRoles(ctx, alex.ID, []string{"auditor"}, nil); err != nil {
		t.Fatal(err)
	}
	ends := time.Now().Add(time.Second)
	if _, err := e.st.ActivateRole(ctx, alex.ID, "auditor", "Quarterly audit", ends); err != nil {
		t.Fatal(err)
	}
	expect(t, e.call(token, "GET", "/api/v1/access-requests", nil), 200)

	time.Sleep(time.Until(ends) + 50*time.Millisecond)
	expect(t, e.call(token, "GET", "/api/v1/access-requests", nil), 403)
	if got := e.audit("role.expire"); len(got) != 0 {
		t.Fatalf("expired before the job ran: %+v", got)
	}

	if err := expireActivations(ctx, e.st, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := e.audit("role.expire"); len(got) != 1 || got[0].TargetID != alex.ID || got[0].ActorID != nil || !strings.HasPrefix(got[0].Summary, "Auditor") {
		t.Fatalf("expiry audit: %+v", got)
	}
	if err := expireActivations(ctx, e.st, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := e.audit("role.expire"); len(got) != 1 {
		t.Fatalf("expired twice: %+v", got)
	}
	expect(t, e.call(token, "POST", "/api/v1/me/role-activations", map[string]any{"role": "auditor", "durationHours": 1, "justification": "Follow-up"}), 201)
}
