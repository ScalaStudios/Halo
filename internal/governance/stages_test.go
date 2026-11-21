package governance

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"halo/internal/config"
	"halo/internal/store"
)

func (e *env) personWith(in store.NewUser) (store.User, string) {
	e.t.Helper()
	ctx := context.Background()
	in.Name, in.Status = strings.Split(in.Email, "@")[0], "active"
	u, err := e.st.CreateUser(ctx, in)
	if err != nil {
		e.t.Fatal(err)
	}
	_, token, err := e.st.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: "passkey"})
	if err != nil {
		e.t.Fatal(err)
	}
	return u, token
}

func (e *env) decide(token, requestID, verdict string, status int) store.AccessRequest {
	e.t.Helper()
	rec := e.call(token, "POST", "/api/v1/access-requests/"+requestID+"/"+verdict, nil)
	expect(e.t, rec, status)
	if status != 200 {
		return store.AccessRequest{}
	}
	return decode[store.AccessRequest](e.t, rec)
}

func approverIDs(req store.AccessRequest) []string {
	return ids(req.Approvers)
}

func TestTwoStageApproval(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, admin := e.person("uma@example.com", "user_admin")
	owner, ownerToken := e.person("olga@example.com")
	mia, miaToken := e.person("mia@example.com")
	sam, samToken := e.personWith(store.NewUser{Email: "sam@example.com", ManagerID: &mia.ID})
	prod := e.group("Production")
	pkg := e.pkg(admin, map[string]any{"name": "Production access", "groupIds": []string{prod.ID}, "requireJustification": false, "maxDays": 7,
		"stages": []map[string]any{{"approver": "manager"}, {"approver": "people", "approverIds": []string{owner.ID}}}})
	if len(pkg.Stages) != 2 || pkg.Stages[0].Approver != "manager" || len(pkg.Stages[1].Approvers) != 1 || pkg.Stages[1].Approvers[0].ID != owner.ID {
		t.Fatalf("stages: %+v", pkg.Stages)
	}

	req := e.request(samToken, map[string]any{"packageId": pkg.ID, "durationDays": 7})
	if req.Stage != 1 || req.Stages != 2 || !slices.Equal(approverIDs(req), []string{mia.ID}) {
		t.Fatalf("first stage: %+v", req)
	}
	if len(e.box.to("mia@example.com")) != 1 || len(e.box.to("olga@example.com")) != 0 {
		t.Fatal("only the approvers of the current stage are emailed")
	}
	e.decide(ownerToken, req.ID, "approve", 403)
	mid := e.decide(miaToken, req.ID, "approve", 200)
	if mid.Status != "pending" || mid.Stage != 2 || !slices.Equal(approverIDs(mid), []string{owner.ID}) || len(mid.StageApprovals) != 1 || mid.StageApprovals[0].By.ID != mia.ID {
		t.Fatalf("second stage: %+v", mid)
	}
	if e.member(prod.ID, sam.ID) {
		t.Fatal("the first stage granted access")
	}
	if msgs := e.box.to("olga@example.com"); len(msgs) != 1 || !strings.Contains(msgs[0].Subject, "sam requests") {
		t.Fatalf("second stage email: %+v", msgs)
	}
	e.decide(miaToken, req.ID, "approve", 403)
	if _, err := e.st.ApproveAccessRequest(ctx, req.ID, 1, mia.ID, "", time.Now()); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("approving a stage the request already left: %v", err)
	}
	if again, _ := e.st.GetAccessRequest(ctx, req.ID); again.Stage != 2 || again.Status != "pending" {
		t.Fatalf("a stale stage approval moved the request: %+v", again)
	}
	done := e.decide(ownerToken, req.ID, "approve", 200)
	if done.Status != "approved" || len(done.StageApprovals) != 2 || !e.member(prod.ID, sam.ID) {
		t.Fatalf("approved: %+v", done)
	}
	if u, _ := e.st.GetUser(ctx, sam.ID); len(u.RoleKeys) != 0 {
		t.Fatalf("a package granted roles: %v", u.RoleKeys)
	}
	approvals := e.audit("access_request.approve")
	if len(approvals) != 2 || !slices.ContainsFunc(approvals, func(a store.AuditEvent) bool {
		return slices.ContainsFunc(a.Changes, func(c store.Change) bool {
			return c.Field == "stage" && string(c.Before) == "1" && string(c.After) == "2"
		})
	}) {
		t.Fatalf("stage audit: %+v", approvals)
	}

	expect(t, e.call(admin, "PUT", "/api/v1/access-packages/"+pkg.ID, map[string]any{"name": "Production access", "groupIds": []string{prod.ID}, "approverIds": []string{owner.ID}}), 422)
	kept := decode[store.AccessPackage](t, e.call(admin, "PUT", "/api/v1/access-packages/"+pkg.ID, map[string]any{"name": "Production access", "description": "Shell access", "groupIds": []string{prod.ID}, "maxDays": 7}))
	if len(kept.Stages) != 2 {
		t.Fatalf("a body without stages dropped them: %+v", kept.Stages)
	}
	if updates := e.audit("access_package.update"); len(updates) != 1 || !slices.ContainsFunc(updates[0].Changes, func(c store.Change) bool { return c.Field == "description" && string(c.After) == `"Shell access"` }) {
		t.Fatalf("package audit: %+v", updates)
	}
	expect(t, e.call(admin, "POST", "/api/v1/access-packages", map[string]any{"name": "Too many", "groupIds": []string{prod.ID},
		"stages": []map[string]any{{"approver": "manager"}, {"approver": "manager"}, {"approver": "manager"}, {"approver": "manager"}}}), 422)
}

func TestStageDenialEndsRequest(t *testing.T) {
	e := setup(t)
	_, admin := e.person("uma@example.com", "user_admin")
	owner, ownerToken := e.person("olga@example.com")
	mia, miaToken := e.person("mia@example.com")
	sam, samToken := e.personWith(store.NewUser{Email: "sam@example.com", ManagerID: &mia.ID})
	prod := e.group("Production")
	pkg := e.pkg(admin, map[string]any{"name": "Production access", "groupIds": []string{prod.ID}, "requireJustification": false,
		"stages": []map[string]any{{"approver": "manager"}, {"approver": "people", "approverIds": []string{owner.ID}}}})

	req := e.request(samToken, map[string]any{"packageId": pkg.ID})
	denied := e.decide(miaToken, req.ID, "deny", 200)
	if denied.Status != "denied" || denied.Stage != 1 {
		t.Fatalf("denied: %+v", denied)
	}
	e.decide(ownerToken, req.ID, "approve", 409)
	if e.member(prod.ID, sam.ID) || len(e.box.to("olga@example.com")) != 0 {
		t.Fatal("a denial at the first stage reached the second stage")
	}
	if msgs := e.box.to("sam@example.com"); len(msgs) != 1 || !strings.Contains(msgs[0].Subject, "denied") {
		t.Fatalf("denial email: %+v", msgs)
	}
}

func TestManagerApprover(t *testing.T) {
	e := setup(t)
	_, admin := e.person("uma@example.com", "user_admin")
	scoped, scopedToken := e.person("lim@example.com")
	mia, miaToken := e.person("mia@example.com")
	_, otherToken := e.person("otto@example.com")
	sam, samToken := e.personWith(store.NewUser{Email: "sam@example.com", ManagerID: &mia.ID})
	until := time.Now().Add(30 * 24 * time.Hour)
	_, patToken := e.personWith(store.NewUser{Email: "pat@partner.example", Guest: true, SponsorID: &mia.ID, ExpiresAt: &until})
	g := e.group("Finance")
	pkg := e.pkg(admin, map[string]any{"name": "Finance", "groupIds": []string{g.ID}, "requireJustification": false, "stages": []map[string]any{{"approver": "manager"}}})
	if err := e.st.SetUserRoles(context.Background(), scoped.ID, []string{"user_admin"}, map[string][]string{"user_admin": {g.ID}}); err != nil {
		t.Fatal(err)
	}

	req := e.request(samToken, map[string]any{"packageId": pkg.ID})
	e.decide(otherToken, req.ID, "approve", 403)
	e.decide(scopedToken, req.ID, "approve", 403)
	expect(t, e.call(scopedToken, "GET", "/api/v1/access-requests", nil), 403)
	if mine := decode[[]store.AccessRequest](t, e.call(miaToken, "GET", "/api/v1/me/approvals", nil)); len(mine) != 1 || mine[0].ID != req.ID {
		t.Fatalf("manager approvals: %+v", mine)
	}
	if theirs := decode[[]store.AccessRequest](t, e.call(otherToken, "GET", "/api/v1/me/approvals", nil)); len(theirs) != 0 {
		t.Fatalf("someone else's report offered for approval: %+v", theirs)
	}
	if msgs := e.box.to("mia@example.com"); len(msgs) != 1 || !strings.Contains(msgs[0].Text, "/account/access") {
		t.Fatalf("manager email: %+v", msgs)
	}
	if done := e.decide(miaToken, req.ID, "approve", 200); done.Status != "approved" || !e.member(g.ID, sam.ID) {
		t.Fatalf("approved: %+v", done)
	}

	guest := e.request(patToken, map[string]any{"packageId": pkg.ID})
	if !slices.Equal(approverIDs(guest), []string{mia.ID}) {
		t.Fatalf("a guest's sponsor decides the manager stage: %+v", guest.Approvers)
	}
	e.decide(patToken, guest.ID, "approve", 403)
	e.decide(miaToken, guest.ID, "approve", 200)
	expect(t, e.call(admin, "POST", "/api/v1/access-packages", map[string]any{"name": "Guest approver", "groupIds": []string{g.ID}, "approverIds": []string{guest.Requester.ID}}), 422)
}

func TestMissingManagerFallback(t *testing.T) {
	e := setup(t)
	_, admin := e.person("uma@example.com", "user_admin")
	_, giaToken := e.person("gia@example.com", "global_admin")
	fred, fredToken := e.person("fred@example.com")
	sam, samToken := e.person("sam@example.com")
	_, otherToken := e.person("otto@example.com")
	g := e.group("Finance")
	pkg := e.pkg(admin, map[string]any{"name": "Finance", "groupIds": []string{g.ID}, "requireJustification": false, "stages": []map[string]any{{"approver": "manager", "fallbackIds": []string{fred.ID}}}})

	req := e.request(samToken, map[string]any{"packageId": pkg.ID})
	if !slices.Equal(approverIDs(req), []string{fred.ID}) {
		t.Fatalf("approvers without a manager: %+v", req.Approvers)
	}
	if len(e.box.to("fred@example.com")) != 1 {
		t.Fatal("the fallback approver was not emailed")
	}
	e.decide(otherToken, req.ID, "approve", 403)
	e.decide(fredToken, req.ID, "approve", 200)
	if !e.member(g.ID, sam.ID) {
		t.Fatal("the fallback approval did not grant access")
	}

	bare := e.pkg(admin, map[string]any{"name": "Payroll", "groupIds": []string{g.ID}, "requireJustification": false, "stages": []map[string]any{{"approver": "manager"}}})
	orphan := e.request(otherToken, map[string]any{"packageId": bare.ID})
	if len(orphan.Approvers) != 0 || len(e.box.to("gia@example.com")) != 1 {
		t.Fatalf("without a manager or fallback, global administrators decide: %+v", orphan.Approvers)
	}
	e.decide(fredToken, orphan.ID, "approve", 403)
	e.decide(giaToken, orphan.ID, "approve", 200)
}

func TestStageTimeout(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, admin := e.person("uma@example.com", "user_admin")
	olga, olgaToken := e.person("olga@example.com")
	fred, fredToken := e.person("fred@example.com")
	_, samToken := e.person("sam@example.com")
	g := e.group("Finance")
	pkg := e.pkg(admin, map[string]any{"name": "Finance", "groupIds": []string{g.ID}, "requireJustification": false,
		"stages": []map[string]any{{"approver": "people", "approverIds": []string{olga.ID}, "fallbackIds": []string{fred.ID}, "timeoutDays": 2}}})
	public, _ := url.Parse("http://localhost:3200")
	h := &handler{st: e.st, cfg: config.Config{PublicURL: public}, mailer: e.box}

	req := e.request(samToken, map[string]any{"packageId": pkg.ID})
	if req.StageDueAt == nil || !slices.Equal(approverIDs(req), []string{olga.ID}) {
		t.Fatalf("pending: %+v", req)
	}
	start := time.Now()
	if err := h.timeOutRequests(ctx, start.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(e.box.to("fred@example.com")) != 0 {
		t.Fatal("escalated before the deadline")
	}
	if err := h.timeOutRequests(ctx, start.Add(49*time.Hour)); err != nil {
		t.Fatal(err)
	}
	escalated, _ := e.st.GetAccessRequest(ctx, req.ID)
	if escalated.Status != "pending" || !slices.Contains(approverIDs(escalated), fred.ID) || !slices.Contains(approverIDs(escalated), olga.ID) {
		t.Fatalf("escalated: %+v", escalated)
	}
	if len(e.box.to("fred@example.com")) != 1 || len(e.box.to("olga@example.com")) != 1 || len(e.audit("access_request.escalate")) != 1 {
		t.Fatal("escalation emails only the fallback approvers once")
	}
	if err := h.timeOutRequests(ctx, start.Add(50*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(e.box.to("fred@example.com")) != 1 {
		t.Fatal("escalated twice")
	}
	if err := h.timeOutRequests(ctx, start.Add(98*time.Hour)); err != nil {
		t.Fatal(err)
	}
	closed, _ := e.st.GetAccessRequest(ctx, req.ID)
	if closed.Status != "expired" || len(e.audit("access_request.timeout")) != 1 {
		t.Fatalf("expired: %+v", closed)
	}
	if msgs := e.box.to("sam@example.com"); len(msgs) != 1 || !strings.Contains(msgs[0].Subject, "expired") {
		t.Fatalf("expiry email: %+v", msgs)
	}
	e.decide(fredToken, req.ID, "approve", 409)

	direct := e.pkg(admin, map[string]any{"name": "Payroll", "groupIds": []string{g.ID}, "requireJustification": false,
		"stages": []map[string]any{{"approver": "people", "approverIds": []string{olga.ID}, "timeoutDays": 1}}})
	quick := e.request(samToken, map[string]any{"packageId": direct.ID})
	if err := h.timeOutRequests(ctx, time.Now().Add(25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if expired, _ := e.st.GetAccessRequest(ctx, quick.ID); expired.Status != "expired" {
		t.Fatalf("a stage without fallback approvers expires at its deadline: %+v", expired)
	}
	e.decide(olgaToken, quick.ID, "approve", 409)
}

func TestQuestionsRequired(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, admin := e.person("uma@example.com", "user_admin")
	olga, olgaToken := e.person("olga@example.com")
	_, samToken := e.person("sam@example.com")
	g := e.group("Finance")
	body := func(questions []map[string]any) map[string]any {
		return map[string]any{"name": "Finance", "groupIds": []string{g.ID}, "approverIds": []string{olga.ID}, "requireJustification": false, "questions": questions}
	}
	expect(t, e.call(admin, "POST", "/api/v1/access-packages", body([]map[string]any{{"text": "Project", "kind": "choice", "choices": []string{"Apollo"}}})), 422)
	expect(t, e.call(admin, "POST", "/api/v1/access-packages", body([]map[string]any{{"text": "Cost", "kind": "number"}})), 422)
	pkg := e.pkg(admin, body([]map[string]any{{"text": "Ticket number", "kind": "text", "required": true}, {"text": "Project", "kind": "choice", "choices": []string{"Apollo", "Gemini"}}}))
	ticket, project := pkg.Questions[0].ID, pkg.Questions[1].ID
	if ticket == "" || project == "" || pkg.Questions[0].Choices == nil {
		t.Fatalf("questions: %+v", pkg.Questions)
	}

	expect(t, e.call(samToken, "POST", "/api/v1/me/access-requests", map[string]any{"packageId": pkg.ID}), 422)
	expect(t, e.call(samToken, "POST", "/api/v1/me/access-requests", map[string]any{"packageId": pkg.ID, "answers": map[string]string{ticket: "  "}}), 422)
	expect(t, e.call(samToken, "POST", "/api/v1/me/access-requests", map[string]any{"packageId": pkg.ID, "answers": map[string]string{ticket: "FIN-1042", project: "Mercury"}}), 422)
	req := e.request(samToken, map[string]any{"packageId": pkg.ID, "answers": map[string]string{ticket: "FIN-1042", project: "Apollo"}})
	if len(req.Answers) != 2 || req.Answers[0] != (store.AccessAnswer{QuestionID: ticket, Question: "Ticket number", Answer: "FIN-1042"}) {
		t.Fatalf("answers: %+v", req.Answers)
	}
	if msgs := e.box.to("olga@example.com"); len(msgs) != 1 || !strings.Contains(msgs[0].Text, "Ticket number: FIN-1042\nProject: Apollo") {
		t.Fatalf("approver email: %+v", msgs)
	}
	if mine := decode[[]store.AccessRequest](t, e.call(olgaToken, "GET", "/api/v1/me/approvals", nil)); len(mine) != 1 || len(mine[0].Answers) != 2 {
		t.Fatalf("approvers see the answers: %+v", mine)
	}

	edited := body([]map[string]any{{"id": ticket, "text": "Incident", "kind": "text", "required": true}})
	expect(t, e.call(admin, "PUT", "/api/v1/access-packages/"+pkg.ID, edited), 200)
	if kept, _ := e.st.GetAccessRequest(ctx, req.ID); kept.Answers[0].Question != "Ticket number" {
		t.Fatalf("answers are kept with the request: %+v", kept.Answers)
	}
	delete(edited, "questions")
	if again := decode[store.AccessPackage](t, e.call(admin, "PUT", "/api/v1/access-packages/"+pkg.ID, edited)); len(again.Questions) != 1 || again.Questions[0].ID != ticket {
		t.Fatalf("a body without questions dropped them: %+v", again.Questions)
	}
}

func TestRequestOnBehalf(t *testing.T) {
	e := setup(t)
	_, admin := e.person("uma@example.com", "user_admin")
	mia, miaToken := e.person("mia@example.com")
	fred, fredToken := e.person("fred@example.com")
	_, ottoToken := e.person("otto@example.com")
	sam, samToken := e.personWith(store.NewUser{Email: "sam@example.com", ManagerID: &mia.ID})
	until := time.Now().Add(30 * 24 * time.Hour)
	pat, patToken := e.personWith(store.NewUser{Email: "pat@partner.example", Guest: true, SponsorID: &mia.ID, ExpiresAt: &until})
	g := e.group("Finance")
	pkg := e.pkg(admin, map[string]any{"name": "Finance", "groupIds": []string{g.ID}, "requireJustification": true, "stages": []map[string]any{{"approver": "manager", "fallbackIds": []string{fred.ID}}}})

	if reports := decode[[]store.NamedRef](t, e.call(miaToken, "GET", "/api/v1/me/direct-reports", nil)); len(reports) != 2 {
		t.Fatalf("direct reports: %+v", reports)
	}
	for _, token := range []string{ottoToken, patToken} {
		rec := e.call(token, "POST", "/api/v1/me/access-requests", map[string]any{"packageId": pkg.ID, "beneficiaryId": sam.ID, "justification": "Quarter close"})
		expect(t, rec, 403)
		if !strings.Contains(rec.Body.String(), "people who report to you") {
			t.Fatalf("refusal: %s", rec.Body)
		}
	}
	expect(t, e.call(miaToken, "POST", "/api/v1/me/access-requests", map[string]any{"packageId": pkg.ID, "beneficiaryId": "usr_missing", "justification": "Quarter close"}), 403)

	req := e.request(miaToken, map[string]any{"packageId": pkg.ID, "beneficiaryId": sam.ID, "justification": "Quarter close"})
	if req.Requester.ID != sam.ID || req.RequestedBy == nil || req.RequestedBy.ID != mia.ID || !slices.Equal(approverIDs(req), []string{fred.ID}) {
		t.Fatalf("on behalf: %+v", req)
	}
	if msgs := e.box.to("sam@example.com"); len(msgs) != 1 || msgs[0].Subject != "mia requested Finance for you" {
		t.Fatalf("beneficiary email: %+v", msgs)
	}
	if msgs := e.box.to("fred@example.com"); len(msgs) != 1 || !strings.Contains(msgs[0].Text, "mia requested the access package Finance for sam") {
		t.Fatalf("approver email: %+v", msgs)
	}
	rec := e.call(miaToken, "POST", "/api/v1/access-requests/"+req.ID+"/approve", nil)
	expect(t, rec, 403)
	if !strings.Contains(rec.Body.String(), "ERR_SELF_APPROVAL") {
		t.Fatalf("requester approval: %s", rec.Body)
	}
	e.decide(samToken, req.ID, "approve", 403)
	if theirs := decode[[]store.AccessRequest](t, e.call(samToken, "GET", "/api/v1/me/access-requests", nil)); len(theirs) != 1 {
		t.Fatalf("the beneficiary sees the request: %+v", theirs)
	}
	expect(t, e.call(miaToken, "POST", "/api/v1/me/access-requests", map[string]any{"packageId": pkg.ID, "beneficiaryId": sam.ID, "justification": "Again"}), 409)
	e.request(miaToken, map[string]any{"packageId": pkg.ID, "justification": "My own"})
	e.decide(fredToken, req.ID, "approve", 200)
	if !e.member(g.ID, sam.ID) {
		t.Fatal("the approved on-behalf request did not grant access")
	}

	guest := e.request(miaToken, map[string]any{"packageId": pkg.ID, "beneficiaryId": pat.ID, "justification": "Audit"})
	if guest.Requester.ID != pat.ID || !slices.Equal(approverIDs(guest), []string{fred.ID}) {
		t.Fatalf("sponsor on behalf of a guest: %+v", guest)
	}
}

func TestExtensionFlow(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, admin := e.person("uma@example.com", "user_admin")
	olga, olgaToken := e.person("olga@example.com")
	sam, samToken := e.person("sam@example.com")
	_, ottoToken := e.person("otto@example.com")
	g := e.group("Finance")
	pkg := e.pkg(admin, map[string]any{"name": "Finance", "groupIds": []string{g.ID}, "approverIds": []string{olga.ID}, "requireJustification": false, "maxDays": 7})

	grant := e.decide(olgaToken, e.request(samToken, map[string]any{"packageId": pkg.ID, "durationDays": 7}).ID, "approve", 200)
	expect(t, e.call(ottoToken, "POST", "/api/v1/me/access-requests", map[string]any{"extendsId": grant.ID, "durationDays": 3}), 422)
	expect(t, e.call(samToken, "POST", "/api/v1/me/access-requests", map[string]any{"extendsId": grant.ID, "durationDays": 8}), 422)
	ext := e.request(samToken, map[string]any{"extendsId": grant.ID, "durationDays": 3})
	if ext.Extends == nil || *ext.Extends != grant.ID || ext.Package.ID != pkg.ID || ext.Status != "pending" {
		t.Fatalf("extension: %+v", ext)
	}
	if msgs := e.box.to("olga@example.com"); len(msgs) != 2 || !strings.Contains(msgs[1].Text, "for 3 more days") {
		t.Fatalf("extension email: %+v", msgs)
	}
	e.decide(samToken, ext.ID, "approve", 403)
	extended := e.decide(olgaToken, ext.ID, "approve", 200)
	if extended.ExpiresAt == nil || extended.ExpiresAt.Sub(*grant.ExpiresAt).Round(time.Minute) != 72*time.Hour {
		t.Fatalf("extended until %v, granted until %v", extended.ExpiresAt, grant.ExpiresAt)
	}
	if old, _ := e.st.GetAccessRequest(ctx, grant.ID); old.EndReason == nil || *old.EndReason != "extended" {
		t.Fatalf("the extended grant: %+v", old)
	}
	if grants := decode[[]store.AccessRequest](t, e.call(samToken, "GET", "/api/v1/me/access-grants", nil)); len(grants) != 1 || grants[0].ID != ext.ID || !e.member(g.ID, sam.ID) {
		t.Fatalf("active grants: %+v", grants)
	}
	expect(t, e.call(samToken, "POST", "/api/v1/me/access-requests", map[string]any{"extendsId": grant.ID, "durationDays": 3}), 409)
	if err := expireGrants(ctx, e.st, grant.ExpiresAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !e.member(g.ID, sam.ID) {
		t.Fatal("access ended at the original end date")
	}
	if err := expireGrants(ctx, e.st, extended.ExpiresAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if e.member(g.ID, sam.ID) {
		t.Fatal("the extension's expiry must remove the membership")
	}

	open := e.pkg(admin, map[string]any{"name": "Open", "groupIds": []string{g.ID}, "approverIds": []string{olga.ID}, "requireJustification": false})
	forever := e.decide(olgaToken, e.request(samToken, map[string]any{"packageId": open.ID}).ID, "approve", 200)
	expect(t, e.call(samToken, "POST", "/api/v1/me/access-requests", map[string]any{"extendsId": forever.ID}), 409)
	week := e.decide(olgaToken, e.request(samToken, map[string]any{"packageId": open.ID, "durationDays": 7}).ID, "approve", 200)
	unlimited := e.decide(olgaToken, e.request(samToken, map[string]any{"extendsId": week.ID, "durationDays": nil}).ID, "approve", 200)
	if old, _ := e.st.GetAccessRequest(ctx, week.ID); unlimited.ExpiresAt != nil || old.EndReason == nil || *old.EndReason != "extended" {
		t.Fatalf("an extension until revoked: %+v, old grant %+v", unlimited, old.EndReason)
	}
}

func TestSelfApprovalRefused(t *testing.T) {
	e := setup(t)
	_, admin := e.person("uma@example.com", "user_admin")
	olga, olgaToken := e.person("olga@example.com")
	sam, samToken := e.person("sam@example.com")
	g := e.group("Finance")
	pkg := e.pkg(admin, map[string]any{"name": "Finance", "groupIds": []string{g.ID}, "requireJustification": false,
		"stages": []map[string]any{{"approver": "people", "approverIds": []string{sam.ID, olga.ID}}, {"approver": "people", "approverIds": []string{sam.ID}, "fallbackIds": []string{olga.ID}}}})

	req := e.request(samToken, map[string]any{"packageId": pkg.ID})
	if !slices.Equal(approverIDs(req), []string{olga.ID}) {
		t.Fatalf("the requester is never an approver: %+v", req.Approvers)
	}
	rec := e.call(samToken, "POST", "/api/v1/access-requests/"+req.ID+"/approve", nil)
	expect(t, rec, 403)
	if !strings.Contains(rec.Body.String(), "ERR_SELF_APPROVAL") {
		t.Fatalf("self approval: %s", rec.Body)
	}
	next := e.decide(olgaToken, req.ID, "approve", 200)
	if next.Stage != 2 || !slices.Equal(approverIDs(next), []string{olga.ID}) {
		t.Fatalf("a stage whose only approver is the requester goes to its fallback: %+v", next)
	}
	e.decide(samToken, req.ID, "approve", 403)
	e.decide(olgaToken, req.ID, "approve", 200)
}
