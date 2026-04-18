package reporter

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jvreagan/perf-test/internal/metrics"
)

func sampleStats() *metrics.Stats {
	return &metrics.Stats{
		TotalRequests: 500,
		SuccessCount:  490,
		ErrorCount:    10,
		RPS:           98.0,
		P50:           45 * time.Millisecond,
		P90:           120 * time.Millisecond,
		P95:           200 * time.Millisecond,
		P99:           310 * time.Millisecond,
		Min:           5 * time.Millisecond,
		Max:           500 * time.Millisecond,
		Avg:           60 * time.Millisecond,
		ActiveVUs:     10,
		Elapsed:       5 * time.Second,
		StatusCodes:   map[int]int64{200: 490, 500: 10},
		ErrorTypes:    map[string]int64{"status_mismatch": 8, "timeout": 2},
		PerEndpoint: map[string]*metrics.EndpointStats{
			"GET /users": {
				Name:          "GET /users",
				TotalRequests: 400,
				SuccessCount:  395,
				ErrorCount:    5,
				P50:           40 * time.Millisecond,
				P90:           100 * time.Millisecond,
				P99:           280 * time.Millisecond,
				StatusCodes:   map[int]int64{200: 395, 500: 5},
				ErrorTypes:    map[string]int64{"status_mismatch": 5},
			},
			"POST /items": {
				Name:          "POST /items",
				TotalRequests: 100,
				SuccessCount:  95,
				ErrorCount:    5,
				P50:           80 * time.Millisecond,
				P90:           180 * time.Millisecond,
				P99:           400 * time.Millisecond,
				StatusCodes:   map[int]int64{200: 95, 500: 5},
				ErrorTypes:    map[string]int64{"status_mismatch": 3, "timeout": 2},
			},
		},
	}
}

func TestPrint_ContainsKeyFields(t *testing.T) {
	var buf bytes.Buffer
	stats := sampleStats()
	Print(&buf, stats)
	out := buf.String()

	checks := []string{"VUs:", "RPS:", "Endpoint", "Reqs", "p50", "p90", "p99", "GET /users", "POST /items"}
	for _, c := range checks {
		if !strings.Contains(out, c) {
			t.Errorf("Print output missing %q\nOutput:\n%s", c, out)
		}
	}
}

func TestSummary_ContainsKeyFields(t *testing.T) {
	var buf bytes.Buffer
	stats := sampleStats()
	Summary(&buf, stats)
	out := buf.String()

	checks := []string{
		"FINAL SUMMARY", "Total Requests", "Success", "Errors", "Avg RPS", "Per-Endpoint",
		"Status Code Breakdown", "200", "500",
		"Error Type Breakdown", "status_mismatch", "timeout",
	}
	for _, c := range checks {
		if !strings.Contains(out, c) {
			t.Errorf("Summary output missing %q\nOutput:\n%s", c, out)
		}
	}
}

func TestWriteJSON_Structure(t *testing.T) {
	stats := sampleStats()
	dir := t.TempDir()
	path := filepath.Join(dir, "results.json")

	if err := WriteJSON(path, stats); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	if _, ok := result["TotalRequests"]; !ok {
		t.Error("JSON missing TotalRequests field")
	}
	if _, ok := result["PerEndpoint"]; !ok {
		t.Error("JSON missing PerEndpoint field")
	}
}

func TestThresholdSummary(t *testing.T) {
	var buf bytes.Buffer
	results := []metrics.ThresholdResult{
		{Name: "p95 latency", Threshold: "<= 500ms", Actual: "200ms", Passed: true},
		{Name: "error rate", Threshold: "<= 5.0%", Actual: "8.0%", Passed: false},
	}
	ThresholdSummary(&buf, results)
	out := buf.String()

	checks := []string{"Threshold Results", "p95 latency", "PASS", "error rate", "FAIL"}
	for _, c := range checks {
		if !strings.Contains(out, c) {
			t.Errorf("ThresholdSummary output missing %q\nOutput:\n%s", c, out)
		}
	}
}

func TestFmtDur(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "-"},
		{500 * time.Microsecond, "500.0µs"},
		{45 * time.Millisecond, "45.0ms"},
		{2 * time.Second, "2.00s"},
	}
	for _, tc := range tests {
		got := FmtDur(tc.d)
		if got != tc.want {
			t.Errorf("FmtDur(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	if got := FormatElapsed(90 * time.Second); got != "01:30" {
		t.Errorf("expected 01:30, got %q", got)
	}
	if got := FormatElapsed(3661 * time.Second); got != "01:01:01" {
		t.Errorf("expected 01:01:01, got %q", got)
	}
}

func TestPrint_UsesInstantRPS(t *testing.T) {
	var buf bytes.Buffer
	stats := sampleStats()
	stats.InstantRPS = 42.5
	stats.RPS = 98.0 // cumulative — should NOT appear in periodic output
	Print(&buf, stats)
	out := buf.String()

	// Periodic print should show InstantRPS (42.5), not cumulative RPS (98.0)
	if !strings.Contains(out, "42.5") {
		t.Errorf("Print should show InstantRPS (42.5)\nOutput:\n%s", out)
	}
}

func TestSummary_UsesCumulativeRPS(t *testing.T) {
	var buf bytes.Buffer
	stats := sampleStats()
	stats.RPS = 98.0
	stats.InstantRPS = 42.5
	Summary(&buf, stats)
	out := buf.String()

	// Summary should show cumulative Avg RPS
	if !strings.Contains(out, "98.00") {
		t.Errorf("Summary should show cumulative RPS (98.00)\nOutput:\n%s", out)
	}
}

func TestPrint_EmptyPerEndpoint(t *testing.T) {
	var buf bytes.Buffer
	stats := &metrics.Stats{
		TotalRequests: 0,
		Elapsed:       1 * time.Second,
		ActiveVUs:     0,
		PerEndpoint:   map[string]*metrics.EndpointStats{},
	}
	// Should not panic with empty PerEndpoint
	Print(&buf, stats)
	out := buf.String()
	if !strings.Contains(out, "VUs:") {
		t.Errorf("Print output should contain VUs header\nOutput:\n%s", out)
	}
}

func TestSummary_NoErrors(t *testing.T) {
	var buf bytes.Buffer
	stats := &metrics.Stats{
		TotalRequests: 100,
		SuccessCount:  100,
		ErrorCount:    0,
		RPS:           10.0,
		P50:           10 * time.Millisecond,
		P90:           20 * time.Millisecond,
		P95:           30 * time.Millisecond,
		P99:           40 * time.Millisecond,
		Min:           5 * time.Millisecond,
		Max:           50 * time.Millisecond,
		Avg:           15 * time.Millisecond,
		Elapsed:       10 * time.Second,
		PerEndpoint:   map[string]*metrics.EndpointStats{},
		StatusCodes:   map[int]int64{200: 100},
		ErrorTypes:    map[string]int64{},
	}
	Summary(&buf, stats)
	out := buf.String()

	if !strings.Contains(out, "Errors:         0") {
		t.Errorf("should show 0 errors\nOutput:\n%s", out)
	}
	// Should not show error type section when empty
	if strings.Contains(out, "Error Type Breakdown") {
		t.Error("should not show error type breakdown when no errors")
	}
}

func TestWriteJSON_IncludesInstantRPS(t *testing.T) {
	stats := sampleStats()
	stats.InstantRPS = 55.5
	dir := t.TempDir()
	path := filepath.Join(dir, "results.json")

	if err := WriteJSON(path, stats); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	irps, ok := result["InstantRPS"]
	if !ok {
		t.Error("JSON missing InstantRPS field")
	}
	if irps.(float64) != 55.5 {
		t.Errorf("expected InstantRPS 55.5, got %v", irps)
	}
}
