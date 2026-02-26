package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/jvreagan/perf-test/internal/config"
	"github.com/jvreagan/perf-test/internal/engine"
	"github.com/jvreagan/perf-test/internal/metrics"
)

// TestRun represents a single load test execution.
type TestRun struct {
	ID         string
	Config     *config.Config
	StartedAt  time.Time
	FinishedAt time.Time
	Status     string // "running", "completed", "failed", "stopped"
	Engine     *engine.Engine
	Cancel     context.CancelFunc
	FinalStats *metrics.Stats
	Error      error
	Output     *bytes.Buffer
}

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
		if tr.Status != "running" {
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
	buf := new(bytes.Buffer)
	eng := engine.New(cfg)

	run := &TestRun{
		ID:        id,
		Config:    cfg,
		StartedAt: time.Now(),
		Status:    "running",
		Engine:    eng,
		Cancel:    cancel,
		Output:    buf,
	}

	s.tests[id] = run
	s.order = append(s.order, id)
	s.activeID = id
	s.mu.Unlock()

	go func() {
		stats, err := eng.Run(ctx, buf)
		run.FinishedAt = time.Now()
		run.FinalStats = stats
		run.Error = err
		if ctx.Err() != nil {
			run.Status = "stopped"
		} else if err != nil {
			run.Status = "failed"
		} else {
			run.Status = "completed"
		}
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
	if tr != nil && tr.Status == "running" && tr.Cancel != nil {
		tr.Cancel()
	}
}

func generateID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}
