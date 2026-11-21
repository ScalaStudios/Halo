package api_test

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"halo/internal/store"
)

type usage struct {
	Method, ID, Name string
	Assigned         int
	SignIns, People  int
}

type periodReport struct {
	Period struct {
		From               time.Time
		RetentionDays      int
		LimitedByRetention bool
	}
	Methods      []usage
	Applications []usage
}

func (p periodReport) method(kind string) [2]int {
	i := slices.IndexFunc(p.Methods, func(m usage) bool { return m.Method == kind })
	if i < 0 {
		return [2]int{-1, -1}
	}
	return [2]int{p.Methods[i].SignIns, p.Methods[i].People}
}

type registrationReport struct {
	People []struct {
		Name    string
		Methods []string
	}
	Methods []struct {
		Method string
		People int
	}
	Counts struct {
		People  int
		Filters map[string]int
	}
}

func (r registrationReport) names() []string {
	names := []string{}
	for _, p := range r.People {
		names = append(names, p.Name)
	}
	return names
}

type inactiveReport struct {
	People []struct {
		Name    string
		Manager *store.NamedRef
		Groups  []store.NamedRef
	}
}

func (r inactiveReport) names() []string {
	names := []string{}
	for _, p := range r.People {
		names = append(names, p.Name)
	}
	return names
}

func readCSV(t *testing.T, rec *httptest.ResponseRecorder) [][]string {
	t.Helper()
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/csv") || !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("csv response: %d %v %s", rec.Code, rec.Header(), rec.Body)
	}
	rows, err := csv.NewReader(rec.Body).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestReportsOnSeededData(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	now := time.Now()
	ago := func(days int) time.Time { return now.AddDate(0, 0, -days) }
	_, auditor := person(t, st, "aud@example.com", "auditor")
	_, nobody := person(t, st, "nora@example.com")
	finance, err := st.CreateGroup(ctx, store.NewGroup{Name: "Finance", Kind: "assigned"})
	must(t, err)

	newPerson := func(name, status string, groups []string, manager *string, lastSignIn time.Time) store.User {
		t.Helper()
		u, err := st.CreateUser(ctx, store.NewUser{Email: strings.ToLower(name) + "@example.com", Name: name, Status: status, GroupIDs: groups, ManagerID: manager})
		must(t, err)
		if !lastSignIn.IsZero() {
			must(t, st.TouchSignIn(ctx, u.ID, lastSignIn))
		}
		return u
	}
	ann := newPerson("Ann", "active", []string{finance.ID}, nil, ago(5))
	bob := newPerson("Bob", "active", []string{finance.ID}, &ann.ID, ago(100))
	cat := newPerson("Cat", "active", nil, nil, ago(40))
	dan := newPerson("Dan", "suspended", nil, nil, ago(200))
	newPerson("Eve", "invited", nil, nil, time.Time{})

	hash := "$argon2id$v=19$m=65536,t=3,p=4$c2FsdHNhbHQ$aGFzaGhhc2hoYXNo"
	_, err = st.AddWebAuthnCredential(ctx, store.WebAuthnCredential{UserID: ann.ID, Kind: "passkey", Label: "Laptop", CredentialID: []byte("ann-key"), PublicKey: []byte("key"), Transports: []string{}})
	must(t, err)
	must(t, st.SetPassword(ctx, ann.ID, hash, false))
	must(t, st.SetPassword(ctx, bob.ID, hash, false))
	must(t, st.SetPassword(ctx, dan.ID, hash, false))
	secretID, err := st.CreateTOTPSecret(ctx, cat.ID, "Phone", st.Sealer.Seal([]byte("JBSWY3DPEHPK3PXP")))
	must(t, err)
	_, err = st.ConfirmTOTPSecret(ctx, secretID, "Phone", 0)
	must(t, err)

	payroll, err := st.CreateApplication(ctx, store.NewApplication{Name: "Payroll", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://payroll.example.com/cb"}, GroupIDs: []string{finance.ID}})
	must(t, err)
	unused, err := st.CreateApplication(ctx, store.NewApplication{Name: "Unused", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://unused.example.com/cb"}})
	must(t, err)
	_, err = st.CreateApplication(ctx, store.NewApplication{Name: "Wiki", Protocol: "bookmark", Type: "web", Homepage: "https://wiki.example.com"})
	must(t, err)
	bot, err := st.CreateServiceAccount(ctx, store.NewServiceAccount{Name: "Deploy bot"})
	must(t, err)

	signIn := func(u string, app *string, method, result string, at time.Time, impersonator *string) {
		t.Helper()
		must(t, st.RecordSignIn(ctx, store.SignInEvent{Time: at, UserID: &u, AppID: app, Method: method, Result: result, ImpersonatorID: impersonator}))
	}
	signIn(ann.ID, &payroll.ID, "passkey", "success", ago(1), nil)
	signIn(ann.ID, &payroll.ID, "passkey", "success", ago(2), nil)
	signIn(ann.ID, nil, "passkey", "success", ago(3), nil)
	signIn(bob.ID, &payroll.ID, "password", "success", ago(4), nil)
	signIn(bob.ID, &payroll.ID, "password", "failure", ago(4), nil)
	signIn(cat.ID, &payroll.ID, "totp", "success", ago(45), nil)
	signIn(ann.ID, &payroll.ID, "password", "success", ago(1), &cat.ID)
	signIn(bot.ID, nil, "federated", "success", ago(1), nil)

	get := func(token, path string, status int) *httptest.ResponseRecorder {
		t.Helper()
		rec := call(t, h, token, "GET", path, nil)
		expect(t, rec, status)
		return rec
	}

	for _, path := range []string{"method-registration", "method-usage", "inactive-people", "application-usage"} {
		get(auditor, "/api/v1/reports/"+path, 200)
		get(nobody, "/api/v1/reports/"+path, 403)
		get("", "/api/v1/reports/"+path, 401)
	}

	reg := decode[registrationReport](t, get(auditor, "/api/v1/reports/method-registration", 200))
	if reg.Counts.People != 5 || !slices.Equal(reg.names(), []string{"Ann", "aud", "Bob", "Cat", "nora"}) {
		t.Fatalf("registration people: %d %v", reg.Counts.People, reg.names())
	}
	wantFilters := map[string]int{"no-passkey": 4, "password-only": 1, "authenticator-only": 1, "no-method": 2}
	for name, n := range wantFilters {
		if reg.Counts.Filters[name] != n {
			t.Fatalf("filter %s: %v", name, reg.Counts.Filters)
		}
	}
	for _, m := range reg.Methods {
		if want := map[string]int{"passkey": 1, "totp": 1, "password": 2}[m.Method]; m.People != want {
			t.Fatalf("registered %s: %d, want %d", m.Method, m.People, want)
		}
	}
	if !slices.Equal(reg.People[0].Methods, []string{"passkey", "password"}) {
		t.Fatalf("ann methods: %v", reg.People[0].Methods)
	}
	if got := decode[registrationReport](t, get(auditor, "/api/v1/reports/method-registration?filter=password-only", 200)).names(); !slices.Equal(got, []string{"Bob"}) {
		t.Fatalf("password only: %v", got)
	}
	if got := decode[registrationReport](t, get(auditor, "/api/v1/reports/method-registration?filter=authenticator-only", 200)).names(); !slices.Equal(got, []string{"Cat"}) {
		t.Fatalf("authenticator only: %v", got)
	}
	grouped := decode[registrationReport](t, get(auditor, "/api/v1/reports/method-registration?filter=no-passkey&group="+finance.ID, 200))
	if grouped.Counts.People != 2 || grouped.Counts.Filters["no-passkey"] != 1 || !slices.Equal(grouped.names(), []string{"Bob"}) {
		t.Fatalf("finance without passkey: %+v", grouped)
	}
	get(auditor, "/api/v1/reports/method-registration?filter=sms", 422)
	get(auditor, "/api/v1/reports/method-registration?group=grp_missing", 422)

	inactive := decode[inactiveReport](t, get(auditor, "/api/v1/reports/inactive-people?days=30", 200))
	if !slices.Equal(inactive.names(), []string{"Bob", "Cat"}) {
		t.Fatalf("inactive for 30 days: %v", inactive.names())
	}
	if b := inactive.People[0]; b.Manager == nil || b.Manager.ID != ann.ID || len(b.Groups) != 1 || b.Groups[0].Name != "Finance" {
		t.Fatalf("bob's manager and groups: %+v", b)
	}
	if got := decode[inactiveReport](t, get(auditor, "/api/v1/reports/inactive-people", 200)).names(); !slices.Equal(got, []string{"Bob"}) {
		t.Fatalf("inactive for 90 days: %v", got)
	}
	if got := decode[inactiveReport](t, get(auditor, "/api/v1/reports/inactive-people?days=30&group="+finance.ID, 200)).names(); !slices.Equal(got, []string{"Bob"}) {
		t.Fatalf("inactive in finance: %v", got)
	}
	get(auditor, "/api/v1/reports/inactive-people?days=0", 422)

	methods := decode[periodReport](t, get(auditor, "/api/v1/reports/method-usage", 200))
	if methods.method("passkey") != ([2]int{3, 1}) || methods.method("password") != ([2]int{1, 1}) || methods.method("totp") != ([2]int{}) || methods.method("federated") != ([2]int{}) || methods.method("access-pass") != ([2]int{}) {
		t.Fatalf("method usage: %+v", methods.Methods)
	}
	from := url.QueryEscape(ago(60).Format(time.RFC3339))
	if got := decode[periodReport](t, get(auditor, "/api/v1/reports/method-usage?from="+from, 200)).method("totp"); got != ([2]int{1, 1}) {
		t.Fatalf("totp over 60 days: %+v", got)
	}
	if got := decode[periodReport](t, get(auditor, "/api/v1/reports/method-usage?from="+from+"&to="+ago(10).Format(time.DateOnly), 200)).method("passkey"); got != ([2]int{}) {
		t.Fatalf("passkey from 60 to 10 days ago: %+v", got)
	}
	get(auditor, "/api/v1/reports/method-usage?from=yesterday", 422)
	get(auditor, "/api/v1/reports/method-usage?from="+now.Format(time.DateOnly)+"&to="+ago(3).Format(time.DateOnly), 422)

	apps := decode[periodReport](t, get(auditor, "/api/v1/reports/application-usage", 200))
	if len(apps.Applications) != 2 || apps.Applications[0] != (usage{ID: payroll.ID, Name: "Payroll", Assigned: 2, SignIns: 3, People: 2}) || apps.Applications[1] != (usage{ID: unused.ID, Name: "Unused"}) {
		t.Fatalf("application usage: %+v", apps.Applications)
	}

	must(t, st.SaveSettingValues(ctx, map[string]json.RawMessage{"signInLogDays": json.RawMessage("30")}))
	st.ForgetSettings()
	limited := decode[periodReport](t, get(auditor, "/api/v1/reports/method-usage?from="+from, 200))
	if !limited.Period.LimitedByRetention || limited.Period.RetentionDays != 30 || limited.method("totp") != ([2]int{}) || limited.Period.From.Before(ago(30).Add(-time.Minute)) {
		t.Fatalf("retention: %+v", limited)
	}
	if rec := get(auditor, "/api/v1/reports/application-usage?from="+from+"&to="+ago(40).Format(time.DateOnly), 422); !strings.Contains(rec.Body.String(), "keeps 30 days") {
		t.Fatalf("period before retention: %s", rec.Body)
	}
	if !slices.Equal(readCSV(t, get(auditor, "/api/v1/reports/application-usage?format=csv", 200))[1][:4], []string{"Payroll", "active", "3", "2"}) {
		t.Fatal("application usage csv")
	}
	inactiveCSV := readCSV(t, get(auditor, "/api/v1/reports/inactive-people?format=csv", 200))
	if len(inactiveCSV) != 2 || !slices.Equal(inactiveCSV[1][:3], []string{"Bob", "bob@example.com", "active"}) || inactiveCSV[1][5] != "Ann" || inactiveCSV[1][7] != "Finance" {
		t.Fatalf("inactive csv: %v", inactiveCSV)
	}
}

func TestReportCSVNeutralisesFormulas(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, auditor := person(t, st, "aud@example.com", "auditor")
	for _, u := range []store.NewUser{
		{Email: "eq@example.com", Name: `=HYPERLINK("https://evil.example","open")`, Department: "@SUM(A1:A9)", Status: "active"},
		{Email: "plus@example.com", Name: "+1+1", Department: `-2, "quoted"`, Status: "active"},
	} {
		_, err := st.CreateUser(ctx, u)
		must(t, err)
	}
	rows := readCSV(t, call(t, h, auditor, "GET", "/api/v1/reports/method-registration?format=csv", nil))
	if !slices.Equal(rows[0][:3], []string{"name", "email", "department"}) || len(rows) != 4 {
		t.Fatalf("csv rows: %v", rows)
	}
	want := map[string]string{
		`'=HYPERLINK("https://evil.example","open")`: "'@SUM(A1:A9)",
		"'+1+1": `'-2, "quoted"`,
		"aud":   "",
	}
	for _, row := range rows[1:] {
		department, ok := want[row[0]]
		if !ok || row[2] != department {
			t.Fatalf("unescaped row: %q", row)
		}
	}
}
