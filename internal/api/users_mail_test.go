package api_test

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"halo/internal/api"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/mail/mailtest"
	"halo/internal/store"
	"halo/internal/testdb"
)

type linkResponse struct {
	EnrollURL string `json:"enrollUrl"`
	Emailed   bool   `json:"emailed"`
}

func mailed(t *testing.T) (*store.Store, http.Handler, *mailtest.Server) {
	t.Helper()
	server := mailtest.Start(t, 0)
	host, port, _ := net.SplitHostPort(server.Addr)
	st := testdb.New(t)
	public, _ := url.Parse("http://localhost:3200")
	cfg := config.Config{PublicURL: public, Dev: true, Organization: "Fernway Systems", SMTP: config.SMTP{Host: host, Port: port, From: "Halo <halo@fernway.example>"}}
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	mux := http.NewServeMux()
	if err := api.Register(mux, httpx.Deps{Config: cfg, Store: st, Auth: authn}); err != nil {
		t.Fatal(err)
	}
	return st, authn.Resolve(authn.SameOrigin(mux)), server
}

func TestInviteAndResetAreEmailed(t *testing.T) {
	st, h, server := mailed(t)
	_, admin := person(t, st, "uma@example.com", "user_admin")

	rec := call(t, h, admin, "POST", "/api/v1/users", map[string]any{"email": "sam@example.com", "name": "Sam Lee"})
	expect(t, rec, 201)
	invite := decode[struct {
		linkResponse
		User struct{ ID string } `json:"user"`
	}](t, rec)
	rec = call(t, h, admin, "POST", "/api/v1/users/"+invite.User.ID+"/reset-authentication", nil)
	expect(t, rec, 200)
	reset := decode[linkResponse](t, rec)

	got := server.Deliveries()
	if !invite.Emailed || !reset.Emailed || len(got) != 2 {
		t.Fatalf("emailed: invite %v, reset %v, %d deliveries", invite.Emailed, reset.Emailed, len(got))
	}
	for i, want := range []struct{ subject, link string }{{"Set up your Halo account", invite.EnrollURL}, {"Reset how you sign in to Halo", reset.EnrollURL}} {
		if got[i].To != "TO:<sam@example.com>" || !strings.Contains(got[i].Data, "Subject: "+want.subject) || !strings.Contains(got[i].Data, "token=") {
			t.Errorf("delivery %d: %+v", i, got[i])
		}
	}
	messages, err := st.ListMail(context.Background(), 2)
	if err != nil || len(messages) != 2 || !strings.Contains(messages[1].Text, invite.EnrollURL) || !strings.Contains(messages[0].Text, reset.EnrollURL) || !strings.Contains(messages[0].Text, "uma reset how you sign in") {
		t.Fatalf("outbox: %+v %v", messages, err)
	}
}

func TestInviteWithoutSMTPIsNotEmailed(t *testing.T) {
	st, h := setup(t)
	_, admin := person(t, st, "uma@example.com", "user_admin")
	rec := call(t, h, admin, "POST", "/api/v1/users", map[string]any{"email": "sam@example.com", "name": "Sam Lee"})
	expect(t, rec, 201)
	out := decode[linkResponse](t, rec)
	messages, err := st.ListMail(context.Background(), 1)
	if out.Emailed || out.EnrollURL == "" || err != nil || len(messages) != 1 || messages[0].SentAt != nil || !strings.Contains(messages[0].Text, out.EnrollURL) {
		t.Fatalf("invite without SMTP: %+v, outbox %+v %v", out, messages, err)
	}
}

func TestEmailChangeAndRemovedMethodAreEmailed(t *testing.T) {
	st, h, server := mailed(t)
	ctx := context.Background()
	_, admin := person(t, st, "uma@example.com", "user_admin")
	sam, _ := person(t, st, "sam@example.com")
	key, err := st.AddWebAuthnCredential(ctx, store.WebAuthnCredential{UserID: sam.ID, Kind: "security-key", Label: "YubiKey", CredentialID: []byte("credential"), PublicKey: []byte("public-key"), Transports: []string{}})
	if err != nil {
		t.Fatal(err)
	}

	expect(t, call(t, h, admin, "PATCH", "/api/v1/users/"+sam.ID, map[string]any{"email": "sam.lee@example.com"}), 200)
	expect(t, call(t, h, admin, "PATCH", "/api/v1/users/"+sam.ID, map[string]any{"title": "Analyst"}), 200)
	expect(t, call(t, h, admin, "DELETE", "/api/v1/users/"+sam.ID+"/methods/"+key.ID, nil), 204)
	settings := store.DefaultSettings
	settings.SecurityEmailsOff = []string{"email_changed"}
	if err := st.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	st.ForgetSettings()
	expect(t, call(t, h, admin, "PATCH", "/api/v1/users/"+sam.ID, map[string]any{"email": "sam@example.com"}), 200)

	got := server.Deliveries()
	want := []struct{ to, subject, text string }{
		{"sam@example.com", "The email address of your Halo account changed", "uma changed the email address of your Halo account from sam@example.com to sam.lee@example.com"},
		{"sam.lee@example.com", "The email address of your Halo account changed", "uma changed the email address of your Halo account from sam@example.com to sam.lee@example.com"},
		{"sam.lee@example.com", "A sign-in method was removed from your Halo account", "YubiKey was removed from your Halo account by uma on"},
	}
	if len(got) != len(want) {
		t.Fatalf("deliveries: %+v", got)
	}
	for i, w := range want {
		if got[i].To != "TO:<"+w.to+">" || !strings.Contains(got[i].Data, "Subject: "+w.subject) || !strings.Contains(strings.NewReplacer("=\r\n", "", "=\n", "").Replace(got[i].Data), w.text) {
			t.Errorf("delivery %d: want %s %q, got %+v", i, w.to, w.subject, got[i])
		}
	}
}
