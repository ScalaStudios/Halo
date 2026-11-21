package oidc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"

	halooidc "halo/internal/oidc"
	"halo/internal/store"
)

const (
	caepEvent         = "https://schemas.openid.net/secevent/caep/event-type/"
	riscEvent         = "https://schemas.openid.net/secevent/risc/event-type/"
	verificationEvent = "https://schemas.openid.net/secevent/ssf/event-type/verification"
	pollMethod        = "urn:ietf:rfc:8936"
)

func TestSharedSignals(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	clientToken := func(app store.Application, secret string, form url.Values) string {
		t.Helper()
		r := h.post("/oauth2/token", form, app.ClientID, secret)
		token, _ := r.body["access_token"].(string)
		if r.status != http.StatusOK || strings.Count(token, ".") != 2 {
			t.Fatalf("access token for %s: %d %v", app.Name, r.status, r.body)
		}
		return token
	}
	receiver := func(name string, enabled bool) (store.Application, string, string) {
		t.Helper()
		app, err := h.st.CreateApplication(ctx, store.NewApplication{Name: name, Protocol: "oauth", Type: "service"})
		must(err)
		_, secret, err := h.st.AddClientSecret(ctx, app.ID, "Test", time.Hour)
		must(err)
		if enabled {
			must(h.st.SaveSignalReceiver(ctx, app.ID, "iss_sub"))
		}
		return app, secret, clientToken(app, secret, url.Values{"grant_type": {"client_credentials"}})
	}
	call := func(method, path, token string, body, out any) int {
		t.Helper()
		var payload io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			payload = bytes.NewReader(b)
		}
		req, _ := http.NewRequest(method, h.srv.URL+path, payload)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := http.DefaultClient.Do(req)
		must(err)
		defer res.Body.Close()
		if out != nil {
			must(json.NewDecoder(res.Body).Decode(out))
		}
		return res.StatusCode
	}
	res, err := http.Get(h.srv.URL + "/oauth2/keys")
	must(err)
	var keys jose.JSONWebKeySet
	must(json.NewDecoder(res.Body).Decode(&keys))
	res.Body.Close()
	verify := func(raw string) map[string]any {
		t.Helper()
		signed, err := jose.ParseSigned(raw, []jose.SignatureAlgorithm{jose.RS256})
		must(err)
		header := signed.Signatures[0].Header
		found := keys.Key(header.KeyID)
		if header.ExtraHeaders["typ"] != "secevent+jwt" || len(found) != 1 {
			t.Fatalf("SET header %+v is not typed or its kid is not in the published key set", header)
		}
		payload, err := signed.Verify(found[0])
		must(err)
		var claims map[string]any
		must(json.Unmarshal(payload, &claims))
		if claims["sub"] != nil || claims["exp"] != nil || claims["iss"] != h.srv.URL || claims["iat"] == nil {
			t.Fatalf("SET claims: %v", claims)
		}
		return claims
	}
	poll := func(token, streamID string, body map[string]any) map[string]string {
		t.Helper()
		if _, set := body["returnImmediately"]; !set {
			body["returnImmediately"] = true
		}
		var out struct {
			Sets          map[string]string `json:"sets"`
			MoreAvailable bool              `json:"moreAvailable"`
		}
		if status := call(http.MethodPost, "/oauth2/ssf/poll/"+streamID, token, body, &out); status != http.StatusOK || out.Sets == nil {
			t.Fatalf("poll: %d %+v", status, out)
		}
		return out.Sets
	}
	pending := func(appID string) []string {
		t.Helper()
		deliveries, err := h.st.PendingSignals(ctx, appID, 100)
		must(err)
		types := []string{}
		for _, d := range deliveries {
			types = append(types, d.EventType)
		}
		return types
	}

	var meta map[string]any
	if call(http.MethodGet, "/.well-known/ssf-configuration", "", nil, &meta) != http.StatusOK || meta["issuer"] != h.srv.URL || meta["jwks_uri"] != h.srv.URL+"/oauth2/keys" ||
		meta["configuration_endpoint"] != h.srv.URL+"/oauth2/ssf/stream" || meta["verification_endpoint"] != h.srv.URL+"/oauth2/ssf/verify" || meta["default_subjects"] != "ALL" {
		t.Fatalf("transmitter metadata: %v", meta)
	}

	app, secret, token := receiver("SIEM", true)
	other, _, otherToken := receiver("Ticketing", true)
	_, _, outsider := receiver("Billing sync", false)
	_, _, revokedToken := receiver("Retired", true)
	must(h.st.RevokeOIDCToken(ctx, claimsOf(t, revokedToken).JWTID))
	ledger, err := h.st.SaveAPIResource(ctx, store.APIResource{Name: "Ledger", Identifier: "https://ledger.example.com", AccessTokenTTL: 600, Scopes: []store.APIScope{{Name: "ledger:read"}}})
	must(err)
	for _, a := range []store.Application{app, h.app} {
		must(h.st.SetAPIResourceGrant(ctx, ledger.ID, a.ID, []string{"ledger:read"}))
	}
	forAPI := clientToken(app, secret, url.Values{"grant_type": {"client_credentials"}, "scope": {"ledger:read"}})
	must(h.st.SaveSignalReceiver(ctx, h.app.ID, "iss_sub"))
	_, cookie := h.session(h.user, "passkey")
	personal := clientToken(h.app, h.secret, url.Values{"grant_type": {"authorization_code"}, "code": {h.silent(cookie, nil).Query().Get("code")}, "redirect_uri": {callback}, "code_verifier": {pkceSecret}})
	if aud := claimsOf(t, personal).Audience; !slices.Equal(aud, []string{h.app.ClientID}) {
		t.Fatalf("the person's access token must be addressed to the application itself: %v", aud)
	}
	for name, c := range map[string]struct {
		token string
		want  int
	}{"no token": {"", 401}, "garbage": {"not-a-token", 401}, "revoked": {revokedToken, 401}, "addressed to an API": {forAPI, 401}, "a person's token": {personal, 401}, "not a receiver": {outsider, 403}, "receiver": {token, 200}} {
		if got := call(http.MethodGet, "/oauth2/ssf/stream", c.token, nil, nil); got != c.want {
			t.Errorf("%s: got %d, want %d", name, got, c.want)
		}
	}

	requested := []string{caepEvent + "session-revoked", caepEvent + "credential-change", riscEvent + "account-disabled", riscEvent + "account-enabled", riscEvent + "identifier-changed", "https://example.com/unknown"}
	var stream map[string]any
	if status := call(http.MethodPost, "/oauth2/ssf/stream", token, map[string]any{"events_requested": requested, "description": "Security events", "extension": true}, &stream); status != http.StatusCreated {
		t.Fatalf("create stream: %d %v", status, stream)
	}
	streamID, _ := stream["stream_id"].(string)
	delivery, _ := stream["delivery"].(map[string]any)
	delivered := []any{caepEvent + "session-revoked", caepEvent + "credential-change", riscEvent + "account-disabled", riscEvent + "account-enabled"}
	if stream["iss"] != h.srv.URL || stream["aud"] != app.ClientID || delivery["method"] != pollMethod || delivery["endpoint_url"] != h.srv.URL+"/oauth2/ssf/poll/"+streamID ||
		!reflect.DeepEqual(stream["events_delivered"], delivered) {
		t.Fatalf("created stream: %v", stream)
	}
	if status := call(http.MethodPost, "/oauth2/ssf/stream", token, map[string]any{}, nil); status != http.StatusConflict {
		t.Fatalf("second stream: %d", status)
	}

	for _, endpoint := range []string{"https://127.0.0.1/events", "https://10.0.0.8/events", "http://receiver.example.com/events", "https://localhost/events"} {
		if status := call(http.MethodPost, "/oauth2/ssf/stream", otherToken, map[string]any{"delivery": map[string]any{"method": "urn:ietf:rfc:8935", "endpoint_url": endpoint}}, nil); status != http.StatusBadRequest {
			t.Errorf("push to %s: got %d, want 400", endpoint, status)
		}
	}
	var otherStream map[string]any
	if status := call(http.MethodPost, "/oauth2/ssf/stream", otherToken, map[string]any{"events_requested": []string{riscEvent + "account-disabled"},
		"delivery": map[string]any{"method": "urn:ietf:rfc:8935", "endpoint_url": "https://receiver.example.com/events", "authorization_header": "Bearer receiver-secret"}}, &otherStream); status != http.StatusCreated {
		t.Fatalf("create push stream: %d %v", status, otherStream)
	}
	otherID, _ := otherStream["stream_id"].(string)
	if _, leaked := otherStream["delivery"].(map[string]any)["authorization_header"]; leaked {
		t.Fatal("the stream configuration returned the authorization header")
	}
	if call(http.MethodGet, "/oauth2/ssf/stream?stream_id="+otherID, token, nil, nil) != http.StatusNotFound || call(http.MethodDelete, "/oauth2/ssf/stream?stream_id="+otherID, token, nil, nil) != http.StatusNotFound ||
		call(http.MethodGet, "/oauth2/ssf/status?stream_id="+otherID, token, nil, nil) != http.StatusNotFound || call(http.MethodPost, "/oauth2/ssf/poll/"+otherID, token, map[string]any{}, nil) != http.StatusNotFound {
		t.Fatal("a receiver reached another receiver's stream")
	}
	var listed []map[string]any
	if call(http.MethodGet, "/oauth2/ssf/stream", token, nil, &listed) != http.StatusOK || len(listed) != 1 || listed[0]["stream_id"] != streamID {
		t.Fatalf("list streams: %v", listed)
	}

	if call(http.MethodPost, "/oauth2/ssf/verify", token, map[string]any{"stream_id": streamID, "state": "check-1"}, nil) != http.StatusNoContent ||
		call(http.MethodPost, "/oauth2/ssf/verify", token, map[string]any{"stream_id": streamID, "state": "check-2"}, nil) != http.StatusTooManyRequests {
		t.Fatal("verification requests are not accepted once per interval")
	}
	sets := poll(token, streamID, map[string]any{})
	if len(sets) != 1 {
		t.Fatalf("verification poll: %v", sets)
	}
	var jti string
	for id, set := range sets {
		jti = id
		claims := verify(set)
		events, _ := claims["events"].(map[string]any)
		if claims["jti"] != jti || claims["aud"] != app.ClientID || !reflect.DeepEqual(claims["sub_id"], map[string]any{"format": "opaque", "id": streamID}) ||
			!reflect.DeepEqual(events, map[string]any{verificationEvent: map[string]any{"state": "check-1"}}) {
			t.Fatalf("verification SET: %v", claims)
		}
	}
	if again := poll(token, streamID, map[string]any{}); again[jti] == "" {
		t.Fatal("an unacknowledged SET was not served again")
	}
	if after := poll(token, streamID, map[string]any{"ack": []string{jti}}); len(after) != 0 {
		t.Fatalf("an acknowledged SET was served again: %v", after)
	}

	sess, _ := h.session(h.user, "passkey")
	must(h.st.RevokeSession(ctx, sess.ID, h.user.ID))
	must(h.st.SetUserStatus(ctx, h.user.ID, "suspended"))
	must(h.st.SetUserStatus(ctx, h.user.ID, "suspended"))
	must(h.st.SetUserStatus(ctx, h.user.ID, "active"))
	must(h.st.SetPassword(ctx, h.user.ID, "first", false))
	must(h.st.SetPassword(ctx, h.user.ID, "second", false))
	h.user.Email = "ada.lovelace@example.com"
	must(h.st.UpdateUserProfile(ctx, h.user))
	robot, err := h.st.CreateServiceAccount(ctx, store.NewServiceAccount{Name: "Deploy bot"})
	must(err)
	must(h.st.SetUserStatus(ctx, robot.ID, "suspended"))
	want := []string{caepEvent + "session-revoked", riscEvent + "account-disabled", riscEvent + "account-enabled", caepEvent + "credential-change", caepEvent + "credential-change"}
	if got := pending(app.ID); !slices.Equal(got, want) {
		t.Fatalf("emitted events:\n got %v\nwant %v", got, want)
	}
	if got := pending(other.ID); !slices.Equal(got, []string{riscEvent + "account-disabled"}) {
		t.Fatalf("push stream events: %v", got)
	}

	byType := map[string][]map[string]any{}
	for _, set := range poll(token, streamID, map[string]any{}) {
		claims := verify(set)
		for event, body := range claims["events"].(map[string]any) {
			byType[event] = append(byType[event], map[string]any{"sub_id": claims["sub_id"], "event": body, "txn": claims["txn"]})
		}
	}
	user := map[string]any{"format": "iss_sub", "iss": h.srv.URL, "sub": h.user.ID}
	revokedSession := byType[caepEvent+"session-revoked"]
	if len(revokedSession) != 1 || !reflect.DeepEqual(revokedSession[0]["sub_id"], map[string]any{"format": "complex", "user": user, "session": map[string]any{"format": "opaque", "id": sess.ID}}) || revokedSession[0]["txn"] == nil {
		t.Fatalf("session-revoked: %v", revokedSession)
	}
	changes := []string{}
	for _, c := range byType[caepEvent+"credential-change"] {
		event := c["event"].(map[string]any)
		if event["credential_type"] != "password" || !reflect.DeepEqual(c["sub_id"], user) {
			t.Fatalf("credential-change: %v", c)
		}
		changes = append(changes, event["change_type"].(string))
	}
	if slices.Sort(changes); !slices.Equal(changes, []string{"create", "update"}) {
		t.Fatalf("credential change types: %v", changes)
	}

	otherPending, err := h.st.PendingSignals(ctx, other.ID, 10)
	must(err)
	poll(token, streamID, map[string]any{"ack": []string{otherPending[0].EventID}})
	if got := pending(other.ID); len(got) != 1 {
		t.Fatal("a receiver acknowledged another receiver's event")
	}

	must(h.st.SaveSignalReceiver(ctx, app.ID, "email"))
	h.user.Email = "ada@lovelace.example.com"
	must(h.st.UpdateUserProfile(ctx, h.user))
	h.user.Email = strings.ToUpper(h.user.Email)
	must(h.st.UpdateUserProfile(ctx, h.user))
	var changed []map[string]any
	for _, set := range poll(token, streamID, map[string]any{}) {
		claims := verify(set)
		if event, ok := claims["events"].(map[string]any)[riscEvent+"identifier-changed"].(map[string]any); ok {
			changed = append(changed, map[string]any{"sub_id": claims["sub_id"], "new-value": event["new-value"]})
		}
	}
	if !reflect.DeepEqual(changed, []map[string]any{{"sub_id": map[string]any{"format": "email", "email": "ada.lovelace@example.com"}, "new-value": "ada@lovelace.example.com"}}) {
		t.Fatalf("identifier-changed: %v", changed)
	}

	if call(http.MethodPatch, "/oauth2/ssf/stream", token, map[string]any{"stream_id": streamID, "iss": "https://elsewhere.example.com"}, nil) != http.StatusBadRequest {
		t.Fatal("PATCH accepted a wrong transmitter-supplied iss")
	}
	var patched map[string]any
	if call(http.MethodPatch, "/oauth2/ssf/stream", token, map[string]any{"stream_id": streamID, "description": "SOC", "iss": h.srv.URL, "aud": app.ClientID}, &patched) != http.StatusOK ||
		patched["description"] != "SOC" || len(patched["events_requested"].([]any)) != len(requested) {
		t.Fatalf("PATCH: %v", patched)
	}
	var status map[string]any
	if call(http.MethodPost, "/oauth2/ssf/status", token, map[string]any{"stream_id": streamID, "status": "paused", "reason": "maintenance"}, &status) != http.StatusOK || status["status"] != "paused" {
		t.Fatalf("pause: %v", status)
	}
	must(h.st.SetPassword(ctx, h.user.ID, "third", false))
	if sets := poll(token, streamID, map[string]any{}); len(sets) != 0 || len(pending(app.ID)) == 0 {
		t.Fatalf("a paused stream sent %d events or dropped them", len(sets))
	}
	if call(http.MethodPost, "/oauth2/ssf/status", token, map[string]any{"stream_id": streamID, "status": "disabled"}, nil) != http.StatusOK || len(pending(app.ID)) != 0 {
		t.Fatal("a disabled stream kept its events")
	}
	must(h.st.SetPassword(ctx, h.user.ID, "fourth", false))
	if len(pending(app.ID)) != 0 {
		t.Fatal("a disabled stream queued an event")
	}
	audit, err := h.st.ListAudit(ctx, 50)
	must(err)
	actions := []string{}
	for _, e := range audit {
		if e.TargetID == app.ID {
			actions = append(actions, e.Action)
		}
	}
	if !slices.Equal(actions, []string{"ssf_stream.status", "ssf_stream.status", "ssf_stream.update", "ssf_stream.create"}) {
		t.Fatalf("audit: %v", actions)
	}
	if call(http.MethodDelete, "/oauth2/ssf/stream?stream_id="+streamID, token, nil, nil) != http.StatusNoContent || call(http.MethodGet, "/oauth2/ssf/stream", token, nil, &listed) != http.StatusOK || len(listed) != 0 {
		t.Fatalf("delete stream: %v", listed)
	}

	received := make(chan *http.Request, 10)
	var answer atomic.Int32
	answer.Store(http.StatusServiceUnavailable)
	rp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		received <- r
		w.WriteHeader(int(answer.Load()))
	}))
	defer rp.Close()
	push, err := h.st.GetSignalReceiver(ctx, other.ID)
	must(err)
	push.Stream.EndpointURL = rp.URL + "/events"
	must(h.st.UpdateSignalStream(ctx, *push.Stream))
	now := time.Now()
	must(halooidc.DeliverSignals(ctx, h.st, h.srv.URL, now))
	deliveries, err := h.st.ListSignalDeliveries(ctx, other.ID, 10)
	must(err)
	if len(received) != 0 || len(deliveries) != 1 || deliveries[0].Attempts != 1 || !strings.Contains(deliveries[0].Error, "private") {
		t.Fatalf("the guarded client reached a loopback push endpoint: %+v", deliveries)
	}
	prev := halooidc.SignalClient
	halooidc.SignalClient = rp.Client()
	t.Cleanup(func() { halooidc.SignalClient = prev })
	must(halooidc.DeliverSignals(ctx, h.st, h.srv.URL, now.Add(31*time.Second)))
	first := <-received
	answer.Store(http.StatusAccepted)
	must(halooidc.DeliverSignals(ctx, h.st, h.srv.URL, now.Add(41*time.Second)))
	if len(received) != 0 {
		t.Fatal("retried before the backoff ended")
	}
	must(halooidc.DeliverSignals(ctx, h.st, h.srv.URL, now.Add(92*time.Second)))
	second := <-received
	if deliveries, _ = h.st.ListSignalDeliveries(ctx, other.ID, 10); deliveries[0].Status != "delivered" || deliveries[0].Attempts != 3 {
		t.Fatalf("after a 202: %+v", deliveries[0])
	}
	var jtis []any
	for _, r := range []*http.Request{first, second} {
		body, _ := io.ReadAll(r.Body)
		claims := verify(string(body))
		if r.Header.Get("Content-Type") != "application/secevent+jwt" || r.Header.Get("Accept") != "application/json" || r.Header.Get("Authorization") != "Bearer receiver-secret" ||
			claims["aud"] != other.ClientID || claims["events"].(map[string]any)[riscEvent+"account-disabled"] == nil {
			t.Fatalf("push request %v: %v", r.Header, claims)
		}
		jtis = append(jtis, claims["jti"])
	}
	if jtis[0] != jtis[1] || jtis[0] != deliveries[0].EventID {
		t.Fatalf("a retry changed the jti: %v", jtis)
	}

	if call(http.MethodPost, "/oauth2/ssf/stream", token, map[string]any{"events_requested": []string{caepEvent + "credential-change"}}, &stream) != http.StatusCreated {
		t.Fatalf("recreate stream: %v", stream)
	}
	streamID, _ = stream["stream_id"].(string)
	wait := *halooidc.PollWait
	t.Cleanup(func() { *halooidc.PollWait = wait })
	*halooidc.PollWait = time.Second
	if sets := poll(token, streamID, map[string]any{"returnImmediately": false}); len(sets) != 0 {
		t.Fatalf("an idle long poll returned %v", sets)
	}
	*halooidc.PollWait = 10 * time.Second
	saved := make(chan error, 1)
	go func() {
		time.Sleep(300 * time.Millisecond)
		saved <- h.st.SetPassword(ctx, h.user.ID, "fifth", false)
	}()
	started := time.Now()
	sets = poll(token, streamID, map[string]any{"returnImmediately": false})
	must(<-saved)
	if elapsed := time.Since(started); len(sets) != 1 || elapsed > 5*time.Second {
		t.Fatalf("a long poll returned %d SETs after %s", len(sets), elapsed)
	}
}
