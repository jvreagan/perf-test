package metrics

import (
	"strings"
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
	StatusCodes   map[int]int64
	ErrorTypes    map[string]int64
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
	StatusCodes      map[int]int64
	ErrorTypes       map[string]int64
	ThresholdResults []ThresholdResult `json:",omitempty"`
}

type endpointData struct {
	reservoir  *Reservoir
	successes  int64
	errors     int64
	bytes      int64
	statusCodes map[int]int64
	errorTypes  map[string]int64
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
		ep = &endpointData{
			reservoir:   NewReservoir(DefaultReservoirSize),
			statusCodes: make(map[int]int64),
			errorTypes:  make(map[string]int64),
		}
		c.endpoints[r.EndpointName] = ep
	}
	ep.reservoir.Add(r.Duration)
	ep.bytes += r.BytesReceived

	if r.StatusCode > 0 {
		ep.statusCodes[r.StatusCode]++
	}

	if r.Success {
		ep.successes++
	} else {
		ep.errors++
		if r.Error != nil {
			ep.errorTypes[classifyError(r.Error)]++
		}
	}
}

// classifyError categorizes an error by inspecting its message.
func classifyError(err error) string {
	msg := strings.ToLower(err.Error())

	switch {
	case strings.Contains(msg, "status mismatch") || strings.Contains(msg, "expected status"):
		return "status_mismatch"
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded"):
		return "timeout"
	case strings.Contains(msg, "connection refused"):
		return "connection_refused"
	case strings.Contains(msg, "no such host") || strings.Contains(msg, "dns"):
		return "dns_error"
	case strings.Contains(msg, "tls") || strings.Contains(msg, "certificate"):
		return "tls_error"
	default:
		return "other"
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
		StatusCodes: make(map[int]int64),
		ErrorTypes:  make(map[string]int64),
	}

	// For global stats, merge all reservoir buffers into a temporary reservoir.
	globalRes := NewReservoir(DefaultReservoirSize)
	var globalMin, globalMax, globalSum time.Duration
	var globalCount int64
	firstGlobal := true

	for name, ep := range c.endpoints {
		total := ep.successes + ep.errors
		es := &EndpointStats{
			Name:          name,
			TotalRequests: total,
			SuccessCount:  ep.successes,
			ErrorCount:    ep.errors,
			TotalBytes:    ep.bytes,
			StatusCodes:   make(map[int]int64),
			ErrorTypes:    make(map[string]int64),
		}

		// Copy status code and error type maps
		for code, count := range ep.statusCodes {
			es.StatusCodes[code] = count
			stats.StatusCodes[code] += count
		}
		for errType, count := range ep.errorTypes {
			es.ErrorTypes[errType] = count
			stats.ErrorTypes[errType] += count
		}

		sorted := ep.reservoir.Sorted()
		if len(sorted) > 0 {
			es.P50 = percentile(sorted, 50)
			es.P90 = percentile(sorted, 90)
			es.P95 = percentile(sorted, 95)
			es.P99 = percentile(sorted, 99)
			es.Min = ep.reservoir.Min()
			es.Max = ep.reservoir.Max()
			if ep.reservoir.Count() > 0 {
				es.Avg = ep.reservoir.Sum() / time.Duration(ep.reservoir.Count())
			}

			// Contribute to global stats
			for _, d := range sorted {
				globalRes.Add(d)
			}
			if firstGlobal {
				globalMin = ep.reservoir.Min()
				globalMax = ep.reservoir.Max()
				firstGlobal = false
			} else {
				if ep.reservoir.Min() < globalMin {
					globalMin = ep.reservoir.Min()
				}
				if ep.reservoir.Max() > globalMax {
					globalMax = ep.reservoir.Max()
				}
			}
			globalSum += ep.reservoir.Sum()
			globalCount += ep.reservoir.Count()
		}

		stats.PerEndpoint[name] = es
		stats.TotalRequests += total
		stats.SuccessCount += ep.successes
		stats.ErrorCount += ep.errors
	}

	if globalRes.Count() > 0 {
		globalSorted := globalRes.Sorted()
		stats.P50 = percentile(globalSorted, 50)
		stats.P90 = percentile(globalSorted, 90)
		stats.P95 = percentile(globalSorted, 95)
		stats.P99 = percentile(globalSorted, 99)
		stats.Min = globalMin
		stats.Max = globalMax
		if globalCount > 0 {
			stats.Avg = globalSum / time.Duration(globalCount)
		}
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
