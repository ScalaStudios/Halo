package mail

import (
	"context"
	"slices"
	"testing"
	"time"

	"halo/internal/store"
	"halo/internal/testdb"
)

type recorder struct{ sent []Message }

func (r *recorder) Send(_ context.Context, m Message) error {
	r.sent = append(r.sent, m)
	return nil
}

func (*recorder) Configured() bool { return true }

func (r *recorder) take() []string {
	to := []string{}
	for _, m := range r.sent {
		to = append(to, m.kind+" "+m.To)
	}
	r.sent = nil
	return to
}

func TestTemplatesListUsableVariables(t *testing.T) {
	for _, tmpl := range Templates {
		if err := tmpl.Validate(tmpl.Default); err != nil {
			t.Errorf("%s: %v", tmpl.Kind, err)
		}
		for _, v := range tmpl.Variables {
			if v.Name == "" || v.Description == "" || v.Example == "" {
				t.Errorf("%s lists an unknown variable: %+v", tmpl.Kind, v)
			}
		}
		if tmpl.Security && slices.ContainsFunc(tmpl.Variables, func(v Variable) bool { return v.Name == "Code" }) {
			t.Errorf("%s is a security email and must not carry a sign-in code", tmpl.Kind)
		}
	}
}

func TestSecurityMessages(t *testing.T) {
	at := time.Date(2026, 3, 4, 13, 45, 0, 0, time.UTC)
	portal := "https://halo.example.com/account/security"
	changed := EmailChanged("Ada Lovelace", "ada@old.example.com", "ada@example.com", "Grace Hopper", "https://halo.example.com/account", at)
	for _, c := range []struct {
		name          string
		got           Message
		subject, text string
	}{
		{"new-sign-in", NewSignIn("ada@example.com", "Ada Lovelace", "Grafana", "Firefox 143 · Linux", "203.0.113.24", "https://halo.example.com/account/sessions", at), "New sign-in to your Halo account",
			"Hi Ada,\n\nYour Halo account signed in to Grafana from a device or network it hasn't used before.\n\nWhen: 4 March 2026, 13:45 UTC\nDevice: Firefox 143 · Linux\nIP address: 203.0.113.24\n\nIf this was you, you don't need to do anything. If it wasn't, sign out that device in the account portal, replace your passkeys and tell your administrator:\n\nhttps://halo.example.com/account/sessions"},
		{"method-added", MethodAdded("ada@example.com", "Ada", "YubiKey 5C", portal, at), "A sign-in method was added to your Halo account",
			"Hi Ada,\n\nYubiKey 5C was added to your Halo account on 4 March 2026, 13:45 UTC and can now sign in as you.\n\nIf you didn't add it, remove it in the account portal and tell your administrator:\n\n" + portal},
		{"method-removed-by-admin", MethodRemoved("ada@example.com", "Ada", "YubiKey 5C", "Grace Hopper", portal, at), "A sign-in method was removed from your Halo account",
			"Hi Ada,\n\nYubiKey 5C was removed from your Halo account by Grace Hopper on 4 March 2026, 13:45 UTC and no longer signs you in.\n\nIf you didn't expect this, check your sign-in methods in the account portal and tell your administrator:\n\n" + portal},
		{"recovery-codes", RecoveryCodes("ada@example.com", "", portal, at), "New recovery codes for your Halo account",
			"Hi,\n\nNew recovery codes were generated for your Halo account on 4 March 2026, 13:45 UTC, and your previous codes no longer work.\n\nIf you didn't do this, generate new codes in the account portal and tell your administrator:\n\n" + portal},
		{"email-changed-old", changed[0], "The email address of your Halo account changed",
			"Hi Ada,\n\nGrace Hopper changed the email address of your Halo account from ada@old.example.com to ada@example.com on 4 March 2026, 13:45 UTC. Halo sent this email to both addresses.\n\nIf you didn't expect this, contact your administrator. Review your account in the account portal:\n\nhttps://halo.example.com/account"},
		{"locked", AccountLocked("ada@example.com", "Ada", 5, at, "https://halo.example.com/account/activity"), "Your Halo account is locked",
			"Hi Ada,\n\nHalo locked your account after 5 incorrect passwords or codes. You can try again after 4 March 2026, 13:45 UTC.\n\nIf these attempts weren't yours, someone may be guessing your password. Review your recent sign-ins in the account portal and tell your administrator:\n\nhttps://halo.example.com/account/activity"},
		{"locked-admin", AccountLockedAdmin("grace@example.com", "Ada Lovelace", "ada@example.com", 5, at, "https://halo.example.com/admin/users/usr_1"), "The Halo account of Ada Lovelace is locked",
			"Halo locked the account of Ada Lovelace (ada@example.com) after 5 incorrect passwords or codes. They can try again after 4 March 2026, 13:45 UTC.\n\nSee the person and their recent sign-ins in Halo: https://halo.example.com/admin/users/usr_1"},
		{"impersonation", Impersonation("ada@example.com", "Ada", "Hana Ito", "Can't open Grafana", "https://halo.example.com/account/sessions", at), "Hana Ito is viewing Halo as you",
			"Hi Ada,\n\nHana Ito started viewing your Halo account as you, to help with: Can't open Grafana\n\nThey can look but not change anything, and their access ends by 4 March 2026, 13:45 UTC. If you did not expect this, tell your IT team, or end it now by signing out of your other sessions:\n\nhttps://halo.example.com/account/sessions"},
	} {
		if c.got.Subject != c.subject || c.got.Text != c.text {
			t.Errorf("%s:\n got %q\n     %q\nwant %q\n     %q", c.name, c.got.Subject, c.got.Text, c.subject, c.text)
		}
	}
	if len(changed) != 2 || changed[0].To != "ada@old.example.com" || changed[1].To != "ada@example.com" || changed[1].Text != changed[0].Text {
		t.Errorf("email change goes to both addresses: %+v", changed)
	}
	if same := EmailChanged("Ada", "Ada@Example.com", "ada@example.com", "Grace Hopper", "https://halo.example.com/account", at); len(same) != 1 {
		t.Errorf("a change of case emails one mailbox once: %+v", same)
	}
}

func TestSendSecurity(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	ada, err := st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	org := func(change func(*store.Settings)) {
		s := store.DefaultSettings
		change(&s)
		if err := st.SaveSettings(ctx, s); err != nil {
			t.Fatal(err)
		}
		st.ForgetSettings()
	}
	box, at, portal := &recorder{}, time.Now(), "https://halo.example.com/account/security"
	expect := func(want ...string) {
		t.Helper()
		if got := box.take(); !slices.Equal(got, want) {
			t.Fatalf("sent %q, want %q", got, want)
		}
	}

	SendSecurity(ctx, box, st, ada.ID, "met_1", MethodAdded(ada.Email, ada.Name, "YubiKey", portal, at))
	SendSecurity(ctx, box, st, ada.ID, "met_1", MethodAdded(ada.Email, ada.Name, "YubiKey", portal, at))
	expect("method_added ada@example.com")
	SendSecurity(ctx, box, st, ada.ID, "met_2", MethodAdded(ada.Email, ada.Name, "Phone", portal, at))
	expect("method_added ada@example.com")

	SendSecurity(ctx, box, st, ada.ID, "ada@new.example.com", EmailChanged(ada.Name, ada.Email, "ada@new.example.com", "Grace Hopper", portal, at)...)
	expect("email_changed ada@example.com", "email_changed ada@new.example.com")

	if err := st.SetSecurityEmailOptOuts(ctx, ada.ID, []string{"new_sign_in"}); err != nil {
		t.Fatal(err)
	}
	SendSecurity(ctx, box, st, ada.ID, "203.0.113.24", NewSignIn(ada.Email, ada.Name, "", "Firefox · Linux", "203.0.113.24", portal, at))
	expect()

	org(func(s *store.Settings) { s.SecurityEmailsOff = []string{"recovery_codes"} })
	SendSecurity(ctx, box, st, ada.ID, "", RecoveryCodes(ada.Email, ada.Name, portal, at))
	expect()

	locked := func(u store.User) {
		SendSecurity(ctx, box, st, u.ID, "", AccountLocked(u.Email, u.Name, 5, at, portal), AccountLockedAdmin("grace@example.com", u.Name, u.Email, 5, at, portal))
	}
	locked(ada)
	locked(ada)
	expect("account_locked ada@example.com")
	bob, err := st.CreateUser(ctx, store.NewUser{Email: "bob@example.com", Name: "Bob", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	org(func(s *store.Settings) { s.LockoutEmailAdmins = true })
	locked(bob)
	expect("account_locked bob@example.com", "account_locked_admin grace@example.com")

	t0 := time.Date(2026, 3, 4, 13, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		at   time.Time
		want bool
	}{{t0, true}, {t0.Add(10 * time.Minute), false}, {t0.Add(16 * time.Minute), true}} {
		if claimed, err := st.ClaimSecurityEmail(ctx, bob.ID, "new_sign_in", "198.51.100.7", c.at, 15*time.Minute); err != nil || claimed != c.want {
			t.Fatalf("claim at %s: %v %v", c.at, claimed, err)
		}
	}
}
