package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jvreagan/perf-test/internal/config"
	"github.com/jvreagan/perf-test/internal/metrics"
)

// --- TestRun Accessor Tests ---

func TestTestRun_GetStatus(t *testing.T) {
	tr := &TestRun{status: "running"}
	if got := tr.GetStatus(); got != "running" {
		t.Errorf("GetStatus() = %q, want %q", got, "running")
	}

	tr.mu.Lock()
	tr.status = "completed"
	tr.mu.Unlock()

	if got := tr.GetStatus(); got != "completed" {
		t.Errorf("GetStatus() = %q, want %q", got, "completed")
	}
}

func TestTestRun_GetFinalStats(t *testing.T) {
	tr := &TestRun{}
	if got := tr.GetFinalStats(); got != nil {
		t.Errorf("expected nil stats, got %v", got)
	}

	stats := &metrics.Stats{TotalRequests: 42}
	tr.mu.Lock()
	tr.finalStats = stats
	tr.mu.Unlock()

	if got := tr.GetFinalStats(); got == nil || got.TotalRequests != 42 {
		t.Errorf("expected stats with 42 requests, got %v", got)
	}
}

func TestTestRun_GetError(t *testing.T) {
	tr := &TestRun{}
	if got := tr.GetError(); got != nil {
		t.Errorf("expected nil error, got %v", got)
	}

	tr.mu.Lock()
	tr.err = fmt.Errorf("test failed")
	tr.mu.Unlock()

	if got := tr.GetError(); got == nil || got.Error() != "test failed" {
		t.Errorf("expected 'test failed' error, got %v", got)
	}
}

func TestTestRun_GetFinishedAt(t *testing.T) {
	tr := &TestRun{}
	if got := tr.GetFinishedAt(); !got.IsZero() {
		t.Errorf("expected zero time, got %v", got)
	}

	now := time.Now()
	tr.mu.Lock()
	tr.finishedAt = now
	tr.mu.Unlock()

	if got := tr.GetFinishedAt(); got != now {
		t.Errorf("expected %v, got %v", now, got)
	}
}

func TestTestRun_ConcurrentAccessors(t *testing.T) {
	tr := &TestRun{status: "running"}
	var wg sync.WaitGroup

	// Writer goroutine: simulates engine completing a test
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
		tr.mu.Lock()
		tr.finishedAt = time.Now()
		tr.finalStats = &metrics.Stats{TotalRequests: 100}
		tr.err = fmt.Errorf("done")
		tr.status = "completed"
		tr.mu.Unlock()
	}()

	// Reader goroutines: continuously read accessors during the transition
	readers := 50
	wg.Add(readers)
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = tr.GetStatus()
				_ = tr.GetFinalStats()
				_ = tr.GetError()
				_ = tr.GetFinishedAt()
			}
		}()
	}

	wg.Wait()

	// After all goroutines complete, final state should be consistent
	if tr.GetStatus() != "completed" {
		t.Errorf("expected completed, got %q", tr.GetStatus())
	}
	if tr.GetFinalStats() == nil || tr.GetFinalStats().TotalRequests != 100 {
		t.Errorf("expected stats with 100 requests")
	}
}

// --- State Edge Cases ---

func TestState_GetTest_Nonexistent(t *testing.T) {
	s := NewState()
	if got := s.GetTest("nonexistent"); got != nil {
		t.Errorf("expected nil for nonexistent test, got %v", got)
	}
}

func TestState_ActiveTest_None(t *testing.T) {
	s := NewState()
	if got := s.ActiveTest(); got != nil {
		t.Errorf("expected nil when no active test, got %v", got)
	}
}

func TestState_RecentTests_Empty(t *testing.T) {
	s := NewState()
	got := s.RecentTests(10)
	if len(got) != 0 {
		t.Errorf("expected 0 recent tests, got %d", len(got))
	}
}

func TestState_RecentTests_ZeroLimit(t *testing.T) {
	s := NewState()
	got := s.RecentTests(0)
	if len(got) != 0 {
		t.Errorf("expected 0 recent tests with limit 0, got %d", len(got))
	}
}

func testConfig(serverURL string) *config.Config {
	return &config.Config{
		Name: "test",
		Load: config.LoadConfig{
			Mode: "vu",
			Stages: []config.Stage{
				{Duration: config.Duration{Duration: 500 * time.Millisecond}, Target: 1},
			},
		},
		HTTP: config.HTTPConfig{
			Timeout: config.Duration{Duration: 5 * time.Second},
		},
		Output: config.OutputConfig{
			Format:   "console",
			Interval: config.Duration{Duration: 10 * time.Second},
		},
		Endpoints: []config.Endpoint{
			{Name: "ep", Method: "GET", URL: serverURL, Weight: 1, Expect: config.ExpectConfig{Status: 200}},
		},
	}
}

func waitForTestDone(t *testing.T, tr *TestRun, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			t.Fatalf("test did not complete within %v", timeout)
		default:
			if tr.GetStatus() != "running" {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func TestState_RecentTests_LimitExceedsCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	s := NewState()
	cfg := testConfig(srv.URL)

	// Run 2 tests
	run1 := s.StartTest(cfg)
	if run1 == nil {
		t.Fatal("failed to start test 1")
	}
	waitForTestDone(t, run1, 5*time.Second)

	run2 := s.StartTest(cfg)
	if run2 == nil {
		t.Fatal("failed to start test 2")
	}
	waitForTestDone(t, run2, 5*time.Second)

	// Ask for 100 recent tests — should get 2
	got := s.RecentTests(100)
	if len(got) != 2 {
		t.Errorf("expected 2 recent tests, got %d", len(got))
	}
}

func TestState_RecentTests_ReverseChronological(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	s := NewState()
	cfg := testConfig(srv.URL)

	run1 := s.StartTest(cfg)
	waitForTestDone(t, run1, 5*time.Second)

	run2 := s.StartTest(cfg)
	waitForTestDone(t, run2, 5*time.Second)

	got := s.RecentTests(10)
	if len(got) < 2 {
		t.Fatal("expected at least 2 recent tests")
	}
	// Most recent first
	if got[0].ID != run2.ID {
		t.Errorf("expected most recent test first, got %s (expected %s)", got[0].ID, run2.ID)
	}
	if got[1].ID != run1.ID {
		t.Errorf("expected second test second, got %s (expected %s)", got[1].ID, run1.ID)
	}
}

func TestState_StopTest_Nonexistent(t *testing.T) {
	s := NewState()
	// Should not panic
	s.StopTest("nonexistent")
}

func TestState_StopTest_AlreadyCompleted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	s := NewState()
	cfg := testConfig(srv.URL)

	run := s.StartTest(cfg)
	if run == nil {
		t.Fatal("failed to start test")
	}
	waitForTestDone(t, run, 5*time.Second)

	// Stopping a completed test should be a no-op (not panic)
	s.StopTest(run.ID)

	if run.GetStatus() != "completed" {
		t.Errorf("expected completed, got %q", run.GetStatus())
	}
}

func TestState_StartTest_BlocksDuplicate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	s := NewState()
	cfg := &config.Config{
		Name: "long-test",
		Load: config.LoadConfig{
			Mode: "vu",
			Stages: []config.Stage{
				{Duration: config.Duration{Duration: 10 * time.Second}, Target: 1},
			},
		},
		HTTP:   config.HTTPConfig{Timeout: config.Duration{Duration: 5 * time.Second}},
		Output: config.OutputConfig{Format: "console", Interval: config.Duration{Duration: 10 * time.Second}},
		Endpoints: []config.Endpoint{
			{Name: "ep", Method: "GET", URL: srv.URL, Weight: 1, Expect: config.ExpectConfig{Status: 200}},
		},
	}

	run1 := s.StartTest(cfg)
	if run1 == nil {
		t.Fatal("failed to start first test")
	}
	defer s.StopTest(run1.ID)

	run2 := s.StartTest(cfg)
	if run2 != nil {
		t.Error("expected nil when starting duplicate test, got non-nil")
		s.StopTest(run2.ID)
	}
}

func TestState_ActiveTest_ReturnsRunning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	s := NewState()
	cfg := &config.Config{
		Name: "active-test",
		Load: config.LoadConfig{
			Mode: "vu",
			Stages: []config.Stage{
				{Duration: config.Duration{Duration: 10 * time.Second}, Target: 1},
			},
		},
		HTTP:   config.HTTPConfig{Timeout: config.Duration{Duration: 5 * time.Second}},
		Output: config.OutputConfig{Format: "console", Interval: config.Duration{Duration: 10 * time.Second}},
		Endpoints: []config.Endpoint{
			{Name: "ep", Method: "GET", URL: srv.URL, Weight: 1, Expect: config.ExpectConfig{Status: 200}},
		},
	}

	run := s.StartTest(cfg)
	if run == nil {
		t.Fatal("failed to start test")
	}
	defer s.StopTest(run.ID)

	active := s.ActiveTest()
	if active == nil {
		t.Fatal("expected active test, got nil")
	}
	if active.ID != run.ID {
		t.Errorf("expected active test ID %s, got %s", run.ID, active.ID)
	}
}

func TestState_ActiveTest_NilAfterComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	s := NewState()
	cfg := testConfig(srv.URL)

	run := s.StartTest(cfg)
	if run == nil {
		t.Fatal("failed to start test")
	}
	waitForTestDone(t, run, 5*time.Second)

	if s.ActiveTest() != nil {
		t.Error("expected nil ActiveTest after completion")
	}
}

func TestState_StopTest_StatusIsStopped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	s := NewState()
	cfg := &config.Config{
		Name: "stop-test",
		Load: config.LoadConfig{
			Mode: "vu",
			Stages: []config.Stage{
				{Duration: config.Duration{Duration: 30 * time.Second}, Target: 1},
			},
		},
		HTTP:   config.HTTPConfig{Timeout: config.Duration{Duration: 5 * time.Second}},
		Output: config.OutputConfig{Format: "console", Interval: config.Duration{Duration: 10 * time.Second}},
		Endpoints: []config.Endpoint{
			{Name: "ep", Method: "GET", URL: srv.URL, Weight: 1, Expect: config.ExpectConfig{Status: 200}},
		},
	}

	run := s.StartTest(cfg)
	if run == nil {
		t.Fatal("failed to start test")
	}

	time.Sleep(100 * time.Millisecond)
	s.StopTest(run.ID)
	waitForTestDone(t, run, 5*time.Second)

	if run.GetStatus() != "stopped" {
		t.Errorf("expected stopped, got %q", run.GetStatus())
	}
}

func TestState_CompletedTest_HasFinalStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	s := NewState()
	cfg := testConfig(srv.URL)

	run := s.StartTest(cfg)
	if run == nil {
		t.Fatal("failed to start test")
	}
	waitForTestDone(t, run, 5*time.Second)

	stats := run.GetFinalStats()
	if stats == nil {
		t.Fatal("expected non-nil FinalStats after completion")
	}
	if stats.TotalRequests == 0 {
		t.Error("expected at least 1 request in FinalStats")
	}
	if run.GetFinishedAt().IsZero() {
		t.Error("expected non-zero FinishedAt after completion")
	}
}

func TestState_FailedTest_HasError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500) // endpoint expects 200
	}))
	defer srv.Close()

	s := NewState()
	cfg := testConfig(srv.URL)

	run := s.StartTest(cfg)
	if run == nil {
		t.Fatal("failed to start test")
	}
	waitForTestDone(t, run, 5*time.Second)

	if run.GetStatus() != "failed" {
		t.Errorf("expected failed, got %q", run.GetStatus())
	}
	if run.GetError() == nil {
		t.Error("expected non-nil error for failed test")
	}
}

// --- Concurrent State Access ---

func TestState_ConcurrentGetTest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	s := NewState()
	cfg := &config.Config{
		Name: "concurrent-get",
		Load: config.LoadConfig{
			Mode: "vu",
			Stages: []config.Stage{
				{Duration: config.Duration{Duration: 2 * time.Second}, Target: 1},
			},
		},
		HTTP:   config.HTTPConfig{Timeout: config.Duration{Duration: 5 * time.Second}},
		Output: config.OutputConfig{Format: "console", Interval: config.Duration{Duration: 10 * time.Second}},
		Endpoints: []config.Endpoint{
			{Name: "ep", Method: "GET", URL: srv.URL, Weight: 1, Expect: config.ExpectConfig{Status: 200}},
		},
	}

	run := s.StartTest(cfg)
	if run == nil {
		t.Fatal("failed to start test")
	}
	defer s.StopTest(run.ID)

	// Concurrently read state from multiple goroutines
	var wg sync.WaitGroup
	wg.Add(50)
	for i := 0; i < 50; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = s.GetTest(run.ID)
				_ = s.ActiveTest()
				_ = s.RecentTests(10)
			}
		}()
	}
	wg.Wait()
}
