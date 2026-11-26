package oidc_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"

	halooidc "halo/internal/oidc"
	"halo/internal/store"
	"halo/internal/testdb"
)

const logoutEvent = "http://schemas.openid.net/event/backchannel-logout"

func TestBackchannelLogout(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	tokens := make(chan string, 10)
	var answer atomic.Int32
	answer.Store(http.StatusServiceUnavailable)
	rp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			http.Error(w, "send a form post", http.StatusBadRequest)
			return
		}
		tokens <- r.PostFormValue("logout_token")
		w.WriteHeader(int(answer.Load()))
	}))
	defer rp.Close()
	prev := halooidc.LogoutClient
	halooidc.LogoutClient = rp.Client()
	t.Cleanup(func() { halooidc.LogoutClient = prev })

	res, err := http.Get(h.srv.URL + "/.well-known/openid-configuration")
	must(err)
	var discovery struct {
		Supported        bool `json:"backchannel_logout_supported"`
		SessionSupported bool `json:"backchannel_logout_session_supported"`
	}
	must(json.NewDecoder(res.Body).Decode(&discovery))
	res.Body.Close()
	if !discovery.Supported || !discovery.SessionSupported {
		t.Fatalf("discovery does not advertise back-channel logout: %+v", discovery)
	}

	sess, cookie := h.session(h.user, "passkey")
	_, _, idToken := h.tokens(h.silent(cookie, nil).Query().Get("code"))
	if idToken.SessionID != sess.ID {
		t.Fatalf("ID token sid is %q, want %q", idToken.SessionID, sess.ID)
	}
	app := h.app
	app.BackchannelLogoutURI, app.TokenPolicy.IDTokenAlg = rp.URL+"/backchannel", "ES256"
	must(h.st.UpdateApplication(ctx, app))
	must(h.st.RevokeSession(ctx, sess.ID, h.user.ID))

	now := time.Now()
	must(halooidc.DeliverLogouts(ctx, h.st, h.srv.URL, now))
	first := <-tokens
	deliveries, err := h.st.ListWebhookDeliveries(ctx, app.ID, 10)
	must(err)
	if len(deliveries) != 1 || deliveries[0].Status != "pending" || deliveries[0].Attempts != 1 || *deliveries[0].ResponseStatus != 503 || deliveries[0].EventID != sess.ID || deliveries[0].NextAttemptAt.Sub(now).Round(time.Second) != 30*time.Second {
		t.Fatalf("after a 503: %+v", deliveries)
	}
	answer.Store(http.StatusOK)
	must(halooidc.DeliverLogouts(ctx, h.st, h.srv.URL, now.Add(10*time.Second)))
	if len(tokens) != 0 {
		t.Fatal("retried before the backoff ended")
	}
	must(halooidc.DeliverLogouts(ctx, h.st, h.srv.URL, now.Add(31*time.Second)))
	second := <-tokens
	if deliveries, _ = h.st.ListWebhookDeliveries(ctx, app.ID, 10); deliveries[0].Status != "delivered" || deliveries[0].Attempts != 2 {
		t.Fatalf("after a 200: %+v", deliveries[0])
	}

	res, err = http.Get(h.srv.URL + "/oauth2/keys")
	must(err)
	var keys jose.JSONWebKeySet
	must(json.NewDecoder(res.Body).Decode(&keys))
	res.Body.Close()
	claims := map[string]any{}
	for _, raw := range []string{first, second} {
		signed, err := jose.ParseSigned(raw, []jose.SignatureAlgorithm{jose.ES256})
		must(err)
		header := signed.Signatures[0].Header
		if header.ExtraHeaders["typ"] != "logout+jwt" {
			t.Fatalf("typ header: %v", header.ExtraHeaders)
		}
		found := keys.Key(header.KeyID)
		if len(found) != 1 {
			t.Fatalf("kid %q is not in the published key set", header.KeyID)
		}
		payload, err := signed.Verify(found[0])
		must(err)
		var c map[string]any
		must(json.Unmarshal(payload, &c))
		if c["jti"] == claims["jti"] {
			t.Fatalf("retry reused jti %v", c["jti"])
		}
		claims = c
	}
	events, _ := claims["events"].(map[string]any)
	if _, ok := events[logoutEvent]; !ok || claims["iss"] != idToken.Issuer || claims["aud"] != app.ClientID || claims["sub"] != h.user.ID || claims["sid"] != sess.ID ||
		claims["exp"].(float64) <= claims["iat"].(float64) || claims["nonce"] != nil {
		t.Fatalf("logout token claims: %v", claims)
	}
}

func TestBackchannelLogoutRefusesPrivateAddresses(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	var calls atomic.Int32
	rp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer rp.Close()
	user, app := logoutApp(t, st, rp.URL)
	sess, _, err := st.CreateSession(ctx, store.NewSession{UserID: user.ID, Method: "passkey"})
	if err != nil {
		t.Fatal(err)
	}
	signedIn(t, st, app, sess)
	if err := st.RevokeSession(ctx, sess.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := halooidc.DeliverLogouts(ctx, st, "https://halo.example.com", time.Now()); err != nil {
		t.Fatal(err)
	}
	deliveries, err := st.ListWebhookDeliveries(ctx, app.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 || len(deliveries) != 1 || deliveries[0].Attempts != 1 || deliveries[0].ResponseStatus != nil || !strings.Contains(deliveries[0].Error, "private") {
		t.Fatalf("calls %d, deliveries %+v", calls.Load(), deliveries)
	}
}

func TestBackchannelLogoutOnEverySessionEnd(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	actor, app := logoutApp(t, st, "https://grafana.example.com/backchannel")
	other, err := st.CreateApplication(ctx, store.NewApplication{Name: "Wiki", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://wiki.example.com/callback"}, Scopes: []string{"openid", "profile", "email", "groups", "offline_access"}})
	must(err)
	settings, err := st.Settings(ctx)
	must(err)
	settings.SessionLimit, settings.SessionLimitAction = 1, "end-oldest"
	must(st.SaveSettings(ctx, settings))
	impersonated := func(u store.User, until time.Time) (store.Session, string) {
		i, err := st.CreateImpersonation(ctx, actor.ID, u.ID, "Support ticket", time.Now().Add(time.Hour))
		must(err)
		sess, _, err := st.StartImpersonation(ctx, i.ID, store.NewSession{UserID: u.ID, Method: "passkey"}, until)
		must(err)
		return sess, i.ID
	}
	for _, path := range []struct {
		name string
		end  func(store.User, store.Session) error
	}{
		{"sign-out", func(u store.User, s store.Session) error { return st.RevokeSession(ctx, s.ID, u.ID) }},
		{"admin revocation", func(u store.User, _ store.Session) error { _, err := st.RevokeUserSessions(ctx, u.ID, ""); return err }},
		{"device block", func(u store.User, _ store.Session) error {
			_, err := st.RevokeDeviceSessions(ctx, u.ID, []byte("laptop"))
			return err
		}},
		{"session limit", func(u store.User, _ store.Session) error {
			return st.Tx(ctx, func(tx *store.Store) error { return tx.MakeRoomForSession(ctx, u, false, "") })
		}},
		{"deletion", func(u store.User, _ store.Session) error { _, err := st.ErasePerson(ctx, u); return err }},
		{"deletion after sign-out", func(u store.User, s store.Session) error {
			if err := st.RevokeSession(ctx, s.ID, u.ID); err != nil {
				return err
			}
			_, err := st.ErasePerson(ctx, u)
			return err
		}},
	} {
		t.Run(path.name, func(t *testing.T) {
			u, err := st.CreateUser(ctx, store.NewUser{Email: strings.ReplaceAll(path.name, " ", "-") + "@example.com", Name: "Sam Lee", Status: "active"})
			must(err)
			sess, _, err := st.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: "passkey", DeviceHash: []byte("laptop")})
			must(err)
			signedIn(t, st, app, sess)
			signedIn(t, st, other, sess)
			must(path.end(u, sess))
			expectLogout(t, st, app, u.ID, sess.ID)
		})
	}
	target, err := st.CreateUser(ctx, store.NewUser{Email: "kim@example.com", Name: "Kim Park", Status: "active"})
	must(err)
	t.Run("impersonation end", func(t *testing.T) {
		sess, impersonation := impersonated(target, time.Now().Add(time.Hour))
		signedIn(t, st, app, sess)
		if _, err := st.EndImpersonation(ctx, impersonation); err != nil {
			t.Fatal(err)
		}
		expectLogout(t, st, app, target.ID, sess.ID)
	})
	t.Run("expiry", func(t *testing.T) {
		sess, _ := impersonated(target, time.Now().Add(-time.Minute))
		signedIn(t, st, app, sess)
		must(st.QueueExpiredSessionLogouts(ctx))
		must(st.QueueExpiredSessionLogouts(ctx))
		expectLogout(t, st, app, target.ID, sess.ID)
	})
	if deliveries, _ := st.ListWebhookDeliveries(ctx, other.ID, 10); len(deliveries) != 0 {
		t.Fatalf("queued for an application without a back-channel logout URI: %+v", deliveries)
	}
}

func logoutApp(t *testing.T, st *store.Store, uri string) (store.User, store.Application) {
	t.Helper()
	ctx := context.Background()
	user, err := st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada Lovelace", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", RedirectURIs: []string{callback}, Scopes: []string{"openid", "profile", "email", "groups", "offline_access"}})
	if err != nil {
		t.Fatal(err)
	}
	app.BackchannelLogoutURI = uri
	if err := st.UpdateApplication(ctx, app); err != nil {
		t.Fatal(err)
	}
	return user, app
}

func signedIn(t *testing.T, st *store.Store, app store.Application, sess store.Session) {
	t.Helper()
	err := st.CreateOIDCTokens(context.Background(), store.OIDCToken{ID: "tok_" + app.ID + sess.ID, Kind: "access", ClientID: app.ClientID, UserID: &sess.UserID, SessionID: &sess.ID, ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
}

func expectLogout(t *testing.T, st *store.Store, app store.Application, userID, sessionID string) {
	t.Helper()
	deliveries, err := st.ListWebhookDeliveries(context.Background(), app.ID, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range deliveries {
		var body struct{ Sub, Sid string }
		if d.EventID == sessionID {
			if err := json.Unmarshal([]byte(d.Body), &body); err != nil || body.Sub != userID || body.Sid != sessionID || d.EventType != "backchannel_logout" || d.Status != "pending" {
				t.Fatalf("logout delivery: %+v", d)
			}
			return
		}
	}
	t.Fatalf("no logout queued for session %s: %+v", sessionID, deliveries)
}
