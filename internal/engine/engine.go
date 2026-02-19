package engine

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/jvreagan/perf-test/internal/config"
	"github.com/jvreagan/perf-test/internal/data"
	"github.com/jvreagan/perf-test/internal/metrics"
	"github.com/jvreagan/perf-test/internal/ratelimit"
	"github.com/jvreagan/perf-test/internal/reporter"
	"github.com/jvreagan/perf-test/internal/scheduler"
	"github.com/jvreagan/perf-test/internal/worker"
)

// Engine orchestrates the entire load test run.
type Engine struct {
	cfg *config.Config
}

// New creates an Engine from the given config.
func New(cfg *config.Config) *Engine {
	return &Engine{cfg: cfg}
}

type workerEntry struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// Run executes the load test and returns a non-nil error if the test had any failures.
func (e *Engine) Run(ctx context.Context) error {
	client := e.buildClient()
	startTime := time.Now()
	collector := metrics.NewCollector(startTime)
	gen := data.NewGenerator(e.cfg.Variables)

	resultCh := make(chan metrics.Result, 1000)
	targetCh := make(chan int, 10)

	var wg sync.WaitGroup

	// Result collector goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for r := range resultCh {
			collector.Record(r)
		}
	}()

	// Scheduler goroutine — closes targetCh when done so the main loop exits.
	sched := scheduler.New(e.cfg.Load.Stages)
	schedDone := make(chan struct{})
	go func() {
		defer close(schedDone)
		sched.Run(ctx, targetCh)
		close(targetCh)
	}()

	// Reporter goroutine
	reportDone := make(chan struct{})
	go func() {
		defer close(reportDone)
		ticker := time.NewTicker(e.cfg.Output.Interval.Duration)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				snap := collector.Snapshot()
				reporter.Print(os.Stdout, snap)
			case <-ctx.Done():
				return
			case <-schedDone:
				return
			}
		}
	}()

	exec := worker.NewExecutor(e.cfg.Endpoints, gen, client)

	if e.cfg.Load.Mode == "arrival_rate" {
		e.runArrivalRate(ctx, exec, collector, resultCh, targetCh)
	} else {
		e.runVU(ctx, exec, collector, resultCh, targetCh)
	}

	// Stop reporter
	<-reportDone

	// Close result channel and wait for collector
	close(resultCh)
	wg.Wait()

	// Final report
	finalStats := collector.Snapshot()
	reporter.Summary(os.Stdout, finalStats)

	// Write JSON if configured
	if e.cfg.Output.File != "" {
		if err := reporter.WriteJSON(e.cfg.Output.File, finalStats); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to write results file: %v\n", err)
		} else {
			fmt.Fprintf(os.Stdout, "Results written to: %s\n", e.cfg.Output.File)
		}
	}

	if finalStats.ErrorCount > 0 {
		return fmt.Errorf("test completed with %d errors out of %d requests", finalStats.ErrorCount, finalStats.TotalRequests)
	}
	return nil
}

// runVU runs the existing VU pool mode, optionally with a global max_rps limiter.
func (e *Engine) runVU(ctx context.Context, exec *worker.Executor, collector *metrics.Collector, resultCh chan<- metrics.Result, targetCh <-chan int) {
	limiter := ratelimit.NewLimiter(ctx, e.cfg.Load.MaxRPS)

	var workers []workerEntry
	var workerMu sync.Mutex

	setWorkerCount := func(target int) {
		workerMu.Lock()
		defer workerMu.Unlock()

		current := len(workers)
		if target > current {
			for i := current; i < target; i++ {
				wCtx, wCancel := context.WithCancel(ctx)
				done := make(chan struct{})
				id := i
				w := worker.New(id, exec, resultCh, e.cfg.Load.ThinkTime.Duration, limiter)
				go func() {
					defer close(done)
					w.Run(wCtx)
				}()
				workers = append(workers, workerEntry{cancel: wCancel, done: done})
			}
		} else if target < current {
			toRemove := workers[target:]
			workers = workers[:target]
			for _, we := range toRemove {
				we.cancel()
				<-we.done
			}
		}
		collector.SetActiveVUs(len(workers))
	}

	for target := range targetCh {
		setWorkerCount(target)
	}
	setWorkerCount(0)
}

// runArrivalRate dispatches requests at a fixed RPS using a ticker-based dispatcher.
// Each tick fires one request goroutine (up to 2x target RPS concurrency limit).
func (e *Engine) runArrivalRate(ctx context.Context, exec *worker.Executor, collector *metrics.Collector, resultCh chan<- metrics.Result, targetCh <-chan int) {
	var dispatchCancel context.CancelFunc
	var dispatchDone chan struct{}

	setDispatchRate := func(rps int) {
		// Stop the previous dispatcher, if any.
		if dispatchCancel != nil {
			dispatchCancel()
			<-dispatchDone
		}
		if rps == 0 {
			dispatchCancel = nil
			dispatchDone = nil
			collector.SetActiveVUs(0)
			return
		}

		// max concurrency = 2x target RPS (prevents unbounded goroutine growth)
		sem := make(chan struct{}, rps*2)
		dCtx, dCancel := context.WithCancel(ctx)
		dispatchCancel = dCancel
		dispatchDone = make(chan struct{})

		go func() {
			defer close(dispatchDone)
			interval := time.Duration(float64(time.Second) / float64(rps))
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-dCtx.Done():
					return
				case <-ticker.C:
					select {
					case sem <- struct{}{}:
						collector.SetActiveVUs(len(sem))
						go func() {
							defer func() { <-sem }()
							ep := exec.SelectEndpoint()
							// Use parent ctx so rate changes don't abort in-flight requests.
							result := exec.Execute(ctx, ep)
							if result.Error != nil && ctx.Err() != nil {
								return
							}
							select {
							case resultCh <- result:
							case <-ctx.Done():
							}
						}()
					default:
						// Semaphore full — system can't keep up; drop tick silently.
					}
				}
			}
		}()
	}

	for target := range targetCh {
		setDispatchRate(target)
	}
	if dispatchCancel != nil {
		dispatchCancel()
		<-dispatchDone
	}
}

func (e *Engine) buildClient() *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: e.cfg.HTTP.InsecureSkipVerify, //nolint:gosec
		},
	}

	redirectPolicy := http.ErrUseLastResponse
	if e.cfg.HTTP.FollowRedirects {
		redirectPolicy = nil
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   e.cfg.HTTP.Timeout.Duration,
	}

	if !e.cfg.HTTP.FollowRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return redirectPolicy
		}
	}

	return client
}
