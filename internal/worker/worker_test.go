package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jvreagan/perf-test/internal/config"
	"github.com/jvreagan/perf-test/internal/data"
	"github.com/jvreagan/perf-test/internal/metrics"
	"github.com/jvreagan/perf-test/internal/ratelimit"
)

func makeEndpoint(name, method, url string, weight, status int) config.Endpoint {
	return config.Endpoint{
		Name:   name,
		Method: method,
		URL:    url,
		Weight: weight,
		Expect: config.ExpectConfig{Status: status},
	}
}

func newWorker(id int, eps []config.Endpoint, gen *data.Generator, client *http.Client, resultCh chan<- metrics.Result, thinkTime time.Duration) *Worker {
	exec := NewExecutor(eps, gen, client)
	return New(id, exec, resultCh, thinkTime, nil)
}

func TestWorker_BasicRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	resultCh := make(chan metrics.Result, 10)
	gen := data.NewGenerator(nil)
	ep := makeEndpoint("test", "GET", srv.URL, 1, 200)
	w := newWorker(1, []config.Endpoint{ep}, gen, srv.Client(), resultCh, 0)

	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel()

	if len(resultCh) == 0 {
		t.Fatal("expected at least one result")
	}
	r := <-resultCh
	if !r.Success {
		t.Errorf("expected success, got error: %v", r.Error)
	}
	if r.StatusCode != 200 {
		t.Errorf("expected 200, got %d", r.StatusCode)
	}
}

func TestWorker_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	resultCh := make(chan metrics.Result, 10)
	gen := data.NewGenerator(nil)
	ep := makeEndpoint("test", "GET", srv.URL, 1, 200) // expects 200, gets 500
	w := newWorker(1, []config.Endpoint{ep}, gen, srv.Client(), resultCh, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	go w.Run(ctx)

	<-ctx.Done()
	if len(resultCh) == 0 {
		t.Fatal("expected results")
	}
	r := <-resultCh
	if r.Success {
		t.Error("expected failure for status mismatch")
	}
}

func TestWorker_ThinkTime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	resultCh := make(chan metrics.Result, 100)
	gen := data.NewGenerator(nil)
	ep := makeEndpoint("test", "GET", srv.URL, 1, 200)
	// 50ms think time: in 200ms we expect ~3-4 requests (not 100+)
	w := newWorker(1, []config.Endpoint{ep}, gen, srv.Client(), resultCh, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	count := len(resultCh)
	if count > 10 {
		t.Errorf("think_time not respected: got %d requests in 200ms with 50ms think_time", count)
	}
}

func TestWorker_WeightedSelection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	eps := []config.Endpoint{
		makeEndpoint("heavy", "GET", srv.URL+"/heavy", 9, 200),
		makeEndpoint("light", "GET", srv.URL+"/light", 1, 200),
	}
	gen := data.NewGenerator(nil)
	resultCh := make(chan metrics.Result, 10000)
	w := newWorker(1, eps, gen, srv.Client(), resultCh, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	counts := map[string]int{}
	for len(resultCh) > 0 {
		r := <-resultCh
		counts[r.EndpointName]++
	}

	total := counts["heavy"] + counts["light"]
	if total == 0 {
		t.Fatal("no results collected")
	}
	heavyRatio := float64(counts["heavy"]) / float64(total)
	// Should be ~90%; allow 10% margin
	if heavyRatio < 0.75 || heavyRatio > 0.99 {
		t.Errorf("unexpected weight distribution: heavy=%.2f%% (expected ~90%%)", heavyRatio*100)
	}
}

func TestWorker_BodyTemplate(t *testing.T) {
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.WriteHeader(200)
	}))
	defer srv.Close()

	resultCh := make(chan metrics.Result, 5)
	gen := data.NewGenerator(map[string]string{"name": "Alice"})
	ep := config.Endpoint{
		Name:   "post",
		Method: "POST",
		URL:    srv.URL,
		Weight: 1,
		Body:   `{"user":"${name}"}`,
		Expect: config.ExpectConfig{Status: 200},
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}
	w := newWorker(1, []config.Endpoint{ep}, gen, srv.Client(), resultCh, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	if receivedBody != `{"user":"Alice"}` {
		t.Errorf("unexpected body: %q", receivedBody)
	}
}

func TestWorker_CtxCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	resultCh := make(chan metrics.Result, 10)
	gen := data.NewGenerator(nil)
	ep := makeEndpoint("slow", "GET", srv.URL, 1, 200)
	w := newWorker(1, []config.Endpoint{ep}, gen, srv.Client(), resultCh, 0)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not exit after context cancellation")
	}
}

func TestWorker_WithLimiter_LimitsRate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	resultCh := make(chan metrics.Result, 1000)
	gen := data.NewGenerator(nil)
	ep := makeEndpoint("test", "GET", srv.URL, 1, 200)
	exec := NewExecutor([]config.Endpoint{ep}, gen, srv.Client())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Limit to 10 RPS; run for ~500ms → expect ~5 requests (allow 2–12 range)
	limiter := ratelimit.NewLimiter(ctx, 10)
	w := New(1, exec, resultCh, 0, limiter)

	runCtx, runCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer runCancel()
	w.Run(runCtx)

	count := len(resultCh)
	if count < 2 || count > 12 {
		t.Errorf("limiter not working: expected 2–12 requests in 500ms at 10 RPS, got %d", count)
	}
}
