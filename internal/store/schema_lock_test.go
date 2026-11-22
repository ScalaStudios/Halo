package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"halo/internal/testdb"
)

func TestOrganizationSchemasHaveIndependentJobLocks(t *testing.T) {
	stores := testdb.Schemas(t, 2)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	unlock, err := stores[0].Lock(ctx, "halo:provisioning")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	lockCtx, stop := context.WithTimeout(ctx, time.Second)
	defer stop()
	unlockOther, err := stores[1].Lock(lockCtx, "halo:provisioning")
	if err != nil {
		t.Fatalf("a provisioning lock in the first schema blocked the second: %v", err)
	}
	unlockOther()

	started := make(chan struct{}, len(stores))
	exited := make(chan error, len(stores))
	for _, st := range stores {
		go func() {
			exited <- st.Lead(ctx, func(leading context.Context) {
				started <- struct{}{}
				<-leading.Done()
			})
		}()
	}
	for range stores {
		select {
		case <-started:
		case err := <-exited:
			t.Fatalf("job leader stopped before both schemas elected a leader: %v", err)
		case <-ctx.Done():
			t.Fatal("both schemas should elect a leader")
		}
	}
	cancel()
	for range stores {
		select {
		case err := <-exited:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("job leader did not finish after cancellation")
		}
	}
}
