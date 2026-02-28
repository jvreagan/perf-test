package metrics

import (
	"testing"
	"time"

	"github.com/jvreagan/perf-test/internal/config"
)

func TestEvaluateThresholds_AllPass(t *testing.T) {
	tc := config.ThresholdConfig{
		P95:        config.Duration{Duration: 500 * time.Millisecond},
		P99:        config.Duration{Duration: 2 * time.Second},
		MaxLatency: config.Duration{Duration: 5 * time.Second},
		ErrorRate:  5.0,
		MinRPS:     10.0,
	}
	stats := &Stats{
		P95:           200 * time.Millisecond,
		P99:           800 * time.Millisecond,
		Max:           1500 * time.Millisecond,
		TotalRequests: 1000,
		ErrorCount:    10, // 1%
		RPS:           50.0,
	}

	results := EvaluateThresholds(tc, stats)
	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
	if AnyFailed(results) {
		for _, r := range results {
			if !r.Passed {
				t.Errorf("expected pass for %q: threshold=%s actual=%s", r.Name, r.Threshold, r.Actual)
			}
		}
	}
}

func TestEvaluateThresholds_SomeFail(t *testing.T) {
	tc := config.ThresholdConfig{
		P95:       config.Duration{Duration: 100 * time.Millisecond},
		ErrorRate: 1.0,
	}
	stats := &Stats{
		P95:           500 * time.Millisecond, // exceeds 100ms
		TotalRequests: 100,
		ErrorCount:    5, // 5% > 1%
		RPS:           20.0,
	}

	results := EvaluateThresholds(tc, stats)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !AnyFailed(results) {
		t.Error("expected at least one failure")
	}

	// p95 should fail
	if results[0].Passed {
		t.Errorf("p95 should have failed: threshold=%s actual=%s", results[0].Threshold, results[0].Actual)
	}
	// error rate should fail
	if results[1].Passed {
		t.Errorf("error rate should have failed: threshold=%s actual=%s", results[1].Threshold, results[1].Actual)
	}
}

func TestEvaluateThresholds_NoThresholds(t *testing.T) {
	tc := config.ThresholdConfig{}
	stats := &Stats{TotalRequests: 100, RPS: 10}

	results := EvaluateThresholds(tc, stats)
	if len(results) != 0 {
		t.Errorf("expected 0 results for no thresholds, got %d", len(results))
	}
	if AnyFailed(results) {
		t.Error("no thresholds should mean no failures")
	}
}

func TestEvaluateThresholds_MinRPSFail(t *testing.T) {
	tc := config.ThresholdConfig{
		MinRPS: 100.0,
	}
	stats := &Stats{RPS: 50.0}

	results := EvaluateThresholds(tc, stats)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Errorf("min RPS should have failed: threshold=%s actual=%s", results[0].Threshold, results[0].Actual)
	}
}
