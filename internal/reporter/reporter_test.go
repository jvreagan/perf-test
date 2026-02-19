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
		PerEndpoint: map[string]*metrics.EndpointStats{
			"GET /users": {
				Name:          "GET /users",
				TotalRequests: 400,
				SuccessCount:  395,
				ErrorCount:    5,
				P50:           40 * time.Millisecond,
				P90:           100 * time.Millisecond,
				P99:           280 * time.Millisecond,
			},
			"POST /items": {
				Name:          "POST /items",
				TotalRequests: 100,
				SuccessCount:  95,
				ErrorCount:    5,
				P50:           80 * time.Millisecond,
				P90:           180 * time.Millisecond,
				P99:           400 * time.Millisecond,
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

	checks := []string{"FINAL SUMMARY", "Total Requests", "Success", "Errors", "Avg RPS", "Per-Endpoint"}
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
		got := fmtDur(tc.d)
		if got != tc.want {
			t.Errorf("fmtDur(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	if got := formatDuration(90 * time.Second); got != "01:30" {
		t.Errorf("expected 01:30, got %q", got)
	}
	if got := formatDuration(3661 * time.Second); got != "01:01:01" {
		t.Errorf("expected 01:01:01, got %q", got)
	}
}
