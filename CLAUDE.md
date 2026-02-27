# perf-test - Config-Driven HTTP Load Testing Tool

## Overview

perf-test is an open-source CLI tool for load testing HTTP APIs. It supports weighted
multi-endpoint tests, stage-based load ramp profiles, data templating, periodic stats
output, and JSON results export.

**Module**: `github.com/jvreagan/perf-test`
**GitHub**: https://github.com/jvreagan/perf-test
**Language**: Go 1.22
**Binary**: `./perf-test`

---

## CLI Commands

```bash
./perf-test run [config.yaml]       # Run a load test (default: perf-test.yaml)
./perf-test validate [config.yaml]  # Validate config without running
./perf-test version                 # Print version
```

---

## Web UI

```bash
go run ./web/cmd                    # Start web UI on localhost:8080
go run ./web/cmd --addr :9090       # Custom address
```

### Routes

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/` | Dashboard: running test + recent completed tests |
| GET | `/configure` | Configuration form for new test |
| POST | `/configure` | Form post-back (add/remove items) or run test |
| GET | `/test/{id}` | Live progress (auto-refresh) or final results |
| GET | `/test/{id}/stop` | Cancel running test |

### Web Architecture

- **Zero JavaScript** — all interactions use HTML form post-back pattern
- **Server-rendered** via Go `html/template` with per-page layout inheritance
- **Live progress** via `<meta http-equiv="refresh" content="2">` (pure HTML)
- **Direct engine import** — web layer calls `engine.Run()` and `engine.Collector().Snapshot()` for live metrics
- **In-memory state** — test runs stored in a mutex-protected map; lost on restart
- **One test at a time** — `State.activeID` enforces single concurrent test

### Form Post-Back Pattern

Dynamic form elements (endpoints, stages, headers, variables) use submit buttons with `name="action"` values like `add_endpoint`, `remove_endpoint_0`, `add_header_1`, `switch_load_style`. The handler checks the action, modifies the `FormData` struct, and re-renders. The main submit uses `action=run`.

### Key Types

- `FormData` — all form fields as strings, with `ParseFormData(r)` and `ToConfig()` methods
- `State` — in-memory map of `TestRun` structs, tracks `activeID`
- `TestRun` — holds engine, cancel func, config, status, and final stats
- `Handlers` — HTTP handlers with `State` and `Templates` dependencies
- `Templates` — per-page template sets, each paired with layout.html

---

## Building

```bash
cd /Users/jamesreagan/code/perf-test
go build ./...                        # Build all packages
go build -o perf-test ./cmd/perf-test # Build CLI binary
go test ./...                         # Run all tests
go run ./web/cmd                      # Start web UI on localhost:8080
```

---

## Architecture

```
cmd/perf-test/main.go           CLI entry point (cobra)
internal/
  config/config.go              YAML parsing, defaults, validation
  engine/engine.go              Orchestrates test run (VU pool or arrival rate)
  scheduler/scheduler.go        Stage-based ramp profile → emits int targets on channel
  worker/worker.go              VU loop: rate-limit → select endpoint → execute
  worker/executor.go            Shared HTTP execution logic (concurrency-safe)
  ratelimit/ratelimit.go        Token-bucket rate limiter (stdlib only)
  metrics/collector.go          Thread-safe result aggregation + percentiles
  reporter/reporter.go          Console table + JSON file output
  data/generator.go             ${token} template engine
web/
  cmd/main.go                   Web UI entry point (go run ./web/cmd)
  server.go                     HTTP server + route registration (Go 1.22 ServeMux)
  handlers.go                   All HTTP handlers (index, configure, test status, stop)
  formdata.go                   Form data parsing + conversion to config.Config
  state.go                      In-memory test run state (TestRun, State)
  templates.go                  Template loading with per-page layout inheritance
  templates/                    Server-rendered HTML templates (no JavaScript)
examples/                       Example YAML configs
```

---

## Config Structure

```yaml
name: "My Test"
description: "Optional description"

load:
  mode: vu            # "vu" (default) or "arrival_rate"
  think_time: 0s      # pause between requests per VU (vu mode only)
  max_rps: 0          # global token-bucket cap (vu mode only; 0 = unlimited)
  stages:
    - duration: 30s
      target: 10
      ramp: linear    # "linear" (default) or "step"

  # Shorthand (alternative to stages, requires max_vus):
  ramp_up: 10s
  steady_state: 60s
  ramp_down: 10s
  max_vus: 50

http:
  timeout: 30s
  follow_redirects: true
  insecure_skip_verify: false

variables:
  base_url: "http://localhost:8080"
  token: "${API_TOKEN}"         # OS env var expansion in variables section only

endpoints:
  - name: "My Endpoint"
    method: GET
    url: "${base_url}/path"
    headers:
      Authorization: "Bearer ${token}"
    body: '{"id": "${random.uuid}"}'
    weight: 1
    expect:
      status: 200

output:
  format: console   # "console", "json", or "csv"
  interval: 5s
  file: results.json
```

---

## Load Modes

### VU Mode (default `mode: vu`)
- Maintains a pool of N concurrent virtual users
- Scheduler ramps VU count up/down per stages
- Each VU loops: [wait for rate-limit token] → select endpoint → execute → think time
- `max_rps` applies a shared token-bucket limiter across all VUs
- `think_time` adds a fixed pause after each request per VU

### Arrival Rate Mode (`mode: arrival_rate`)
- Dispatches requests at a fixed RPS target regardless of concurrency
- Stage `target` = desired requests per second
- Concurrency is bounded at `2 × target RPS` (semaphore)
- Dropped ticks are silent (system at capacity)
- `think_time` and `max_rps` are irrelevant in this mode
- In-flight count shown as "VUs" in stats output

---

## Scheduler / Stage Ramp Types

| `ramp` value | Behavior |
|---|---|
| `""` or `"linear"` | Linearly interpolate from previous stage target to this stage target |
| `"step"` | Jump instantly to `target` when entering this stage (no interpolation) |

**Note**: The scheduler emits integer targets every 100ms. Sub-1 values are rounded,
so arrival_rate targets below 1 RPS effectively emit 0 (no dispatching). For slow
rates (< 1 RPS), use VU mode with `think_time` instead.

**Approximating fractional RPS with VU mode + think_time**:
- Rate per VU ≈ `1 / (think_time + avg_response_time)`
- Example: `think_time: 9s` + ~100ms response → ~0.1 RPS per VU
- Ramp from 1→10 VUs to go from 0.1→1 RPS

---

## Data Templating

Use `${token}` syntax in URLs, headers, and bodies:

| Token | Output |
|---|---|
| `${random.uuid}` | UUID v4 |
| `${random.email}` | Random email |
| `${random.bool}` | `true` or `false` |
| `${random.int(1,100)}` | Random integer in range |
| `${random.float(0.0,1.0)}` | Random float in range |
| `${random.string(8)}` | Random alphanumeric string |
| `${random.choice(a,b,c)}` | Random choice from list |
| `${var.name}` or `${name}` | Value from `variables` section |

Environment variables are expanded only in `variables` section values (not in URLs/bodies),
so `${base_url}` style tokens survive as template tokens.

---

## Validation Rules

| Rule | Error |
|---|---|
| No endpoints defined | `at least one endpoint is required` |
| Empty endpoint URL | `endpoint[N] "name": URL is required` |
| No stages defined | `load stages are required (...)` |
| Stage duration ≤ 0 | `stage[N]: duration must be positive` |
| Stage target < 0 | `stage[N]: target VUs/RPS must be >= 0` |
| `stage.ramp` not in `{"", "linear", "step"}` | `stage[N]: ramp must be "linear" or "step"` |
| `load.mode` not in `{"", "vu", "arrival_rate"}` | `load.mode must be "vu" or "arrival_rate"` |
| `max_rps > 0` with `mode: arrival_rate` | `load.max_rps is only valid in vu mode` |
| `max_rps < 0` | `load.max_rps must be >= 0` |
| Invalid output format | `output.format must be one of: console, json, csv` |

---

## Example Configs

| File | Description |
|---|---|
| `examples/basic.yaml` | Single endpoint, shorthand ramp |
| `examples/advanced.yaml` | Multi-endpoint with templating, stages, max_rps |
| `examples/step-ramp.yaml` | Instant VU jump using `ramp: step` |
| `examples/arrival-rate.yaml` | Fixed RPS dispatch mode |
| `examples/max-rps-cap.yaml` | VU pool with global token-bucket cap |

---

## Public Test Targets

Good APIs to test against (no auth required, light use only):

- **https://httpbin.org/get** — echoes request details as JSON (p50 ~85ms from US)
- **https://httpbin.org/post** — echoes POST body, good for template verification
- **https://httpbin.org/delay/1** — artificial N-second delay, good for latency tests
- **https://jsonplaceholder.typicode.com/posts** — fake REST API, very stable

**httpbin.org rate limit**: keep VUs ≤ 20 or you'll get 429s.

---

## Observed Behavior Notes

- **RPS shown in periodic reports is cumulative average** (total requests / elapsed seconds),
  not instantaneous. Instantaneous rate is higher at end of a ramp.
- **httpbin.org avg latency**: ~85–170ms p50 from US; p99 can spike to 600ms+
- **think_time interacts with response time**: effective per-VU rate =
  `1 / (think_time + actual_response_time)`, not `1 / think_time`
- **Yellow health on EB** is acceptable (Elastic Beanstalk health, unrelated to this tool)

---

## Implementation History

### v0.1.0 — Initial Release (2026-02-21)
Full initial implementation including:
- Config system with YAML parsing, env var expansion, defaults, normalization, validation
- Stage-based scheduler with linear interpolation (100ms tick)
- VU worker pool with weighted endpoint selection (binary search, O(log n))
- HTTP execution with body templating, header templating, status assertion
- Thread-safe metrics collector (p50/p90/p95/p99, per-endpoint breakdowns)
- Console reporter (periodic table) and JSON file output
- Data generator: uuid, email, bool, int, float, string, choice, variables
- **Step ramp**: `ramp: step` on any stage jumps instantly to target
- **Arrival rate mode**: `mode: arrival_rate` dispatches at fixed RPS via ticker + semaphore
- **Max RPS cap**: `max_rps` token-bucket limiter (stdlib only) shared across all VUs
- Executor extracted from Worker for reuse in arrival rate dispatcher
- Graceful shutdown on SIGINT/SIGTERM
- Example configs for all three new features

### v0.2.0 — Web UI (2026-02-26)
Server-rendered web UI with zero JavaScript:
- Full config form: test info, load mode/stages/shorthand, HTTP settings, variables, endpoints with headers, output settings
- Dynamic form elements via HTML post-back pattern (add/remove endpoints, stages, headers, variables)
- Live test progress with `<meta http-equiv="refresh">` auto-refresh showing VUs, RPS, latency, per-endpoint breakdown
- Final results page with full latency summary and per-endpoint stats
- Dashboard with active test and recent test history
- Engine modified: `Run()` accepts `io.Writer`, returns `(*metrics.Stats, error)`, exposes `Collector()` for live snapshots
- Config methods `ApplyDefaults()` and `NormalizeStages()` exported for web layer reuse
- 30 tests (unit + integration) using `net/http/httptest`

### Live Test Run (2026-02-21)
Ran a 2-minute ramp from 0.1 RPS → 1 RPS against httpbin.org/get:
- Config: VU mode, `think_time: 9s`, 1→10 VUs over 2 minutes
- Result: 77 requests, 0 errors, avg RPS 0.64, p50 88ms, p99 610ms
- VU count climbed as expected: 2→3→4→5→7→8→9→10

---

## Support / Related Repos

- **perf-test repo**: `/Users/jamesreagan/code/perf-test`
- **GitHub**: https://github.com/jvreagan/perf-test

---

**Last Updated**: 2026-02-26
**Status**: v0.2.0 ✅
