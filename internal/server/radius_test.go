package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	layeh "layeh.com/radius"
	"layeh.com/radius/rfc2865"
	"layeh.com/radius/rfc2866"
	"layeh.com/radius/rfc2869"

	"halo/internal/metrics"
	"halo/internal/secret"
	"halo/internal/store"
)

type radiusEnv struct {
	*env
	staff  store.Group
	admin  string
	server string
}

func setupRADIUS(t *testing.T, secondFactor bool) *radiusEnv {
	t.Helper()
	e := setup(t)
	ctx := context.Background()
	if err := e.st.SetMethodEnabled(ctx, "password", true); err != nil {
		t.Fatal(err)
	}
	s, err := e.st.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	s.PasswordSecondFactor, s.PasswordBreachCheck = secondFactor, false
	if err := e.st.SaveSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	e.st.ForgetSettings()
	_, admin, _ := e.person("admin@example.com", "passkey", false, "global_admin")
	staff, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Staff", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	serving, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- e.app.RADIUS.Serve(serving, conn) }()
	t.Cleanup(func() {
		stop()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	return &radiusEnv{env: e, staff: staff, admin: admin, server: conn.LocalAddr().String()}
}

func (r *radiusEnv) client(addresses ...string) (store.Application, string) {
	r.t.Helper()
	rec := r.expect(r.admin, "POST", "/api/v1/applications", map[string]any{"name": "VPN", "protocol": "radius", "redirectUris": addresses, "groupIds": []string{r.staff.ID}}, http.StatusCreated, "")
	var out struct {
		Application store.Application `json:"application"`
		Secret      *string           `json:"clientSecret"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		r.t.Fatal(err)
	}
	if out.Secret == nil || len(*out.Secret) < 32 || len(out.Application.Credentials) != 1 || strings.Contains(rec.Body.String(), "sealed") {
		r.t.Fatalf("a RADIUS client must get one shared secret, shown once: %s", rec.Body)
	}
	return out.Application, *out.Secret
}

func (r *radiusEnv) member(email, password string, withTOTP bool) (store.User, string) {
	r.t.Helper()
	ctx := context.Background()
	u, _, seed := r.person(email, "passkey", withTOTP)
	if err := r.st.SetPassword(ctx, u.ID, secret.HashPassword(password), false); err != nil {
		r.t.Fatal(err)
	}
	if err := r.st.AddGroupMember(ctx, r.staff.ID, u.ID); err != nil {
		r.t.Fatal(err)
	}
	return u, seed
}

func accessRequest(shared, email, password string, attrs ...func(*layeh.Packet)) *layeh.Packet {
	p := layeh.New(layeh.CodeAccessRequest, []byte(shared))
	_ = rfc2865.UserName_SetString(p, email)
	_ = rfc2865.UserPassword_SetString(p, password)
	for _, fn := range attrs {
		fn(p)
	}
	return p
}

func signed(p *layeh.Packet) *layeh.Packet {
	_ = rfc2869.MessageAuthenticator_Set(p, make([]byte, 16))
	b, _ := p.MarshalBinary()
	mac := hmac.New(md5.New, p.Secret)
	mac.Write(b)
	_ = rfc2869.MessageAuthenticator_Set(p, mac.Sum(nil))
	return p
}

func encoded(t *testing.T, p *layeh.Packet) []byte {
	t.Helper()
	b, err := p.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (r *radiusEnv) dial() net.Conn {
	r.t.Helper()
	conn, err := net.Dial("udp", r.server)
	if err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(func() { conn.Close() })
	return conn
}

func (r *radiusEnv) exchange(conn net.Conn, request []byte, shared string, wait time.Duration) *layeh.Packet {
	r.t.Helper()
	if _, err := conn.Write(request); err != nil {
		r.t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(wait))
	b := make([]byte, layeh.MaxPacketLength)
	n, err := conn.Read(b)
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return nil
	}
	if err != nil {
		r.t.Fatal(err)
	}
	b = b[:n]
	if !layeh.IsAuthenticResponse(b, request, []byte(shared)) {
		r.t.Fatalf("reply must carry a valid Response Authenticator: %x", b)
	}
	if layeh.Code(b[0]) != layeh.CodeAccountingResponse && !replySigned(b, request, []byte(shared)) {
		r.t.Fatalf("every Access reply must start with a valid Message-Authenticator: %x", b)
	}
	reply, err := layeh.Parse(b, []byte(shared))
	if err != nil {
		r.t.Fatal(err)
	}
	return reply
}

func (r *radiusEnv) send(p *layeh.Packet) *layeh.Packet {
	r.t.Helper()
	return r.exchange(r.dial(), encoded(r.t, p), string(p.Secret), 10*time.Second)
}

func (r *radiusEnv) ignored(p *layeh.Packet) bool {
	r.t.Helper()
	return r.exchange(r.dial(), encoded(r.t, p), string(p.Secret), 500*time.Millisecond) == nil
}

func replySigned(reply, request, shared []byte) bool {
	if len(reply) < 38 || reply[20] != 80 || reply[21] != 18 {
		return false
	}
	b := bytes.Clone(reply)
	copy(b[4:20], request[4:20])
	sent := bytes.Clone(b[22:38])
	clear(b[22:38])
	mac := hmac.New(md5.New, shared)
	mac.Write(b)
	return hmac.Equal(mac.Sum(nil), sent)
}

func (r *radiusEnv) events(appID string) []store.SignInEvent {
	r.t.Helper()
	events, err := r.st.ListSignIns(context.Background(), store.SignInFilter{AppID: appID})
	if err != nil {
		r.t.Fatal(err)
	}
	return events
}

func TestRADIUSSignsInAssignedPeople(t *testing.T) {
	r := setupRADIUS(t, false)
	app, shared := r.client("127.0.0.0/8")
	r.expect(r.admin, "PATCH", "/api/v1/applications/"+app.ID, map[string]any{"maxAuthAge": 3600}, http.StatusOK, "")
	r.member("ada@example.com", "correct horse battery staple", false)
	if _, err := r.st.CreateUser(context.Background(), store.NewUser{Email: "grace@example.com", Name: "grace", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	caller := func(p *layeh.Packet) { _ = rfc2865.CallingStationID_SetString(p, "203.0.113.7") }

	accepted := r.send(signed(accessRequest(shared, "ada@example.com", "correct horse battery staple", caller)))
	if accepted == nil || accepted.Code != layeh.CodeAccessAccept || rfc2865.SessionTimeout_Get(accepted) != 3600 {
		t.Fatalf("an assigned person with the right password must be accepted with the sign-in frequency as Session-Timeout: %+v", accepted)
	}
	if wrong := r.send(signed(accessRequest(shared, "ada@example.com", "wrong", caller))); wrong == nil || wrong.Code != layeh.CodeAccessReject {
		t.Fatalf("a wrong password must be rejected: %+v", wrong)
	}
	if unassigned := r.send(signed(accessRequest(shared, "grace@example.com", "anything"))); unassigned == nil || unassigned.Code != layeh.CodeAccessReject {
		t.Fatalf("someone outside the assigned groups must be rejected: %+v", unassigned)
	}
	events := r.events(app.ID)
	if len(events) != 3 || events[2].Result != "success" || events[2].IP != "203.0.113.7" || events[2].Device != "RADIUS · 127.0.0.1" || events[1].Reason != "The password did not match." || events[0].IP != "127.0.0.1" {
		t.Fatalf("each sign-in must be logged against the RADIUS client with the caller's address: %+v", events)
	}

	stop := layeh.New(layeh.CodeAccountingRequest, []byte(shared))
	_ = rfc2866.AcctStatusType_Set(stop, rfc2866.AcctStatusType_Value_Stop)
	_ = rfc2865.UserName_SetString(stop, "ada@example.com")
	_ = rfc2866.AcctSessionTime_Set(stop, 3725)
	_ = rfc2866.AcctTerminateCause_Set(stop, rfc2866.AcctTerminateCause_Value_UserRequest)
	if reply := r.send(stop); reply == nil || reply.Code != layeh.CodeAccountingResponse {
		t.Fatalf("accounting must be acknowledged: %+v", reply)
	}
	stop.Secret = []byte("not the shared secret")
	if !r.ignored(stop) {
		t.Fatal("accounting with another secret must be ignored")
	}
	if ended := r.events(app.ID)[0]; ended.Result != "ended" || ended.Reason != "The RADIUS session ended after 1h2m5s (User-Request)." {
		t.Fatalf("an accounting stop must be logged as an ended session: %+v", ended)
	}
	r.expect(r.admin, "GET", "/api/v1/applications/"+app.ID, nil, http.StatusOK, "")
	if got, err := r.st.GetApplication(context.Background(), app.ID); err != nil || got.SignIns7d != 3 || got.FailureRate7d != 66.7 {
		t.Fatalf("ended sessions must not count as sign-ins or failures: %v %+v", err, got)
	}
}

func TestRADIUSIgnoresUnknownAndUnsignedRequests(t *testing.T) {
	r := setupRADIUS(t, false)
	app, shared := r.client("127.0.0.1")
	_, elsewhere := r.client("10.0.0.0/8")
	r.member("ada@example.com", "correct horse battery staple", false)
	request := func(shared string) *layeh.Packet {
		return accessRequest(shared, "ada@example.com", "correct horse battery staple")
	}
	if ok := r.send(signed(request(shared))); ok == nil || ok.Code != layeh.CodeAccessAccept {
		t.Fatalf("a signed request from an allowed address must be accepted: %+v", ok)
	}
	dropped := metrics.RADIUSRequests.Value("dropped")
	tampered := signed(request(shared))
	tampered.Get(rfc2869.MessageAuthenticator_Type)[0] ^= 1
	twice := signed(request(shared))
	twice.Add(rfc2869.MessageAuthenticator_Type, rfc2869.MessageAuthenticator_Get(twice))
	for name, p := range map[string]*layeh.Packet{
		"unknown secret":              signed(request("not the shared secret")),
		"address not allowed":         signed(request(elsewhere)),
		"no Message-Authenticator":    request(shared),
		"wrong Message-Authenticator": tampered,
		"two Message-Authenticators":  twice,
	} {
		if !r.ignored(p) {
			t.Fatalf("%s: the request must get no reply", name)
		}
	}
	if got := metrics.RADIUSRequests.Value("dropped") - dropped; got != 5 {
		t.Fatalf("dropped requests must be counted: %v", got)
	}
	if events := r.events(app.ID); len(events) != 1 {
		t.Fatalf("dropped requests must not reach the sign-in log: %+v", events)
	}
}

func TestRADIUSAnswersNothingWhileTurnedOff(t *testing.T) {
	r := setupRADIUS(t, false)
	_, shared := r.client("127.0.0.1")
	r.member("ada@example.com", "correct horse battery staple", false)
	request := func() *layeh.Packet {
		return signed(accessRequest(shared, "ada@example.com", "correct horse battery staple"))
	}
	if body := r.expect(r.admin, "PATCH", "/api/v1/settings", map[string]any{"featuresOff": []string{"radius"}}, http.StatusConflict, "ERR_FEATURE_IN_USE").Body.String(); !strings.Contains(body, "1 RADIUS client is registered") {
		t.Fatal(body)
	}
	r.expect(r.admin, "PATCH", "/api/v1/settings", map[string]any{"featuresOff": []string{"radius"}, "confirmFeatures": true}, http.StatusOK, "")
	if !r.ignored(request()) {
		t.Fatal("the RADIUS listener must not answer while the feature is off")
	}
	r.expect(r.admin, "PATCH", "/api/v1/settings", map[string]any{"featuresOff": []string{}}, http.StatusOK, "")
	if ok := r.send(request()); ok == nil || ok.Code != layeh.CodeAccessAccept {
		t.Fatalf("the RADIUS listener must answer again once the feature is on: %+v", ok)
	}
}

func TestRADIUSLocksOutAfterFailedPasswords(t *testing.T) {
	r := setupRADIUS(t, false)
	app, shared := r.client("127.0.0.1")
	r.member("ada@example.com", "correct horse battery staple", false)
	for range 5 {
		if reply := r.send(signed(accessRequest(shared, "ada@example.com", "wrong"))); reply == nil || reply.Code != layeh.CodeAccessReject {
			t.Fatalf("a wrong password must be rejected: %+v", reply)
		}
	}
	if reply := r.send(signed(accessRequest(shared, "ada@example.com", "correct horse battery staple"))); reply == nil || reply.Code != layeh.CodeAccessReject {
		t.Fatalf("the right password must be rejected while the account is locked: %+v", reply)
	}
	if latest := r.events(app.ID)[0]; latest.Result != "failure" || latest.Reason != "Too many failed attempts." {
		t.Fatalf("the lockout must be logged: %+v", latest)
	}
}

func TestRADIUSChallengesForAuthenticatorCode(t *testing.T) {
	r := setupRADIUS(t, true)
	app, shared := r.client("127.0.0.1")
	_, seed := r.member("ada@example.com", "correct horse battery staple", true)
	challenge := func() []byte {
		reply := r.send(signed(accessRequest(shared, "ada@example.com", "correct horse battery staple")))
		if reply == nil || reply.Code != layeh.CodeAccessChallenge || rfc2865.ReplyMessage_GetString(reply) != "Enter the code from your authenticator app." || len(rfc2865.State_Get(reply)) == 0 {
			t.Fatalf("the right password must be answered with a challenge for the code: %+v", reply)
		}
		return rfc2865.State_Get(reply)
	}
	answer := func(state []byte, code string) *layeh.Packet {
		return signed(accessRequest(shared, "ada@example.com", code, func(p *layeh.Packet) { _ = rfc2865.State_Set(p, state) }))
	}

	stale, _ := totp.GenerateCode(seed, time.Now().Add(-time.Hour))
	if reply := r.send(answer(challenge(), stale)); reply == nil || reply.Code != layeh.CodeAccessReject {
		t.Fatalf("a wrong code must be rejected: %+v", reply)
	}
	state := challenge()
	code, _ := totp.GenerateCode(seed, time.Now())
	conn, request := r.dial(), encoded(t, answer(state, code))
	first := r.exchange(conn, request, shared, 10*time.Second)
	again := r.exchange(conn, request, shared, 10*time.Second)
	if first == nil || first.Code != layeh.CodeAccessAccept || again == nil || again.Code != layeh.CodeAccessAccept {
		t.Fatalf("the right code must be accepted, and a retransmission must get the same answer: %+v %+v", first, again)
	}
	if replay := r.send(answer(state, code)); replay == nil || replay.Code != layeh.CodeAccessReject {
		t.Fatalf("a challenge state works once: %+v", replay)
	}
	results := []string{}
	for _, e := range r.events(app.ID) {
		results = append(results, e.Result)
	}
	if strings.Join(results, ",") != "failure,success,failure" {
		t.Fatalf("the retransmission must not sign in twice: %v", results)
	}
}

func TestRADIUSAppliesPoliciesAndRotatedSecrets(t *testing.T) {
	r := setupRADIUS(t, false)
	ctx := context.Background()
	app, old := r.client("127.0.0.1")
	rec := r.expect(r.admin, "POST", "/api/v1/applications/"+app.ID+"/secrets", nil, http.StatusCreated, "")
	var rotated struct {
		Secret string `json:"clientSecret"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rotated); err != nil || len(rotated.Secret) < 32 || strings.HasPrefix(rotated.Secret, "hls_") || rotated.Secret == old {
		t.Fatalf("rotating must create a new shared secret, not a client secret: %v %s", err, rec.Body)
	}
	r.member("ada@example.com", "correct horse battery staple", false)
	bob, _ := r.member("bob@example.com", "correct horse battery staple", false)
	eve, _ := r.member("eve@example.com", "correct horse battery staple", false)
	if _, err := r.st.CreateAccessPolicy(ctx, store.AccessPolicy{Name: "No VPN for Bob", Enabled: true, Mode: "enforce", Effect: "block", Conditions: store.PolicyConditions{UserIDs: []string{bob.ID}, AppIDs: []string{app.ID}}}); err != nil {
		t.Fatal(err)
	}
	s, err := r.st.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	s.EmergencyAccessIDs = []string{eve.ID}
	if err := r.st.SaveSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	r.st.ForgetSettings()

	for _, shared := range []string{old, rotated.Secret} {
		if reply := r.send(signed(accessRequest(shared, "ada@example.com", "correct horse battery staple"))); reply == nil || reply.Code != layeh.CodeAccessAccept {
			t.Fatalf("both secrets must work during the grace period: %+v", reply)
		}
	}
	for email, reason := range map[string]string{"bob@example.com": "Policy “No VPN for Bob”", "eve@example.com": "Emergency access accounts sign in to Halo only, not over RADIUS."} {
		if reply := r.send(signed(accessRequest(rotated.Secret, email, "correct horse battery staple"))); reply == nil || reply.Code != layeh.CodeAccessReject {
			t.Fatalf("%s must be rejected: %+v", email, reply)
		}
		if latest := r.events(app.ID)[0]; latest.Email != email || !strings.HasPrefix(latest.Reason, reason) {
			t.Fatalf("%s: the rejection must be logged with its reason: %+v", email, latest)
		}
	}
}

func TestRADIUSWaitsForIdentityCheck(t *testing.T) {
	r := setupRADIUS(t, false)
	app, shared := r.client("127.0.0.0/8")
	ctx := context.Background()
	s, err := r.st.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	s.IdentityProofingInvites = true
	if err := r.st.SaveSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	r.st.ForgetSettings()
	const password = "correct horse battery staple"
	invite := func(email string, groupIDs ...string) store.User {
		u, err := r.st.CreateUser(ctx, store.NewUser{Email: email, Name: strings.Split(email, "@")[0], GroupIDs: groupIDs})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.st.SetPassword(ctx, u.ID, secret.HashPassword(password), false); err != nil {
			t.Fatal(err)
		}
		return u
	}
	ada, grace := invite("ada@example.com", r.staff.ID), invite("grace@example.com")

	if reply := r.send(signed(accessRequest(shared, "ada@example.com", password))); reply == nil || reply.Code != layeh.CodeAccessReject {
		t.Fatalf("an invited person without an identity check must be rejected: %+v", reply)
	}
	if events := r.events(app.ID); len(events) != 1 || events[0].Reason != "The account is waiting for an identity check." {
		t.Fatalf("the refusal must be logged: %+v", events)
	}
	for _, u := range []store.User{ada, grace} {
		if _, err := r.st.CreateIdentityProof(ctx, store.NewIdentityProof{UserID: u.ID, Method: "in-person", Status: "verified", Document: "passport", Note: "Front desk"}); err != nil {
			t.Fatal(err)
		}
	}
	if reply := r.send(signed(accessRequest(shared, "grace@example.com", password))); reply == nil || reply.Code != layeh.CodeAccessReject {
		t.Fatalf("a checked person outside the assigned groups must still be rejected: %+v", reply)
	}
	if events := r.events(app.ID); events[0].Reason != "User is not assigned to this application." {
		t.Fatalf("the refusal must name the assignment: %+v", events[0])
	}
	if reply := r.send(signed(accessRequest(shared, "ada@example.com", password))); reply == nil || reply.Code != layeh.CodeAccessAccept {
		t.Fatalf("an assigned person with a successful identity check must be accepted: %+v", reply)
	}
}
