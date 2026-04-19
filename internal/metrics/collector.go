package metrics

import (
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// JSONDuration wraps time.Duration with human-readable JSON marshalling.
type JSONDuration time.Duration

func (d JSONDuration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *JSONDuration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		// Fall back to nanosecond integer
		var ns int64
		if err := json.Unmarshal(b, &ns); err != nil {
			return err
		}
		*d = JSONDuration(ns)
		return nil
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = JSONDuration(dur)
	return nil
}

// Duration returns the underlying time.Duration.
func (d JSONDuration) Duration() time.Duration {
	return time.Duration(d)
}

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
	Name          string       `json:"name"`
	TotalRequests int64        `json:"total_requests"`
	SuccessCount  int64        `json:"success_count"`
	ErrorCount    int64        `json:"error_count"`
	TotalBytes    int64        `json:"total_bytes"`
	P50           JSONDuration `json:"p50"`
	P90           JSONDuration `json:"p90"`
	P95           JSONDuration `json:"p95"`
	P99           JSONDuration `json:"p99"`
	Min           JSONDuration `json:"min"`
	Max           JSONDuration `json:"max"`
	Avg           JSONDuration `json:"avg"`
	StatusCodes   map[int]int64    `json:"status_codes,omitempty"`
	ErrorTypes    map[string]int64 `json:"error_types,omitempty"`
}

// Stats is a point-in-time snapshot of all collected metrics.
type Stats struct {
	TotalRequests    int64                     `json:"total_requests"`
	SuccessCount     int64                     `json:"success_count"`
	ErrorCount       int64                     `json:"error_count"`
	RPS              float64                   `json:"rps"`
	InstantRPS       float64                   `json:"instant_rps"`
	P50              JSONDuration              `json:"p50"`
	P90              JSONDuration              `json:"p90"`
	P95              JSONDuration              `json:"p95"`
	P99              JSONDuration              `json:"p99"`
	Min              JSONDuration              `json:"min"`
	Max              JSONDuration              `json:"max"`
	Avg              JSONDuration              `json:"avg"`
	TotalBytes       int64                     `json:"total_bytes"`
	PerEndpoint      map[string]*EndpointStats `json:"per_endpoint,omitempty"`
	ActiveVUs        int                       `json:"active_vus"`
	Elapsed          JSONDuration              `json:"elapsed"`
	StatusCodes      map[int]int64             `json:"status_codes,omitempty"`
	ErrorTypes       map[string]int64          `json:"error_types,omitempty"`
	ThresholdResults []ThresholdResult         `json:"threshold_results,omitempty"`
}

type endpointData struct {
	reservoir  *Reservoir
	successes  int64
	errors     int64
	bytes      int64
	statusCodes map[int]int64
	errorTypes  map[string]int64
}

const rpsWindow = 10 // sliding window size in seconds

// Collector gathers Results from concurrent workers thread-safely.
type Collector struct {
	mu        sync.Mutex
	startTime time.Time
	endpoints map[string]*endpointData
	global    *Reservoir // global reservoir maintained alongside per-endpoint
	activeVUs int

	// Sliding window for instantaneous RPS
	recentCounts [rpsWindow]int64
	recentIdx    int
	recentTime   time.Time // truncated to second of last record
}

// NewCollector creates a Collector with the given start time.
func NewCollector(start time.Time) *Collector {
	return &Collector{
		startTime:  start,
		endpoints:  make(map[string]*endpointData),
		global:     NewReservoir(DefaultReservoirSize),
		recentTime: start.Truncate(time.Second),
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

	// Advance sliding window ring buffer
	now := time.Now().Truncate(time.Second)
	if now.After(c.recentTime) {
		elapsed := int(now.Sub(c.recentTime) / time.Second)
		if elapsed >= rpsWindow {
			// Clear entire window
			c.recentCounts = [rpsWindow]int64{}
			c.recentIdx = 0
		} else {
			for i := 0; i < elapsed; i++ {
				c.recentIdx = (c.recentIdx + 1) % rpsWindow
				c.recentCounts[c.recentIdx] = 0
			}
		}
		c.recentTime = now
	}
	c.recentCounts[c.recentIdx]++

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
	c.global.Add(r.Duration)
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
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	elapsed := now.Sub(c.startTime)
	stats := &Stats{
		Elapsed:     JSONDuration(elapsed),
		ActiveVUs:   c.activeVUs,
		PerEndpoint: make(map[string]*EndpointStats),
		StatusCodes: make(map[int]int64),
		ErrorTypes:  make(map[string]int64),
	}

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
			es.P50 = JSONDuration(percentile(sorted, 50))
			es.P90 = JSONDuration(percentile(sorted, 90))
			es.P95 = JSONDuration(percentile(sorted, 95))
			es.P99 = JSONDuration(percentile(sorted, 99))
			es.Min = JSONDuration(ep.reservoir.Min())
			es.Max = JSONDuration(ep.reservoir.Max())
			if ep.reservoir.Count() > 0 {
				es.Avg = JSONDuration(ep.reservoir.Sum() / time.Duration(ep.reservoir.Count()))
			}
		}

		stats.PerEndpoint[name] = es
		stats.TotalRequests += total
		stats.SuccessCount += ep.successes
		stats.ErrorCount += ep.errors
		stats.TotalBytes += ep.bytes
	}

	// Global percentiles from the maintained global reservoir
	if c.global.Count() > 0 {
		globalSorted := c.global.Sorted()
		stats.P50 = JSONDuration(percentile(globalSorted, 50))
		stats.P90 = JSONDuration(percentile(globalSorted, 90))
		stats.P95 = JSONDuration(percentile(globalSorted, 95))
		stats.P99 = JSONDuration(percentile(globalSorted, 99))
		stats.Min = JSONDuration(c.global.Min())
		stats.Max = JSONDuration(c.global.Max())
		stats.Avg = JSONDuration(c.global.Sum() / time.Duration(c.global.Count()))
	}

	if elapsed.Seconds() > 0 {
		stats.RPS = float64(stats.TotalRequests) / elapsed.Seconds()
	}

	// Compute instantaneous RPS from sliding window
	nowTrunc := now.Truncate(time.Second)
	windowElapsed := int(nowTrunc.Sub(c.recentTime) / time.Second)
	if windowElapsed < rpsWindow {
		var sum int64
		validBuckets := 0
		for i := 0; i < rpsWindow; i++ {
			idx := (c.recentIdx - i + rpsWindow) % rpsWindow
			age := i + windowElapsed
			if age >= rpsWindow {
				break
			}
			sum += c.recentCounts[idx]
			validBuckets++
		}
		if validBuckets > 0 {
			span := validBuckets + windowElapsed
			if span > rpsWindow {
				span = rpsWindow
			}
			stats.InstantRPS = float64(sum) / float64(span)
		}
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
