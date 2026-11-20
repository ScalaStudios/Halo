package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"halo/internal/httpx"
	"halo/internal/store"
)

type importResponse struct {
	DryRun  bool `json:"dryRun"`
	Ready   int  `json:"ready"`
	Skipped int  `json:"skipped"`
	Rows    []struct {
		Row       int          `json:"row"`
		Email     string       `json:"email"`
		Title     string       `json:"title"`
		Groups    []string     `json:"groups"`
		Error     *httpx.Error `json:"error"`
		UserID    string       `json:"userId"`
		EnrollURL string       `json:"enrollUrl"`
		Emailed   bool         `json:"emailed"`
	} `json:"rows"`
}

func upload(t *testing.T, h http.Handler, token, query, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/v1/users/import"+query, strings.NewReader(body))
	req.Header.Set("Content-Type", "text/csv; charset=utf-8")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: token})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func writes(t *testing.T, st *store.Store) [3]int {
	t.Helper()
	ctx := context.Background()
	users, err := st.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	audit, err := st.ListAudit(ctx, 1000)
	if err != nil {
		t.Fatal(err)
	}
	mail, err := st.ListMail(ctx, 1000)
	if err != nil {
		t.Fatal(err)
	}
	return [3]int{len(users), len(audit), len(mail)}
}

const formula = `=cmd|' /C calc'!A0`

const people = "\xef\xbb\xbfEmail,Name,Title,Department,Manager Email,Groups\r\n" +
	`ana@example.com,"Silva, Ana","` + formula + `",Engineering,,Ops` + "\r\n" +
	`ben@example.com,Ben Ode,"Lead` + "\n" + `Engineer",Engineering,ANA@example.com,ops; ` + "\r\n" +
	",,,,,\r\n" +
	"Ana@Example.com,Ana Again,,,,\r\n" +
	"uma@example.com,Uma,,,,\r\n" +
	"cy@example.com,,,,,\r\n" +
	"dee@example.com,Dee,,,nobody@example.com,\r\n" +
	"eve@example.com,Eve,,,fay@example.com,\r\n" +
	"fay@example.com,Fay,,,eve@example.com,\r\n" +
	"gus@example.com,Gus,,,,Everyone\r\n" +
	"hal@example.com,Hal,,,hal@example.com,\r\n"

func TestImportPreviewsThenCreatesValidRows(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "uma@example.com", "user_admin")
	ops, _ := st.CreateGroup(ctx, store.NewGroup{Name: "Ops", Kind: "assigned"})
	rule := `user.email endsWith "@example.com"`
	if _, err := st.CreateGroup(ctx, store.NewGroup{Name: "Everyone", Kind: "dynamic", Rule: &rule}); err != nil {
		t.Fatal(err)
	}

	before := writes(t, st)
	rec := upload(t, h, admin, "?dryRun=true&sendInvitations=true", people)
	expect(t, rec, 200)
	preview := decode[importResponse](t, rec)
	if after := writes(t, st); after != before {
		t.Fatalf("dry run wrote users, audit events or mail: %v then %v", before, after)
	}
	want := map[int]string{2: "", 3: "", 6: "Row 2 already has the email", 7: "ERR_CONFLICT", 8: "Enter the person's name", 9: "Nobody with the email nobody@example.com", 10: "manager on row 11 can't be imported", 11: "lead back to this row", 12: "dynamic group", 13: "own manager"}
	if !preview.DryRun || preview.Ready != 2 || preview.Skipped != 8 || len(preview.Rows) != len(want) {
		t.Fatalf("preview: %+v", preview)
	}
	for _, row := range preview.Rows {
		problem, ok := want[row.Row]
		switch {
		case !ok:
			t.Errorf("unexpected row %d", row.Row)
		case problem == "" && row.Error != nil:
			t.Errorf("row %d: %s", row.Row, row.Error.Message)
		case problem != "" && (row.Error == nil || !strings.Contains(row.Error.Code+" "+row.Error.Message, problem)):
			t.Errorf("row %d: want %q, got %+v", row.Row, problem, row.Error)
		}
	}
	if preview.Rows[0].Title != formula || !slices.Equal(preview.Rows[1].Groups, []string{"ops"}) {
		t.Fatalf("parsed rows: %+v", preview.Rows[:2])
	}

	rec = upload(t, h, admin, "", people)
	expect(t, rec, 200)
	applied := decode[importResponse](t, rec)
	if applied.DryRun || applied.Ready != 2 {
		t.Fatalf("apply: %+v", applied)
	}
	ana, err := st.GetUserByEmail(ctx, "ana@example.com")
	if err != nil {
		t.Fatal(err)
	}
	ben, err := st.GetUserByEmail(ctx, "ben@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if ana.Title != formula || ana.Name != "Silva, Ana" || ana.Status != "invited" || len(ana.RoleKeys) > 0 || !slices.Contains(ana.GroupIDs, ops.ID) {
		t.Fatalf("ana: %+v", ana)
	}
	if ben.ManagerID == nil || *ben.ManagerID != ana.ID || ben.Title != "Lead\nEngineer" || !slices.Contains(ben.GroupIDs, ops.ID) || len(ben.RoleKeys) > 0 {
		t.Fatalf("ben: %+v", ben)
	}
	for _, row := range applied.Rows[:2] {
		got, purpose, err := st.PeekEnrollmentToken(ctx, strings.TrimPrefix(row.EnrollURL, "http://localhost:3200/enroll?token="))
		if err != nil || got.ID != row.UserID || purpose != "invite" || row.Emailed {
			t.Fatalf("setup link for row %d: %+v %s %v", row.Row, got, purpose, err)
		}
	}
	for _, id := range []string{"cy", "dee", "eve", "fay", "gus", "hal"} {
		if _, err := st.GetUserByEmail(ctx, id+"@example.com"); err == nil {
			t.Errorf("%s was imported despite an error", id)
		}
	}
	audit, _ := st.ListAudit(ctx, 1000)
	invited := 0
	summary := slices.IndexFunc(audit, func(e store.AuditEvent) bool { return e.Action == "user.import" })
	for _, e := range audit {
		if e.Action == "user.invite" {
			invited++
		}
	}
	if invited != 2 || summary < 0 || audit[summary].Summary != "Imported 2 of 10 rows and held the invitations" {
		t.Fatalf("audit: %+v", audit)
	}
	if mail, _ := st.ListMail(ctx, 10); len(mail) != 0 {
		t.Fatalf("held invitations were emailed: %+v", mail)
	}

	rec = upload(t, h, admin, "", people)
	expect(t, rec, 200)
	if again := decode[importResponse](t, rec); again.Ready != 0 || again.Rows[0].Error == nil || again.Rows[0].Error.Code != "ERR_CONFLICT" {
		t.Fatalf("second import: %+v", again)
	}
}

func TestImportFromJSONSendsInvitations(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "uma@example.com", "user_admin")

	rec := call(t, h, admin, "POST", "/api/v1/users/import?sendInvitations=true", map[string]any{"people": []map[string]any{{"email": "kim@example.com", "name": "Kim", "roles": []string{"global_admin"}}}})
	expect(t, rec, 400)
	if !strings.Contains(rec.Body.String(), "ERR_UNKNOWN_FIELD") {
		t.Fatalf("roles accepted: %s", rec.Body)
	}

	rec = call(t, h, admin, "POST", "/api/v1/users/import?sendInvitations=true", map[string]any{"people": []map[string]any{{"email": "kim@example.com", "name": "Kim"}}})
	expect(t, rec, 200)
	out := decode[importResponse](t, rec)
	mail, err := st.ListMail(ctx, 10)
	if out.Ready != 1 || out.Rows[0].Row != 1 || err != nil || len(mail) != 1 || !strings.Contains(mail[0].Text, out.Rows[0].EnrollURL) {
		t.Fatalf("invitation not queued: %+v %+v %v", out, mail, err)
	}
	if kim, err := st.GetUserByEmail(ctx, "kim@example.com"); err != nil || len(kim.RoleKeys) > 0 {
		t.Fatalf("kim: %+v %v", kim, err)
	}
}

func TestImportRefusesUnreadableFiles(t *testing.T) {
	st, h := setup(t)
	_, admin := person(t, st, "uma@example.com", "user_admin")
	_, helpdesk := person(t, st, "hana@example.com", "helpdesk_admin")
	before := writes(t, st)
	tooMany := "email,name\n" + strings.Repeat("x@example.com,X\n", 501)
	for _, c := range []struct {
		name, query, body string
		status            int
		want              string
	}{
		{"bare quote", "", "email,name\nsam@example.com,Sam \"the\" Lee\n", 422, "Line 2"},
		{"unclosed quote", "", "email,name\n\"sam@example.com,Sam\n", 422, "can't be read"},
		{"missing field", "", "email,name\nsam@example.com\n", 422, "Line 2"},
		{"not utf-8", "", "email,name\nsam@example.com,S\xe9bastien\n", 422, "CSV UTF-8"},
		{"empty", "", "", 422, "empty"},
		{"roles column", "", "email,name,Roles\nsam@example.com,Sam,global_admin\n", 422, "never grant administrator roles"},
		{"unknown column", "", "email,name,phone\nsam@example.com,Sam,1\n", 422, "phone"},
		{"no name column", "", "email\nsam@example.com\n", 422, "name column"},
		{"too many rows", "", tooMany, 422, "between 1 and 500"},
		{"too large", "", "email,name\n" + strings.Repeat("a", 1<<20), 413, "ERR_TOO_LARGE"},
		{"bad flag", "?dryRun=maybe", "email,name\nsam@example.com,Sam\n", 422, "dryRun must be true or false"},
	} {
		rec := upload(t, h, admin, c.query, c.body)
		if rec.Code != c.status || !strings.Contains(rec.Body.String(), c.want) {
			t.Errorf("%s: want %d %q, got %d %s", c.name, c.status, c.want, rec.Code, rec.Body)
		}
	}
	expect(t, upload(t, h, helpdesk, "", "email,name\nsam@example.com,Sam\n"), 403)
	if after := writes(t, st); after != before {
		t.Fatalf("refused imports wrote: %v then %v", before, after)
	}
}
