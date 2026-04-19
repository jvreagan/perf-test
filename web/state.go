package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"sync"
	"time"

	"github.com/jvreagan/perf-test/internal/config"
	"github.com/jvreagan/perf-test/internal/engine"
	"github.com/jvreagan/perf-test/internal/metrics"
)

// TestRun represents a single load test execution.
type TestRun struct {
	ID        string
	Config    *config.Config
	StartedAt time.Time
	Engine    *engine.Engine
	Cancel    context.CancelFunc

	mu         sync.RWMutex
	finishedAt time.Time
	status     string // "running", "completed", "failed", "stopped"
	finalStats *metrics.Stats
	err        error
}

// GetStatus returns the test run status thread-safely.
func (tr *TestRun) GetStatus() string {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	return tr.status
}

// GetFinalStats returns the final stats snapshot thread-safely.
func (tr *TestRun) GetFinalStats() *metrics.Stats {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	return tr.finalStats
}

// GetError returns the test run error thread-safely.
func (tr *TestRun) GetError() error {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	return tr.err
}

// GetFinishedAt returns the time the test finished thread-safely.
func (tr *TestRun) GetFinishedAt() time.Time {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	return tr.finishedAt
}

const maxTestHistory = 100

// State manages in-memory test run state.
type State struct {
	mu       sync.RWMutex
	tests    map[string]*TestRun
	activeID string
	order    []string
}

// NewState creates an empty State.
func NewState() *State {
	return &State{
		tests: make(map[string]*TestRun),
	}
}

// GetTest returns a test run by ID, or nil if not found.
func (s *State) GetTest(id string) *TestRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tests[id]
}

// ActiveTest returns the currently running test, or nil.
func (s *State) ActiveTest() *TestRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.activeID == "" {
		return nil
	}
	return s.tests[s.activeID]
}

// RecentTests returns completed tests in reverse chronological order, up to limit.
func (s *State) RecentTests(limit int) []*TestRun {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*TestRun
	for i := len(s.order) - 1; i >= 0 && len(result) < limit; i-- {
		tr := s.tests[s.order[i]]
		if tr.GetStatus() != "running" {
			result = append(result, tr)
		}
	}
	return result
}

// StartTest begins a new test run in a background goroutine.
// Returns nil if another test is already running.
func (s *State) StartTest(cfg *config.Config) *TestRun {
	s.mu.Lock()
	if s.activeID != "" {
		s.mu.Unlock()
		return nil
	}

	id := generateID()
	ctx, cancel := context.WithCancel(context.Background())
	eng := engine.New(cfg)

	run := &TestRun{
		ID:        id,
		Config:    cfg,
		StartedAt: time.Now(),
		status:    "running",
		Engine:    eng,
		Cancel:    cancel,
	}

	s.tests[id] = run
	s.order = append(s.order, id)
	s.activeID = id

	// Evict oldest completed tests if history exceeds limit
	for len(s.order) > maxTestHistory {
		oldID := s.order[0]
		if oldID == s.activeID {
			break // don't evict the active test
		}
		delete(s.tests, oldID)
		s.order = s.order[1:]
	}
	s.mu.Unlock()

	go func() {
		stats, err := eng.Run(ctx, io.Discard)

		run.mu.Lock()
		run.finishedAt = time.Now()
		run.finalStats = stats
		run.err = err
		if ctx.Err() != nil {
			run.status = "stopped"
		} else if err != nil {
			run.status = "failed"
		} else {
			run.status = "completed"
		}
		run.mu.Unlock()

		s.mu.Lock()
		s.activeID = ""
		s.mu.Unlock()
	}()

	return run
}

// StopTest cancels a running test.
func (s *State) StopTest(id string) {
	s.mu.RLock()
	tr := s.tests[id]
	s.mu.RUnlock()
	if tr != nil && tr.GetStatus() == "running" && tr.Cancel != nil {
		tr.Cancel()
	}
}

func generateID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}
