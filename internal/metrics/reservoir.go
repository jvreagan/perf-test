package metrics

import (
	"math/rand"
	"sort"
	"time"
)

// DefaultReservoirSize is the maximum number of samples retained.
const DefaultReservoirSize = 10_000

// Reservoir implements Algorithm R for reservoir sampling.
// It keeps a bounded buffer of samples while tracking exact min/max/sum/count.
type Reservoir struct {
	buf   []time.Duration
	size  int
	count int64
	min   time.Duration
	max   time.Duration
	sum   time.Duration
	rng   *rand.Rand
}

// NewReservoir creates a Reservoir with the given maximum size.
func NewReservoir(size int) *Reservoir {
	return &Reservoir{
		buf:  make([]time.Duration, 0, size),
		size: size,
		rng:  rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Add records a duration sample using Algorithm R.
func (r *Reservoir) Add(d time.Duration) {
	r.count++
	r.sum += d

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

	if int(r.count) <= r.size {
		r.buf = append(r.buf, d)
	} else {
		// Replace with probability size/count
		j := r.rng.Int63n(r.count)
		if j < int64(r.size) {
			r.buf[j] = d
		}
	}
}

// Sorted returns a sorted copy of the reservoir buffer for percentile computation.
func (r *Reservoir) Sorted() []time.Duration {
	if len(r.buf) == 0 {
		return nil
	}
	sorted := make([]time.Duration, len(r.buf))
	copy(sorted, r.buf)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
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
