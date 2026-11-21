package policy_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"halo/internal/access"
	"halo/internal/policy"
	"halo/internal/store"
	"halo/internal/testdb"
)

func TestRiskResponses(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	eng := policy.NewEngine(st)
	box := &outbox{}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	audited := func() []store.AuditEvent {
		t.Helper()
		events, err := st.ListAudit(ctx, 100)
		must(err)
		return slices.DeleteFunc(events, func(e store.AuditEvent) bool { return e.Action != "risk_event.respond" })
	}
	respond := func(responses ...store.RiskResponse) {
		t.Helper()
		raw, _ := json.Marshal(responses)
		must(st.SaveSettingValues(ctx, map[string]json.RawMessage{"riskResponses": raw}))
		settled := len(audited())
		must(policy.Respond(ctx, st, box, "https://halo.example.com", time.Now()))
		if len(audited()) != settled {
			t.Fatal("a risk event must settle before Halo acts on it, so the risky session exists")
		}
		must(policy.Respond(ctx, st, box, "https://halo.example.com", time.Now().Add(time.Minute)))
		must(policy.Respond(ctx, st, box, "https://halo.example.com", time.Now().Add(time.Minute)))
	}
	sessions := func(u store.User) int {
		t.Helper()
		list, err := st.ListSessions(ctx, u.ID)
		must(err)
		return len(list)
	}
	zone(t, st, "Tor exit nodes", "risky", "185.220.101.0/24")
	ada := member(t, st, "ada@example.com")
	erin := member(t, st, "erin@example.com")
	emergency(t, st, erin.ID)
	for _, u := range []store.User{ada, erin} {
		_, _, err := st.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: "passkey"})
		must(err)
	}
	evaluate(t, eng, ada, "totp", "185.220.101.47", "")
	evaluate(t, eng, erin, "totp", "185.220.101.47", "")

	respond(store.RiskResponse{Signal: "risky-network", Level: "high", Action: "require-passkey"}, store.RiskResponse{Level: "medium", Action: "notify"}, store.RiskResponse{Signal: "new-device", Level: "medium", Action: "suspend"})
	if sessions(ada) != 0 || sessions(erin) != 1 {
		t.Fatalf("ada's session must end and the emergency access account's must stay: %d %d", sessions(ada), sessions(erin))
	}
	events := audited()
	if len(events) != 1 || events[0].TargetType != "risk_event" || events[0].ActorID != nil || events[0].TargetLabel != ada.Name ||
		events[0].Summary != "Sign-in from a risky network: Ended 1 session and revoked their tokens; Asked for a passkey or security key at the next sign-in; Emailed ada@example.com" {
		t.Fatalf("one response must be audited for ada, and none for the emergency access account: %+v", events)
	}
	if len(box.sent) != 1 || box.sent[0].To != ada.Email || !strings.Contains(box.sent[0].Text, "185.220.101.47 is in the risky network") || !strings.Contains(box.sent[0].Text, "Your next sign-in needs a passkey or security key.") {
		t.Fatalf("ada must be emailed once: %+v", box.sent)
	}
	if u, err := st.GetUser(ctx, ada.ID); err != nil || u.Status != "active" {
		t.Fatalf("a suspend response for another signal must not apply: %+v %v", u, err)
	}

	d := evaluate(t, eng, ada, "totp", "198.51.100.7", "")
	if d.Effect != access.RequirePhishingResistant || d.Reason != "A risk response asks for a passkey or security key at the next sign-in." {
		t.Fatalf("the next sign-in must need a passkey: %+v", d)
	}
	want(t, evaluate(t, eng, ada, "passkey", "198.51.100.7", ""), access.Allow, "")
	want(t, evaluate(t, eng, ada, "totp", "198.51.100.7", ""), access.Allow, "")

	receiver, err := st.CreateApplication(ctx, store.NewApplication{Name: "SIEM", Protocol: "oauth", Type: "service"})
	must(err)
	must(st.SaveSignalReceiver(ctx, receiver.ID, "iss_sub"))
	_, err = st.CreateSignalStream(ctx, store.SignalStream{AppID: receiver.ID, EventsRequested: store.SignalEvents, DeliveryMethod: store.PollDelivery})
	must(err)
	evaluate(t, eng, ada, "totp", "185.220.101.47", "")
	evaluate(t, eng, erin, "totp", "185.220.101.47", "")
	respond(store.RiskResponse{Level: "high", Action: "suspend"})
	if u, err := st.GetUser(ctx, ada.ID); err != nil || u.Status != "suspended" {
		t.Fatalf("ada must be suspended: %+v %v", u, err)
	}
	if u, err := st.GetUser(ctx, erin.ID); err != nil || u.Status != "active" || sessions(erin) != 1 {
		t.Fatalf("the emergency access account must stay untouched: %+v %v", u, err)
	}
	if events := audited(); len(events) != 2 || events[0].Summary != "Sign-in from a risky network: Suspended the account and ended 0 sessions" {
		t.Fatalf("each risk event must get one response: %+v", events)
	}
	if signals, err := st.PendingSignals(ctx, receiver.ID, 10); err != nil || len(signals) != 1 || !strings.HasSuffix(signals[0].EventType, "/account-disabled") {
		t.Fatalf("the suspension must be sent to shared signals receivers once: %+v %v", signals, err)
	}
}
