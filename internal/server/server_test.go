package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/store"
	"halo/internal/testdb"
)

type env struct {
	t   *testing.T
	st  *store.Store
	h   http.Handler
	app *App
}

func setup(t *testing.T, configure ...func(*config.Config)) *env {
	t.Helper()
	st := testdb.New(t)
	public, _ := url.Parse("http://localhost:3200")
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	cfg := config.Config{Organization: "Test", PublicURL: public, SecretKey: key, Dev: true}
	for _, fn := range configure {
		fn(&cfg)
	}
	app, err := New(cfg, st)
	if err != nil {
		t.Fatal(err)
	}
	return &env{t: t, st: st, h: app.Handler, app: app}
}

func (e *env) person(email, method string, withTOTP bool, roles ...string) (store.User, string, string) {
	e.t.Helper()
	ctx := context.Background()
	u, err := e.st.CreateUser(ctx, store.NewUser{Email: email, Name: strings.Split(email, "@")[0], Status: "active", Roles: roles})
	if err != nil {
		e.t.Fatal(err)
	}
	seed := ""
	if withTOTP {
		key, _ := totp.Generate(totp.GenerateOpts{Issuer: "Halo", AccountName: email})
		seed = key.Secret()
		secretID, err := e.st.CreateTOTPSecret(ctx, u.ID, "Authenticator app", e.st.Sealer.Seal([]byte(seed)))
		if err == nil {
			_, err = e.st.ConfirmTOTPSecret(ctx, secretID, "Authenticator app", 0)
		}
		if err != nil {
			e.t.Fatal(err)
		}
	}
	_, token, err := e.st.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: method})
	if err != nil {
		e.t.Fatal(err)
	}
	return u, token, seed
}

func (e *env) call(credential, method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if strings.HasPrefix(credential, "hlk_") {
		req.Header.Set("Authorization", "Bearer "+credential)
	} else if credential != "" {
		req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: credential})
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e *env) expect(credential, method, path string, body any, status int, code string) *httptest.ResponseRecorder {
	e.t.Helper()
	rec := e.call(credential, method, path, body)
	if rec.Code != status || code != "" && !strings.Contains(rec.Body.String(), `"`+code+`"`) {
		e.t.Fatalf("%s %s: want %d %s, got %d: %s", method, path, status, code, rec.Code, rec.Body)
	}
	return rec
}

func (e *env) reauthenticate(credential, seed string, at time.Time) {
	e.t.Helper()
	e.expect(credential, "POST", "/api/v1/me/reauthenticate/begin", nil, http.StatusOK, "")
	code, _ := totp.GenerateCode(seed, at)
	e.expect(credential, "POST", "/api/v1/me/reauthenticate", map[string]string{"code": code}, http.StatusNoContent, "")
}

func TestEverySensitiveRouteAsksForRecentSignIn(t *testing.T) {
	e := setup(t)
	_, stale, _ := e.person("ada@example.com", "totp", true, "global_admin")
	param := regexp.MustCompile(`\{[^}]+\}`)
	for _, route := range slices.Concat(sensitiveRoutes, ownMethodRoutes) {
		method, path, _ := strings.Cut(route, " ")
		e.expect(stale, method, param.ReplaceAllString(path, "x"), nil, http.StatusUnauthorized, "ERR_REAUTH_REQUIRED")
	}
}

func TestRecentSignIn(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	admin, stale, seed := e.person("ada@example.com", "totp", true, "global_admin")
	_, fresh, _ := e.person("grace@example.com", "passkey", false, "global_admin")
	_, noMethod, _ := e.person("alan@example.com", "magic-link", false, "global_admin")

	e.expect(stale, "POST", "/api/v1/signing-keys/rotate", nil, http.StatusUnauthorized, "ERR_REAUTH_REQUIRED")
	e.expect(stale, "GET", "/api/v1/signing-keys", nil, http.StatusOK, "")
	e.expect(stale, "POST", "/api/v1/me/recovery-codes", nil, http.StatusUnauthorized, "ERR_REAUTH_REQUIRED")
	e.expect(stale, "POST", "/api/v1/users", map[string]any{"email": "sam@example.com", "name": "Sam", "roles": []string{"auditor"}}, http.StatusUnauthorized, "ERR_REAUTH_REQUIRED")
	e.expect(stale, "POST", "/api/v1/users", map[string]any{"email": "kim@example.com", "name": "Kim"}, http.StatusCreated, "")
	e.expect(fresh, "POST", "/api/v1/signing-keys/rotate", nil, http.StatusOK, "")

	e.expect(noMethod, "POST", "/api/v1/signing-keys/rotate", nil, http.StatusForbidden, "ERR_REAUTH_UNAVAILABLE")
	e.expect(noMethod, "POST", "/api/v1/me/reauthenticate/begin", nil, http.StatusForbidden, "ERR_REAUTH_UNAVAILABLE")
	e.expect(noMethod, "POST", "/api/v1/me/totp/begin", nil, http.StatusOK, "")

	begun := e.expect(stale, "POST", "/api/v1/me/reauthenticate/begin", nil, http.StatusOK, "")
	if !strings.Contains(begun.Body.String(), `"method":"totp"`) {
		t.Fatalf("begin: %s", begun.Body)
	}
	e.expect(stale, "POST", "/api/v1/me/reauthenticate", map[string]string{"code": "000000"}, http.StatusUnauthorized, "ERR_CODE_MISMATCH")
	e.reauthenticate(stale, seed, time.Now())
	e.expect(stale, "POST", "/api/v1/signing-keys/rotate", nil, http.StatusOK, "")
	e.expect(stale, "POST", "/api/v1/users", map[string]any{"email": "sam@example.com", "name": "Sam", "roles": []string{"auditor"}}, http.StatusCreated, "")

	events, err := e.st.ListAudit(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(events, func(a store.AuditEvent) bool {
		return a.Action == "user.reauthenticate" && a.TargetID == admin.ID && strings.Contains(a.Summary, "authenticator app")
	}) {
		t.Fatal("re-authentication is not in the audit log")
	}

	account, err := e.st.CreateServiceAccount(ctx, store.NewServiceAccount{Name: "Terraform", Roles: []string{"global_admin"}})
	if err != nil {
		t.Fatal(err)
	}
	_, key, err := e.st.CreateAPIKey(ctx, account.ID, "Terraform key", []string{"api"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	e.expect(key, "POST", "/api/v1/signing-keys/rotate", nil, http.StatusOK, "")
}

func TestApprovedChangeAppliesAfterApproverReauthenticates(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, requester, _ := e.person("ada@example.com", "passkey", false, "global_admin")
	_, approver, seed := e.person("grace@example.com", "totp", true, "global_admin")
	target, _, _ := e.person("sam@example.com", "passkey", false)

	e.expect(requester, "PATCH", "/api/v1/settings", map[string]any{"approvalActions": []string{"global-admin"}}, http.StatusOK, "")
	var held store.ApprovalRequest
	if err := json.Unmarshal(e.expect(requester, "PUT", "/api/v1/users/"+target.ID+"/roles", map[string]any{"roles": []string{"global_admin"}}, http.StatusAccepted, "").Body.Bytes(), &held); err != nil {
		t.Fatal(err)
	}
	sessions, err := e.st.ListSessions(ctx, held.Requester.ID)
	if err != nil || len(sessions) != 1 {
		t.Fatal(sessions, err)
	}
	if err := e.st.RevokeSession(ctx, sessions[0].ID, ""); err != nil {
		t.Fatal(err)
	}

	e.expect(approver, "POST", "/api/v1/approvals/"+held.ID+"/approve", nil, http.StatusUnauthorized, "ERR_REAUTH_REQUIRED")
	if pending, err := e.st.GetApprovalRequest(ctx, held.ID); err != nil || pending.Status != "pending" {
		t.Fatal(pending.Status, err)
	}
	e.reauthenticate(approver, seed, time.Now())
	e.expect(approver, "POST", "/api/v1/approvals/"+held.ID+"/approve", nil, http.StatusOK, `approved`)
	granted, err := e.st.GetUser(ctx, target.ID)
	if err != nil || !granted.HasRole("global_admin") {
		t.Fatalf("role not applied after approval: %v %v", granted.RoleKeys, err)
	}
}
