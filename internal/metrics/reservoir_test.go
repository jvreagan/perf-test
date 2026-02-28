package metrics

import (
	"testing"
	"time"
)

func TestReservoir_Fill(t *testing.T) {
	r := NewReservoir(100)
	for i := 1; i <= 50; i++ {
		r.Add(time.Duration(i) * time.Millisecond)
	}
	if r.Count() != 50 {
		t.Errorf("expected count 50, got %d", r.Count())
	}
	sorted := r.Sorted()
	if len(sorted) != 50 {
		t.Errorf("expected 50 samples, got %d", len(sorted))
	}
	if sorted[0] != 1*time.Millisecond {
		t.Errorf("expected min 1ms in sorted, got %v", sorted[0])
	}
	if sorted[len(sorted)-1] != 50*time.Millisecond {
		t.Errorf("expected max 50ms in sorted, got %v", sorted[len(sorted)-1])
	}
}

func TestReservoir_KnownDataPercentiles(t *testing.T) {
	r := NewReservoir(DefaultReservoirSize)
	// 100 values: 1ms .. 100ms — well under reservoir size
	for i := 1; i <= 100; i++ {
		r.Add(time.Duration(i) * time.Millisecond)
	}
	sorted := r.Sorted()
	// With all samples retained, percentiles match exactly.
	p50 := percentile(sorted, 50)
	p99 := percentile(sorted, 99)
	if p50 != 50*time.Millisecond {
		t.Errorf("p50: expected 50ms, got %v", p50)
	}
	if p99 != 99*time.Millisecond {
		t.Errorf("p99: expected 99ms, got %v", p99)
	}
	if r.Min() != 1*time.Millisecond {
		t.Errorf("min: expected 1ms, got %v", r.Min())
	}
	if r.Max() != 100*time.Millisecond {
		t.Errorf("max: expected 100ms, got %v", r.Max())
	}
}

func TestReservoir_MemoryBounded(t *testing.T) {
	r := NewReservoir(DefaultReservoirSize)
	n := 1_000_000
	for i := 0; i < n; i++ {
		r.Add(time.Duration(i+1) * time.Microsecond)
	}
	if r.Count() != int64(n) {
		t.Errorf("expected count %d, got %d", n, r.Count())
	}
	sorted := r.Sorted()
	if len(sorted) != DefaultReservoirSize {
		t.Errorf("expected buffer capped at %d, got %d", DefaultReservoirSize, len(sorted))
	}
	if r.Min() != 1*time.Microsecond {
		t.Errorf("min: expected 1µs, got %v", r.Min())
	}
	if r.Max() != time.Duration(n)*time.Microsecond {
		t.Errorf("max: expected %v, got %v", time.Duration(n)*time.Microsecond, r.Max())
	}
}

func TestReservoir_EmptyPercentile(t *testing.T) {
	r := NewReservoir(100)
	sorted := r.Sorted()
	if sorted != nil {
		t.Errorf("expected nil sorted for empty reservoir, got %v", sorted)
	}
	p := percentile(sorted, 50)
	if p != 0 {
		t.Errorf("expected 0 for empty percentile, got %v", p)
	}
}

func TestReservoir_ExactStats(t *testing.T) {
	r := NewReservoir(10)
	// Add 100 items (exceeds reservoir size) — exact stats still correct
	for i := 1; i <= 100; i++ {
		r.Add(time.Duration(i) * time.Millisecond)
	}
	if r.Count() != 100 {
		t.Errorf("count: expected 100, got %d", r.Count())
	}
	if r.Min() != 1*time.Millisecond {
		t.Errorf("min: expected 1ms, got %v", r.Min())
	}
	if r.Max() != 100*time.Millisecond {
		t.Errorf("max: expected 100ms, got %v", r.Max())
	}
	expectedSum := time.Duration(5050) * time.Millisecond // 1+2+...+100 = 5050
	if r.Sum() != expectedSum {
		t.Errorf("sum: expected %v, got %v", expectedSum, r.Sum())
	}
}
