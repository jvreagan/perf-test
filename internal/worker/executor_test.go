package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jvreagan/perf-test/internal/config"
	"github.com/jvreagan/perf-test/internal/data"
)

func TestExecutor_SelectEndpoint_SingleEndpoint(t *testing.T) {
	ep := makeEndpoint("only", "GET", "http://example.com", 1, 200)
	gen := data.NewGenerator(nil)
	exec := NewExecutor([]config.Endpoint{ep}, gen, http.DefaultClient)

	for i := 0; i < 20; i++ {
		got := exec.SelectEndpoint()
		if got.Name != "only" {
			t.Errorf("expected 'only', got %q", got.Name)
		}
	}
}

func TestExecutor_SelectEndpoint_WeightedDistribution(t *testing.T) {
	gen := data.NewGenerator(nil)
	eps := []config.Endpoint{
		makeEndpoint("heavy", "GET", "http://example.com/heavy", 9, 200),
		makeEndpoint("light", "GET", "http://example.com/light", 1, 200),
	}
	exec := NewExecutor(eps, gen, http.DefaultClient)

	counts := map[string]int{}
	for i := 0; i < 1000; i++ {
		ep := exec.SelectEndpoint()
		counts[ep.Name]++
	}

	total := counts["heavy"] + counts["light"]
	ratio := float64(counts["heavy"]) / float64(total)
	if ratio < 0.80 || ratio > 0.98 {
		t.Errorf("expected ~90%% heavy, got %.2f%%", ratio*100)
	}
}

func TestExecutor_Execute_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("hello"))
	}))
	defer srv.Close()

	gen := data.NewGenerator(nil)
	ep := makeEndpoint("test", "GET", srv.URL, 1, 200)
	exec := NewExecutor([]config.Endpoint{ep}, gen, srv.Client())

	result := exec.Execute(context.Background(), ep)
	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if result.StatusCode != 200 {
		t.Errorf("expected 200, got %d", result.StatusCode)
	}
	if result.BytesReceived != 5 {
		t.Errorf("expected 5 bytes, got %d", result.BytesReceived)
	}
}

func TestExecutor_Execute_StatusMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	gen := data.NewGenerator(nil)
	ep := makeEndpoint("test", "GET", srv.URL, 1, 200) // expects 200, gets 404
	exec := NewExecutor([]config.Endpoint{ep}, gen, srv.Client())

	result := exec.Execute(context.Background(), ep)
	if result.Success {
		t.Error("expected failure for status mismatch")
	}
	if result.Error == nil {
		t.Error("expected non-nil error")
	}
}

func TestExecutor_Execute_CtxCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block indefinitely
		<-r.Context().Done()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	gen := data.NewGenerator(nil)
	ep := makeEndpoint("slow", "GET", srv.URL, 1, 200)
	exec := NewExecutor([]config.Endpoint{ep}, gen, srv.Client())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	result := exec.Execute(ctx, ep)
	if result.Success {
		t.Error("expected failure with cancelled context")
	}
}
