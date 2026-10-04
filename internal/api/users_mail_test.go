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
	"halo/internal/testdb"
)

type linkResponse struct {
	EnrollURL string `json:"enrollUrl"`
	Emailed   bool   `json:"emailed"`
}

func TestInviteAndResetAreEmailed(t *testing.T) {
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
	h := authn.Resolve(authn.SameOrigin(mux))
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
