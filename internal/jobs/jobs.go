package jobs

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type job struct {
	name     string
	interval time.Duration
	run      func(context.Context) error
}

type Scheduler struct {
	mu   sync.Mutex
	jobs []job
}

func (s *Scheduler) Every(name string, interval time.Duration, run func(context.Context) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append(s.jobs, job{name: name, interval: interval, run: run})
}

func (s *Scheduler) Run(ctx context.Context) {
	s.mu.Lock()
	jobs := append([]job(nil), s.jobs...)
	s.mu.Unlock()
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(j.interval)
			defer ticker.Stop()
			for {
				if err := j.run(ctx); err != nil && ctx.Err() == nil {
					slog.ErrorContext(ctx, "background job failed", "job", j.name, "error", err)
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}
	wg.Wait()
}
