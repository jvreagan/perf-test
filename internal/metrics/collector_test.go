package metrics

import (
	"fmt"
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

func TestStatusCodeTracking(t *testing.T) {
	c := NewCollector(time.Now())
	c.Record(Result{EndpointName: "ep", StatusCode: 200, Duration: 10 * time.Millisecond, Success: true})
	c.Record(Result{EndpointName: "ep", StatusCode: 200, Duration: 10 * time.Millisecond, Success: true})
	c.Record(Result{EndpointName: "ep", StatusCode: 404, Duration: 10 * time.Millisecond, Success: false, Error: fmt.Errorf("expected status 200, got 404")})
	c.Record(Result{EndpointName: "ep", StatusCode: 500, Duration: 10 * time.Millisecond, Success: false, Error: fmt.Errorf("expected status 200, got 500")})

	snap := c.Snapshot()

	// Global status codes
	if snap.StatusCodes[200] != 2 {
		t.Errorf("expected 2x 200, got %d", snap.StatusCodes[200])
	}
	if snap.StatusCodes[404] != 1 {
		t.Errorf("expected 1x 404, got %d", snap.StatusCodes[404])
	}
	if snap.StatusCodes[500] != 1 {
		t.Errorf("expected 1x 500, got %d", snap.StatusCodes[500])
	}

	// Per-endpoint status codes
	epStats := snap.PerEndpoint["ep"]
	if epStats.StatusCodes[200] != 2 {
		t.Errorf("ep: expected 2x 200, got %d", epStats.StatusCodes[200])
	}
}

func TestErrorTypeClassification(t *testing.T) {
	c := NewCollector(time.Now())
	c.Record(Result{EndpointName: "ep", Duration: 10 * time.Millisecond, Success: false, Error: fmt.Errorf("context deadline exceeded (timeout)")})
	c.Record(Result{EndpointName: "ep", Duration: 10 * time.Millisecond, Success: false, Error: fmt.Errorf("dial tcp: connection refused")})
	c.Record(Result{EndpointName: "ep", StatusCode: 500, Duration: 10 * time.Millisecond, Success: false, Error: fmt.Errorf("expected status 200, got 500: status mismatch")})

	snap := c.Snapshot()

	if snap.ErrorTypes["timeout"] != 1 {
		t.Errorf("expected 1 timeout, got %d", snap.ErrorTypes["timeout"])
	}
	if snap.ErrorTypes["connection_refused"] != 1 {
		t.Errorf("expected 1 connection_refused, got %d", snap.ErrorTypes["connection_refused"])
	}
	if snap.ErrorTypes["status_mismatch"] != 1 {
		t.Errorf("expected 1 status_mismatch, got %d", snap.ErrorTypes["status_mismatch"])
	}

	// Per-endpoint
	epStats := snap.PerEndpoint["ep"]
	if epStats.ErrorTypes["timeout"] != 1 {
		t.Errorf("ep: expected 1 timeout, got %d", epStats.ErrorTypes["timeout"])
	}
}

func TestClassifyError_AllTypes(t *testing.T) {
	tests := []struct {
		msg  string
		want string
	}{
		{"expected status 200, got 500", "status_mismatch"},
		{"status mismatch: 200 vs 404", "status_mismatch"},
		{"context deadline exceeded", "timeout"},
		{"request timeout after 30s", "timeout"},
		{"dial tcp 127.0.0.1:8080: connection refused", "connection_refused"},
		{"lookup example.invalid: no such host", "dns_error"},
		{"DNS resolution failed", "dns_error"},
		{"tls: handshake failure", "tls_error"},
		{"x509: certificate signed by unknown authority", "tls_error"},
		{"something completely unexpected", "other"},
	}
	for _, tc := range tests {
		got := classifyError(fmt.Errorf(tc.msg))
		if got != tc.want {
			t.Errorf("classifyError(%q) = %q, want %q", tc.msg, got, tc.want)
		}
	}
}

// --- InstantRPS Sliding Window Tests ---

func TestInstantRPS_NoRecords(t *testing.T) {
	c := NewCollector(time.Now())
	snap := c.Snapshot()
	if snap.InstantRPS != 0 {
		t.Errorf("expected InstantRPS 0 with no records, got %f", snap.InstantRPS)
	}
}

func TestInstantRPS_ImmediateBurst(t *testing.T) {
	// Record N results in the same second, snapshot immediately.
	// All N land in one bucket; other 9 buckets are zero.
	// InstantRPS = N / 10 (window is 10 seconds wide).
	c := NewCollector(time.Now())
	n := 50
	for i := 0; i < n; i++ {
		c.Record(Result{EndpointName: "ep", Duration: 1 * time.Millisecond, Success: true})
	}
	snap := c.Snapshot()

	// All records in current bucket, 10 total buckets visible
	// InstantRPS = 50 / 10 = 5.0
	expected := float64(n) / float64(rpsWindow)
	if snap.InstantRPS < expected*0.9 || snap.InstantRPS > expected*1.1 {
		t.Errorf("expected InstantRPS ~%.1f, got %.1f", expected, snap.InstantRPS)
	}
}

func TestInstantRPS_DirectRingBuffer_FullWindow(t *testing.T) {
	// Directly set ring buffer to test deterministic calculation.
	// Fill all 10 buckets with 5 requests each → 50 total / 10 = 5 RPS.
	c := NewCollector(time.Now())
	for i := 0; i < rpsWindow; i++ {
		c.recentCounts[i] = 5
	}
	c.recentIdx = rpsWindow - 1
	c.recentTime = time.Now().Truncate(time.Second)

	snap := c.Snapshot()
	if snap.InstantRPS < 4.5 || snap.InstantRPS > 5.5 {
		t.Errorf("expected InstantRPS ~5.0, got %.2f", snap.InstantRPS)
	}
}

func TestInstantRPS_DirectRingBuffer_Wraparound(t *testing.T) {
	// Set recentIdx to 2, fill buckets 0-9 with known values.
	// This tests that the modulo wraparound in Snapshot works correctly.
	c := NewCollector(time.Now())
	// Buckets: [10, 20, 30, 0, 0, 0, 0, 0, 0, 0]
	// recentIdx = 2 → current second is bucket 2 (30 reqs)
	c.recentCounts = [rpsWindow]int64{10, 20, 30, 0, 0, 0, 0, 0, 0, 0}
	c.recentIdx = 2
	c.recentTime = time.Now().Truncate(time.Second)

	snap := c.Snapshot()
	// Sum = 10+20+30 = 60, window = 10 buckets, InstantRPS = 60/10 = 6.0
	if snap.InstantRPS < 5.5 || snap.InstantRPS > 6.5 {
		t.Errorf("expected InstantRPS ~6.0, got %.2f", snap.InstantRPS)
	}
}

func TestInstantRPS_DirectRingBuffer_StaleWindow(t *testing.T) {
	// recentTime is > rpsWindow seconds ago → all buckets are stale → InstantRPS = 0.
	c := NewCollector(time.Now().Add(-30 * time.Second))
	c.recentCounts = [rpsWindow]int64{100, 100, 100, 100, 100, 100, 100, 100, 100, 100}
	c.recentTime = time.Now().Add(-15 * time.Second).Truncate(time.Second) // 15 seconds ago

	snap := c.Snapshot()
	if snap.InstantRPS != 0 {
		t.Errorf("expected InstantRPS 0 for stale window, got %.2f", snap.InstantRPS)
	}
}

func TestInstantRPS_DirectRingBuffer_PartialStaleness(t *testing.T) {
	// recentTime is 3 seconds ago → 3 seconds of gap + 7 valid buckets.
	// Only the 7 most recent written buckets are valid.
	c := NewCollector(time.Now().Add(-20 * time.Second))
	c.recentCounts = [rpsWindow]int64{10, 10, 10, 10, 10, 10, 10, 10, 10, 10}
	c.recentIdx = 4
	c.recentTime = time.Now().Add(-3 * time.Second).Truncate(time.Second)

	snap := c.Snapshot()
	// windowElapsed=3, loop iterates i=0..9, but age = i+3 breaks at i=7 (age=10)
	// Valid buckets: i=0..6 (7 buckets), sum=70
	// span = min(7+3, 10) = 10
	// InstantRPS = 70/10 = 7.0
	if snap.InstantRPS < 6.5 || snap.InstantRPS > 7.5 {
		t.Errorf("expected InstantRPS ~7.0, got %.2f", snap.InstantRPS)
	}
}

func TestInstantRPS_RecordAdvancesWindow(t *testing.T) {
	// Verify that Record() advances the ring buffer when time moves forward.
	now := time.Now().Truncate(time.Second)
	c := NewCollector(now.Add(-5 * time.Second))
	c.recentCounts = [rpsWindow]int64{99, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	c.recentIdx = 0
	c.recentTime = now // current second

	// Record a result — should stay in same bucket since time hasn't advanced
	c.Record(Result{EndpointName: "ep", Duration: 1 * time.Millisecond, Success: true})

	// The current bucket should now be 99+1=100
	if c.recentCounts[c.recentIdx] != 100 {
		t.Errorf("expected current bucket to be 100, got %d", c.recentCounts[c.recentIdx])
	}
}

func TestInstantRPS_WindowClearOnLargeGap(t *testing.T) {
	// If no records for >= rpsWindow seconds, Record() should clear entire window.
	c := NewCollector(time.Now().Add(-30 * time.Second))
	c.recentCounts = [rpsWindow]int64{99, 88, 77, 66, 55, 44, 33, 22, 11, 5}
	c.recentIdx = 5
	c.recentTime = time.Now().Add(-15 * time.Second).Truncate(time.Second)

	// Record — gap is 15s > rpsWindow(10), so entire window should clear
	c.Record(Result{EndpointName: "ep", Duration: 1 * time.Millisecond, Success: true})

	// After clearing, only the new record's bucket should be 1
	if c.recentCounts[0] != 1 {
		t.Errorf("expected bucket 0 to be 1 after clear, got %d", c.recentCounts[0])
	}
	if c.recentIdx != 0 {
		t.Errorf("expected recentIdx to be 0 after clear, got %d", c.recentIdx)
	}
	// All other buckets should be 0
	for i := 1; i < rpsWindow; i++ {
		if c.recentCounts[i] != 0 {
			t.Errorf("expected bucket %d to be 0 after clear, got %d", i, c.recentCounts[i])
		}
	}
}

// --- Concurrent Record + Snapshot Tests ---

func TestConcurrentRecordAndSnapshot(t *testing.T) {
	c := NewCollector(time.Now())
	var wg sync.WaitGroup

	// 500 goroutines recording + 100 goroutines taking snapshots concurrently
	recorders := 500
	snappers := 100

	wg.Add(recorders + snappers)
	for i := 0; i < recorders; i++ {
		go func() {
			defer wg.Done()
			c.Record(Result{
				EndpointName: "concurrent",
				Duration:     10 * time.Millisecond,
				Success:      true,
				StatusCode:   200,
			})
		}()
	}
	for i := 0; i < snappers; i++ {
		go func() {
			defer wg.Done()
			snap := c.Snapshot()
			// Snapshot should never return nil or panic
			if snap == nil {
				t.Error("Snapshot returned nil")
			}
		}()
	}
	wg.Wait()

	final := c.Snapshot()
	if final.TotalRequests != int64(recorders) {
		t.Errorf("expected %d requests, got %d", recorders, final.TotalRequests)
	}
}

func TestConcurrentSetActiveVUsAndSnapshot(t *testing.T) {
	c := NewCollector(time.Now())
	var wg sync.WaitGroup

	wg.Add(200)
	for i := 0; i < 100; i++ {
		n := i
		go func() {
			defer wg.Done()
			c.SetActiveVUs(n)
		}()
		go func() {
			defer wg.Done()
			snap := c.Snapshot()
			if snap == nil {
				t.Error("Snapshot returned nil")
			}
		}()
	}
	wg.Wait()
}

// --- Edge Cases ---

func TestCollector_EmptySnapshot(t *testing.T) {
	c := NewCollector(time.Now())
	snap := c.Snapshot()
	if snap.TotalRequests != 0 {
		t.Errorf("expected 0 total requests, got %d", snap.TotalRequests)
	}
	if snap.RPS != 0 {
		t.Errorf("expected 0 RPS, got %f", snap.RPS)
	}
	if snap.InstantRPS != 0 {
		t.Errorf("expected 0 InstantRPS, got %f", snap.InstantRPS)
	}
	if len(snap.PerEndpoint) != 0 {
		t.Errorf("expected 0 endpoints, got %d", len(snap.PerEndpoint))
	}
	if snap.P50 != 0 || snap.P90 != 0 || snap.P99 != 0 {
		t.Error("expected zero percentiles for empty snapshot")
	}
}

func TestCollector_ErrorWithoutErrorObject(t *testing.T) {
	// Error=nil but Success=false — should count as error but not classify
	c := NewCollector(time.Now())
	c.Record(Result{EndpointName: "ep", Duration: 10 * time.Millisecond, Success: false, Error: nil})
	snap := c.Snapshot()
	if snap.ErrorCount != 1 {
		t.Errorf("expected 1 error, got %d", snap.ErrorCount)
	}
	if len(snap.ErrorTypes) != 0 {
		t.Errorf("expected 0 error types (nil error), got %d", len(snap.ErrorTypes))
	}
}

func TestCollector_AllErrors(t *testing.T) {
	// All requests fail — no successes
	c := NewCollector(time.Now())
	for i := 0; i < 10; i++ {
		c.Record(Result{
			EndpointName: "ep",
			Duration:     time.Duration(i+1) * time.Millisecond,
			Success:      false,
			StatusCode:   500,
			Error:        fmt.Errorf("expected status 200, got 500"),
		})
	}
	snap := c.Snapshot()
	if snap.SuccessCount != 0 {
		t.Errorf("expected 0 successes, got %d", snap.SuccessCount)
	}
	if snap.ErrorCount != 10 {
		t.Errorf("expected 10 errors, got %d", snap.ErrorCount)
	}
	if snap.P50 == 0 {
		t.Error("expected non-zero p50 even with all errors")
	}
}
