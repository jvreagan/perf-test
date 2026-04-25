package metrics

import (
	"math/rand/v2"
	"sort"
	"time"
)

// DefaultReservoirSize is the maximum number of samples retained.
const DefaultReservoirSize = 10_000

// Reservoir implements Algorithm R for reservoir sampling.
// It keeps a bounded buffer of samples while tracking exact min/max/sum/count.
type Reservoir struct {
	buf    []time.Duration
	size   int
	count  int64
	min    time.Duration
	max    time.Duration
	sum    time.Duration
	dirty  bool
	cached []time.Duration
}

// NewReservoir creates a Reservoir with the given maximum size.
func NewReservoir(size int) *Reservoir {
	return &Reservoir{
		buf:  make([]time.Duration, 0, size),
		size: size,
	}
}

// Add records a duration sample using Algorithm R.
func (r *Reservoir) Add(d time.Duration) {
	r.count++
	r.sum += d
	r.dirty = true

	if r.count == 1 {
		r.min = d
		r.max = d
	} else {
		if d < r.min {
			r.min = d
		}
		if d > r.max {
			r.max = d
		}
	}

	if r.count <= int64(r.size) {
		r.buf = append(r.buf, d)
	} else {
		// Replace with probability size/count
		j := rand.Int64N(r.count)
		if j < int64(r.size) {
			r.buf[j] = d
		}
	}
}

// Sorted returns a sorted copy of the reservoir buffer for percentile computation.
// The result is cached and only re-sorted when new data has been added.
func (r *Reservoir) Sorted() []time.Duration {
	if len(r.buf) == 0 {
		return nil
	}
	if !r.dirty && r.cached != nil {
		return r.cached
	}
	sorted := make([]time.Duration, len(r.buf))
	copy(sorted, r.buf)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	r.cached = sorted
	r.dirty = false
	return sorted
}

// Count returns the total number of samples added.
func (r *Reservoir) Count() int64 {
	return r.count
}

// Min returns the exact minimum duration.
func (r *Reservoir) Min() time.Duration {
	return r.min
}

// Max returns the exact maximum duration.
func (r *Reservoir) Max() time.Duration {
	return r.max
}

// Sum returns the exact sum of all durations.
func (r *Reservoir) Sum() time.Duration {
	return r.sum
}
