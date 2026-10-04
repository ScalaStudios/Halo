package apikeys_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/server"
	"halo/internal/store"
	"halo/internal/testdb"
)

func setup(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	st := testdb.New(t)
	public, _ := url.Parse("http://localhost:3200")
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	app, err := server.New(config.Config{Organization: "Test", PublicURL: public, SecretKey: key, Dev: true}, st)
	if err != nil {
		t.Fatal(err)
	}
	return st, app.Handler
}

func person(t *testing.T, st *store.Store, email string, roles ...string) (store.User, string) {
	t.Helper()
	ctx := context.Background()
	u, err := st.CreateUser(ctx, store.NewUser{Email: email, Name: strings.Split(email, "@")[0], Status: "active", Roles: roles})
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := st.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: "passkey"})
	if err != nil {
		t.Fatal(err)
	}
	return u, token
}

func account(t *testing.T, st *store.Store, name string, roles []string, scopes ...string) (store.ServiceAccount, string) {
	t.Helper()
	ctx := context.Background()
	a, err := st.CreateServiceAccount(ctx, store.NewServiceAccount{Name: name, Roles: roles})
	if err != nil {
		t.Fatal(err)
	}
	_, value, err := st.CreateAPIKey(ctx, a.ID, name+" key", scopes, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return a, value
}

func call(t *testing.T, h http.Handler, credential, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	if strings.HasPrefix(credential, "hlk_") {
		req.Header.Set("Authorization", "Bearer "+credential)
	} else if credential != "" {
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: credential})
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func expect(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("want %d, got %d: %s", status, rec.Code, rec.Body)
	}
}

func TestKeyScopesGateAPIAndSCIM(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	workday, scimKey := account(t, st, "Workday connector", []string{"user_admin"}, "scim")
	terraform, apiKey := account(t, st, "Terraform", []string{"app_admin"}, "api")
	_, terraformSCIM := account(t, st, "Terraform SCIM", []string{"app_admin"}, "scim")
	_, admin := person(t, st, "gia@example.com", "global_admin")

	expect(t, call(t, h, scimKey, "GET", "/api/v1/users", nil), 401)
	expect(t, call(t, h, apiKey, "GET", "/api/v1/applications", nil), 200)
	expect(t, call(t, h, apiKey, "POST", "/api/v1/users", map[string]any{"email": "x@example.com", "name": "X"}), 403)
	expect(t, call(t, h, apiKey, "GET", "/scim/v2/Users", nil), 401)
	expect(t, call(t, h, scimKey, "GET", "/scim/v2/Users", nil), 200)
	expect(t, call(t, h, terraformSCIM, "GET", "/scim/v2/Users", nil), 403)
	expect(t, call(t, h, "hlk_not-a-real-key", "GET", "/api/v1/applications", nil), 401)

	rec := call(t, h, apiKey, "POST", "/api/v1/applications", map[string]any{"name": "Grafana", "protocol": "oidc", "type": "web", "redirectUris": []string{"https://grafana.example.com/login/generic_oauth"}})
	expect(t, rec, 201)
	audit, err := st.ListAudit(ctx, 1)
	if err != nil || len(audit) != 1 || audit[0].ActorID == nil || *audit[0].ActorID != terraform.ID {
		t.Fatalf("audit actor must be the service account: %+v %v", audit, err)
	}

	keys, err := st.ListAPIKeys(ctx, terraform.ID)
	if err != nil || len(keys) != 1 || keys[0].LastUsedAt == nil {
		t.Fatalf("last used not recorded: %+v %v", keys, err)
	}

	people, err := st.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(people, func(u store.User) bool { return u.ID == workday.ID || u.ID == terraform.ID }) || len(people) != 1 {
		t.Fatalf("service accounts must not be listed as people: %d people", len(people))
	}
	rec = call(t, h, admin, "GET", "/api/v1/users", nil)
	expect(t, rec, 200)
	if strings.Contains(rec.Body.String(), workday.ID) {
		t.Fatalf("users endpoint lists a service account: %s", rec.Body)
	}
}

func TestRevokedExpiredAndDisabledKeysAreRejected(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, security := person(t, st, "sec@example.com", "security_admin")
	terraform, apiKey := account(t, st, "Terraform", []string{"auditor"}, "api")
	expect(t, call(t, h, apiKey, "GET", "/api/v1/applications", nil), 200)

	past := time.Now().Add(-time.Minute)
	_, expired, err := st.CreateAPIKey(ctx, terraform.ID, "Old", []string{"api"}, &past, nil)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, call(t, h, expired, "GET", "/api/v1/applications", nil), 401)
	if _, _, err := st.ResolveAPIKey(ctx, expired); err != store.ErrNotFound {
		t.Fatalf("expired key resolved: %v", err)
	}

	expect(t, call(t, h, security, "PATCH", "/api/v1/service-accounts/"+terraform.ID, map[string]any{"disabled": true}), 200)
	expect(t, call(t, h, apiKey, "GET", "/api/v1/applications", nil), 401)
	expect(t, call(t, h, security, "PATCH", "/api/v1/service-accounts/"+terraform.ID, map[string]any{"disabled": false}), 200)
	expect(t, call(t, h, apiKey, "GET", "/api/v1/applications", nil), 200)

	keys, err := st.ListAPIKeys(ctx, terraform.ID)
	if err != nil {
		t.Fatal(err)
	}
	active := keys[slices.IndexFunc(keys, func(k store.APIKey) bool { return k.Label == "Terraform key" })]
	expect(t, call(t, h, security, "DELETE", "/api/v1/api-keys/"+active.ID, nil), 204)
	expect(t, call(t, h, security, "DELETE", "/api/v1/api-keys/"+active.ID, nil), 204)
	expect(t, call(t, h, apiKey, "GET", "/api/v1/applications", nil), 401)
}

func TestServiceAccountAndKeyRoleEnforcement(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, auditor := person(t, st, "audrey@example.com", "auditor")
	_, userAdmin := person(t, st, "uma@example.com", "user_admin")
	security, securityToken := person(t, st, "sec@example.com", "security_admin")
	_, global := person(t, st, "gia@example.com", "global_admin")
	appBot, _ := account(t, st, "Deploy bot", []string{"app_admin"}, "api")
	globalBot, globalKey := account(t, st, "Break glass", []string{"global_admin"}, "api")

	expect(t, call(t, h, auditor, "GET", "/api/v1/service-accounts", nil), 200)
	expect(t, call(t, h, auditor, "GET", "/api/v1/api-keys", nil), 200)
	expect(t, call(t, h, auditor, "GET", "/api/v1/service-accounts/"+appBot.ID, nil), 200)
	expect(t, call(t, h, auditor, "POST", "/api/v1/service-accounts", map[string]any{"name": "Nope"}), 403)
	expect(t, call(t, h, userAdmin, "POST", "/api/v1/service-accounts", map[string]any{"name": "Nope"}), 403)
	expect(t, call(t, h, auditor, "DELETE", "/api/v1/api-keys/key_missing", nil), 403)
	expect(t, call(t, h, "", "GET", "/api/v1/service-accounts", nil), 401)

	expect(t, call(t, h, securityToken, "POST", "/api/v1/service-accounts", map[string]any{"name": "Escalation", "roles": []string{"global_admin"}}), 403)
	expect(t, call(t, h, securityToken, "POST", "/api/v1/service-accounts/"+appBot.ID+"/keys", map[string]any{"label": "Mine", "scopes": []string{"api"}}), 403)
	expect(t, call(t, h, securityToken, "PUT", "/api/v1/service-accounts/"+appBot.ID+"/roles", map[string]any{"roles": []string{}}), 403)
	expect(t, call(t, h, securityToken, "PATCH", "/api/v1/service-accounts/"+globalBot.ID, map[string]any{"disabled": true}), 200)
	expect(t, call(t, h, globalKey, "GET", "/api/v1/users", nil), 401)
	expect(t, call(t, h, securityToken, "PATCH", "/api/v1/service-accounts/"+globalBot.ID, map[string]any{"disabled": false}), 403)

	rec := call(t, h, securityToken, "POST", "/api/v1/service-accounts", map[string]any{"name": "SIEM export", "description": "Reads the audit log.", "ownerId": security.ID, "roles": []string{"security_admin"}})
	expect(t, rec, 201)
	var created store.ServiceAccount
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.Email != "siem-export@service.halo.invalid" || created.Status != "active" || !slices.Equal(created.Roles, []string{"security_admin"}) || created.OwnerID == nil || *created.OwnerID != security.ID {
		t.Fatalf("created: %+v", created)
	}
	expect(t, call(t, h, securityToken, "POST", "/api/v1/service-accounts", map[string]any{"name": "siem export"}), 409)
	expect(t, call(t, h, securityToken, "POST", "/api/v1/service-accounts", map[string]any{"name": "Owned by a bot", "ownerId": appBot.ID}), 422)

	expect(t, call(t, h, securityToken, "POST", "/api/v1/service-accounts/"+created.ID+"/keys", map[string]any{"label": "SIEM", "scopes": []string{"admin"}}), 422)
	expect(t, call(t, h, securityToken, "POST", "/api/v1/service-accounts/"+created.ID+"/keys", map[string]any{"label": "SIEM", "scopes": []string{"api"}, "expiresAt": time.Now().Add(-time.Hour)}), 422)
	rec = call(t, h, securityToken, "POST", "/api/v1/service-accounts/"+created.ID+"/keys", map[string]any{"label": "SIEM", "scopes": []string{"api", "api"}, "expiresAt": time.Now().Add(90 * 24 * time.Hour)})
	expect(t, rec, 201)
	var issued struct {
		Key    store.APIKey `json:"key"`
		Secret string       `json:"secret"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &issued)
	if !strings.HasPrefix(issued.Secret, "hlk_") || len(issued.Secret) != 47 || !strings.HasPrefix(issued.Secret, issued.Key.Prefix) || !slices.Equal(issued.Key.Scopes, []string{"api"}) || issued.Key.ExpiresAt == nil {
		t.Fatalf("issued key: %+v", issued)
	}
	rec = call(t, h, auditor, "GET", "/api/v1/service-accounts/"+created.ID, nil)
	expect(t, rec, 200)
	if strings.Contains(rec.Body.String(), issued.Secret) || !strings.Contains(rec.Body.String(), issued.Key.Prefix) {
		t.Fatalf("detail must show the prefix and never the secret: %s", rec.Body)
	}
	rec = call(t, h, global, "GET", "/api/v1/api-keys", nil)
	expect(t, rec, 200)
	if strings.Contains(rec.Body.String(), issued.Secret) {
		t.Fatal("key list leaks the secret")
	}

	expect(t, call(t, h, global, "PUT", "/api/v1/service-accounts/"+appBot.ID+"/roles", map[string]any{"roles": []string{"app_admin", "user_admin"}}), 200)
	refreshed, err := st.GetServiceAccount(ctx, appBot.ID)
	if err != nil || !slices.Equal(refreshed.Roles, []string{"app_admin", "user_admin"}) {
		t.Fatalf("roles: %+v %v", refreshed.Roles, err)
	}
	audit, err := st.ListAudit(ctx, 1)
	if err != nil || audit[0].Action != "service_account.roles.update" || audit[0].TargetID != appBot.ID {
		t.Fatalf("audit: %+v %v", audit, err)
	}
}

func TestServiceAccountsCannotSignInInteractively(t *testing.T) {
	st, _ := setup(t)
	ctx := context.Background()
	bot, _ := account(t, st, "Terraform", []string{"app_admin"}, "api")
	if _, _, err := st.CreateSession(ctx, store.NewSession{UserID: bot.ID, Method: "passkey"}); err == nil {
		t.Fatal("a service account got an interactive session")
	}
	if _, _, err := st.CreateEnrollmentToken(ctx, bot.ID, "reset", nil); err == nil {
		t.Fatal("a service account got a setup link")
	}
}

func TestSeedProvisioning(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	if err := st.SeedDemo(ctx); err != nil {
		t.Fatal(err)
	}
	accounts, err := st.ListServiceAccounts(ctx)
	if err != nil || len(accounts) != 2 {
		t.Fatalf("accounts: %+v %v", accounts, err)
	}
	for _, a := range accounts {
		if len(a.Keys) != 1 || a.OwnerID == nil {
			t.Fatalf("seeded account: %+v", a)
		}
	}
	people, err := st.ListUsers(ctx)
	if err != nil || len(people) != 68 {
		t.Fatalf("people after seeding: %d %v", len(people), err)
	}
	_, admin := person(t, st, "seed-admin@example.com", "global_admin")
	rec := call(t, h, admin, "GET", "/api/v1/api-keys", nil)
	expect(t, rec, 200)
	if strings.Count(rec.Body.String(), `"prefix":"hlk_`) != 2 {
		t.Fatalf("seeded keys: %s", rec.Body)
	}
}
