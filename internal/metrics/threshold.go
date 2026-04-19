package metrics

import (
	"fmt"

	"github.com/jvreagan/perf-test/internal/config"
)

// ThresholdResult holds the outcome of a single threshold evaluation.
type ThresholdResult struct {
	Name      string `json:"name"`
	Threshold string `json:"threshold"`
	Actual    string `json:"actual"`
	Passed    bool   `json:"passed"`
}

// EvaluateThresholds checks stats against configured thresholds.
func EvaluateThresholds(tc config.ThresholdConfig, stats *Stats) []ThresholdResult {
	var results []ThresholdResult

	if tc.P95.Duration > 0 {
		results = append(results, ThresholdResult{
			Name:      "p95 latency",
			Threshold: fmt.Sprintf("<= %s", tc.P95.Duration),
			Actual:    stats.P95.Duration().String(),
			Passed:    stats.P95.Duration() <= tc.P95.Duration,
		})
	}

	if tc.P99.Duration > 0 {
		results = append(results, ThresholdResult{
			Name:      "p99 latency",
			Threshold: fmt.Sprintf("<= %s", tc.P99.Duration),
			Actual:    stats.P99.Duration().String(),
			Passed:    stats.P99.Duration() <= tc.P99.Duration,
		})
	}

	if tc.MaxLatency.Duration > 0 {
		results = append(results, ThresholdResult{
			Name:      "max latency",
			Threshold: fmt.Sprintf("<= %s", tc.MaxLatency.Duration),
			Actual:    stats.Max.Duration().String(),
			Passed:    stats.Max.Duration() <= tc.MaxLatency.Duration,
		})
	}

	if tc.ErrorRate > 0 {
		actualRate := 0.0
		if stats.TotalRequests > 0 {
			actualRate = float64(stats.ErrorCount) / float64(stats.TotalRequests) * 100
		}
		results = append(results, ThresholdResult{
			Name:      "error rate",
			Threshold: fmt.Sprintf("<= %.1f%%", tc.ErrorRate),
			Actual:    fmt.Sprintf("%.1f%%", actualRate),
			Passed:    actualRate <= tc.ErrorRate,
		})
	}

	if tc.MinRPS > 0 {
		results = append(results, ThresholdResult{
			Name:      "min RPS",
			Threshold: fmt.Sprintf(">= %.1f", tc.MinRPS),
			Actual:    fmt.Sprintf("%.1f", stats.RPS),
			Passed:    stats.RPS >= tc.MinRPS,
		})
	}

	return results
}

// AnyFailed returns true if any threshold result failed.
func AnyFailed(results []ThresholdResult) bool {
	for _, r := range results {
		if !r.Passed {
			return true
		}
	}
	return false
}
