package server

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"halo/internal/features"
	"halo/internal/store"
)

func TestFeaturesAreOnByDefault(t *testing.T) {
	e := setup(t)
	_, admin, _ := e.person("ada@example.com", "passkey", false, "global_admin")
	type view struct {
		ID      string   `json:"id"`
		Enabled bool     `json:"enabled"`
		Usage   []string `json:"usage"`
	}
	var list []view
	if err := json.Unmarshal(e.expect(admin, "GET", "/api/v1/features", nil, http.StatusOK, "").Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != len(features.All) || slices.ContainsFunc(list, func(f view) bool { return !f.Enabled || f.Usage == nil }) {
		t.Fatalf("features on a new installation: %+v", list)
	}
	if body := e.expect("", "GET", "/api/v1/organization", nil, http.StatusOK, "").Body.String(); !strings.Contains(body, `"featuresOff":[]`) {
		t.Fatal(body)
	}
	e.expect("", "GET", "/saml/metadata", nil, http.StatusOK, "")
	e.expect(admin, "GET", "/api/v1/webhooks", nil, http.StatusOK, "")
}

func TestTurnedOffFeatureReturnsNotFound(t *testing.T) {
	e := setup(t)
	_, admin, _ := e.person("ada@example.com", "passkey", false, "global_admin")
	upstream := e.call("", "GET", "/saml/sp/idp_x/metadata", nil)

	e.expect(admin, "PATCH", "/api/v1/settings", map[string]any{"featuresOff": []string{"webhooks", "saml", "saml"}}, http.StatusOK, "")
	if body := e.expect("", "GET", "/api/v1/organization", nil, http.StatusOK, "").Body.String(); !strings.Contains(body, `"featuresOff":["saml","webhooks"]`) {
		t.Fatal(body)
	}
	if rec := e.call("", "GET", "/saml/metadata", nil); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "EntityDescriptor") {
		t.Fatalf("SAML metadata with SAML off: %d %s", rec.Code, rec.Body)
	}
	e.expect(admin, "GET", "/api/v1/webhooks", nil, http.StatusNotFound, "ERR_NO_ROUTE")
	e.expect(admin, "GET", "/api/v1/webhooks/whk_x", nil, http.StatusNotFound, "ERR_NO_ROUTE")
	e.expect(admin, "PUT", "/api/v1/applications/app_x/saml", map[string]any{}, http.StatusNotFound, "ERR_NO_ROUTE")
	e.expect(admin, "GET", "/api/v1/applications", nil, http.StatusOK, "")
	if rec := e.call("", "GET", "/saml/sp/idp_x/metadata", nil); rec.Code != upstream.Code || rec.Body.String() != upstream.Body.String() {
		t.Fatalf("SAML identity provider callbacks changed with SAML off: %d %s, was %d %s", rec.Code, rec.Body, upstream.Code, upstream.Body)
	}

	audit := e.expect(admin, "GET", "/api/v1/audit", nil, http.StatusOK, "").Body.String()
	if !strings.Contains(audit, "Changed the features turned off") || !strings.Contains(audit, `"featuresOff"`) {
		t.Fatal(audit)
	}

	e.expect(admin, "PATCH", "/api/v1/settings", map[string]any{"featuresOff": []string{}}, http.StatusOK, "")
	e.expect("", "GET", "/saml/metadata", nil, http.StatusOK, "")
	e.expect(admin, "GET", "/api/v1/webhooks", nil, http.StatusOK, "")
}

func TestEveryFeatureRouteIsGated(t *testing.T) {
	e := setup(t)
	_, admin, _ := e.person("ada@example.com", "passkey", false, "global_admin")
	all := []string{}
	for _, f := range features.All {
		all = append(all, f.ID)
	}
	e.expect(admin, "PATCH", "/api/v1/settings", map[string]any{"featuresOff": all, "confirmFeatures": true}, http.StatusOK, "")
	for _, f := range features.All {
		for _, p := range f.Paths {
			p = strings.ReplaceAll(p, "{id}", "x")
			for _, path := range []string{p, p + "/x"} {
				code := ""
				if strings.HasPrefix(path, "/api/") {
					code = "ERR_NO_ROUTE"
				}
				e.expect(admin, "POST", path, nil, http.StatusNotFound, code)
			}
		}
	}
	e.expect("", "GET", "/scim/v2/Users", nil, http.StatusNotFound, "")
	e.expect(admin, "GET", "/api/v1/users", nil, http.StatusOK, "")
}

func TestFeatureJobsAreSkippedWhileOff(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	var mu sync.Mutex
	registered := map[string]bool{}
	e.app.Jobs.Skip = func(_ context.Context, name string) bool {
		mu.Lock()
		defer mu.Unlock()
		registered[name] = true
		return true
	}
	stopped, cancel := context.WithCancel(ctx)
	cancel()
	e.app.Jobs.Run(stopped)

	skip := features.SkipJob(e.st)
	all := []string{}
	for _, f := range features.All {
		all = append(all, f.ID)
		for _, job := range f.Jobs {
			if !registered[job] {
				t.Fatalf("%s lists the job %q, which is not registered", f.ID, job)
			}
			if skip(ctx, job) {
				t.Fatalf("%q skipped while %s is on", job, f.ID)
			}
		}
	}
	if err := e.st.SaveSettingValues(ctx, map[string]json.RawMessage{"featuresOff": mustJSON(all)}); err != nil {
		t.Fatal(err)
	}
	e.st.ForgetSettings()
	for _, f := range features.All {
		for _, job := range f.Jobs {
			if !skip(ctx, job) {
				t.Fatalf("%q ran while %s is off", job, f.ID)
			}
		}
	}
	for _, job := range []string{"expire access grants", "expire role activations", "expire guest accounts", "purge expired records"} {
		if !registered[job] || skip(ctx, job) {
			t.Fatalf("%q must keep running with every feature off", job)
		}
	}
}

func TestTurningOffAFeatureInUse(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, admin, _ := e.person("ada@example.com", "passkey", false, "global_admin")
	_, security, _ := e.person("grace@example.com", "passkey", false, "security_admin")
	if _, err := e.st.CreateApplication(ctx, store.NewApplication{Name: "Slack", Protocol: "saml", Type: "web", ClientID: "https://slack.com", RedirectURIs: []string{"https://example.slack.com/sso/saml"}}); err != nil {
		t.Fatal(err)
	}

	if body := e.expect(admin, "GET", "/api/v1/features", nil, http.StatusOK, "").Body.String(); !strings.Contains(body, "1 application signs in with SAML") {
		t.Fatal(body)
	}
	rec := e.expect(admin, "PATCH", "/api/v1/settings", map[string]any{"featuresOff": []string{"saml"}}, http.StatusConflict, "ERR_FEATURE_IN_USE")
	if !strings.Contains(rec.Body.String(), "SAML identity provider is in use: 1 application signs in with SAML") {
		t.Fatal(rec.Body)
	}
	e.expect("", "GET", "/saml/metadata", nil, http.StatusOK, "")
	e.expect(admin, "PATCH", "/api/v1/settings", map[string]any{"featuresOff": []string{"nope"}}, http.StatusUnprocessableEntity, "ERR_INVALID")
	e.expect(security, "PATCH", "/api/v1/settings", map[string]any{"featuresOff": []string{"ssh"}}, http.StatusForbidden, "ERR_FORBIDDEN")
	e.expect(admin, "PATCH", "/api/v1/settings", map[string]any{"featuresOff": []string{"saml"}, "confirmFeatures": true}, http.StatusOK, "")
	e.expect("", "GET", "/saml/metadata", nil, http.StatusNotFound, "")
}

func TestTurningFeaturesOffNeedsApproval(t *testing.T) {
	e := setup(t)
	_, requester, _ := e.person("ada@example.com", "passkey", false, "global_admin")
	_, approver, _ := e.person("grace@example.com", "passkey", false, "global_admin")
	e.expect(requester, "PATCH", "/api/v1/settings", map[string]any{"approvalActions": []string{"user-deletion"}}, http.StatusOK, "")

	var held store.ApprovalRequest
	if err := json.Unmarshal(e.expect(requester, "PATCH", "/api/v1/settings", map[string]any{"featuresOff": []string{"ssh"}}, http.StatusAccepted, "").Body.Bytes(), &held); err != nil {
		t.Fatal(err)
	}
	if held.Action != "features" || held.Summary != "Features off after this change: SSH certificate authority" {
		t.Fatalf("%+v", held)
	}
	e.expect(requester, "GET", "/api/v1/ssh/authority", nil, http.StatusOK, "")
	e.expect(approver, "POST", "/api/v1/approvals/"+held.ID+"/approve", nil, http.StatusOK, "approved")
	e.expect(requester, "GET", "/api/v1/ssh/authority", nil, http.StatusNotFound, "ERR_NO_ROUTE")
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
