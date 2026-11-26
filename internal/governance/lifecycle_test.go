package governance

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"halo/internal/store"
)

func scheduledRuns(t *testing.T, e *env, admin string, now time.Time) []store.LifecycleRun {
	t.Helper()
	if err := runScheduledLifecycle(context.Background(), e.st, now); err != nil {
		t.Fatal(err)
	}
	return decode[[]store.LifecycleRun](t, e.call(admin, "GET", "/api/v1/lifecycle/runs", nil))
}

func emergency(t *testing.T, st *store.Store, ids ...string) {
	t.Helper()
	ctx := context.Background()
	settings, err := st.CurrentSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.EmergencyAccessIDs = ids
	if err := st.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
}

func TestDateLifecycleRulesFireOncePerDate(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	now := time.Now().UTC()
	day := func(days int) string { return now.AddDate(0, 0, days).Format(time.DateOnly) }
	_, admin := e.person("uma@example.com", "user_admin")
	for _, a := range []store.Attribute{{Name: "endDate", Label: "End date", Type: "date"}, {Name: "leaveDate", Label: "Leave date", Type: "date", SelfService: true}, {Name: "badge", Label: "Badge", Type: "string"}} {
		if _, err := e.st.CreateAttribute(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	offboarding := e.group("Offboarding")
	add := []map[string]string{{"type": "add-to-group", "groupId": offboarding.ID}}
	for _, bad := range []map[string]any{
		{"attribute": "leaveDate", "days": 0},
		{"attribute": "badge", "days": 0},
		{"attribute": "missing", "days": 0},
		{"attribute": "endDate"},
		{"attribute": "endDate", "days": 4000},
	} {
		body := map[string]any{"name": "Bad", "trigger": "date", "enabled": true, "actions": add}
		for k, v := range bad {
			body[k] = v
		}
		expect(t, e.call(admin, "POST", "/api/v1/lifecycle/rules", body), 422)
	}
	expect(t, e.call(admin, "POST", "/api/v1/lifecycle/rules", map[string]any{"name": "Contract ending", "trigger": "date", "attribute": "endDate", "days": -7, "enabled": true, "actions": add}), 201)
	expect(t, e.call(admin, "POST", "/api/v1/lifecycle/rules", map[string]any{"name": "Contract ended", "trigger": "date", "attribute": "endDate", "days": 0, "enabled": true, "actions": []map[string]string{{"type": "suspend"}}}), 201)

	person := func(email, end string, roles ...string) store.User {
		t.Helper()
		u, _ := e.person(email, roles...)
		if err := e.st.SetUserAttributes(ctx, u.ID, map[string]any{"endDate": end}); err != nil {
			t.Fatal(err)
		}
		return u
	}
	ada := person("ada@example.com", day(5))
	bob := person("bob@example.com", day(10))
	cy := person("cy@example.com", day(-1))
	erin := person("erin@example.com", day(0))
	gia := person("gia@example.com", day(0), "global_admin")
	emergency(t, e.st, erin.ID)

	got := scheduledRuns(t, e, admin, now)
	if len(got) != 7 {
		t.Fatalf("want 7 runs, got %+v", got)
	}
	byPerson := map[string][]store.LifecycleRun{}
	for _, r := range got {
		if r.Trigger != "date" {
			t.Fatalf("trigger: %+v", r)
		}
		byPerson[r.User.ID] = append(byPerson[r.User.ID], r)
	}
	if runs := byPerson[ada.ID]; len(runs) != 1 || runs[0].Change != "7 days before End date ("+day(5)+")" || runs[0].Steps[0] != "Added to Offboarding" {
		t.Fatalf("ada: %+v", runs)
	}
	if len(byPerson[bob.ID]) != 0 {
		t.Fatalf("bob's date is not reached: %+v", byPerson[bob.ID])
	}
	if cy, _ := e.st.GetUser(ctx, cy.ID); cy.Status != "suspended" || len(byPerson[cy.ID]) != 2 {
		t.Fatalf("cy: %s %+v", cy.Status, byPerson[cy.ID])
	}
	for _, c := range []struct {
		u    store.User
		step string
	}{
		{erin, "Did not suspend erin because they are an emergency access account"},
		{gia, "Did not suspend gia because they hold or are eligible for an administrator role, so only a global administrator can change their account"},
	} {
		u, step := c.u, c.step
		runs := byPerson[u.ID]
		i := slices.IndexFunc(runs, func(r store.LifecycleRun) bool { return r.Rule.Name == "Contract ended" })
		if i < 0 || runs[i].Result != "failed" || runs[i].Change != "End date reached ("+day(0)+")" || !slices.Equal(runs[i].Steps, []string{step}) {
			t.Fatalf("%s must not be suspended: %+v", u.Email, runs)
		}
		if after, _ := e.st.GetUser(ctx, u.ID); after.Status != "active" {
			t.Fatalf("%s was suspended", u.Email)
		}
	}

	if again := scheduledRuns(t, e, admin, now); len(again) != 7 {
		t.Fatalf("date rules ran twice: %+v", again)
	}
	if later := scheduledRuns(t, e, admin, now.AddDate(0, 0, 4)); len(later) != 8 || later[0].User.ID != bob.ID {
		t.Fatalf("bob's date is reached four days later: %+v", later)
	}
	if err := e.st.SetUserAttributes(ctx, ada.ID, map[string]any{"endDate": day(6)}); err != nil {
		t.Fatal(err)
	}
	if moved := scheduledRuns(t, e, admin, now); len(moved) != 9 || moved[0].User.ID != ada.ID || moved[0].Steps[0] != "Already in Offboarding" {
		t.Fatalf("a new date is a new occurrence: %+v", moved)
	}
}

func TestInactivityLifecycleRule(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	now := time.Now()
	ago := func(days int) time.Time { return now.Add(-time.Duration(days) * 24 * time.Hour) }
	uma, admin := e.person("uma@example.com", "user_admin")
	dormant := e.group("Dormant")
	signedIn := func(u store.User, at time.Time) store.User {
		t.Helper()
		if err := e.st.TouchSignIn(ctx, u.ID, at); err != nil {
			t.Fatal(err)
		}
		return u
	}
	signedIn(uma, now)
	dora := signedIn(func() store.User { u, _ := e.person("dora@example.com"); return u }(), ago(100))
	eve := signedIn(func() store.User { u, _ := e.person("eve@example.com"); return u }(), ago(10))
	em := signedIn(func() store.User { u, _ := e.person("em@example.com"); return u }(), ago(100))
	sus := signedIn(func() store.User { u, _ := e.person("sus@example.com"); return u }(), ago(100))
	if err := e.st.SetUserStatus(ctx, sus.ID, "suspended"); err != nil {
		t.Fatal(err)
	}
	until := now.AddDate(1, 0, 0)
	guest, err := e.st.CreateUser(ctx, store.NewUser{Email: "gus@partner.example", Name: "gus", Status: "active", Guest: true, SponsorID: &uma.ID, ExpiresAt: &until})
	if err != nil {
		t.Fatal(err)
	}
	signedIn(guest, ago(100))
	bot, err := e.st.CreateServiceAccount(ctx, store.NewServiceAccount{Name: "Deploy bot"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.TouchSignIn(ctx, bot.ID, ago(100)); err != nil {
		t.Fatal(err)
	}
	emergency(t, e.st, em.ID)

	actions := []map[string]string{{"type": "add-to-group", "groupId": dormant.ID}, {"type": "suspend"}}
	expect(t, e.call(admin, "POST", "/api/v1/lifecycle/rules", map[string]any{"name": "Bad", "trigger": "inactivity", "days": 0, "enabled": true, "actions": actions}), 422)
	expect(t, e.call(admin, "POST", "/api/v1/lifecycle/preview", map[string]any{"name": "Joiners", "trigger": "joiner", "enabled": true, "actions": actions}), 422)

	audit, err := e.st.ListAudit(ctx, 1000)
	if err != nil {
		t.Fatal(err)
	}
	draft := map[string]any{"name": "Dormant accounts", "trigger": "inactivity", "days": 90, "enabled": true, "actions": actions}
	rec := e.call(admin, "POST", "/api/v1/lifecycle/preview", draft)
	expect(t, rec, 200)
	preview := decode[[]struct {
		User   store.NamedRef `json:"user"`
		Change string         `json:"change"`
	}](t, rec)
	if len(preview) != 1 || preview[0].User.ID != dora.ID || preview[0].Change != "Last signed in "+ago(100).UTC().Format(time.DateOnly)+", 100 days ago" {
		t.Fatalf("preview: %+v", preview)
	}
	fired, err := e.st.LifecycleOccurrences(ctx)
	if err != nil {
		t.Fatal(err)
	}
	after, err := e.st.ListAudit(ctx, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(fired) != 0 || len(after) != len(audit) || e.member(dormant.ID, dora.ID) {
		t.Fatalf("preview wrote something: %v %d %d", fired, len(audit), len(after))
	}
	if dora, _ := e.st.GetUser(ctx, dora.ID); dora.Status != "active" {
		t.Fatal("preview suspended someone")
	}
	expect(t, e.call(admin, "POST", "/api/v1/lifecycle/rules", draft), 201)
	rules := decode[[]store.LifecycleRule](t, e.call(admin, "GET", "/api/v1/lifecycle/rules", nil))

	got := scheduledRuns(t, e, admin, now)
	if len(got) != 1 || got[0].User.ID != dora.ID || got[0].Trigger != "inactivity" || !slices.Equal(got[0].Steps, []string{"Added to Dormant", "Suspended the account and ended 1 session"}) {
		t.Fatalf("runs: %+v", got)
	}
	if again := scheduledRuns(t, e, admin, now); len(again) != 1 {
		t.Fatalf("inactivity ran twice: %+v", again)
	}
	if left := decode[[]any](t, e.call(admin, "POST", "/api/v1/lifecycle/preview?rule="+rules[0].ID, draft)); len(left) != 0 {
		t.Fatalf("preview of a saved rule leaves out people it already ran for: %+v", left)
	}
	if err := e.st.SetUserStatus(ctx, dora.ID, "active"); err != nil {
		t.Fatal(err)
	}
	if restored := scheduledRuns(t, e, admin, now); len(restored) != 1 {
		t.Fatalf("restoring someone who has not signed in is not a new occurrence: %+v", restored)
	}

	signedIn(dora, now)
	nia, _ := e.person("nia@example.com")
	if err := runScheduledLifecycle(ctx, e.st, now.AddDate(0, 0, 91)); err != nil {
		t.Fatal(err)
	}
	runs, err := e.st.ListLifecycleRuns(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	var people []string
	for _, r := range runs[:len(runs)-1] {
		people = append(people, r.User.ID)
		if r.User.ID == nia.ID && !strings.HasPrefix(r.Change, "Never signed in; created ") {
			t.Fatalf("nia: %+v", r)
		}
	}
	want := []string{dora.ID, eve.ID, nia.ID, uma.ID}
	slices.Sort(people)
	slices.Sort(want)
	if !slices.Equal(people, want) {
		t.Fatalf("dormant again 91 days later: got %v, want %v", people, want)
	}
}
