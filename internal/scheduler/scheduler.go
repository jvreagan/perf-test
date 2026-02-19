package scheduler

import (
	"context"
	"math"
	"time"

	"github.com/jvreagan/perf-test/internal/config"
)

// Scheduler drives the VU ramp profile by sending target VU counts
// on a channel as time progresses through the configured stages.
type Scheduler struct {
	stages []config.Stage
}

// New creates a Scheduler from the given stages.
func New(stages []config.Stage) *Scheduler {
	return &Scheduler{stages: stages}
}

// Run starts the scheduler in the current goroutine, sending target VU counts
// on targetCh whenever the value changes. It sends a final 0 on completion or
// ctx cancellation. The channel is NOT closed by Run.
func (s *Scheduler) Run(ctx context.Context, targetCh chan<- int) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	start := time.Now()
	lastSent := -1

	// sendIfChanged sends v on targetCh if it differs from lastSent.
	// Returns false if ctx was cancelled before the send completed.
	sendIfChanged := func(v int) bool {
		if v == lastSent {
			return true
		}
		select {
		case targetCh <- v:
			lastSent = v
			return true
		case <-ctx.Done():
			return false
		}
	}

	// sendFinal does a best-effort non-blocking send of 0 for graceful shutdown.
	// Using non-blocking because ctx is already cancelled and we can't block.
	sendFinal := func() {
		if lastSent == 0 {
			return
		}
		select {
		case targetCh <- 0:
		default:
		}
	}

	for {
		select {
		case <-ctx.Done():
			sendFinal()
			return
		case t := <-ticker.C:
			elapsed := t.Sub(start)
			target, done := s.targetAt(elapsed)
			if !sendIfChanged(target) {
				return
			}
			if done {
				return
			}
		}
	}
}

// targetAt returns the interpolated VU target at the given elapsed time.
// done is true when we have passed all stages.
func (s *Scheduler) targetAt(elapsed time.Duration) (target int, done bool) {
	var stageStart time.Duration
	prev := 0

	for i, stage := range s.stages {
		stageEnd := stageStart + stage.Duration.Duration

		if elapsed <= stageEnd {
			// We're within this stage
			stageDur := stage.Duration.Duration
			if stageDur == 0 || stage.Ramp == "step" {
				return stage.Target, i == len(s.stages)-1
			}
			pct := float64(elapsed-stageStart) / float64(stageDur)
			if pct < 0 {
				pct = 0
			}
			if pct > 1 {
				pct = 1
			}
			interpolated := int(math.Round(float64(prev) + float64(stage.Target-prev)*pct))
			return interpolated, false
		}

		prev = stage.Target
		stageStart = stageEnd
	}

	// Past all stages
	return 0, true
}
