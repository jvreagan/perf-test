package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestNewLimiter_ZeroRPS_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	if l := NewLimiter(ctx, 0); l != nil {
		t.Error("expected nil limiter for rps=0")
	}
	if l := NewLimiter(ctx, -5); l != nil {
		t.Error("expected nil limiter for rps=-5")
	}
}

func TestLimiter_NilSafeWait(t *testing.T) {
	var l *Limiter
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	// nil Wait should return true immediately without blocking
	start := time.Now()
	ok := l.Wait(ctx)
	elapsed := time.Since(start)
	if !ok {
		t.Error("nil Wait should return true")
	}
	if elapsed > 20*time.Millisecond {
		t.Errorf("nil Wait should not block, took %v", elapsed)
	}
}

func TestLimiter_CtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	l := NewLimiter(ctx, 1) // very slow, tokens won't arrive quickly

	// Drain any pre-filled token
	select {
	case <-l.tokens:
	default:
	}

	cancel() // cancel before a token is available

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer waitCancel()

	ok := l.Wait(waitCtx)
	if ok {
		t.Error("Wait should return false when ctx is cancelled")
	}
}

func TestLimiter_RateAccuracy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rps := 20.0
	l := NewLimiter(ctx, rps)

	// Drain bucket first to start from empty
	time.Sleep(10 * time.Millisecond)
	for {
		select {
		case <-l.tokens:
		default:
			goto drained
		}
	}
drained:

	start := time.Now()
	count := 0
	testDur := 500 * time.Millisecond
	deadline := time.Now().Add(testDur)

	for time.Now().Before(deadline) {
		waitCtx, waitCancel := context.WithDeadline(context.Background(), deadline)
		ok := l.Wait(waitCtx)
		waitCancel()
		if !ok {
			break
		}
		count++
	}

	elapsed := time.Since(start)
	actualRPS := float64(count) / elapsed.Seconds()
	// Allow ±50% of target (token bucket under test conditions can be imprecise)
	if actualRPS < rps*0.5 || actualRPS > rps*1.5 {
		t.Errorf("rate inaccurate: expected ~%.0f RPS, got %.1f RPS (%d in %v)", rps, actualRPS, count, elapsed)
	}
}
