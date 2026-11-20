package oidc_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"

	"halo/internal/store"
)

func (h *harness) post(path string, form url.Values, clientID, secret string) result {
	h.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, secret)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	r := result{status: res.StatusCode}
	_ = json.NewDecoder(res.Body).Decode(&r.body)
	return r
}

func claimsOf(t *testing.T, token string) oidc.AccessTokenClaims {
	t.Helper()
	parts := strings.Split(token, ".")
	var claims oidc.AccessTokenClaims
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %q", token)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err == nil {
		err = json.Unmarshal(payload, &claims)
	}
	if err != nil {
		t.Fatal(err)
	}
	return claims
}

func TestTokenExchange(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	billing, err := h.st.SaveAPIResource(ctx, store.APIResource{Name: "Billing", Identifier: "https://billing.example.com", AccessTokenTTL: 300, Scopes: []store.APIScope{{Name: "billing:read"}}})
	must(err)
	ledger, err := h.st.SaveAPIResource(ctx, store.APIResource{Name: "Ledger", Identifier: "https://ledger.example.com", AccessTokenTTL: 3600, Scopes: []store.APIScope{{Name: "ledger:read"}, {Name: "ledger:write"}}})
	must(err)
	must(h.st.SetAPIResourceGrant(ctx, billing.ID, h.app.ID, []string{"billing:read"}))
	service := func(name string, exchange bool, from ...string) (store.Application, string) {
		t.Helper()
		app, err := h.st.CreateApplication(ctx, store.NewApplication{Name: name, Protocol: "oidc", Type: "service"})
		must(err)
		_, secret, err := h.st.AddClientSecret(ctx, app.ID, "Test", time.Hour)
		must(err)
		must(h.st.SetAPIResourceGrant(ctx, ledger.ID, app.ID, []string{"ledger:read", "ledger:write"}))
		app.TokenPolicy.Exchange, app.TokenPolicy.ExchangeFrom = exchange, from
		must(h.st.UpdateApplication(ctx, app))
		return app, secret
	}
	invoices, invoicesSecret := service("Invoices", true, billing.ID, ledger.ID)
	reports, reportsSecret := service("Reports", false)
	payroll, payrollSecret := service("Payroll", true, ledger.ID)
	audit, auditSecret := service("Audit", true)
	spa, err := h.st.CreateApplication(ctx, store.NewApplication{Name: "Dashboard", Protocol: "oidc", Type: "spa", RedirectURIs: []string{"https://dash.example.com/cb"}})
	must(err)
	_, spaSecret, err := h.st.AddClientSecret(ctx, spa.ID, "Test", time.Hour)
	must(err)
	must(h.st.SetAPIResourceGrant(ctx, ledger.ID, spa.ID, []string{"ledger:read"}))
	spa.TokenPolicy.Exchange = true
	must(h.st.UpdateApplication(ctx, spa))
	_, sessionToken := h.session(h.user, "passkey")
	signIn := func(scope string) (string, string) {
		t.Helper()
		code := h.silent(sessionToken, url.Values{"scope": {scope}}).Query().Get("code")
		r := h.post("/oauth2/token", url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callback}, "code_verifier": {pkceSecret}}, h.app.ClientID, h.secret)
		access, _ := r.body["access_token"].(string)
		idToken, _ := r.body["id_token"].(string)
		if r.status != http.StatusOK || access == "" {
			t.Fatalf("sign in: %d %v", r.status, r.body)
		}
		return access, idToken
	}
	exchange := func(clientID, secret, subject string, extra url.Values) result {
		t.Helper()
		form := url.Values{"grant_type": {string(oidc.GrantTypeTokenExchange)}, "subject_token": {subject}, "subject_token_type": {string(oidc.AccessTokenType)}}
		for k, v := range extra {
			form[k] = v
		}
		return h.post("/oauth2/token", form, clientID, secret)
	}
	unaccepted := func(r result) bool {
		description, _ := r.body["error_description"].(string)
		return r.status == 400 && r.body["error"] == "invalid_request" && strings.Contains(description, "accepts tokens from") && r.body["access_token"] == nil
	}
	exchanges := func() int {
		t.Helper()
		events, err := h.st.ListAudit(ctx, 1000)
		must(err)
		return len(slices.DeleteFunc(events, func(e store.AuditEvent) bool { return e.Action != "token.exchange" }))
	}

	subject, idToken := signIn("openid billing:read")
	subjectClaims := claimsOf(t, subject)
	r := exchange(invoices.ClientID, invoicesSecret, subject, url.Values{"audience": {ledger.Identifier}, "scope": {"ledger:read"}})
	exchanged, _ := r.body["access_token"].(string)
	if r.status != http.StatusOK || r.body["issued_token_type"] != string(oidc.AccessTokenType) || r.body["token_type"] != "Bearer" || r.body["scope"] != "ledger:read" || r.body["refresh_token"] != nil {
		t.Fatalf("exchange: %d %v", r.status, r.body)
	}
	claims := claimsOf(t, exchanged)
	if claims.Subject != h.user.ID || !slices.Equal(claims.Audience, []string{ledger.Identifier}) || claims.ClientID != invoices.ClientID || claims.Claims["scope"] != "ledger:read" ||
		claims.Actor == nil || claims.Actor.Subject != invoices.ClientID || claims.Actor.Actor != nil {
		t.Fatalf("exchanged claims: %+v act %+v", claims, claims.Actor)
	}
	if claims.Expiration > subjectClaims.Expiration {
		t.Fatalf("exchanged token outlives its subject: %d > %d", claims.Expiration, subjectClaims.Expiration)
	}
	events, err := h.st.ListAudit(ctx, 1)
	must(err)
	if e := events[0]; e.Action != "token.exchange" || e.TargetID != h.user.ID || e.ActorID != nil || e.Summary != "Invoices exchanged a token of ada@example.com for https://ledger.example.com with ledger:read" {
		t.Fatalf("audit: %+v", e)
	}
	if r := h.post("/oauth2/introspect", url.Values{"token": {exchanged}}, invoices.ClientID, invoicesSecret); r.body["active"] != true || r.body["sub"] != h.user.ID || r.body["client_id"] != invoices.ClientID {
		t.Fatalf("introspect exchanged token: %v", r.body)
	}

	r = exchange(invoices.ClientID, invoicesSecret, subject, url.Values{"resource": {ledger.Identifier}})
	if claims := claimsOf(t, r.body["access_token"].(string)); r.body["scope"] != "ledger:read ledger:write" || !slices.Equal(claims.Audience, []string{ledger.Identifier}) {
		t.Fatalf("resource only: %v %+v", r.body, claims)
	}
	r = exchange(invoices.ClientID, invoicesSecret, subject, url.Values{"audience": {ledger.Identifier}, "resource": {ledger.Identifier}, "scope": {"ledger:write"}})
	if claims := claimsOf(t, r.body["access_token"].(string)); r.body["scope"] != "ledger:write" || !slices.Equal(claims.Audience, []string{ledger.Identifier}) {
		t.Fatalf("audience and resource: %v %+v", r.body, claims)
	}
	r = exchange(invoices.ClientID, invoicesSecret, exchanged, url.Values{"audience": {ledger.Identifier}, "scope": {"ledger:read"}})
	if claims := claimsOf(t, r.body["access_token"].(string)); claims.Actor == nil || claims.Actor.Subject != invoices.ClientID || claims.Actor.Actor == nil || claims.Actor.Actor.Subject != invoices.ClientID {
		t.Fatalf("chained act: %v %+v", r.body, claims.Actor)
	}
	if n := exchanges(); n != 4 {
		t.Fatalf("want 4 token.exchange audit events, got %d", n)
	}

	cc := h.post("/oauth2/token", url.Values{"grant_type": {"client_credentials"}, "scope": {"ledger:read"}}, invoices.ClientID, invoicesSecret)
	ledgerOnly := url.Values{"audience": {ledger.Identifier}}
	for name, c := range map[string]struct {
		clientID, secret, subject string
		extra                     url.Values
		status                    int
		code                      string
	}{
		"exchange turned off":      {reports.ClientID, reportsSecret, subject, ledgerOnly, 400, "unauthorized_client"},
		"wrong secret":             {invoices.ClientID, "hls_wrong", subject, ledgerOnly, 401, "invalid_client"},
		"api not granted":          {invoices.ClientID, invoicesSecret, subject, url.Values{"audience": {billing.Identifier}}, 400, "invalid_target"},
		"unknown audience":         {invoices.ClientID, invoicesSecret, subject, url.Values{"audience": {"https://evil.example.com"}}, 400, "invalid_target"},
		"unknown resource":         {invoices.ClientID, invoicesSecret, subject, url.Values{"audience": {ledger.Identifier}, "resource": {"https://evil.example.com"}}, 400, "invalid_target"},
		"no target":                {invoices.ClientID, invoicesSecret, subject, url.Values{"scope": {"ledger:read"}}, 400, "invalid_request"},
		"scope of another api":     {invoices.ClientID, invoicesSecret, subject, url.Values{"audience": {ledger.Identifier}, "scope": {"billing:read"}}, 400, "invalid_scope"},
		"openid scope":             {invoices.ClientID, invoicesSecret, subject, url.Values{"audience": {ledger.Identifier}, "scope": {"openid ledger:read"}}, 400, "invalid_scope"},
		"id token subject":         {invoices.ClientID, invoicesSecret, idToken, url.Values{"audience": {ledger.Identifier}, "subject_token_type": {string(oidc.IDTokenType)}}, 400, "invalid_request"},
		"refresh token requested":  {invoices.ClientID, invoicesSecret, subject, url.Values{"audience": {ledger.Identifier}, "requested_token_type": {string(oidc.RefreshTokenType)}}, 400, "invalid_request"},
		"id token requested":       {invoices.ClientID, invoicesSecret, subject, url.Values{"audience": {ledger.Identifier}, "requested_token_type": {string(oidc.IDTokenType)}}, 400, "invalid_request"},
		"actor token":              {invoices.ClientID, invoicesSecret, subject, url.Values{"audience": {ledger.Identifier}, "actor_token": {subject}, "actor_token_type": {string(oidc.AccessTokenType)}}, 400, "invalid_request"},
		"client credentials token": {invoices.ClientID, invoicesSecret, cc.body["access_token"].(string), ledgerOnly, 400, "invalid_request"},
		"garbage subject token":    {invoices.ClientID, invoicesSecret, "not-a-token", ledgerOnly, 400, "invalid_request"},
		"public client":            {spa.ClientID, spaSecret, subject, ledgerOnly, 400, "unauthorized_client"},
		"no client authentication": {"", "", subject, ledgerOnly, 401, "invalid_client"},
	} {
		if r := exchange(c.clientID, c.secret, c.subject, c.extra); r.status != c.status || r.body["error"] != c.code || r.body["access_token"] != nil {
			t.Errorf("%s: want %d %s, got %d %v", name, c.status, c.code, r.status, r.body)
		}
	}
	if r := exchange(payroll.ClientID, payrollSecret, subject, ledgerOnly); !unaccepted(r) {
		t.Errorf("subject token for an API the application doesn't accept: %d %v", r.status, r.body)
	}
	if r := exchange(audit.ClientID, auditSecret, subject, ledgerOnly); !unaccepted(r) {
		t.Errorf("exchange on with no accepted APIs: %d %v", r.status, r.body)
	}

	if r := h.post("/oauth2/revoke", url.Values{"token": {subject}}, h.app.ClientID, h.secret); r.status != http.StatusOK {
		t.Fatalf("revoke: %d %v", r.status, r.body)
	}
	if r := exchange(invoices.ClientID, invoicesSecret, subject, ledgerOnly); r.status != 400 || r.body["error"] != "invalid_request" {
		t.Fatalf("revoked subject token: %d %v", r.status, r.body)
	}
	if r := h.post("/oauth2/introspect", url.Values{"token": {exchanged}}, invoices.ClientID, invoicesSecret); r.body["active"] != false {
		t.Fatalf("a token exchanged from a revoked token must be inactive: %v", r.body)
	}

	subject, _ = signIn("openid billing:read")
	must(h.st.RemoveGroupMember(ctx, h.group.ID, h.user.ID))
	if r := exchange(invoices.ClientID, invoicesSecret, subject, ledgerOnly); r.status != 400 || r.body["error"] != "invalid_request" {
		t.Fatalf("subject no longer assigned to the application: %d %v", r.status, r.body)
	}
	must(h.st.AddGroupMember(ctx, h.group.ID, h.user.ID))

	must(h.st.SetAPIResourceGrant(ctx, billing.ID, h.app.ID, nil))
	subject, _ = signIn("openid")
	if r := exchange(invoices.ClientID, invoicesSecret, subject, ledgerOnly); strings.Count(subject, ".") == 2 || !unaccepted(r) {
		t.Fatalf("opaque subject token addressed to its client id: %d %v", r.status, r.body)
	}
	app, err := h.st.GetApplication(ctx, h.app.ID)
	must(err)
	app.TokenPolicy.AccessTokenTTL = 1
	must(h.st.UpdateApplication(ctx, app))
	subject, _ = signIn("openid")
	time.Sleep(1100 * time.Millisecond)
	if r := exchange(invoices.ClientID, invoicesSecret, subject, ledgerOnly); r.status != 400 || r.body["error"] != "invalid_request" || unaccepted(r) {
		t.Fatalf("expired subject token: %d %v", r.status, r.body)
	}
	if n := exchanges(); n != 4 {
		t.Fatalf("failed exchanges must not be audited, got %d events", n)
	}
}
