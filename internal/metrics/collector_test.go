package metrics

import (
	"sync"
	"testing"
	"time"
)

func TestRecord_Basic(t *testing.T) {
	c := NewCollector(time.Now())
	c.Record(Result{
		EndpointName: "test",
		Duration:     100 * time.Millisecond,
		Success:      true,
	})
	snap := c.Snapshot()
	if snap.TotalRequests != 1 {
		t.Errorf("expected 1 request, got %d", snap.TotalRequests)
	}
	if snap.SuccessCount != 1 {
		t.Errorf("expected 1 success, got %d", snap.SuccessCount)
	}
	if snap.ErrorCount != 0 {
		t.Errorf("expected 0 errors, got %d", snap.ErrorCount)
	}
}

func TestRecord_Errors(t *testing.T) {
	c := NewCollector(time.Now())
	c.Record(Result{EndpointName: "ep", Duration: 50 * time.Millisecond, Success: true})
	c.Record(Result{EndpointName: "ep", Duration: 60 * time.Millisecond, Success: false})
	snap := c.Snapshot()
	if snap.ErrorCount != 1 {
		t.Errorf("expected 1 error, got %d", snap.ErrorCount)
	}
	if snap.SuccessCount != 1 {
		t.Errorf("expected 1 success, got %d", snap.SuccessCount)
	}
}

func TestPercentiles_KnownDataset(t *testing.T) {
	c := NewCollector(time.Now())
	// 100 values: 1ms, 2ms, ..., 100ms
	for i := 1; i <= 100; i++ {
		c.Record(Result{
			EndpointName: "ep",
			Duration:     time.Duration(i) * time.Millisecond,
			Success:      true,
		})
	}
	snap := c.Snapshot()
	// p50 index = 49 → 50ms, p90 index = 89 → 90ms, p99 index = 98 → 99ms
	if snap.P50 != 50*time.Millisecond {
		t.Errorf("p50: expected 50ms, got %v", snap.P50)
	}
	if snap.P90 != 90*time.Millisecond {
		t.Errorf("p90: expected 90ms, got %v", snap.P90)
	}
	if snap.P99 != 99*time.Millisecond {
		t.Errorf("p99: expected 99ms, got %v", snap.P99)
	}
	if snap.Min != 1*time.Millisecond {
		t.Errorf("min: expected 1ms, got %v", snap.Min)
	}
	if snap.Max != 100*time.Millisecond {
		t.Errorf("max: expected 100ms, got %v", snap.Max)
	}
}

func TestConcurrentRecord(t *testing.T) {
	c := NewCollector(time.Now())
	var wg sync.WaitGroup
	n := 1000
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			c.Record(Result{
				EndpointName: "concurrent",
				Duration:     10 * time.Millisecond,
				Success:      true,
			})
		}()
	}
	wg.Wait()
	snap := c.Snapshot()
	if snap.TotalRequests != int64(n) {
		t.Errorf("expected %d requests, got %d", n, snap.TotalRequests)
	}
}

func TestRPS_Calculation(t *testing.T) {
	start := time.Now().Add(-10 * time.Second) // pretend we started 10s ago
	c := NewCollector(start)
	for i := 0; i < 100; i++ {
		c.Record(Result{EndpointName: "ep", Duration: 5 * time.Millisecond, Success: true})
	}
	snap := c.Snapshot()
	// ~100 reqs / ~10s = ~10 RPS; allow for timing jitter
	if snap.RPS < 5 || snap.RPS > 25 {
		t.Errorf("unexpected RPS: %f (expected ~10)", snap.RPS)
	}
}

func TestMultipleEndpoints(t *testing.T) {
	c := NewCollector(time.Now())
	c.Record(Result{EndpointName: "alpha", Duration: 10 * time.Millisecond, Success: true})
	c.Record(Result{EndpointName: "beta", Duration: 20 * time.Millisecond, Success: true})
	c.Record(Result{EndpointName: "beta", Duration: 30 * time.Millisecond, Success: false})

	snap := c.Snapshot()
	if len(snap.PerEndpoint) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(snap.PerEndpoint))
	}
	if snap.PerEndpoint["alpha"].TotalRequests != 1 {
		t.Errorf("alpha: expected 1, got %d", snap.PerEndpoint["alpha"].TotalRequests)
	}
	if snap.PerEndpoint["beta"].TotalRequests != 2 {
		t.Errorf("beta: expected 2, got %d", snap.PerEndpoint["beta"].TotalRequests)
	}
	if snap.PerEndpoint["beta"].ErrorCount != 1 {
		t.Errorf("beta errors: expected 1, got %d", snap.PerEndpoint["beta"].ErrorCount)
	}
	if snap.TotalRequests != 3 {
		t.Errorf("total: expected 3, got %d", snap.TotalRequests)
	}
}

func TestSetActiveVUs(t *testing.T) {
	c := NewCollector(time.Now())
	c.SetActiveVUs(42)
	snap := c.Snapshot()
	if snap.ActiveVUs != 42 {
		t.Errorf("expected 42 active VUs, got %d", snap.ActiveVUs)
	}
}
