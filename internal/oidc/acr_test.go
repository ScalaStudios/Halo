package oidc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	"halo/internal/access"
	"halo/internal/httpx"
	"halo/internal/store"
)

func TestStepUpWithACRValues(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	if _, err := h.st.CreateAuthContext(ctx, store.AuthContext{ID: "passkey-recent", Name: "Passkey in the last 5 minutes", Methods: []string{"passkey", "security-key", "totp"}, MaxAge: 300}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.st.CreateAuthContext(ctx, store.AuthContext{ID: "fresh", Name: "Signed in this second", MaxAge: 1}); err != nil {
		t.Fatal(err)
	}

	res, err := http.Get(h.srv.URL + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatal(err)
	}
	var discovery oidc.DiscoveryConfiguration
	_ = json.NewDecoder(res.Body).Decode(&discovery)
	res.Body.Close()
	if !slices.Equal(discovery.ACRValuesSupported, []string{"fresh", "passkey-recent"}) || !slices.Contains(discovery.ClaimsSupported, "acr") {
		t.Fatalf("discovery: acr values %v, claims %v", discovery.ACRValuesSupported, discovery.ClaimsSupported)
	}

	_, password := h.session(h.user, "password")
	_, passkey := h.session(h.user, "passkey")
	errorOf := func(u *url.URL) string { return u.Query().Get("error") }
	acrOf := func(u *url.URL) (string, []string) {
		t.Helper()
		_, _, claims := h.tokens(u.Query().Get("code"))
		return claims.AuthenticationContextClassReference, claims.AuthenticationMethodsReferences
	}

	if got := errorOf(h.silent(password, url.Values{"acr_values": {"passkey-recent"}})); got != "login_required" {
		t.Fatalf("prompt=none with a session that does not meet the context: %s", got)
	}
	if acr, amr := acrOf(h.silent(passkey, url.Values{"acr_values": {"gold passkey-recent"}})); acr != "passkey-recent" || !slices.Equal(amr, []string{"hwk", "mfa"}) {
		t.Fatalf("a passkey session meets passkey-recent and unknown values are skipped: acr %q amr %v", acr, amr)
	}
	if acr, _ := acrOf(h.silent(password, url.Values{"acr_values": {"gold"}})); acr != "" {
		t.Fatalf("only unknown values: acr %q", acr)
	}
	if acr, _ := acrOf(h.silent(password, nil)); acr != "" {
		t.Fatalf("no acr_values: acr %q", acr)
	}
	essential := func(values ...string) url.Values {
		claims, _ := json.Marshal(map[string]any{"id_token": map[string]any{"acr": map[string]any{"essential": true, "values": values}}})
		return url.Values{"claims": {string(claims)}}
	}
	if got := errorOf(h.silent(passkey, essential("gold"))); got != "access_denied" {
		t.Fatalf("an essential acr Halo cannot meet: %s", got)
	}
	if acr, _ := acrOf(h.silent(passkey, essential("gold", "passkey-recent"))); acr != "passkey-recent" {
		t.Fatalf("an essential acr from the claims parameter: %q", acr)
	}
	mixed := essential("gold")
	mixed.Set("acr_values", "passkey-recent")
	if got := errorOf(h.silent(passkey, mixed)); got != "access_denied" {
		t.Fatalf("acr_values must not stand in for an essential acr Halo cannot meet: %s", got)
	}
	time.Sleep(1100 * time.Millisecond)
	if got := errorOf(h.silent(passkey, url.Values{"acr_values": {"fresh"}})); got != "login_required" {
		t.Fatalf("a session older than the context's maximum age: %s", got)
	}

	const seed = "JBSWY3DPEHPK3PXP"
	secretID, err := h.st.CreateTOTPSecret(ctx, h.user.ID, "Authenticator app", h.st.Sealer.Seal([]byte(seed)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.st.ConfirmTOTPSecret(ctx, secretID, "Authenticator app", 0); err != nil {
		t.Fatal(err)
	}
	q := url.Values{"client_id": {h.app.ClientID}, "redirect_uri": {callback}, "response_type": {"code"}, "scope": {"openid"}, "state": {"st-1"},
		"acr_values": {"passkey-recent"}, "code_challenge": {oidc.NewSHACodeChallenge(pkceSecret)}, "code_challenge_method": {"S256"}}
	status, location := h.get("/oauth2/authorize?"+q.Encode(), password)
	request := location.Query().Get("authRequest")
	if status != http.StatusFound || location.Path != "/sign-in" || request == "" {
		t.Fatalf("authorize: %d %s", status, location)
	}
	before, err := h.st.ListSignIns(ctx, store.SignInFilter{UserID: h.user.ID})
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if status, code, _ := h.signIn(password, "/api/v1/auth/continue", map[string]string{"authRequest": request}); status != http.StatusForbidden || code != "ERR_STEP_UP_REQUIRED" {
			t.Fatalf("continue with a password session: %d %s", status, code)
		}
	}
	after, err := h.st.ListSignIns(ctx, store.SignInFilter{UserID: h.user.ID})
	if err != nil || len(after) != len(before) {
		t.Fatalf("asking for a stronger sign-in is not a failed sign-in: %d events before, %d after (%v)", len(before), len(after), err)
	}

	code, _ := totp.GenerateCode(seed, time.Now())
	status, redirect, cookie := h.signIn("", "/api/v1/auth/totp", map[string]string{"email": h.user.Email, "code": code, "authRequest": request})
	if status != http.StatusOK || cookie == "" {
		t.Fatalf("step-up with an authenticator app: %d %s", status, redirect)
	}
	status, location = h.get(redirect, cookie)
	if status != http.StatusFound || location.Query().Get("code") == "" {
		t.Fatalf("the original request continues after the step-up: %d %s", status, location)
	}
	if acr, amr := acrOf(location); acr != "passkey-recent" || !slices.Equal(amr, []string{"otp"}) {
		t.Fatalf("after step-up: acr %q amr %v", acr, amr)
	}

	_, location = h.get("/oauth2/authorize?"+q.Encode(), passkey)
	status, redirect, _ = h.signIn(passkey, "/api/v1/auth/continue", map[string]string{"authRequest": location.Query().Get("authRequest")})
	if status != http.StatusOK {
		t.Fatalf("a session that already meets the context continues without a new sign-in: %d %s", status, redirect)
	}
	if _, location = h.get(redirect, passkey); location.Query().Get("code") == "" {
		t.Fatalf("callback: %s", location)
	}
	if acr, _ := acrOf(location); acr != "passkey-recent" {
		t.Fatalf("acr without a new sign-in: %q", acr)
	}
}

func TestPolicyContextsApplyToPromptNone(t *testing.T) {
	keys := store.AuthContext{ID: "keys", Name: "Security keys", Methods: []string{"security-key"}}
	h := newHarness(t, policyFunc(func(access.Input) access.Decision {
		return access.Decision{Effect: access.RequireContext, Contexts: []store.AuthContext{keys}}
	}))
	_, passkey := h.session(h.user, "passkey")
	if got := h.silent(passkey, nil).Query().Get("error"); got != "login_required" {
		t.Fatalf("a passkey session under a security-key policy: %s", got)
	}
	_, key := h.session(h.user, "security-key")
	_, _, claims := h.tokens(h.silent(key, nil).Query().Get("code"))
	if claims.AuthenticationContextClassReference != "keys" {
		t.Fatalf("the policy's context is the acr: %q", claims.AuthenticationContextClassReference)
	}
}

func (h *harness) signIn(sessionToken, path string, body any) (int, string, string) {
	h.t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if sessionToken != "" {
		req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: sessionToken})
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	var out struct {
		Redirect string                `json:"redirect"`
		Error    struct{ Code string } `json:"error"`
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	cookie := ""
	for _, c := range res.Cookies() {
		if c.Name == httpx.SessionCookie {
			cookie = c.Value
		}
	}
	if res.StatusCode != http.StatusOK {
		return res.StatusCode, out.Error.Code, cookie
	}
	return res.StatusCode, out.Redirect, cookie
}
