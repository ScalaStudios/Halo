package governance

import (
	"context"
	"net/url"
	"slices"
	"testing"
	"time"

	"halo/internal/config"
	"halo/internal/store"
)

func TestGuestExpiryNoticesSponsorThenSuspends(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	public, _ := url.Parse("http://localhost:3200")
	h := &handler{st: e.st, cfg: config.Config{PublicURL: public}, mailer: e.box}
	_, admin := e.person("gia@example.com", "global_admin")
	sponsor, _ := e.person("sam@example.com")
	leaver, _ := e.person("lea@example.com")
	now := time.Now()
	guest := func(email string, sponsorID string, until time.Time) store.User {
		t.Helper()
		u, err := e.st.CreateUser(ctx, store.NewUser{Email: email, Name: email, Status: "active", Guest: true, SponsorID: &sponsorID, ExpiresAt: &until})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	pat := guest("pat@partner.example", sponsor.ID, now.Add(3*24*time.Hour))
	lee := guest("lee@partner.example", leaver.ID, now.Add(5*24*time.Hour))
	guest("kim@partner.example", sponsor.ID, now.Add(60*24*time.Hour))
	if err := e.st.SetUserStatus(ctx, leaver.ID, "deprovisioned"); err != nil {
		t.Fatal(err)
	}
	if err := runLifecycle(ctx, e.st); err != nil {
		t.Fatal(err)
	}
	expect(t, e.call(admin, "POST", "/api/v1/lifecycle/rules", map[string]any{"name": "Leavers", "trigger": "leaver", "enabled": true, "actions": []map[string]string{{"type": "revoke-sessions"}}}), 201)

	for range 2 {
		if err := h.noticeGuestExpiry(ctx, now); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.box.to(sponsor.Email); len(got) != 1 || got[0].Subject != "Guest access for pat@partner.example ends soon" {
		t.Fatalf("sponsor notices: %+v", got)
	}
	if got := e.box.to("gia@example.com"); len(got) != 1 || got[0].Subject != "Guest access for lee@partner.example ends soon" {
		t.Fatalf("an inactive sponsor's guest must be reported to global administrators: %+v", got)
	}
	if got := e.box.to(leaver.Email); len(got) != 0 {
		t.Fatalf("deprovisioned sponsor was emailed: %+v", got)
	}

	if err := expireGuests(ctx, e.st, now.Add(6*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{pat.ID, lee.ID} {
		if u, _ := e.st.GetUser(ctx, id); u.Status != "suspended" {
			t.Fatalf("%s is %s after expiry", u.Email, u.Status)
		}
	}
	events, err := e.st.ListAudit(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	expired := 0
	for _, ev := range events {
		if ev.Action == "guest.expire" {
			expired++
		}
	}
	if expired != 2 {
		t.Fatalf("want 2 expiry events, got %d", expired)
	}
	if err := runLifecycle(ctx, e.st); err != nil {
		t.Fatal(err)
	}
	runs := decode[[]store.LifecycleRun](t, e.call(admin, "GET", "/api/v1/lifecycle/runs", nil))
	if !slices.ContainsFunc(runs, func(r store.LifecycleRun) bool { return r.Trigger == "leaver" && r.User.ID == pat.ID }) {
		t.Fatalf("expiry did not run leaver rules: %+v", runs)
	}
}

func TestLifecycleRulesDoNotAddGuestsToGroups(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, admin := e.person("uma@example.com", "user_admin")
	if err := runLifecycle(ctx, e.st); err != nil {
		t.Fatal(err)
	}
	everyone := e.group("Everyone")
	expect(t, e.call(admin, "POST", "/api/v1/lifecycle/rules", map[string]any{"name": "All joiners", "trigger": "joiner", "enabled": true, "actions": []map[string]string{{"type": "add-to-group", "groupId": everyone.ID}}}), 201)
	sponsor, _ := e.person("sam@example.com")
	until := time.Now().Add(24 * time.Hour)
	pat, err := e.st.CreateUser(ctx, store.NewUser{Email: "pat@partner.example", Name: "Pat", Status: "active", Guest: true, SponsorID: &sponsor.ID, ExpiresAt: &until})
	if err != nil {
		t.Fatal(err)
	}
	if err := runLifecycle(ctx, e.st); err != nil {
		t.Fatal(err)
	}
	if e.member(everyone.ID, pat.ID) {
		t.Fatal("a joiner rule added a guest to a group")
	}
	if !e.member(everyone.ID, sponsor.ID) {
		t.Fatal("the joiner rule stopped working for members")
	}
}
