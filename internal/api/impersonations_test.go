package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"halo/internal/api"
	"halo/internal/apikeys"
	"halo/internal/approval"
	"halo/internal/auth"
	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/mail"
	"halo/internal/store"
	"halo/internal/testdb"
)

type mailbox struct{ sent []mail.Message }

func (m *mailbox) Send(_ context.Context, msg mail.Message) error {
	m.sent = append(m.sent, msg)
	return nil
}

func (m *mailbox) Configured() bool { return true }

type impersonationEnv struct {
	t     *testing.T
	st    *store.Store
	h     http.Handler
	box   *mailbox
	group store.Group
}

func impersonationSetup(t *testing.T) *impersonationEnv {
	t.Helper()
	st := testdb.New(t)
	public, _ := url.Parse("http://localhost:3200")
	authn := &httpx.Auth{Store: st, Public: public, Dev: true, Keys: apikeys.Resolver(st)}
	box := &mailbox{}
	d := httpx.Deps{Config: config.Config{PublicURL: public, Dev: true}, Store: st, Auth: authn, Mailer: box}
	mux := http.NewServeMux()
	for _, register := range []func(*http.ServeMux, httpx.Deps) error{auth.Register, api.Register, approval.Register} {
		if err := register(mux, d); err != nil {
			t.Fatal(err)
		}
	}
	group, err := st.CreateGroup(context.Background(), store.NewGroup{Name: "Frontline", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	e := &impersonationEnv{t: t, st: st, h: authn.Resolve(authn.KeyScope("api", authn.SameOrigin(mux))), box: box, group: group}
	e.configure(map[string]any{"impersonationGroupIds": []string{group.ID}})
	return e
}

func (e *impersonationEnv) configure(values map[string]any) {
	e.t.Helper()
	raw := map[string]json.RawMessage{}
	for key, value := range values {
		raw[key], _ = json.Marshal(value)
	}
	if err := e.st.SaveSettingValues(context.Background(), raw); err != nil {
		e.t.Fatal(err)
	}
	e.st.ForgetSettings()
}

func (e *impersonationEnv) person(email string, inGroup bool, roles ...string) (store.User, string) {
	e.t.Helper()
	in := store.NewUser{Email: email, Name: strings.Split(email, "@")[0], Status: "active", Roles: roles}
	if inGroup {
		in.GroupIDs = []string{e.group.ID}
	}
	u, err := e.st.CreateUser(context.Background(), in)
	if err != nil {
		e.t.Fatal(err)
	}
	return u, session(e.t, e.st, u.ID)
}

func (e *impersonationEnv) call(credential, method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	encoded, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if strings.HasPrefix(credential, "hlk_") {
		req.Header.Set("Authorization", "Bearer "+credential)
	} else {
		req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: credential})
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e *impersonationEnv) start(admin string, target store.User) (store.Impersonation, string) {
	e.t.Helper()
	grant := decode[store.Impersonation](e.t, expectStatus(e.t, e.call(admin, "POST", "/api/v1/users/"+target.ID+"/impersonations", map[string]any{"reason": "Ticket 4512: can't open Payroll"}), 201))
	rec := expectStatus(e.t, e.call(admin, "POST", "/api/v1/impersonations/"+grant.ID+"/start", nil), 200)
	for _, c := range rec.Result().Cookies() {
		if c.Name == httpx.SessionCookie && c.Value != "" {
			return grant, c.Value
		}
	}
	e.t.Fatal("starting an impersonation must set a session cookie")
	return grant, ""
}

func (e *impersonationEnv) audit(action string) []store.AuditEvent {
	e.t.Helper()
	events, err := e.st.ListAudit(context.Background(), 1000)
	if err != nil {
		e.t.Fatal(err)
	}
	var matched []store.AuditEvent
	for _, ev := range events {
		if ev.Action == action {
			matched = append(matched, ev)
		}
	}
	return matched
}

func expectStatus(t *testing.T, rec *httptest.ResponseRecorder, status int) *httptest.ResponseRecorder {
	t.Helper()
	expect(t, rec, status)
	return rec
}

func TestImpersonationRefusesForbiddenTargets(t *testing.T) {
	e := impersonationSetup(t)
	ctx := context.Background()
	hana, helpdesk := e.person("hana@example.com", true, "helpdesk_admin")
	_, global := e.person("gia@example.com", false, "global_admin")
	_, auditor := e.person("audrey@example.com", false, "auditor")
	pat, _ := e.person("pat@example.com", true)
	ada, _ := e.person("ada@example.com", true, "auditor")
	eli, _ := e.person("eli@example.com", true)
	if _, err := e.st.SetEligibleRoles(ctx, eli.ID, []string{"user_admin"}); err != nil {
		t.Fatal(err)
	}
	otto, _ := e.person("otto@example.com", false)
	bot, err := e.st.CreateServiceAccount(ctx, store.NewServiceAccount{Name: "Helpdesk bot", Roles: []string{"helpdesk_admin"}})
	if err != nil {
		t.Fatal(err)
	}
	_, key, err := e.st.CreateAPIKey(ctx, bot.ID, "automation", []string{"api"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	reason := map[string]any{"reason": "Ticket 4512"}
	ask := func(token string, target string) *httptest.ResponseRecorder {
		return e.call(token, "POST", "/api/v1/users/"+target+"/impersonations", reason)
	}

	expect(t, ask(helpdesk, hana.ID), 403)
	expect(t, ask(helpdesk, ada.ID), 403)
	expect(t, ask(global, ada.ID), 403)
	expect(t, ask(helpdesk, eli.ID), 403)
	expect(t, ask(helpdesk, otto.ID), 403)
	expect(t, ask(helpdesk, bot.ID), 403)
	expect(t, ask(auditor, pat.ID), 403)
	expect(t, ask(key, pat.ID), 403)
	expect(t, e.call(helpdesk, "POST", "/api/v1/users/"+pat.ID+"/impersonations", map[string]any{"reason": "  "}), 422)

	e.configure(map[string]any{"emergencyAccessIds": []string{pat.ID}})
	expect(t, ask(helpdesk, pat.ID), 403)
	e.configure(map[string]any{"emergencyAccessIds": []string{}, "impersonationGroupIds": []string{}})
	expect(t, ask(helpdesk, pat.ID), 403)
	e.configure(map[string]any{"impersonationGroupIds": []string{e.group.ID}})
	grant := decode[store.Impersonation](t, expectStatus(t, ask(helpdesk, pat.ID), 201))
	if grant.Status != "ready" || grant.Actor.ID != hana.ID || grant.User.ID != pat.ID {
		t.Fatalf("grant: %+v", grant)
	}
	expect(t, e.call(key, "POST", "/api/v1/impersonations/"+grant.ID+"/start", nil), 403)
	expect(t, e.call(global, "POST", "/api/v1/impersonations/"+grant.ID+"/start", nil), 403)
}

func TestImpersonationIsReadOnlyAndAuditedWithBothPeople(t *testing.T) {
	e := impersonationSetup(t)
	ctx := context.Background()
	hana, helpdesk := e.person("hana@example.com", false, "helpdesk_admin")
	pat, _ := e.person("pat@example.com", true)
	e.configure(map[string]any{"sessionLimit": 2, "sessionLimitAction": "refuse"})

	grant, token := e.start(helpdesk, pat)
	expect(t, e.call(helpdesk, "GET", "/api/v1/me", nil), 401)
	me := decode[struct {
		ID            string              `json:"id"`
		Impersonation store.Impersonation `json:"impersonation"`
	}](t, expectStatus(t, e.call(token, "GET", "/api/v1/me", nil), 200))
	if me.ID != pat.ID || me.Impersonation.Actor.ID != hana.ID || me.Impersonation.Status != "active" {
		t.Fatalf("me while impersonating: %+v", me)
	}
	expect(t, e.call(token, "GET", "/api/v1/me/sessions", nil), 200)

	for _, blocked := range []struct{ method, path string }{
		{"POST", "/api/v1/me/passkeys/begin"},
		{"PUT", "/api/v1/me/password"},
		{"POST", "/api/v1/me/totp/begin"},
		{"POST", "/api/v1/me/recovery-codes"},
		{"DELETE", "/api/v1/me/methods/mth_any"},
		{"POST", "/api/v1/me/sessions/revoke-others"},
		{"POST", "/api/v1/approvals/apr_any/approve"},
		{"POST", "/api/v1/users/" + pat.ID + "/impersonations"},
		{"GET", "/api/v1/users"},
		{"GET", "/api/v1/auth/federated/idp_any/start"},
	} {
		rec := e.call(token, blocked.method, blocked.path, map[string]any{})
		if rec.Code != 403 || !strings.Contains(rec.Body.String(), "ERR_IMPERSONATING") {
			t.Fatalf("%s %s while impersonating: %d %s", blocked.method, blocked.path, rec.Code, rec.Body)
		}
	}

	if err := e.st.MakeRoomForSession(ctx, pat, false, ""); err != nil {
		t.Fatalf("the impersonation must not use up %s's session limit: %v", pat.Name, err)
	}
	started := e.audit("impersonation.start")
	if len(started) != 1 || *started[0].ActorID != hana.ID || started[0].TargetID != pat.ID || !strings.Contains(started[0].Summary, "Payroll") {
		t.Fatalf("start audit: %+v", started)
	}
	signIns, err := e.st.ListSignIns(ctx, store.SignInFilter{UserID: pat.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(signIns) != 1 || signIns[0].ImpersonatorID == nil || *signIns[0].ImpersonatorID != hana.ID {
		t.Fatalf("sign-in log: %+v", signIns)
	}
	if len(e.box.sent) != 1 || e.box.sent[0].To != pat.Email || !strings.Contains(e.box.sent[0].Text, "Payroll") {
		t.Fatalf("notification: %+v", e.box.sent)
	}

	expect(t, e.call(token, "POST", "/api/v1/impersonations/"+grant.ID+"/end", nil), 204)
	expect(t, e.call(token, "GET", "/api/v1/me", nil), 401)
	ended := e.audit("impersonation.end")
	if len(ended) != 1 || *ended[0].ActorID != hana.ID || ended[0].TargetID != pat.ID {
		t.Fatalf("end audit: %+v", ended)
	}
	expect(t, e.call(session(t, e.st, hana.ID), "POST", "/api/v1/impersonations/"+grant.ID+"/start", nil), 409)
}

func TestImpersonationExpiresOnTheNextRequest(t *testing.T) {
	e := impersonationSetup(t)
	ctx := context.Background()
	_, helpdesk := e.person("hana@example.com", false, "helpdesk_admin")
	pat, _ := e.person("pat@example.com", true)
	grant := decode[store.Impersonation](t, expectStatus(t, e.call(helpdesk, "POST", "/api/v1/users/"+pat.ID+"/impersonations", map[string]any{"reason": "Ticket 4512"}), 201))
	ends := time.Now().Add(time.Second)
	_, token, err := e.st.StartImpersonation(ctx, grant.ID, store.NewSession{UserID: pat.ID, Method: "passkey"}, ends)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, e.call(token, "GET", "/api/v1/me", nil), 200)
	time.Sleep(time.Until(ends) + 50*time.Millisecond)
	expect(t, e.call(token, "GET", "/api/v1/me", nil), 401)
	if _, _, err := e.st.StartImpersonation(ctx, grant.ID, store.NewSession{UserID: pat.ID, Method: "passkey"}, time.Now().Add(time.Hour)); err != store.ErrConflict {
		t.Fatalf("an impersonation must start once: %v", err)
	}
}

func TestImpersonationEndsWhenThePersonBecomesProtected(t *testing.T) {
	e := impersonationSetup(t)
	_, helpdesk := e.person("hana@example.com", false, "helpdesk_admin")
	pat, _ := e.person("pat@example.com", true)
	_, token := e.start(helpdesk, pat)
	expect(t, e.call(token, "GET", "/api/v1/me", nil), 200)
	if _, err := e.st.SetEligibleRoles(context.Background(), pat.ID, []string{"auditor"}); err != nil {
		t.Fatal(err)
	}
	expect(t, e.call(token, "GET", "/api/v1/me", nil), 401)
}

func TestImpersonationWaitsForApproval(t *testing.T) {
	e := impersonationSetup(t)
	hana, helpdesk := e.person("hana@example.com", false, "helpdesk_admin")
	_, ben := e.person("ben@example.com", false, "global_admin")
	uma, userAdmin := e.person("uma@example.com", false, "user_admin")
	pat, _ := e.person("pat@example.com", true)
	e.configure(map[string]any{"approvalActions": []string{"impersonation"}})

	held := decode[store.ApprovalRequest](t, expectStatus(t, e.call(helpdesk, "POST", "/api/v1/users/"+pat.ID+"/impersonations", map[string]any{"reason": "Ticket 4512"}), 202))
	if held.Status != "pending" || held.Action != "impersonation" || held.TargetID != pat.ID {
		t.Fatalf("held request: %+v", held)
	}
	if list := decode[[]store.Impersonation](t, expectStatus(t, e.call(helpdesk, "GET", "/api/v1/impersonations?user="+pat.ID, nil), 200)); len(list) != 0 {
		t.Fatalf("nothing may be granted before approval: %+v", list)
	}
	expect(t, e.call(helpdesk, "POST", "/api/v1/approvals/"+held.ID+"/approve", nil), 403)
	expect(t, e.call(ben, "POST", "/api/v1/approvals/"+held.ID+"/approve", nil), 200)
	list := decode[[]store.Impersonation](t, expectStatus(t, e.call(helpdesk, "GET", "/api/v1/impersonations?user="+pat.ID, nil), 200))
	if len(list) != 1 || list[0].Status != "ready" || list[0].Actor.ID != hana.ID {
		t.Fatalf("approved grant: %+v", list)
	}

	rec := expectStatus(t, e.call(helpdesk, "POST", "/api/v1/impersonations/"+list[0].ID+"/start", nil), 200)
	token := ""
	for _, c := range rec.Result().Cookies() {
		if c.Name == httpx.SessionCookie {
			token = c.Value
		}
	}
	expect(t, e.call(token, "GET", "/api/v1/me", nil), 200)
	expect(t, e.call(userAdmin, "POST", "/api/v1/impersonations/"+list[0].ID+"/end", nil), 204)
	expect(t, e.call(token, "GET", "/api/v1/me", nil), 401)
	ended := e.audit("impersonation.end")
	if len(ended) != 1 || *ended[0].ActorID != uma.ID || ended[0].TargetID != pat.ID || !strings.Contains(ended[0].Summary, "hana") {
		t.Fatalf("console end audit: %+v", ended)
	}
}
