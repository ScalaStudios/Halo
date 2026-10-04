package governance

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/jobs"
	"halo/internal/mail"
	"halo/internal/store"
	"halo/internal/testdb"
)

type outbox struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (o *outbox) Send(_ context.Context, m mail.Message) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sent = append(o.sent, m)
	return nil
}

func (o *outbox) Configured() bool { return true }

func (o *outbox) to(address string) []mail.Message {
	o.mu.Lock()
	defer o.mu.Unlock()
	var out []mail.Message
	for _, m := range o.sent {
		if m.To == address {
			out = append(out, m)
		}
	}
	return out
}

type env struct {
	t   *testing.T
	st  *store.Store
	h   http.Handler
	box *outbox
}

func setup(t *testing.T) *env {
	t.Helper()
	st := testdb.New(t)
	public, _ := url.Parse("http://localhost:3200")
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	box := &outbox{}
	mux := http.NewServeMux()
	if err := Register(mux, httpx.Deps{Config: config.Config{PublicURL: public, Dev: true}, Store: st, Auth: authn, Mailer: box, Jobs: &jobs.Scheduler{}}); err != nil {
		t.Fatal(err)
	}
	return &env{t: t, st: st, h: authn.Resolve(authn.SameOrigin(mux)), box: box}
}

func (e *env) person(email string, roles ...string) (store.User, string) {
	e.t.Helper()
	ctx := context.Background()
	u, err := e.st.CreateUser(ctx, store.NewUser{Email: email, Name: strings.Split(email, "@")[0], Status: "active", Department: "Engineering", Roles: roles})
	if err != nil {
		e.t.Fatal(err)
	}
	_, token, err := e.st.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: "passkey"})
	if err != nil {
		e.t.Fatal(err)
	}
	return u, token
}

func (e *env) group(name string) store.Group {
	e.t.Helper()
	g, err := e.st.CreateGroup(context.Background(), store.NewGroup{Name: name, Kind: "assigned"})
	if err != nil {
		e.t.Fatal(err)
	}
	return g
}

func (e *env) member(groupID, userID string) bool {
	e.t.Helper()
	u, err := e.st.GetUser(context.Background(), userID)
	if err != nil {
		e.t.Fatal(err)
	}
	return slices.Contains(u.GroupIDs, groupID)
}

func (e *env) call(token, method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: token})
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func expect(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("want %d, got %d: %s", status, rec.Code, rec.Body)
	}
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return v
}

func (e *env) pkg(token string, body map[string]any) store.AccessPackage {
	e.t.Helper()
	rec := e.call(token, "POST", "/api/v1/access-packages", body)
	expect(e.t, rec, 201)
	return decode[store.AccessPackage](e.t, rec)
}

func (e *env) request(token string, body map[string]any) store.AccessRequest {
	e.t.Helper()
	rec := e.call(token, "POST", "/api/v1/me/access-requests", body)
	expect(e.t, rec, 201)
	return decode[store.AccessRequest](e.t, rec)
}

func TestApprovalGrantsOnlyMissingMembershipsAndExpiryRemovesThem(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, admin := e.person("uma@example.com", "user_admin")
	_, globalAdmin := e.person("gia@example.com", "global_admin")
	approver, approverToken := e.person("priya@example.com")
	requester, requesterToken := e.person("grace@example.com")
	prod, dash := e.group("Production access"), e.group("Grafana editors")
	if err := e.st.AddGroupMember(ctx, dash.ID, requester.ID); err != nil {
		t.Fatal(err)
	}
	pkg := e.pkg(admin, map[string]any{"name": "Production access", "groupIds": []string{prod.ID, dash.ID}, "approverIds": []string{approver.ID}, "maxDays": 7, "requireJustification": true})

	expect(t, e.call(requesterToken, "POST", "/api/v1/me/access-requests", map[string]any{"packageId": pkg.ID, "durationDays": 7}), 422)
	expect(t, e.call(requesterToken, "POST", "/api/v1/me/access-requests", map[string]any{"packageId": pkg.ID, "durationDays": 8, "justification": "On-call week"}), 422)
	req := e.request(requesterToken, map[string]any{"packageId": pkg.ID, "durationDays": 7, "justification": "On-call week"})
	expect(t, e.call(requesterToken, "POST", "/api/v1/me/access-requests", map[string]any{"packageId": pkg.ID, "durationDays": 7, "justification": "Again"}), 409)
	if msgs := e.box.to("priya@example.com"); len(msgs) != 1 || !strings.Contains(msgs[0].Text, "On-call week") || !strings.Contains(msgs[0].Text, "http://localhost:3200/account/access") {
		t.Fatalf("approver email: %+v", msgs)
	}

	rec := e.call(approverToken, "POST", "/api/v1/access-requests/"+req.ID+"/approve", map[string]any{"note": "Covering the on-call week"})
	expect(t, rec, 200)
	approved := decode[store.AccessRequest](t, rec)
	if approved.Status != "approved" || approved.ExpiresAt == nil || time.Until(*approved.ExpiresAt) < 6*24*time.Hour || approved.DecidedBy == nil || approved.DecidedBy.ID != approver.ID {
		t.Fatalf("approved: %+v", approved)
	}
	if !e.member(prod.ID, requester.ID) || !e.member(dash.ID, requester.ID) {
		t.Fatal("approval must grant every group in the package")
	}
	if msgs := e.box.to("grace@example.com"); len(msgs) != 1 || !strings.Contains(msgs[0].Subject, "approved") || !strings.Contains(msgs[0].Text, "Covering the on-call week") {
		t.Fatalf("requester email: %+v", msgs)
	}

	if err := expireGrants(ctx, e.st, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !e.member(prod.ID, requester.ID) {
		t.Fatal("grant expired before its end date")
	}
	if err := expireGrants(ctx, e.st, time.Now().Add(8*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if e.member(prod.ID, requester.ID) {
		t.Fatal("expiry must remove the membership the grant added")
	}
	if !e.member(dash.ID, requester.ID) {
		t.Fatal("expiry removed a membership the person had before the grant")
	}
	if ended, _ := e.st.GetAccessRequest(ctx, req.ID); ended.EndReason == nil || *ended.EndReason != "expired" {
		t.Fatalf("end reason: %+v", ended.EndReason)
	}

	oncall := e.pkg(admin, map[string]any{"name": "On-call", "groupIds": []string{prod.ID}, "requireJustification": false})
	first := e.request(requesterToken, map[string]any{"packageId": pkg.ID, "durationDays": 2, "justification": "Incident follow-up"})
	expect(t, e.call(approverToken, "POST", "/api/v1/access-requests/"+first.ID+"/approve", nil), 200)
	second := e.request(requesterToken, map[string]any{"packageId": oncall.ID})
	expect(t, e.call(admin, "POST", "/api/v1/access-requests/"+second.ID+"/approve", nil), 403)
	expect(t, e.call(globalAdmin, "POST", "/api/v1/access-requests/"+second.ID+"/approve", nil), 200)
	if grants := decode[[]store.AccessRequest](t, e.call(requesterToken, "GET", "/api/v1/me/access-grants", nil)); len(grants) != 2 {
		t.Fatalf("active grants: %d", len(grants))
	}
	if err := expireGrants(ctx, e.st, time.Now().Add(3*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !e.member(prod.ID, requester.ID) {
		t.Fatal("expiry removed a membership another active grant still needs")
	}
	expect(t, e.call(admin, "POST", "/api/v1/access-requests/"+second.ID+"/revoke", nil), 200)
	expect(t, e.call(admin, "POST", "/api/v1/access-requests/"+second.ID+"/revoke", nil), 409)
	if e.member(prod.ID, requester.ID) {
		t.Fatal("revoking the last grant must remove the membership it inherited")
	}
	if !e.member(dash.ID, requester.ID) {
		t.Fatal("revocation removed a membership the person had before")
	}
	audit, _ := e.st.ListAudit(ctx, 50)
	actions := map[string]int{}
	for _, a := range audit {
		actions[a.Action]++
	}
	if actions["access_request.create"] != 3 || actions["access_request.approve"] != 3 || actions["access_request.expire"] != 2 || actions["access_request.revoke"] != 1 || actions["access_package.create"] != 2 {
		t.Fatalf("audit: %v", actions)
	}
}

func TestDenyCancelAndSelfApproval(t *testing.T) {
	e := setup(t)
	_, admin := e.person("uma@example.com", "user_admin")
	gia, giaToken := e.person("gia@example.com", "global_admin")
	approver, approverToken := e.person("priya@example.com")
	requester, requesterToken := e.person("hana@example.com")
	_, outsiderToken := e.person("otto@example.com")
	g := e.group("Grafana editors")
	pkg := e.pkg(admin, map[string]any{"name": "Grafana editors", "groupIds": []string{g.ID}, "approverIds": []string{approver.ID, gia.ID}, "maxDays": 30})

	req := e.request(requesterToken, map[string]any{"packageId": pkg.ID, "durationDays": 30, "justification": "Support dashboard"})
	expect(t, e.call(outsiderToken, "POST", "/api/v1/access-requests/"+req.ID+"/approve", nil), 403)
	rec := e.call(approverToken, "POST", "/api/v1/access-requests/"+req.ID+"/deny", map[string]any{"note": "Use the shared dashboard instead"})
	expect(t, rec, 200)
	if denied := decode[store.AccessRequest](t, rec); denied.Status != "denied" || denied.DecisionNote != "Use the shared dashboard instead" {
		t.Fatalf("denied: %+v", denied)
	}
	if e.member(g.ID, requester.ID) {
		t.Fatal("denial granted access")
	}
	if msgs := e.box.to("hana@example.com"); len(msgs) != 1 || !strings.Contains(msgs[0].Subject, "denied") || !strings.Contains(msgs[0].Text, "Use the shared dashboard instead") {
		t.Fatalf("denial email: %+v", msgs)
	}
	expect(t, e.call(approverToken, "POST", "/api/v1/access-requests/"+req.ID+"/approve", nil), 409)

	cancelled := e.request(requesterToken, map[string]any{"packageId": pkg.ID, "durationDays": 7, "justification": "Second try"})
	expect(t, e.call(requesterToken, "POST", "/api/v1/me/access-requests/"+cancelled.ID+"/cancel", nil), 200)
	expect(t, e.call(requesterToken, "POST", "/api/v1/me/access-requests/"+cancelled.ID+"/cancel", nil), 409)
	expect(t, e.call(approverToken, "POST", "/api/v1/access-requests/"+cancelled.ID+"/approve", nil), 409)

	own := e.request(approverToken, map[string]any{"packageId": pkg.ID, "durationDays": 1, "justification": "Fixing an alert"})
	rec = e.call(approverToken, "POST", "/api/v1/access-requests/"+own.ID+"/approve", nil)
	expect(t, rec, 403)
	if body := rec.Body.String(); !strings.Contains(body, "ERR_SELF_APPROVAL") || !strings.Contains(body, "Ask gia or a global administrator") {
		t.Fatalf("self approval: %s", rec.Body)
	}
	if mine := decode[[]store.AccessRequest](t, e.call(approverToken, "GET", "/api/v1/me/approvals", nil)); len(mine) != 0 {
		t.Fatalf("own request offered for approval: %+v", mine)
	}
	if slices.ContainsFunc(e.box.to("priya@example.com"), func(m mail.Message) bool { return strings.HasPrefix(m.Subject, "priya requests") }) {
		t.Fatal("approvers must not be emailed about their own requests")
	}
	if !slices.ContainsFunc(e.box.to("gia@example.com"), func(m mail.Message) bool { return strings.HasPrefix(m.Subject, "priya requests") }) {
		t.Fatal("the other approver must be emailed")
	}
	expect(t, e.call(giaToken, "POST", "/api/v1/access-requests/"+own.ID+"/approve", nil), 200)

	selfRequest := e.request(giaToken, map[string]any{"packageId": pkg.ID, "durationDays": 1, "justification": "Break-glass check"})
	rec = e.call(giaToken, "POST", "/api/v1/access-requests/"+selfRequest.ID+"/approve", nil)
	expect(t, rec, 403)
	if body := rec.Body.String(); !strings.Contains(body, "ERR_SELF_APPROVAL") || !strings.Contains(body, "Ask priya or another global administrator") {
		t.Fatalf("global administrator self approval: %s", body)
	}
	if e.member(g.ID, gia.ID) {
		t.Fatal("a refused self-approval granted access")
	}
	expect(t, e.call(approverToken, "POST", "/api/v1/access-requests/"+selfRequest.ID+"/approve", nil), 200)
	if !e.member(g.ID, gia.ID) {
		t.Fatal("approval by another approver did not grant access")
	}
}

func TestReviewCompletionAppliesRemovals(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, admin := e.person("uma@example.com", "user_admin")
	_, auditor := e.person("audrey@example.com", "auditor")
	reviewer, reviewerToken := e.person("rita@example.com")
	_, outsider := e.person("otto@example.com")
	g := e.group("Production access")
	var members []store.User
	for _, email := range []string{"xavier@example.com", "yara@example.com", "zoe@example.com"} {
		u, _ := e.person(email)
		if err := e.st.AddGroupMember(ctx, g.ID, u.ID); err != nil {
			t.Fatal(err)
		}
		members = append(members, u)
	}
	x, y, z := members[0], members[1], members[2]
	body := map[string]any{"name": "Production access · Q4", "groupId": g.ID, "reviewerIds": []string{reviewer.ID}, "dueAt": time.Now().Add(72 * time.Hour), "autoApply": true}
	expect(t, e.call(auditor, "POST", "/api/v1/access-reviews", body), 403)
	rec := e.call(admin, "POST", "/api/v1/access-reviews", body)
	expect(t, rec, 201)
	review := decode[store.AccessReview](t, rec)
	if review.Total != 3 || len(review.Items) != 3 || review.Status != "in-progress" {
		t.Fatalf("review: %+v", review)
	}
	path := "/api/v1/access-reviews/" + review.ID

	if queue := decode[[]store.AccessReview](t, e.call(reviewerToken, "GET", "/api/v1/me/reviews", nil)); len(queue) != 1 || queue[0].ID != review.ID {
		t.Fatalf("reviewer queue: %+v", queue)
	}
	if got := decode[store.AccessReview](t, e.call(reviewerToken, "GET", path, nil)); !got.CanDecide || !got.CanComplete {
		t.Fatalf("reviewer permissions: %+v", got)
	}
	expect(t, e.call(outsider, "GET", path, nil), 404)
	expect(t, e.call(outsider, "PUT", path+"/decisions/"+x.ID, map[string]any{"decision": "keep"}), 404)
	if got := decode[store.AccessReview](t, e.call(auditor, "GET", path, nil)); got.CanDecide || got.CanComplete {
		t.Fatalf("auditor permissions: %+v", got)
	}
	expect(t, e.call(auditor, "PUT", path+"/decisions/"+x.ID, map[string]any{"decision": "keep"}), 403)
	expect(t, e.call(reviewerToken, "PUT", path+"/decisions/"+x.ID, map[string]any{"decision": "maybe"}), 422)
	expect(t, e.call(reviewerToken, "PUT", path+"/decisions/"+reviewer.ID, map[string]any{"decision": "keep"}), 404)
	expect(t, e.call(reviewerToken, "PUT", path+"/decisions/"+x.ID, map[string]any{"decision": "keep"}), 200)
	rec = e.call(reviewerToken, "PUT", path+"/decisions/"+y.ID, map[string]any{"decision": "remove", "note": "Moved to the mobile team"})
	expect(t, rec, 200)
	if got := decode[store.AccessReview](t, rec); got.Decided != 2 || got.Removals != 1 {
		t.Fatalf("progress: %+v", got)
	}

	rec = e.call(reviewerToken, "POST", path+"/complete", nil)
	expect(t, rec, 200)
	done := decode[store.AccessReview](t, rec)
	outcomes := map[string]string{}
	for _, item := range done.Items {
		outcomes[item.User.ID] = *item.Outcome
	}
	if done.Status != "completed" || outcomes[x.ID] != "kept" || outcomes[y.ID] != "removed" || outcomes[z.ID] != "no-decision" {
		t.Fatalf("outcomes: %s %v", done.Status, outcomes)
	}
	if !e.member(g.ID, x.ID) || e.member(g.ID, y.ID) || !e.member(g.ID, z.ID) {
		t.Fatal("completion must remove exactly the people marked for removal")
	}
	expect(t, e.call(reviewerToken, "POST", path+"/complete", nil), 409)
	expect(t, e.call(reviewerToken, "PUT", path+"/decisions/"+x.ID, map[string]any{"decision": "remove"}), 409)
	if queue := decode[[]store.AccessReview](t, e.call(reviewerToken, "GET", "/api/v1/me/reviews", nil)); len(queue) != 0 {
		t.Fatalf("completed review still queued: %+v", queue)
	}

	body["autoApply"], body["name"] = false, "Production access · recorded only"
	manual := decode[store.AccessReview](t, e.call(admin, "POST", "/api/v1/access-reviews", body))
	expect(t, e.call(reviewerToken, "PUT", "/api/v1/access-reviews/"+manual.ID+"/decisions/"+x.ID, map[string]any{"decision": "remove"}), 200)
	done = decode[store.AccessReview](t, e.call(admin, "POST", "/api/v1/access-reviews/"+manual.ID+"/complete", nil))
	if !e.member(g.ID, x.ID) || done.Items[0].Outcome == nil || *done.Items[0].Outcome != "not-removed" {
		t.Fatalf("without automatic removal the decision is only recorded: %+v", done.Items[0])
	}

	if err := markOverdueReviews(ctx, e.st, time.Now()); err != nil {
		t.Fatal(err)
	}
	late := decode[store.AccessReview](t, e.call(admin, "POST", "/api/v1/access-reviews", map[string]any{"name": "Late", "groupId": g.ID, "reviewerIds": []string{reviewer.ID}, "dueAt": time.Now().Add(time.Hour)}))
	if err := markOverdueReviews(ctx, e.st, time.Now().Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.st.GetAccessReview(ctx, late.ID); got.Status != "overdue" {
		t.Fatalf("overdue: %s", got.Status)
	}
	audit, _ := e.st.ListAudit(ctx, 100)
	if !slices.ContainsFunc(audit, func(a store.AuditEvent) bool {
		return a.Action == "group.member.remove" && a.TargetID == g.ID && strings.Contains(a.Summary, "yara")
	}) {
		t.Fatal("removal by review must be audited")
	}
}

func TestRuleBasedGroupsCannotBeReviewedOrPackaged(t *testing.T) {
	e := setup(t)
	_, admin := e.person("uma@example.com", "user_admin")
	reviewer, _ := e.person("rita@example.com")
	rule := `user.department == "Engineering"`
	dynamic, err := e.st.CreateGroup(context.Background(), store.NewGroup{Name: "Engineering", Kind: "dynamic", Rule: &rule})
	if err != nil {
		t.Fatal(err)
	}
	rec := e.call(admin, "POST", "/api/v1/access-reviews", map[string]any{"name": "Engineering", "groupId": dynamic.ID, "reviewerIds": []string{reviewer.ID}, "dueAt": time.Now().Add(72 * time.Hour)})
	expect(t, rec, 422)
	if !strings.Contains(rec.Body.String(), "rule-based") || !strings.Contains(rec.Body.String(), `user.department == \"Engineering\"`) {
		t.Fatalf("explanation missing: %s", rec.Body)
	}
	expect(t, e.call(admin, "POST", "/api/v1/access-packages", map[string]any{"name": "Engineering", "groupIds": []string{dynamic.ID}}), 422)
	expect(t, e.call(admin, "POST", "/api/v1/lifecycle/rules", map[string]any{"name": "Joiners", "trigger": "joiner", "enabled": true, "actions": []map[string]string{{"type": "add-to-group", "groupId": dynamic.ID}}}), 422)
}

func TestLifecycleRunsOncePerChange(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, admin := e.person("uma@example.com", "user_admin")
	if err := runLifecycle(ctx, e.st); err != nil {
		t.Fatal(err)
	}
	tools := e.group("Engineering tools")
	rule := func(body map[string]any) {
		t.Helper()
		expect(t, e.call(admin, "POST", "/api/v1/lifecycle/rules", body), 201)
	}
	rule(map[string]any{"name": "Engineering joiners", "trigger": "joiner", "condition": `user.department == "Engineering"`, "enabled": true, "actions": []map[string]string{{"type": "add-to-group", "groupId": tools.ID}}})
	rule(map[string]any{"name": "Movers start fresh", "trigger": "mover", "enabled": true, "actions": []map[string]string{{"type": "revoke-sessions"}}})
	rule(map[string]any{"name": "Leavers", "trigger": "leaver", "enabled": true, "actions": []map[string]string{{"type": "revoke-sessions"}, {"type": "remove-from-group", "groupId": tools.ID}}})
	rule(map[string]any{"name": "Disabled", "trigger": "joiner", "enabled": false, "actions": []map[string]string{{"type": "suspend"}}})
	expect(t, e.call(admin, "POST", "/api/v1/lifecycle/rules", map[string]any{"name": "Bad", "trigger": "joiner", "condition": "department = Sales", "enabled": true, "actions": []map[string]string{{"type": "revoke-sessions"}}}), 422)

	runs := func() []store.LifecycleRun {
		t.Helper()
		if err := runLifecycle(ctx, e.st); err != nil {
			t.Fatal(err)
		}
		return decode[[]store.LifecycleRun](t, e.call(admin, "GET", "/api/v1/lifecycle/runs", nil))
	}

	ada, _ := e.person("ada@example.com")
	if _, err := e.st.CreateUser(ctx, store.NewUser{Email: "sam@example.com", Name: "Sam", Department: "Support", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	got := runs()
	if len(got) != 1 || got[0].Trigger != "joiner" || got[0].User.ID != ada.ID || got[0].Steps[0] != "Added to Engineering tools" || got[0].Result != "succeeded" {
		t.Fatalf("joiner runs: %+v", got)
	}
	if !e.member(tools.ID, ada.ID) {
		t.Fatal("joiner rule did not add the group")
	}
	if ada, _ := e.st.GetUser(ctx, ada.ID); ada.Status != "active" {
		t.Fatal("disabled rule ran")
	}
	if again := runs(); len(again) != 1 {
		t.Fatalf("joiner ran twice: %+v", again)
	}

	claimed, err := e.st.ClaimLifecycleChange(ctx, store.LifecycleChange{
		User:     store.User{ID: ada.ID, Department: "Sales", Title: "", Location: "", Status: "active"},
		Previous: &store.LifecycleSnapshot{Department: "Engineering", Status: "active"},
	})
	if err != nil || !claimed {
		t.Fatalf("simulate a move: %v %v", claimed, err)
	}
	got = runs()
	if len(got) != 2 || got[0].Trigger != "mover" || got[0].Change != "Department changed from Sales to Engineering" || got[0].Steps[0] != "Ended 1 session" {
		t.Fatalf("mover runs: %+v", got)
	}
	if again := runs(); len(again) != 2 {
		t.Fatalf("mover ran twice: %+v", again)
	}

	if err := e.st.SetUserStatus(ctx, ada.ID, "suspended"); err != nil {
		t.Fatal(err)
	}
	got = runs()
	if len(got) != 3 || got[0].Trigger != "leaver" || !slices.Equal(got[0].Steps, []string{"Ended 0 sessions", "Removed from Engineering tools"}) {
		t.Fatalf("leaver runs: %+v", got)
	}
	if e.member(tools.ID, ada.ID) {
		t.Fatal("leaver rule did not remove the group")
	}
	if again := runs(); len(again) != 3 {
		t.Fatalf("leaver ran twice: %+v", again)
	}
	if err := e.st.SetUserStatus(ctx, ada.ID, "active"); err != nil {
		t.Fatal(err)
	}
	if after := runs(); len(after) != 3 {
		t.Fatalf("restoring someone is not a lifecycle event: %+v", after)
	}
	rules := decode[[]store.LifecycleRule](t, e.call(admin, "GET", "/api/v1/lifecycle/rules", nil))
	if len(rules) != 4 || rules[0].Runs != 1 || rules[0].LastRunAt == nil {
		t.Fatalf("rules: %+v", rules)
	}
}

func TestLifecycleRulesOnlyMatchPeople(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, admin := e.person("uma@example.com", "user_admin")
	if err := runLifecycle(ctx, e.st); err != nil {
		t.Fatal(err)
	}
	everyone := e.group("Everyone")
	expect(t, e.call(admin, "POST", "/api/v1/lifecycle/rules", map[string]any{"name": "All joiners", "trigger": "joiner", "enabled": true, "actions": []map[string]string{{"type": "add-to-group", "groupId": everyone.ID}}}), 201)
	if _, err := e.st.CreateServiceAccount(ctx, store.NewServiceAccount{Name: "Deploy bot"}); err != nil {
		t.Fatal(err)
	}
	ada, _ := e.person("ada@example.com")
	if err := runLifecycle(ctx, e.st); err != nil {
		t.Fatal(err)
	}
	runs := decode[[]store.LifecycleRun](t, e.call(admin, "GET", "/api/v1/lifecycle/runs", nil))
	if len(runs) != 1 || runs[0].User.ID != ada.ID {
		t.Fatalf("lifecycle rules must only run for people: %+v", runs)
	}
}

func TestDeleteLifecycleRule(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, admin := e.person("uma@example.com", "user_admin")
	_, auditor := e.person("audrey@example.com", "auditor")
	rec := e.call(admin, "POST", "/api/v1/lifecycle/rules", map[string]any{"name": "Leavers", "trigger": "leaver", "enabled": true, "actions": []map[string]string{{"type": "revoke-sessions"}}})
	expect(t, rec, 201)
	rule := decode[store.LifecycleRule](t, rec)

	expect(t, e.call(auditor, "DELETE", "/api/v1/lifecycle/rules/"+rule.ID, nil), 403)
	expect(t, e.call(admin, "DELETE", "/api/v1/lifecycle/rules/"+rule.ID, nil), 204)
	if rules := decode[[]store.LifecycleRule](t, e.call(admin, "GET", "/api/v1/lifecycle/rules", nil)); len(rules) != 0 {
		t.Fatalf("rule not deleted: %+v", rules)
	}
	audit, err := e.st.ListAudit(ctx, 1)
	if err != nil || audit[0].Action != "lifecycle_rule.delete" || audit[0].Summary != "Deleted the leaver rule" || audit[0].TargetID != rule.ID || audit[0].TargetLabel != "Leavers" {
		t.Fatalf("audit: %+v %v", audit, err)
	}
	expect(t, e.call(admin, "DELETE", "/api/v1/lifecycle/rules/"+rule.ID, nil), 404)
}

func TestRoleEnforcement(t *testing.T) {
	e := setup(t)
	_, auditor := e.person("audrey@example.com", "auditor")
	_, nobody := e.person("nora@example.com")
	_, admin := e.person("uma@example.com", "user_admin")
	g := e.group("Ops")

	for _, path := range []string{"/api/v1/access-packages", "/api/v1/access-requests?status=pending", "/api/v1/access-reviews", "/api/v1/lifecycle/rules", "/api/v1/lifecycle/runs"} {
		expect(t, e.call(auditor, "GET", path, nil), 200)
		expect(t, e.call(nobody, "GET", path, nil), 403)
		expect(t, e.call("", "GET", path, nil), 401)
	}
	expect(t, e.call(auditor, "GET", "/api/v1/access-requests?status=granted", nil), 422)
	body := map[string]any{"name": "Ops", "groupIds": []string{g.ID}}
	expect(t, e.call(auditor, "POST", "/api/v1/access-packages", body), 403)
	expect(t, e.call(nobody, "POST", "/api/v1/access-packages", body), 403)
	pkg := e.pkg(admin, body)
	expect(t, e.call(admin, "POST", "/api/v1/access-packages", body), 409)
	expect(t, e.call(auditor, "PUT", "/api/v1/access-packages/"+pkg.ID, body), 403)
	expect(t, e.call(auditor, "DELETE", "/api/v1/access-packages/"+pkg.ID, nil), 403)
	expect(t, e.call(auditor, "POST", "/api/v1/lifecycle/rules", map[string]any{"name": "R", "trigger": "leaver", "enabled": true, "actions": []map[string]string{{"type": "revoke-sessions"}}}), 403)

	req := e.request(nobody, map[string]any{"packageId": pkg.ID})
	expect(t, e.call(auditor, "POST", "/api/v1/access-requests/"+req.ID+"/revoke", nil), 403)
	expect(t, e.call(admin, "DELETE", "/api/v1/access-packages/"+pkg.ID, nil), 409)
	body["archived"] = true
	rec := e.call(admin, "PUT", "/api/v1/access-packages/"+pkg.ID, body)
	expect(t, rec, 200)
	if !decode[store.AccessPackage](t, rec).Archived {
		t.Fatal("archive flag not saved")
	}
	expect(t, e.call(nobody, "POST", "/api/v1/me/access-requests", map[string]any{"packageId": pkg.ID}), 409)
	if offered := decode[[]store.AccessPackage](t, e.call(nobody, "GET", "/api/v1/me/access-packages", nil)); len(offered) != 0 {
		t.Fatalf("archived package offered: %+v", offered)
	}
	unused := e.pkg(admin, map[string]any{"name": "Unused", "groupIds": []string{g.ID}})
	expect(t, e.call(admin, "DELETE", "/api/v1/access-packages/"+unused.ID, nil), 204)
	expect(t, e.call(admin, "DELETE", "/api/v1/access-packages/"+unused.ID, nil), 404)
}

func TestAccountEndpointsOnlyShowTheCallersData(t *testing.T) {
	e := setup(t)
	_, admin := e.person("uma@example.com", "user_admin")
	approver, approverToken := e.person("priya@example.com")
	a, aToken := e.person("ana@example.com")
	b, bToken := e.person("ben@example.com")
	g := e.group("Grafana editors")
	pkg := e.pkg(admin, map[string]any{"name": "Grafana editors", "groupIds": []string{g.ID}, "approverIds": []string{approver.ID}})
	reqA := e.request(aToken, map[string]any{"packageId": pkg.ID})
	reqB := e.request(bToken, map[string]any{"packageId": pkg.ID})

	mine := decode[[]store.AccessRequest](t, e.call(aToken, "GET", "/api/v1/me/access-requests", nil))
	if len(mine) != 1 || mine[0].ID != reqA.ID || mine[0].Requester.ID != a.ID {
		t.Fatalf("requests leaked: %+v", mine)
	}
	expect(t, e.call(aToken, "POST", "/api/v1/me/access-requests/"+reqB.ID+"/cancel", nil), 404)
	if still, _ := e.st.GetAccessRequest(context.Background(), reqB.ID); still.Status != "pending" {
		t.Fatalf("another person's request changed: %s", still.Status)
	}
	if approvals := decode[[]store.AccessRequest](t, e.call(aToken, "GET", "/api/v1/me/approvals", nil)); len(approvals) != 0 {
		t.Fatalf("non-approver sees approvals: %+v", approvals)
	}
	if approvals := decode[[]store.AccessRequest](t, e.call(approverToken, "GET", "/api/v1/me/approvals", nil)); len(approvals) != 2 || !approvals[0].CanDecide {
		t.Fatalf("approver queue: %+v", approvals)
	}
	expect(t, e.call(approverToken, "POST", "/api/v1/access-requests/"+reqA.ID+"/approve", nil), 200)
	if grants := decode[[]store.AccessRequest](t, e.call(aToken, "GET", "/api/v1/me/access-grants", nil)); len(grants) != 1 || grants[0].ID != reqA.ID {
		t.Fatalf("grants: %+v", grants)
	}
	if grants := decode[[]store.AccessRequest](t, e.call(bToken, "GET", "/api/v1/me/access-grants", nil)); len(grants) != 0 {
		t.Fatalf("grants leaked: %+v", grants)
	}

	if err := e.st.AddGroupMember(context.Background(), g.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	review := decode[store.AccessReview](t, e.call(admin, "POST", "/api/v1/access-reviews", map[string]any{"name": "Grafana", "groupId": g.ID, "reviewerIds": []string{a.ID}, "dueAt": time.Now().Add(48 * time.Hour)}))
	if queue := decode[[]store.AccessReview](t, e.call(aToken, "GET", "/api/v1/me/reviews", nil)); len(queue) != 1 || queue[0].ID != review.ID {
		t.Fatalf("reviewer queue: %+v", queue)
	}
	if queue := decode[[]store.AccessReview](t, e.call(bToken, "GET", "/api/v1/me/reviews", nil)); len(queue) != 0 {
		t.Fatalf("reviews leaked: %+v", queue)
	}
	expect(t, e.call(bToken, "GET", "/api/v1/access-reviews/"+review.ID, nil), 404)
	expect(t, e.call(aToken, "GET", "/api/v1/access-reviews/"+review.ID, nil), 200)
}
