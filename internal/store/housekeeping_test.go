package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"halo/internal/store"
	"halo/internal/testdb"
)

func TestPruneLogs(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	add := func(days, signIns, audits int) {
		t.Helper()
		at := time.Now().AddDate(0, 0, -days)
		for range signIns {
			if err := st.RecordSignIn(ctx, store.SignInEvent{Time: at, Result: "success", Method: "passkey"}); err != nil {
				t.Fatal(err)
			}
		}
		for range audits {
			if err := st.RecordAudit(ctx, store.AuditEvent{Time: at, Action: "user.update", Summary: "Changed a profile"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	count := func() (int, []store.AuditEvent) {
		t.Helper()
		signIns, err := st.ListSignIns(ctx, store.SignInFilter{Limit: 1000})
		if err != nil {
			t.Fatal(err)
		}
		audit, err := st.ListAudit(ctx, 1000)
		if err != nil {
			t.Fatal(err)
		}
		return len(signIns), audit
	}
	add(5000, 5, 3)
	add(40, 1, 1)
	add(0, 2, 1)

	if err := st.Purge(ctx); err != nil {
		t.Fatal(err)
	}
	if signIns, audit := count(); signIns != 8 || len(audit) != 5 {
		t.Fatalf("default keeps everything: %d sign-ins, %d audit events", signIns, len(audit))
	}

	values := map[string]json.RawMessage{"signInLogDays": json.RawMessage("30"), "auditLogDays": json.RawMessage("90")}
	if err := st.SaveSettingValues(ctx, values); err != nil {
		t.Fatal(err)
	}
	if err := st.PruneLogs(ctx, 2); err != nil {
		t.Fatal(err)
	}
	signIns, audit := count()
	if signIns != 2 || len(audit) != 3 {
		t.Fatalf("after pruning: %d sign-ins, %d audit events", signIns, len(audit))
	}
	if audit[0].Action != "log.prune" || audit[0].ActorID != nil || audit[0].Summary != "Deleted 6 entries older than 30 days from the sign-in log and 3 entries older than 90 days from the audit log" {
		t.Fatalf("summary: %+v", audit[0])
	}

	add(5000, 1, 1)
	if err := st.PruneLogs(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if signIns, audit := count(); signIns != 3 || len(audit) != 4 {
		t.Fatalf("pruning runs once a day: %d sign-ins, %d audit events", signIns, len(audit))
	}
}
