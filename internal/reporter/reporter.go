package reporter

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jvreagan/perf-test/internal/metrics"
)

// Print writes a periodic stats table to w.
func Print(w io.Writer, stats *metrics.Stats) {
	if stats == nil {
		return
	}
	errPct := 0.0
	if stats.TotalRequests > 0 {
		errPct = float64(stats.ErrorCount) / float64(stats.TotalRequests) * 100
	}

	elapsed := FormatElapsed(stats.Elapsed.Duration())
	fmt.Fprintf(w, "\n[ %s ] VUs: %d  RPS: %.1f  Reqs: %d  Errors: %d (%.1f%%)\n",
		elapsed, stats.ActiveVUs, stats.InstantRPS, stats.TotalRequests, stats.ErrorCount, errPct)
	fmt.Fprintln(w, strings.Repeat("─", 65))
	fmt.Fprintf(w, "%-30s %6s  %8s  %8s  %8s\n", "Endpoint", "Reqs", "p50", "p90", "p99")
	fmt.Fprintln(w, strings.Repeat("─", 65))

	names := sortedKeys(stats.PerEndpoint)
	for _, name := range names {
		es := stats.PerEndpoint[name]
		fmt.Fprintf(w, "%-30s %6d  %8s  %8s  %8s\n",
			truncate(name, 30),
			es.TotalRequests,
			FmtDur(es.P50.Duration()),
			FmtDur(es.P90.Duration()),
			FmtDur(es.P99.Duration()),
		)
	}
	fmt.Fprintln(w, strings.Repeat("─", 65))
}

// Summary writes the final summary report to w.
func Summary(w io.Writer, stats *metrics.Stats) {
	if stats == nil {
		return
	}
	fmt.Fprintln(w, "\n"+strings.Repeat("═", 65))
	fmt.Fprintln(w, "  FINAL SUMMARY")
	fmt.Fprintln(w, strings.Repeat("═", 65))
	fmt.Fprintf(w, "  Duration:       %s\n", FormatElapsed(stats.Elapsed.Duration()))
	fmt.Fprintf(w, "  Total Requests: %d\n", stats.TotalRequests)
	fmt.Fprintf(w, "  Success:        %d\n", stats.SuccessCount)
	fmt.Fprintf(w, "  Errors:         %d\n", stats.ErrorCount)
	fmt.Fprintf(w, "  Avg RPS:        %.2f\n", stats.RPS)
	fmt.Fprintf(w, "  Data Received:  %s\n", FmtBytes(stats.TotalBytes))
	fmt.Fprintln(w, strings.Repeat("─", 65))
	fmt.Fprintf(w, "  %-10s  %10s  %10s  %10s  %10s\n", "Metric", "p50", "p90", "p95", "p99")
	fmt.Fprintln(w, strings.Repeat("─", 65))
	fmt.Fprintf(w, "  %-10s  %10s  %10s  %10s  %10s\n", "Latency",
		FmtDur(stats.P50.Duration()), FmtDur(stats.P90.Duration()), FmtDur(stats.P95.Duration()), FmtDur(stats.P99.Duration()))
	fmt.Fprintf(w, "  Min: %s  Max: %s  Avg: %s\n", FmtDur(stats.Min.Duration()), FmtDur(stats.Max.Duration()), FmtDur(stats.Avg.Duration()))

	if len(stats.PerEndpoint) > 0 {
		fmt.Fprintln(w, strings.Repeat("─", 65))
		fmt.Fprintln(w, "  Per-Endpoint:")
		fmt.Fprintf(w, "  %-28s %6s %8s %8s %8s %8s\n", "Endpoint", "Reqs", "p50", "p90", "p99", "Errors")
		names := sortedKeys(stats.PerEndpoint)
		for _, name := range names {
			es := stats.PerEndpoint[name]
			fmt.Fprintf(w, "  %-28s %6d %8s %8s %8s %8d\n",
				truncate(name, 28),
				es.TotalRequests,
				FmtDur(es.P50.Duration()),
				FmtDur(es.P90.Duration()),
				FmtDur(es.P99.Duration()),
				es.ErrorCount,
			)
		}
	}

	if len(stats.StatusCodes) > 0 {
		fmt.Fprintln(w, strings.Repeat("─", 65))
		fmt.Fprintln(w, "  Status Code Breakdown:")
		fmt.Fprintf(w, "  %-12s %10s\n", "Status", "Count")
		for _, code := range sortedIntKeys(stats.StatusCodes) {
			fmt.Fprintf(w, "  %-12d %10d\n", code, stats.StatusCodes[code])
		}
	}

	if len(stats.ErrorTypes) > 0 {
		fmt.Fprintln(w, strings.Repeat("─", 65))
		fmt.Fprintln(w, "  Error Type Breakdown:")
		fmt.Fprintf(w, "  %-20s %10s\n", "Type", "Count")
		for _, errType := range sortedStringKeys(stats.ErrorTypes) {
			fmt.Fprintf(w, "  %-20s %10d\n", errType, stats.ErrorTypes[errType])
		}
	}

	fmt.Fprintln(w, strings.Repeat("═", 65))
}

// ThresholdSummary writes the threshold evaluation results table.
func ThresholdSummary(w io.Writer, results []metrics.ThresholdResult) {
	fmt.Fprintln(w, strings.Repeat("─", 65))
	fmt.Fprintln(w, "  Threshold Results:")
	fmt.Fprintf(w, "  %-16s %-18s %-14s %s\n", "Metric", "Threshold", "Actual", "Result")
	fmt.Fprintln(w, "  "+strings.Repeat("─", 60))
	for _, r := range results {
		status := "PASS"
		if !r.Passed {
			status = "FAIL"
		}
		fmt.Fprintf(w, "  %-16s %-18s %-14s %s\n", r.Name, r.Threshold, r.Actual, status)
	}
}

// WriteJSON writes the stats snapshot as JSON to the given file path.
func WriteJSON(path string, stats *metrics.Stats) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(stats); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// FmtDur formats a latency duration in a human-friendly way.
func FmtDur(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%.1fµs", float64(d)/float64(time.Microsecond))
	}
	if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d)/float64(time.Millisecond))
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

// FormatElapsed formats an elapsed duration as MM:SS or HH:MM:SS.
func FormatElapsed(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func sortedKeys(m map[string]*metrics.EndpointStats) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedIntKeys(m map[int]int64) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

func sortedStringKeys(m map[string]int64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// FmtBytes formats a byte count in a human-friendly way.
func FmtBytes(b int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}
