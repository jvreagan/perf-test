package metrics

import (
	"sort"
	"sync"
	"time"
)

// Result holds the outcome of a single HTTP request.
type Result struct {
	EndpointName  string
	StatusCode    int
	Duration      time.Duration
	BytesReceived int64
	Error         error
	Timestamp     time.Time
	Success       bool
}

// EndpointStats holds per-endpoint aggregated metrics.
type EndpointStats struct {
	Name          string
	TotalRequests int64
	SuccessCount  int64
	ErrorCount    int64
	TotalBytes    int64
	P50           time.Duration
	P90           time.Duration
	P95           time.Duration
	P99           time.Duration
	Min           time.Duration
	Max           time.Duration
	Avg           time.Duration
}

// Stats is a point-in-time snapshot of all collected metrics.
type Stats struct {
	TotalRequests int64
	SuccessCount  int64
	ErrorCount    int64
	RPS           float64
	P50           time.Duration
	P90           time.Duration
	P95           time.Duration
	P99           time.Duration
	Min           time.Duration
	Max           time.Duration
	Avg           time.Duration
	PerEndpoint   map[string]*EndpointStats
	ActiveVUs     int
	Elapsed       time.Duration
}

type endpointData struct {
	durations []time.Duration
	successes int64
	errors    int64
	bytes     int64
}

// Collector gathers Results from concurrent workers thread-safely.
type Collector struct {
	mu        sync.Mutex
	startTime time.Time
	endpoints map[string]*endpointData
	activeVUs int
}

// NewCollector creates a Collector with the given start time.
func NewCollector(start time.Time) *Collector {
	return &Collector{
		startTime: start,
		endpoints: make(map[string]*endpointData),
	}
}

// SetActiveVUs updates the active VU count (called by engine).
func (c *Collector) SetActiveVUs(n int) {
	c.mu.Lock()
	c.activeVUs = n
	c.mu.Unlock()
}

// Record adds a Result to the collector.
func (c *Collector) Record(r Result) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ep, ok := c.endpoints[r.EndpointName]
	if !ok {
		ep = &endpointData{}
		c.endpoints[r.EndpointName] = ep
	}
	ep.durations = append(ep.durations, r.Duration)
	ep.bytes += r.BytesReceived
	if r.Success {
		ep.successes++
	} else {
		ep.errors++
	}
}

// Snapshot computes and returns a point-in-time Stats snapshot.
func (c *Collector) Snapshot() *Stats {
	c.mu.Lock()
	defer c.mu.Unlock()

	elapsed := time.Since(c.startTime)
	stats := &Stats{
		Elapsed:     elapsed,
		ActiveVUs:   c.activeVUs,
		PerEndpoint: make(map[string]*EndpointStats),
	}

	var allDurations []time.Duration

	for name, ep := range c.endpoints {
		total := ep.successes + ep.errors
		es := &EndpointStats{
			Name:          name,
			TotalRequests: total,
			SuccessCount:  ep.successes,
			ErrorCount:    ep.errors,
			TotalBytes:    ep.bytes,
		}
		if len(ep.durations) > 0 {
			sorted := make([]time.Duration, len(ep.durations))
			copy(sorted, ep.durations)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

			es.P50 = percentile(sorted, 50)
			es.P90 = percentile(sorted, 90)
			es.P95 = percentile(sorted, 95)
			es.P99 = percentile(sorted, 99)
			es.Min = sorted[0]
			es.Max = sorted[len(sorted)-1]
			es.Avg = average(sorted)

			allDurations = append(allDurations, ep.durations...)
		}
		stats.PerEndpoint[name] = es
		stats.TotalRequests += total
		stats.SuccessCount += ep.successes
		stats.ErrorCount += ep.errors
	}

	if len(allDurations) > 0 {
		sort.Slice(allDurations, func(i, j int) bool { return allDurations[i] < allDurations[j] })
		stats.P50 = percentile(allDurations, 50)
		stats.P90 = percentile(allDurations, 90)
		stats.P95 = percentile(allDurations, 95)
		stats.P99 = percentile(allDurations, 99)
		stats.Min = allDurations[0]
		stats.Max = allDurations[len(allDurations)-1]
		stats.Avg = average(allDurations)
	}

	if elapsed.Seconds() > 0 {
		stats.RPS = float64(stats.TotalRequests) / elapsed.Seconds()
	}

	return stats
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p / 100.0)
	return sorted[idx]
}

func average(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	var sum time.Duration
	for _, d := range durations {
		sum += d
	}
	return sum / time.Duration(len(durations))
}
