package api_test

import (
	"context"
	"slices"
	"testing"

	"halo/internal/store"
)

func pendingSignUp(t *testing.T, st *store.Store, email string) store.SignUp {
	t.Helper()
	ctx := context.Background()
	token, _, err := st.RequestSignUp(ctx, email, "Grace Hopper")
	if err != nil {
		t.Fatal(err)
	}
	su, err := st.VerifySignUp(ctx, token, store.WebAuthnCredential{Kind: "passkey", Label: "Passkey", CredentialID: []byte(email), PublicKey: []byte{1}, Transports: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	return su
}

func TestSignUpQueue(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "admin@example.com", "user_admin")
	_, auditor := person(t, st, "auditor@example.com", "auditor")
	members, err := st.CreateGroup(ctx, store.NewGroup{Name: "Members", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := st.Settings(ctx)
	settings.SignUp, settings.SignUpGroupIDs = "approval", []string{members.ID}
	if err := st.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	st.ForgetSettings()
	if _, _, err := st.RequestSignUp(ctx, "unconfirmed@example.com", "Unconfirmed"); err != nil {
		t.Fatal(err)
	}
	grace, mallory := pendingSignUp(t, st, "grace@example.com"), pendingSignUp(t, st, "mallory@example.com")

	queue := decode[[]store.SignUp](t, call(t, h, auditor, "GET", "/api/v1/sign-ups", nil))
	if len(queue) != 2 || queue[0].ID != grace.ID || queue[1].ID != mallory.ID {
		t.Fatalf("queue lists only confirmed sign-ups: %+v", queue)
	}
	expect(t, call(t, h, auditor, "POST", "/api/v1/sign-ups/"+grace.ID+"/approve", nil), 403)
	expect(t, call(t, h, auditor, "DELETE", "/api/v1/sign-ups/"+mallory.ID, nil), 403)

	u := decode[store.User](t, call(t, h, admin, "POST", "/api/v1/sign-ups/"+grace.ID+"/approve", nil))
	if u.Status != "active" || u.Email != "grace@example.com" || !u.EmailVerified || !slices.Equal(u.GroupIDs, []string{members.ID}) || len(u.RoleKeys) != 0 || len(u.Methods) != 1 {
		t.Fatalf("approved account: %+v", u)
	}
	expect(t, call(t, h, admin, "POST", "/api/v1/sign-ups/"+grace.ID+"/approve", nil), 404)
	expect(t, call(t, h, admin, "DELETE", "/api/v1/sign-ups/"+mallory.ID, nil), 204)
	if _, err := st.GetUserByEmail(ctx, "mallory@example.com"); err == nil {
		t.Fatal("a rejected sign-up got an account")
	}

	taken := pendingSignUp(t, st, "taken@example.com")
	if _, err := st.CreateUser(ctx, store.NewUser{Email: "taken@example.com", Name: "Taken"}); err != nil {
		t.Fatal(err)
	}
	expect(t, call(t, h, admin, "POST", "/api/v1/sign-ups/"+taken.ID+"/approve", nil), 409)

	if left := decode[[]store.SignUp](t, call(t, h, admin, "GET", "/api/v1/sign-ups", nil)); len(left) != 1 || left[0].ID != taken.ID {
		t.Fatalf("queue after decisions: %+v", left)
	}
	audit, err := st.ListAudit(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"user.sign_up.approve", "user.sign_up.reject"} {
		if !slices.ContainsFunc(audit, func(a store.AuditEvent) bool { return a.Action == action && a.ActorID != nil }) {
			t.Errorf("no %s in the audit log", action)
		}
	}
}
