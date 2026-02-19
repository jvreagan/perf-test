package worker

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jvreagan/perf-test/internal/config"
	"github.com/jvreagan/perf-test/internal/data"
	"github.com/jvreagan/perf-test/internal/metrics"
)

// Executor holds shared, immutable state for executing HTTP requests.
// It is safe for concurrent use by multiple goroutines.
type Executor struct {
	endpoints   []config.Endpoint
	cumWeights  []int
	totalWeight int
	gen         *data.Generator
	client      *http.Client
}

// NewExecutor creates an Executor with pre-computed cumulative weights.
func NewExecutor(endpoints []config.Endpoint, gen *data.Generator, client *http.Client) *Executor {
	cum := make([]int, len(endpoints))
	total := 0
	for i, ep := range endpoints {
		w := ep.Weight
		if w <= 0 {
			w = 1
		}
		total += w
		cum[i] = total
	}
	return &Executor{
		endpoints:   endpoints,
		cumWeights:  cum,
		totalWeight: total,
		gen:         gen,
		client:      client,
	}
}

// SelectEndpoint picks an endpoint using weighted random selection (binary search).
func (e *Executor) SelectEndpoint() config.Endpoint {
	if len(e.endpoints) == 1 {
		return e.endpoints[0]
	}
	r := rand.Intn(e.totalWeight)
	idx := sort.SearchInts(e.cumWeights, r+1)
	if idx >= len(e.endpoints) {
		idx = len(e.endpoints) - 1
	}
	return e.endpoints[idx]
}

// Execute performs a single HTTP request and returns the Result.
func (e *Executor) Execute(ctx context.Context, ep config.Endpoint) metrics.Result {
	url := e.gen.Generate(ep.URL)
	method := ep.Method

	var bodyReader io.Reader
	if ep.Body != "" {
		body := e.gen.Generate(ep.Body)
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return metrics.Result{
			EndpointName: ep.Name,
			Error:        fmt.Errorf("building request: %w", err),
			Timestamp:    time.Now(),
			Success:      false,
		}
	}

	for k, v := range ep.Headers {
		req.Header.Set(k, e.gen.Generate(v))
	}

	start := time.Now()
	resp, err := e.client.Do(req)
	duration := time.Since(start)

	if err != nil {
		return metrics.Result{
			EndpointName: ep.Name,
			Duration:     duration,
			Error:        err,
			Timestamp:    start,
			Success:      false,
		}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bytesReceived := int64(len(body))

	success := true
	if ep.Expect.Status != 0 && resp.StatusCode != ep.Expect.Status {
		success = false
		err = fmt.Errorf("expected status %d, got %d", ep.Expect.Status, resp.StatusCode)
	}

	return metrics.Result{
		EndpointName:  ep.Name,
		StatusCode:    resp.StatusCode,
		Duration:      duration,
		BytesReceived: bytesReceived,
		Error:         err,
		Timestamp:     start,
		Success:       success,
	}
}
