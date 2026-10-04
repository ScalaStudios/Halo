package policy_test

import (
	"context"
	"strings"
	"testing"

	"halo/internal/policy"
	"halo/internal/secret"
	"halo/internal/store"
)

func TestBlockingADeviceEndsItsSessions(t *testing.T) {
	st, h := server(t)
	ctx := context.Background()
	_, token := admin(t, st, "sec@example.com", "security_admin")
	owner, _ := admin(t, st, "tia@example.com")
	laptop, phone := strings.Repeat("l", 43), strings.Repeat("p", 43)
	evaluate(t, policy.NewEngine(st), owner, "passkey", "198.51.100.7", laptop)
	devices := decode[[]store.Device](t, call(t, h, token, "GET", "/api/v1/devices?user="+owner.ID, nil))
	if len(devices) != 1 {
		t.Fatalf("devices: %+v", devices)
	}
	for _, d := range []string{laptop, laptop, phone} {
		if _, _, err := st.CreateSession(ctx, store.NewSession{UserID: owner.ID, Method: "passkey", DeviceHash: secret.Hash(d)}); err != nil {
			t.Fatal(err)
		}
	}
	expect(t, call(t, h, token, "PUT", "/api/v1/devices/"+devices[0].ID, map[string]any{"trust": "blocked"}), 200)
	sessions, err := st.ListSessions(ctx, owner.ID)
	if err != nil || len(sessions) != 2 {
		t.Fatalf("sessions left: %d %v", len(sessions), err)
	}
	if audit, _ := st.ListAudit(ctx, 1); audit[0].Summary != "Blocked Firefox on Linux computer and ended 2 sessions" {
		t.Fatalf("audit: %+v", audit[0])
	}
}
