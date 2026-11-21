package store_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"halo/internal/store"
	"halo/internal/testdb"
)

func TestSignalTriggers(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "SIEM", Protocol: "oauth", Type: "service"})
	must(err)
	must(st.SaveSignalReceiver(ctx, app.ID, "email"))
	stream, err := st.CreateSignalStream(ctx, store.SignalStream{AppID: app.ID, EventsRequested: store.SignalEvents, DeliveryMethod: store.PollDelivery})
	must(err)
	u, err := st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada", Status: "active"})
	must(err)
	expect := func(step string, want ...map[string]any) {
		t.Helper()
		pending, err := st.PendingSignals(ctx, app.ID, 100)
		must(err)
		got, jtis := []map[string]any{}, []string{}
		for _, d := range pending {
			var sig store.Signal
			must(json.Unmarshal([]byte(d.Body), &sig))
			delete(sig.Event, "event_timestamp")
			event := map[string]any{"type": d.EventType[strings.LastIndex(d.EventType, "/")+1:], "user": sig.User, "email": sig.Email}
			for k, v := range sig.Event {
				event[k] = v
			}
			if sig.Device != "" {
				event["device"] = sig.Device
			}
			got, jtis = append(got, event), append(jtis, d.EventID)
		}
		if !reflect.DeepEqual(got, append([]map[string]any{}, want...)) {
			t.Fatalf("%s:\n got %v\nwant %v", step, got, want)
		}
		must(st.AckSignals(ctx, app.ID, jtis, nil))
	}
	credential := func(kind, change, name string) map[string]any {
		e := map[string]any{"type": "credential-change", "user": u.ID, "email": u.Email, "credential_type": kind, "change_type": change}
		if name != "" {
			e["friendly_name"] = name
		}
		return e
	}

	passkey, err := st.AddWebAuthnCredential(ctx, store.WebAuthnCredential{UserID: u.ID, Kind: "passkey", Label: "MacBook", CredentialID: []byte("c1"), PublicKey: []byte("k1"), Transports: []string{}})
	must(err)
	_, err = st.AddWebAuthnCredential(ctx, store.WebAuthnCredential{UserID: u.ID, Kind: "security-key", Label: "YubiKey", CredentialID: []byte("c2"), PublicKey: []byte("k2"), Transports: []string{}})
	must(err)
	totp, err := st.CreateTOTPSecret(ctx, u.ID, "Phone", st.Sealer.Seal([]byte("seed")))
	must(err)
	_, err = st.ConfirmTOTPSecret(ctx, totp, "Phone", 1)
	must(err)
	must(st.ReplaceRecoveryCodes(ctx, u.ID, [][]byte{st.RecoveryCodeHash("abcd")}))
	expect("adding methods", credential("fido2-platform", "create", "MacBook"), credential("fido2-roaming", "create", "YubiKey"), credential("app", "create", "Phone"))
	must(st.DeleteMethod(ctx, u.ID, passkey.ID))
	must(st.DeleteMethod(ctx, u.ID, "recovery-codes"))
	expect("removing a passkey and recovery codes", credential("fido2-platform", "delete", "MacBook"))
	must(st.DeleteAuthenticationMethods(ctx, u.ID))
	expect("resetting sign-in methods", credential("fido2-roaming", "revoke", "YubiKey"), credential("app", "revoke", "Phone"))

	sourceID, err := st.CreateComplianceSource(ctx, store.ComplianceSource{Name: "Intune", Kind: "intune", TenantID: "contoso", ClientID: "halo", MaxAgeMinutes: 60}, "secret")
	must(err)
	src, err := st.GetComplianceSource(ctx, sourceID)
	must(err)
	deviceID, err := st.TouchDevice(ctx, u.ID, []byte("device-token"), "Mozilla/5.0 (Macintosh)", "198.51.100.7", true)
	must(err)
	external := "mdm-1"
	report := func(state string) {
		t.Helper()
		must(st.ReportCompliance(ctx, src, []store.ComplianceRecord{{ExternalID: external, Name: "ADA-MAC", State: state}}, time.Now()))
	}
	report("compliant")
	must(st.LinkDeviceCompliance(ctx, deviceID, &sourceID, &external))
	for _, state := range []string{"compliant", "non-compliant", "unknown", "compliant"} {
		report(state)
	}
	expect("compliance reports", map[string]any{"type": "device-compliance-change", "user": u.ID, "email": u.Email, "device": deviceID, "previous_status": "compliant", "current_status": "not-compliant"})

	past := time.Now().Add(-time.Hour)
	guest, err := st.CreateUser(ctx, store.NewUser{Email: "guest@partner.example.com", Name: "Guest", Status: "active", Guest: true, ExpiresAt: &past})
	must(err)
	_, err = st.ClaimExpiredGuests(ctx, time.Now())
	must(err)
	_, err = st.ClaimExpiredGuests(ctx, time.Now())
	must(err)
	expect("guest expiry", map[string]any{"type": "account-disabled", "user": guest.ID, "email": guest.Email})

	_, err = st.ErasePerson(ctx, u)
	must(err)
	expect("deletion", map[string]any{"type": "account-purged", "user": u.ID, "email": "ada@example.com"})

	must(st.SetSignalStreamStatus(ctx, stream, "disabled", ""))
	must(st.SetUserStatus(ctx, guest.ID, "active"))
	expect("disabled stream")
}
