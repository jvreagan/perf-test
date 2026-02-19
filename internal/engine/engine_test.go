package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

	err := e.Run(ctx)
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

	err := e.Run(ctx)
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
		done <- e.Run(ctx)
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

	if err := e.Run(ctx); err != nil {
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

	if err := e.Run(ctx); err != nil {
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

	if err := e.Run(ctx); err != nil {
		t.Errorf("unexpected error: %v", err)
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

	if err := e.Run(ctx); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
