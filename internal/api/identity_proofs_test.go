package api_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"halo/internal/store"
)

func TestInPersonIdentityCheck(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	support, err := st.CreateGroup(ctx, store.NewGroup{Name: "Support", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateGroup(ctx, store.NewGroup{Name: "Other", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	helpdesk, helpdeskToken := person(t, st, "hana@example.com", "helpdesk_admin")
	_, appAdmin := person(t, st, "app@example.com", "app_admin")
	_, global := person(t, st, "global@example.com", "global_admin")
	_, scoped := scopedAdmin(t, st, "sue@example.com", "helpdesk_admin", support.ID)
	ada, err := st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", GroupIDs: []string{support.ID}})
	if err != nil {
		t.Fatal(err)
	}
	outside, err := st.CreateUser(ctx, store.NewUser{Email: "out@example.com", Name: "Outside", GroupIDs: []string{other.ID}})
	if err != nil {
		t.Fatal(err)
	}
	protected, err := st.CreateUser(ctx, store.NewUser{Email: "sec@example.com", Name: "Sec", Roles: []string{"security_admin"}})
	if err != nil {
		t.Fatal(err)
	}
	check := map[string]any{"document": "passport", "note": "Met at the Lisbon office and compared the photo"}
	path := func(u store.User) string { return "/api/v1/users/" + u.ID + "/identity-proofs" }

	expect(t, call(t, h, appAdmin, "POST", path(ada), check), 403)
	expect(t, call(t, h, helpdeskToken, "POST", "/api/v1/users/"+helpdesk.ID+"/identity-proofs", check), 422)
	expect(t, call(t, h, helpdeskToken, "POST", path(protected), check), 403)
	expect(t, call(t, h, scoped, "POST", path(outside), check), 403)
	expect(t, call(t, h, helpdeskToken, "POST", path(ada), map[string]any{"document": "passport", "note": ""}), 422)
	expect(t, call(t, h, helpdeskToken, "POST", path(ada), map[string]any{"document": "library-card", "note": "Checked"}), 422)
	expect(t, call(t, h, helpdeskToken, "POST", path(ada), map[string]any{"document": "passport", "note": "Passport number 123456789"}), 422)
	if list := decode[[]store.IdentityProof](t, call(t, h, appAdmin, "GET", path(ada), nil)); len(list) != 0 {
		t.Fatalf("refused checks were recorded: %+v", list)
	}

	got := decode[store.IdentityProof](t, call(t, h, scoped, "POST", path(ada), check))
	if got.Method != "in-person" || got.Status != "verified" || got.Document != "passport" || got.CompletedAt == nil || got.CheckedBy == nil || got.CheckedBy.Name != "sue" {
		t.Fatalf("recorded check: %+v", got)
	}
	expect(t, call(t, h, global, "POST", path(protected), check), 201)
	list := decode[[]store.IdentityProof](t, call(t, h, appAdmin, "GET", path(ada), nil))
	if len(list) != 1 || list[0].ID != got.ID {
		t.Fatalf("listed checks: %+v", list)
	}
	audit, err := st.ListAudit(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	recorded := 0
	for _, a := range audit {
		if a.Action == "user.identity_proof" && a.TargetID == ada.ID && a.Summary == "Checked the identity of Ada Lovelace in person with a passport" {
			recorded++
		}
	}
	if recorded != 1 {
		t.Fatalf("audit events for the check: %d", recorded)
	}

	settings, _ := st.Settings(ctx)
	settings.IdentityProofingGroupIDs = []string{other.ID}
	if err := st.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	st.ForgetSettings()
	rec := call(t, h, global, "DELETE", "/api/v1/groups/"+other.ID, nil)
	expect(t, rec, 409)
	if !strings.Contains(rec.Body.String(), "Identity checks") {
		t.Fatalf("group in use by the identity check requirement: %s", rec.Body)
	}
}

func TestRestoringAnInvitationRequiresApplicableIdentityCheck(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "uma@example.com", "user_admin")
	group, err := st.CreateGroup(ctx, store.NewGroup{Name: "Staff", Kind: "assigned"})
	must(t, err)
	settings, err := st.CurrentSettings(ctx)
	must(t, err)
	settings.IdentityProofingInvites = true
	settings.IdentityProofingGroupIDs = []string{group.ID}
	must(t, st.SaveSettings(ctx, settings))

	pending, err := st.CreateUser(ctx, store.NewUser{Email: "pat@example.com", Name: "Pat", GroupIDs: []string{group.ID}})
	must(t, err)
	must(t, st.SetPassword(ctx, pending.ID, "test-password-hash", false))
	if err := st.TouchSignIn(ctx, pending.ID, time.Now()); !errors.Is(err, store.ErrProofRequired) {
		t.Fatalf("first sign-in must check proofing: %v", err)
	}
	must(t, st.SetUserStatus(ctx, pending.ID, "suspended"))

	outside, err := st.CreateUser(ctx, store.NewUser{Email: "eli@example.com", Name: "Eli"})
	must(t, err)
	must(t, st.SetPassword(ctx, outside.ID, "test-password-hash", false))
	must(t, st.SetUserStatus(ctx, outside.ID, "suspended"))

	path := "/api/v1/users/" + pending.ID
	rec := call(t, h, admin, "POST", path+"/restore", nil)
	expect(t, rec, 409)
	if !strings.Contains(rec.Body.String(), "ERR_IDENTITY_CHECK_REQUIRED") {
		t.Fatalf("restore response: %s", rec.Body)
	}
	rec = call(t, h, admin, "POST", "/api/v1/users/bulk/restore", map[string]any{"userIds": []string{pending.ID, outside.ID}})
	expect(t, rec, 200)
	codes := decode[bulkResponse](t, rec).codes()
	if codes[pending.ID] != "ERR_IDENTITY_CHECK_REQUIRED" || codes[outside.ID] != "ok" || status(t, st, pending.ID) != "suspended" || status(t, st, outside.ID) != "active" {
		t.Fatalf("bulk restore: %s", rec.Body)
	}

	expect(t, call(t, h, admin, "POST", path+"/identity-proofs", map[string]string{"document": "passport", "note": "Checked the photo in person"}), 201)
	expect(t, call(t, h, admin, "POST", path+"/restore", nil), 200)
	if got := status(t, st, pending.ID); got != "active" {
		t.Fatalf("verified account status: %s", got)
	}
}

func TestSignUpApprovalNeedsIdentityCheck(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "admin@example.com", "user_admin")
	_, helpdesk := person(t, st, "hana@example.com", "helpdesk_admin")
	settings, _ := st.Settings(ctx)
	settings.SignUp, settings.IdentityProofingSignUps = "approval", true
	if err := st.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	st.ForgetSettings()
	grace, mallory := pendingSignUp(t, st, "grace@example.com"), pendingSignUp(t, st, "mallory@example.com")
	check := map[string]any{"document": "driving-licence", "note": "Video call, compared the photo with the person"}

	rec := call(t, h, admin, "POST", "/api/v1/sign-ups/"+grace.ID+"/approve", nil)
	expect(t, rec, 409)
	if _, err := st.GetUserByEmail(ctx, "grace@example.com"); err == nil {
		t.Fatal("a sign-up without an identity check became an account")
	}
	expect(t, call(t, h, helpdesk, "POST", "/api/v1/sign-ups/"+grace.ID+"/identity-proofs", check), 403)
	expect(t, call(t, h, admin, "POST", "/api/v1/sign-ups/"+grace.ID+"/identity-proofs", check), 201)
	expect(t, call(t, h, admin, "POST", "/api/v1/sign-ups/"+mallory.ID+"/identity-proofs", check), 201)
	queue := decode[[]store.SignUp](t, call(t, h, admin, "GET", "/api/v1/sign-ups", nil))
	if len(queue) != 2 || queue[0].Identity != "verified" {
		t.Fatalf("queue shows the check: %+v", queue)
	}

	u := decode[store.User](t, call(t, h, admin, "POST", "/api/v1/sign-ups/"+grace.ID+"/approve", nil))
	if u.Status != "active" {
		t.Fatalf("approved account: %+v", u)
	}
	moved := decode[[]store.IdentityProof](t, call(t, h, admin, "GET", "/api/v1/users/"+u.ID+"/identity-proofs", nil))
	if len(moved) != 1 || moved[0].Document != "driving-licence" {
		t.Fatalf("the check must follow the sign-up to the account: %+v", moved)
	}

	expect(t, call(t, h, admin, "DELETE", "/api/v1/sign-ups/"+mallory.ID, nil), 204)
	if _, err := st.IdentityCheck(ctx, "", mallory.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("rejecting a sign-up must delete its identity checks: %v", err)
	}

	settings.IdentityProofingSignUps = false
	if err := st.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	st.ForgetSettings()
	plain := pendingSignUp(t, st, "plain@example.com")
	expect(t, call(t, h, admin, "POST", "/api/v1/sign-ups/"+plain.ID+"/approve", nil), 200)
}

func TestRenamedSignUpLosesItsIdentityCheck(t *testing.T) {
	st, _ := setup(t)
	ctx := context.Background()
	token, _, err := st.RequestSignUp(ctx, "lin@example.com", "Lin Wu")
	if err != nil {
		t.Fatal(err)
	}
	su, err := st.PeekSignUp(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateIdentityProof(ctx, store.NewIdentityProof{SignUpID: su.ID, Method: "provider", Provider: "generic", Reference: "ses_1", Status: "verified"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.RequestSignUp(ctx, "lin@example.com", "Lin Wu"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.IdentityCheck(ctx, "", su.ID); err != nil {
		t.Fatalf("a new link for the same name must keep the check: %v", err)
	}
	if _, _, err := st.RequestSignUp(ctx, "LIN@example.com", "Someone Else"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.IdentityCheck(ctx, "", su.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("a sign-up under another name must lose the check: %v", err)
	}
}
