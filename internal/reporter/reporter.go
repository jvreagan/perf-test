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
	errPct := 0.0
	if stats.TotalRequests > 0 {
		errPct = float64(stats.ErrorCount) / float64(stats.TotalRequests) * 100
	}

	elapsed := formatDuration(stats.Elapsed)
	fmt.Fprintf(w, "\n[ %s ] VUs: %d  RPS: %.1f  Reqs: %d  Errors: %d (%.1f%%)\n",
		elapsed, stats.ActiveVUs, stats.RPS, stats.TotalRequests, stats.ErrorCount, errPct)
	fmt.Fprintln(w, strings.Repeat("─", 65))
	fmt.Fprintf(w, "%-30s %6s  %8s  %8s  %8s\n", "Endpoint", "Reqs", "p50", "p90", "p99")
	fmt.Fprintln(w, strings.Repeat("─", 65))

	names := sortedKeys(stats.PerEndpoint)
	for _, name := range names {
		es := stats.PerEndpoint[name]
		fmt.Fprintf(w, "%-30s %6d  %8s  %8s  %8s\n",
			truncate(name, 30),
			es.TotalRequests,
			fmtDur(es.P50),
			fmtDur(es.P90),
			fmtDur(es.P99),
		)
	}
	fmt.Fprintln(w, strings.Repeat("─", 65))
}

// Summary writes the final summary report to w.
func Summary(w io.Writer, stats *metrics.Stats) {
	fmt.Fprintln(w, "\n"+strings.Repeat("═", 65))
	fmt.Fprintln(w, "  FINAL SUMMARY")
	fmt.Fprintln(w, strings.Repeat("═", 65))
	fmt.Fprintf(w, "  Duration:       %s\n", formatDuration(stats.Elapsed))
	fmt.Fprintf(w, "  Total Requests: %d\n", stats.TotalRequests)
	fmt.Fprintf(w, "  Success:        %d\n", stats.SuccessCount)
	fmt.Fprintf(w, "  Errors:         %d\n", stats.ErrorCount)
	fmt.Fprintf(w, "  Avg RPS:        %.2f\n", stats.RPS)
	fmt.Fprintln(w, strings.Repeat("─", 65))
	fmt.Fprintf(w, "  %-10s  %10s  %10s  %10s  %10s\n", "Metric", "p50", "p90", "p95", "p99")
	fmt.Fprintln(w, strings.Repeat("─", 65))
	fmt.Fprintf(w, "  %-10s  %10s  %10s  %10s  %10s\n", "Latency",
		fmtDur(stats.P50), fmtDur(stats.P90), fmtDur(stats.P95), fmtDur(stats.P99))
	fmt.Fprintf(w, "  Min: %s  Max: %s  Avg: %s\n", fmtDur(stats.Min), fmtDur(stats.Max), fmtDur(stats.Avg))

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
				fmtDur(es.P50),
				fmtDur(es.P90),
				fmtDur(es.P99),
				es.ErrorCount,
			)
		}
	}
	fmt.Fprintln(w, strings.Repeat("═", 65))
}

// WriteJSON writes the stats snapshot as JSON to the given file path.
func WriteJSON(path string, stats *metrics.Stats) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(stats)
}

func fmtDur(d time.Duration) string {
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

func formatDuration(d time.Duration) string {
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
