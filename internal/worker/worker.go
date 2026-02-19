package worker

import (
	"context"
	"time"

	"github.com/jvreagan/perf-test/internal/metrics"
	"github.com/jvreagan/perf-test/internal/ratelimit"
)

// Worker executes HTTP requests against weighted endpoints.
type Worker struct {
	id        int
	exec      *Executor
	resultCh  chan<- metrics.Result
	thinkTime time.Duration
	limiter   *ratelimit.Limiter // nil = no rate limiting
}

// New creates a Worker that delegates execution to exec and optionally rate-limits via limiter.
func New(id int, exec *Executor, resultCh chan<- metrics.Result, thinkTime time.Duration, limiter *ratelimit.Limiter) *Worker {
	return &Worker{
		id:        id,
		exec:      exec,
		resultCh:  resultCh,
		thinkTime: thinkTime,
		limiter:   limiter,
	}
}

// Run executes requests in a loop until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Acquire rate-limit token before dispatching (nil-safe no-op when no limiter).
		if !w.limiter.Wait(ctx) {
			return
		}

		ep := w.exec.SelectEndpoint()
		result := w.exec.Execute(ctx, ep)

		// Discard results caused by context cancellation (shutdown artifacts).
		if result.Error != nil && ctx.Err() != nil {
			return
		}

		select {
		case w.resultCh <- result:
		case <-ctx.Done():
			return
		}

		if w.thinkTime > 0 {
			select {
			case <-time.After(w.thinkTime):
			case <-ctx.Done():
				return
			}
		}
	}
}
