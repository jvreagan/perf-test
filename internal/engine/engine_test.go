package engine

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jvreagan/perf-test/internal/config"
)

func makeConfig(serverURL string) *config.Config {
	return &config.Config{
		Name: "engine-test",
		Load: config.LoadConfig{
			Mode: "vu",
			Stages: []config.Stage{
				{Duration: config.Duration{Duration: 300 * time.Millisecond}, Target: 3},
			},
		},
		HTTP: config.HTTPConfig{
			Timeout:         config.Duration{Duration: 5 * time.Second},
			FollowRedirects: true,
		},
		Endpoints: []config.Endpoint{
			{
				Name:   "health",
				Method: "GET",
				URL:    serverURL + "/health",
				Weight: 1,
				Expect: config.ExpectConfig{Status: 200},
			},
		},
		Output: config.OutputConfig{
			Format:   "console",
			Interval: config.Duration{Duration: 500 * time.Millisecond},
		},
	}
}

func TestEngine_Run_BasicSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg := makeConfig(srv.URL)
	e := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := e.Run(ctx, io.Discard)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEngine_Run_WithErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	cfg := makeConfig(srv.URL)
	e := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := e.Run(ctx, io.Discard)
	if err == nil {
		t.Error("expected error when server returns 500 but endpoint expects 200")
	}
	if !strings.Contains(err.Error(), "errors") {
		t.Errorf("error message should mention errors: %v", err)
	}
}

func TestEngine_Run_GracefulShutdown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := makeConfig(srv.URL)
	cfg.Load.Stages[0].Duration.Duration = 5 * time.Second // long test
	e := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := e.Run(ctx, io.Discard)
		done <- err
	}()

	select {
	case <-done:
		// completed after ctx cancel
	case <-time.After(3 * time.Second):
		t.Fatal("engine did not shut down gracefully after context cancellation")
	}
}

func TestEngine_Run_ArrivalRate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg := makeConfig(srv.URL)
	cfg.Load.Mode = "arrival_rate"
	// Target 20 RPS for 300ms → expect at least a few requests
	cfg.Load.Stages[0].Target = 20
	e := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := e.Run(ctx, io.Discard); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEngine_Run_MaxRPSCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := makeConfig(srv.URL)
	cfg.Load.MaxRPS = 10 // cap at 10 RPS
	cfg.Load.Stages[0].Target = 50 // many VUs, but rate-limited
	cfg.Load.Stages[0].Duration.Duration = 300 * time.Millisecond
	e := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := e.Run(ctx, io.Discard); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEngine_Run_StepRampVUMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := makeConfig(srv.URL)
	cfg.Load.Stages = []config.Stage{
		{Duration: config.Duration{Duration: 200 * time.Millisecond}, Target: 5, Ramp: "step"},
		{Duration: config.Duration{Duration: 100 * time.Millisecond}, Target: 0, Ramp: "step"},
	}
	e := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := e.Run(ctx, io.Discard); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEngine_Run_ThresholdPass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := makeConfig(srv.URL)
	cfg.Thresholds.P95 = config.Duration{Duration: 5 * time.Second}
	cfg.Thresholds.ErrorRate = 10.0
	e := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats, err := e.Run(ctx, io.Discard)
	if err != nil {
		t.Errorf("expected no error when thresholds pass, got: %v", err)
	}
	if len(stats.ThresholdResults) == 0 {
		t.Error("expected threshold results to be populated")
	}
}

func TestEngine_Run_ThresholdFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	cfg := makeConfig(srv.URL)
	// Set a very tight error rate threshold that will be breached
	cfg.Thresholds.ErrorRate = 0.1
	e := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := e.Run(ctx, io.Discard)
	if err == nil {
		t.Error("expected error when thresholds fail")
	}
	if err != nil && !strings.Contains(err.Error(), "threshold(s) breached") {
		t.Errorf("expected threshold breach error, got: %v", err)
	}
}

func TestEngine_Run_MultipleEndpoints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := makeConfig(srv.URL)
	cfg.Endpoints = []config.Endpoint{
		{Name: "ep1", Method: "GET", URL: srv.URL + "/ep1", Weight: 1, Expect: config.ExpectConfig{Status: 200}},
		{Name: "ep2", Method: "GET", URL: srv.URL + "/ep2", Weight: 1, Expect: config.ExpectConfig{Status: 200}},
	}

	e := New(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := e.Run(ctx, io.Discard); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Collector Mutex Tests ---

func TestEngine_Collector_NilBeforeRun(t *testing.T) {
	cfg := makeConfig("http://localhost:0")
	e := New(cfg)
	if c := e.Collector(); c != nil {
		t.Errorf("expected nil collector before Run, got %v", c)
	}
}

func TestEngine_Collector_ConcurrentAccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg := makeConfig(srv.URL)
	cfg.Load.Stages[0].Duration.Duration = 1 * time.Second
	e := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		e.Run(ctx, io.Discard)
	}()

	// Give Run a moment to set the collector
	time.Sleep(50 * time.Millisecond)

	// Concurrently access Collector from multiple goroutines while Run is active
	var wg sync.WaitGroup
	wg.Add(50)
	for i := 0; i < 50; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				c := e.Collector()
				if c != nil {
					snap := c.Snapshot()
					if snap == nil {
						t.Error("Snapshot returned nil")
					}
				}
			}
		}()
	}

	wg.Wait()
	cancel()
	<-done
}

// --- Arrival Rate Graceful Shutdown (no panic on close) ---

func TestEngine_ArrivalRate_GracefulShutdown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg := makeConfig(srv.URL)
	cfg.Load.Mode = "arrival_rate"
	cfg.Load.Stages[0].Target = 50 // High RPS so in-flight goroutines exist at shutdown
	cfg.Load.Stages[0].Duration.Duration = 500 * time.Millisecond
	e := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// This should not panic from send-on-closed-channel
	_, err := e.Run(ctx, io.Discard)
	// err may be non-nil due to request errors, but no panic is the key assertion
	_ = err
}

func TestEngine_ArrivalRate_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // slow so goroutines are in-flight
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := makeConfig(srv.URL)
	cfg.Load.Mode = "arrival_rate"
	cfg.Load.Stages[0].Target = 30
	cfg.Load.Stages[0].Duration.Duration = 10 * time.Second // long test
	e := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := e.Run(ctx, io.Discard)
		done <- err
	}()

	select {
	case <-done:
		// Completed without panic — success
	case <-time.After(5 * time.Second):
		t.Fatal("engine did not shut down after context cancellation")
	}
}

func TestEngine_ArrivalRate_RateChange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg := makeConfig(srv.URL)
	cfg.Load.Mode = "arrival_rate"
	cfg.Load.Stages = []config.Stage{
		{Duration: config.Duration{Duration: 200 * time.Millisecond}, Target: 10},
		{Duration: config.Duration{Duration: 200 * time.Millisecond}, Target: 50},
		{Duration: config.Duration{Duration: 200 * time.Millisecond}, Target: 0},
	}
	e := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats, err := e.Run(ctx, io.Discard)
	_ = err // may have timing errors
	if stats == nil {
		t.Fatal("expected non-nil stats")
	}
	if stats.TotalRequests == 0 {
		t.Error("expected at least some requests during rate changes")
	}
}

// --- Stats InstantRPS Integration ---

func TestEngine_Run_StatsHaveInstantRPS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg := makeConfig(srv.URL)
	cfg.Load.Stages[0].Duration.Duration = 1 * time.Second
	cfg.Load.Stages[0].Target = 5
	e := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats, err := e.Run(ctx, io.Discard)
	_ = err
	if stats == nil {
		t.Fatal("expected non-nil stats")
	}
	// After a 1s test with 5 VUs, there should be some RPS
	if stats.RPS == 0 {
		t.Error("expected non-zero cumulative RPS")
	}
	// InstantRPS may be 0 if the test finished long before the snapshot,
	// but the field should exist and be non-negative
	if stats.InstantRPS < 0 {
		t.Errorf("InstantRPS should not be negative: %f", stats.InstantRPS)
	}
}

func TestEngine_Run_CollectorSnapshotDuringRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg := makeConfig(srv.URL)
	cfg.Load.Stages[0].Duration.Duration = 2 * time.Second
	cfg.Load.Stages[0].Target = 3
	e := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		e.Run(ctx, io.Discard)
	}()

	// Wait for collector to be available
	time.Sleep(100 * time.Millisecond)

	c := e.Collector()
	if c == nil {
		t.Fatal("expected non-nil collector during run")
	}

	// Take multiple snapshots during the run
	for i := 0; i < 5; i++ {
		snap := c.Snapshot()
		if snap == nil {
			t.Fatal("Snapshot returned nil during run")
		}
		time.Sleep(100 * time.Millisecond)
	}

	cancel()
	<-done
}
