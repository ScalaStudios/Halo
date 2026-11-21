package jobs

import (
	"context"
	"testing"
	"time"
)

func TestSkippedJobDoesNotRun(t *testing.T) {
	for _, skip := range []bool{true, false} {
		ran := false
		s := &Scheduler{Skip: func(context.Context, string) bool { return skip }}
		s.Every("job", time.Hour, func(context.Context) error {
			ran = true
			return nil
		})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		s.Run(ctx)
		if ran == skip {
			t.Fatalf("skip %v: ran %v", skip, ran)
		}
	}
}
