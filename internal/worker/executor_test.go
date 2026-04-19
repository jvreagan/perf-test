package worker

import (
	"context"
	"fmt"
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

	result := exec.Execute(context.Background(), &ep)
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

	result := exec.Execute(context.Background(), &ep)
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

	result := exec.Execute(ctx, &ep)
	if result.Success {
		t.Error("expected failure with cancelled context")
	}
}

// --- io.Copy Discard Tests ---

func TestExecutor_Execute_LargeBody(t *testing.T) {
	// Response body of 1MB — io.Copy should stream without OOM
	bodySize := 1024 * 1024 // 1 MB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		buf := make([]byte, 4096)
		for i := range buf {
			buf[i] = 'X'
		}
		written := 0
		for written < bodySize {
			chunk := bodySize - written
			if chunk > len(buf) {
				chunk = len(buf)
			}
			w.Write(buf[:chunk])
			written += chunk
		}
	}))
	defer srv.Close()

	gen := data.NewGenerator(nil)
	ep := makeEndpoint("large", "GET", srv.URL, 1, 200)
	exec := NewExecutor([]config.Endpoint{ep}, gen, srv.Client())

	result := exec.Execute(context.Background(), &ep)
	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if result.BytesReceived != int64(bodySize) {
		t.Errorf("expected %d bytes received, got %d", bodySize, result.BytesReceived)
	}
}

func TestExecutor_Execute_EmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204) // No Content
	}))
	defer srv.Close()

	gen := data.NewGenerator(nil)
	ep := makeEndpoint("empty", "GET", srv.URL, 1, 204)
	exec := NewExecutor([]config.Endpoint{ep}, gen, srv.Client())

	result := exec.Execute(context.Background(), &ep)
	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if result.BytesReceived != 0 {
		t.Errorf("expected 0 bytes, got %d", result.BytesReceived)
	}
}

func TestExecutor_Execute_PostWithBody(t *testing.T) {
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.WriteHeader(200)
		w.Write([]byte("response"))
	}))
	defer srv.Close()

	gen := data.NewGenerator(map[string]string{"id": "123"})
	ep := config.Endpoint{
		Name:   "post",
		Method: "POST",
		URL:    srv.URL,
		Weight: 1,
		Body:   `{"id":"${id}"}`,
		Expect: config.ExpectConfig{Status: 200},
	}
	exec := NewExecutor([]config.Endpoint{ep}, gen, srv.Client())

	result := exec.Execute(context.Background(), &ep)
	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if receivedBody != `{"id":"123"}` {
		t.Errorf("expected body with substituted variable, got %q", receivedBody)
	}
	if result.BytesReceived != 8 { // "response" = 8 bytes
		t.Errorf("expected 8 bytes received, got %d", result.BytesReceived)
	}
}

func TestExecutor_Execute_HeaderTemplating(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	gen := data.NewGenerator(map[string]string{"token": "abc123"})
	ep := config.Endpoint{
		Name:    "auth",
		Method:  "GET",
		URL:     srv.URL,
		Weight:  1,
		Headers: map[string]string{"Authorization": "Bearer ${token}"},
		Expect:  config.ExpectConfig{Status: 200},
	}
	exec := NewExecutor([]config.Endpoint{ep}, gen, srv.Client())

	result := exec.Execute(context.Background(), &ep)
	if !result.Success {
		t.Errorf("expected success: %v", result.Error)
	}
	if gotAuth != "Bearer abc123" {
		t.Errorf("expected 'Bearer abc123', got %q", gotAuth)
	}
}

func TestExecutor_Execute_InvalidURL(t *testing.T) {
	gen := data.NewGenerator(nil)
	ep := makeEndpoint("bad", "GET", "://invalid-url", 1, 200)
	exec := NewExecutor([]config.Endpoint{ep}, gen, http.DefaultClient)

	result := exec.Execute(context.Background(), &ep)
	if result.Success {
		t.Error("expected failure for invalid URL")
	}
	if result.Error == nil {
		t.Error("expected non-nil error for invalid URL")
	}
}

func TestExecutor_SelectEndpoint_EqualWeights(t *testing.T) {
	gen := data.NewGenerator(nil)
	eps := []config.Endpoint{
		makeEndpoint("a", "GET", "http://example.com/a", 1, 200),
		makeEndpoint("b", "GET", "http://example.com/b", 1, 200),
		makeEndpoint("c", "GET", "http://example.com/c", 1, 200),
	}
	exec := NewExecutor(eps, gen, http.DefaultClient)

	counts := map[string]int{}
	n := 3000
	for i := 0; i < n; i++ {
		ep := exec.SelectEndpoint()
		counts[ep.Name]++
	}

	// Each should be roughly 33%
	for _, name := range []string{"a", "b", "c"} {
		ratio := float64(counts[name]) / float64(n)
		if ratio < 0.25 || ratio > 0.42 {
			t.Errorf("endpoint %s: expected ~33%%, got %.1f%%", name, ratio*100)
		}
	}
}

func TestExecutor_Execute_BytesReceivedAccuracy(t *testing.T) {
	// Verify exact byte count for known response sizes
	sizes := []int{0, 1, 100, 4096, 65536}
	for _, size := range sizes {
		t.Run(fmt.Sprintf("size_%d", size), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				if size > 0 {
					w.Write(make([]byte, size))
				}
			}))
			defer srv.Close()

			gen := data.NewGenerator(nil)
			ep := makeEndpoint("test", "GET", srv.URL, 1, 200)
			exec := NewExecutor([]config.Endpoint{ep}, gen, srv.Client())

			result := exec.Execute(context.Background(), &ep)
			if !result.Success {
				t.Fatalf("unexpected error: %v", result.Error)
			}
			if result.BytesReceived != int64(size) {
				t.Errorf("expected %d bytes, got %d", size, result.BytesReceived)
			}
		})
	}
}
