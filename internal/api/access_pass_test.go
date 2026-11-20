package api_test

import (
	"context"
	"strings"
	"testing"

	"halo/internal/store"
)

func TestAccessPass(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	hal, helpdesk := person(t, st, "hal@example.com", "helpdesk_admin")
	_, auditor := person(t, st, "aud@example.com", "auditor")
	_, global := person(t, st, "gia@example.com", "global_admin")
	security, _ := person(t, st, "sec@example.com", "security_admin")
	root, _ := person(t, st, "root@example.com", "global_admin")
	ada, _ := person(t, st, "ada@example.com")

	path := "/api/v1/users/" + ada.ID + "/access-pass"
	expect(t, call(t, h, auditor, "POST", path, map[string]any{}), 403)
	expect(t, call(t, h, helpdesk, "POST", "/api/v1/users/"+security.ID+"/access-pass", map[string]any{}), 403)
	expect(t, call(t, h, helpdesk, "POST", "/api/v1/users/"+root.ID+"/access-pass", map[string]any{}), 403)
	expect(t, call(t, h, helpdesk, "POST", "/api/v1/users/"+hal.ID+"/access-pass", map[string]any{}), 422)
	expect(t, call(t, h, helpdesk, "POST", path, map[string]any{"lifetimeMinutes": 5}), 422)
	expect(t, call(t, h, helpdesk, "POST", path, map[string]any{"uses": 11}), 422)
	expect(t, call(t, h, global, "POST", "/api/v1/users/"+security.ID+"/access-pass", map[string]any{}), 201)

	rec := call(t, h, helpdesk, "POST", path, map[string]any{"lifetimeMinutes": 120, "uses": 2})
	expect(t, rec, 201)
	out := decode[struct {
		Pass string `json:"pass"`
		Uses int    `json:"uses"`
	}](t, rec)
	if len(out.Pass) != 19 || out.Uses != 2 {
		t.Fatalf("created pass: %s", rec.Body)
	}
	if used, err := st.UseAccessPass(ctx, ada.ID, out.Pass); err != nil || !used {
		t.Fatalf("the shown pass works: %v %v", used, err)
	}

	user := decode[store.User](t, call(t, h, auditor, "GET", "/api/v1/users/"+ada.ID, nil))
	if len(user.Methods) != 1 || user.Methods[0].ID != "access-pass" || !strings.HasPrefix(user.Methods[0].Detail, "1 use left, expires ") {
		t.Fatalf("pass in the user's methods: %+v", user.Methods)
	}
	audit, _ := st.ListAudit(ctx, 1)
	if audit[0].Action != "user.access-pass.create" || audit[0].TargetID != ada.ID || strings.Contains(audit[0].Summary, out.Pass) {
		t.Fatalf("audit: %+v", audit[0])
	}
	messages, err := st.ListMail(ctx, 1)
	if err != nil || len(messages) != 1 || messages[0].To != "ada@example.com" || !strings.Contains(messages[0].Text, "hal created a temporary access pass") || strings.Contains(messages[0].Text, out.Pass) {
		t.Fatalf("notification: %+v %v", messages, err)
	}

	expect(t, call(t, h, auditor, "DELETE", "/api/v1/users/"+ada.ID+"/methods/access-pass", nil), 403)
	expect(t, call(t, h, helpdesk, "DELETE", "/api/v1/users/"+ada.ID+"/methods/access-pass", nil), 204)
	if used, err := st.UseAccessPass(ctx, ada.ID, out.Pass); used || err != store.ErrNotFound {
		t.Fatalf("revoked pass: %v %v", used, err)
	}
}
