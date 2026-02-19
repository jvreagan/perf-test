package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/jvreagan/perf-test/internal/config"
)

func stages(durs []time.Duration, targets []int) []config.Stage {
	s := make([]config.Stage, len(durs))
	for i := range durs {
		s[i] = config.Stage{
			Duration: config.Duration{Duration: durs[i]},
			Target:   targets[i],
		}
	}
	return s
}

func TestTargetAt_SingleStage(t *testing.T) {
	s := New(stages([]time.Duration{10 * time.Second}, []int{100}))

	// At 0s: 0% through stage, target = 0
	v, done := s.targetAt(0)
	if v != 0 || done {
		t.Errorf("at 0s: expected (0, false), got (%d, %v)", v, done)
	}

	// At 5s: 50% through stage, target = 50
	v, done = s.targetAt(5 * time.Second)
	if v != 50 || done {
		t.Errorf("at 5s: expected (50, false), got (%d, %v)", v, done)
	}

	// At 10s: 100% through stage, target = 100
	v, done = s.targetAt(10 * time.Second)
	if v != 100 {
		t.Errorf("at 10s: expected 100, got %d", v)
	}
	_ = done
}

func TestTargetAt_MultiStage(t *testing.T) {
	// Stage 1: 0→50 over 10s, Stage 2: hold 50 for 20s, Stage 3: 50→0 over 10s
	s := New(stages(
		[]time.Duration{10 * time.Second, 20 * time.Second, 10 * time.Second},
		[]int{50, 50, 0},
	))

	// 5s into stage 1: 50% ramp → 25
	v, _ := s.targetAt(5 * time.Second)
	if v != 25 {
		t.Errorf("at 5s: expected 25, got %d", v)
	}

	// 15s (5s into stage 2): hold 50
	v, _ = s.targetAt(15 * time.Second)
	if v != 50 {
		t.Errorf("at 15s: expected 50, got %d", v)
	}

	// 35s (5s into stage 3): 50% ramp-down → 25
	v, _ = s.targetAt(35 * time.Second)
	if v != 25 {
		t.Errorf("at 35s: expected 25, got %d", v)
	}

	// Past all stages
	v, done := s.targetAt(100 * time.Second)
	if v != 0 || !done {
		t.Errorf("after all stages: expected (0, true), got (%d, %v)", v, done)
	}
}

func TestLinearInterpolation(t *testing.T) {
	s := New(stages([]time.Duration{100 * time.Second}, []int{100}))
	for pct := 0; pct <= 100; pct++ {
		elapsed := time.Duration(pct) * time.Second
		v, _ := s.targetAt(elapsed)
		if v != pct {
			t.Errorf("at %d%%: expected %d, got %d", pct, pct, v)
		}
	}
}

func TestRun_CtxCancel(t *testing.T) {
	s := New(stages([]time.Duration{10 * time.Second}, []int{50}))
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan int, 20)

	done := make(chan struct{})
	go func() {
		s.Run(ctx, ch)
		close(done)
	}()

	// Let it tick a few times
	time.Sleep(250 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after context cancellation")
	}

	// Drain channel and verify 0 was sent (shutdown signal)
	var values []int
	for len(ch) > 0 {
		values = append(values, <-ch)
	}
	found0 := false
	for _, v := range values {
		if v == 0 {
			found0 = true
			break
		}
	}
	if !found0 {
		t.Errorf("expected 0 to be sent on ctx cancel; got values: %v", values)
	}
}

func TestTargetAt_StepRamp(t *testing.T) {
	// Step ramp: immediately jump to target when entering stage (no linear interpolation).
	s := New([]config.Stage{
		{Duration: config.Duration{Duration: 10 * time.Second}, Target: 100, Ramp: "step"},
		{Duration: config.Duration{Duration: 10 * time.Second}, Target: 50, Ramp: "step"},
	})

	// At 1s (1% into stage 1): step ramp → should already be at 100
	v, done := s.targetAt(1 * time.Second)
	if v != 100 || done {
		t.Errorf("step ramp at 1s: expected (100, false), got (%d, %v)", v, done)
	}

	// At exactly 10s: still in stage 1 (elapsed <= stageEnd), so target = 100
	v, done = s.targetAt(10 * time.Second)
	if v != 100 || done {
		t.Errorf("step ramp at 10s boundary: expected (100, false), got (%d, %v)", v, done)
	}

	// At 10s+1ms: now in stage 2 (last stage), step ramp → immediately 50, done=true
	v, done = s.targetAt(10*time.Second + time.Millisecond)
	if v != 50 || !done {
		t.Errorf("step ramp at 10s+1ms: expected (50, true), got (%d, %v)", v, done)
	}

	// At 15s (middle of stage 2): step ramp → 50
	v, _ = s.targetAt(15 * time.Second)
	if v != 50 {
		t.Errorf("step ramp at 15s: expected 50, got %d", v)
	}
}

func TestTargetAt_StepRamp_PrevCarry(t *testing.T) {
	// With linear ramp first, then step, the step should jump from prev to target instantly.
	s := New([]config.Stage{
		{Duration: config.Duration{Duration: 10 * time.Second}, Target: 50},        // linear
		{Duration: config.Duration{Duration: 10 * time.Second}, Target: 100, Ramp: "step"}, // step
	})

	// Midway through linear ramp (5s): 25 VUs
	v, _ := s.targetAt(5 * time.Second)
	if v != 25 {
		t.Errorf("linear at 5s: expected 25, got %d", v)
	}

	// At 11s (1s into step stage): should immediately be 100, not interpolated
	v, _ = s.targetAt(11 * time.Second)
	if v != 100 {
		t.Errorf("step ramp at 11s: expected 100, got %d", v)
	}
}

func TestRun_Completes(t *testing.T) {
	s := New(stages([]time.Duration{300 * time.Millisecond}, []int{10}))
	ctx := context.Background()
	ch := make(chan int, 100)

	done := make(chan struct{})
	go func() {
		s.Run(ctx, ch)
		close(done)
	}()

	select {
	case <-done:
		// ok, completed naturally
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not complete within timeout")
	}
}
