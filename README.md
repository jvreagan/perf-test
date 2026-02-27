# perf-test

A production-quality open-source performance testing tool for HTTP APIs. Use the CLI with YAML configs or the built-in web UI — no JavaScript required.

## Features

- **CLI + Web UI** — Run tests from the command line or configure them in a browser
- **Config-file driven** — YAML-based test configuration
- **Two load modes** — VU pool or constant arrival rate
- **Stage-based load profiles** — Linear or instant-step ramp with arbitrary stages
- **Global RPS cap** — Token-bucket rate limiter across all VUs
- **Weighted multi-endpoint tests** — Distribute traffic across endpoints by weight
- **Data templating** — Generate random UUIDs, emails, integers, strings, and more
- **Periodic stats output** — Live p50/p90/p99 latency tables during the run
- **JSON results export** — Machine-readable results for CI/CD pipelines
- **Graceful shutdown** — SIGINT/SIGTERM handled cleanly

## Installation

```bash
go install github.com/jvreagan/perf-test/cmd/perf-test@latest
```

Or build from source:

```bash
git clone https://github.com/jvreagan/perf-test
cd perf-test
go build -o perf-test ./cmd/perf-test
```

## Quick Start

```bash
# Run with a config file
perf-test run examples/basic.yaml

# Validate a config file without running
perf-test validate examples/advanced.yaml

# Show version
perf-test version
```

## Web UI

perf-test includes a browser-based interface for configuring and running tests. It uses server-rendered HTML with no JavaScript — all interactions work through standard HTML forms.

### Starting the Web UI

```bash
# From the project root
go run ./web/cmd

# Custom address
go run ./web/cmd --addr localhost:9090
```

Open http://localhost:8080 in your browser.

### Using the Web UI

**Dashboard** (`/`) — Shows any running test and a list of recent completed tests. Click "New Test" to configure a test, or click any completed test to review its results.

**Configure a test** (`/configure`) — A form-based editor for all perf-test settings:

1. **Test Info** — Name and description for your test.
2. **Load Configuration** — Choose the load mode and how to define the load profile:
   - **Load Mode**: "Virtual Users (VU pool)" maintains N concurrent users; "Arrival Rate (fixed RPS)" dispatches requests at a target rate.
   - **Configuration Style**: "Simple" uses ramp up / steady state / ramp down with a single target. "Advanced" lets you define arbitrary stages, each with its own duration, target, and ramp type (linear or step).
   - To switch between Simple and Advanced, select the style and click "Apply Style Change".
   - In VU mode, you can also set **Think Time** (pause between requests per VU) and **Max RPS Cap** (global rate limit).
3. **HTTP Settings** — Request timeout, follow redirects, and TLS verification skip.
4. **Variables** — Key-value pairs you can reference in URLs, headers, and bodies as `${varname}`. Values support environment variable expansion. Click "+ Add Variable" to add more.
5. **Endpoints** — One or more HTTP endpoints to test. Each endpoint has:
   - Name, HTTP method, URL
   - Weight (for distributing traffic — higher weight = more traffic)
   - Expected status code
   - Request body (supports data templating tokens like `${random.uuid}`)
   - Headers (click "+ Add Header" within each endpoint)
   - Click "+ Add Endpoint" to add more endpoints, or "Remove" to delete one.
6. **Output Settings** — Report format, interval, and optional results file path.

Click **"Run Test"** to validate and start the test.

**Live progress** (`/test/{id}`) — While a test is running, this page auto-refreshes every 2 seconds showing:
- Progress bar (based on total stage duration)
- Active VUs, current RPS, total requests, error count
- Per-endpoint table with request counts and p50/p90/p99 latency
- A "Stop Test" button to cancel early

**Results** (`/test/{id}`) — After a test completes (or is stopped), shows the final report:
- Total requests, success/error counts, average RPS
- Latency summary: p50, p90, p95, p99, min, max, avg
- Per-endpoint breakdown with all the same metrics

Only one test can run at a time. If you try to start a second test while one is running, the form will show an error and link you to the running test.

## Load Modes

### VU Mode (default)

Maintains a pool of N concurrent virtual users. Each VU loops continuously:
acquire rate-limit token → select endpoint → send request → think time → repeat.

```yaml
load:
  mode: vu          # default; can be omitted
  think_time: 100ms # optional pause between requests per VU
  max_rps: 500      # optional global token-bucket cap across all VUs
  stages:
    - duration: 30s
      target: 10    # ramp from 0 to 10 VUs
    - duration: 60s
      target: 10    # hold
    - duration: 30s
      target: 0     # ramp down
```

### Arrival Rate Mode

Dispatches requests at a fixed RPS target regardless of how long each request takes.
Use this when you want to control throughput rather than concurrency.

```yaml
load:
  mode: arrival_rate
  stages:
    - duration: 1m
      target: 50    # ramp from 0 to 50 RPS
    - duration: 2m
      target: 50    # hold at 50 RPS
    - duration: 30s
      target: 0     # ramp down
```

Concurrency is bounded automatically at `2 × target RPS`. Dropped ticks (when the
system can't keep up) are silent — the tool never queues unbounded work.
`think_time` and `max_rps` are ignored in arrival rate mode.

## Stage Ramp Types

Each stage can specify how it transitions to its target:

| `ramp` value | Behavior |
|---|---|
| `linear` (default) | Interpolate smoothly from the previous stage's target to this one |
| `step` | Jump instantly to `target` when the stage begins |

```yaml
load:
  stages:
    - duration: 5m
      target: 100
      ramp: step    # spawn all 100 VUs at t=0, no gradual ramp
    - duration: 5s
      target: 0
      ramp: step    # shut down instantly at end
```

## Max RPS Cap

In VU mode, `max_rps` applies a shared token-bucket limiter across all VUs.
Useful when you want many VUs for concurrency but need to cap total throughput.

```yaml
load:
  mode: vu
  max_rps: 200      # never exceed 200 RPS regardless of VU count
  stages:
    - duration: 2m
      target: 50
```

`max_rps` is not valid in `arrival_rate` mode (the stage target already controls RPS directly).

## Config Reference

```yaml
name: "My API Test"
description: "Load test my endpoints"

load:
  mode: vu                # "vu" (default) or "arrival_rate"
  think_time: 100ms       # vu mode: pause between requests per VU
  max_rps: 500            # vu mode: global token-bucket cap (0 = unlimited)

  # Option A: Explicit stages
  stages:
    - duration: 30s
      target: 10          # VUs (vu mode) or RPS (arrival_rate mode)
      ramp: linear        # "linear" (default) or "step"

  # Option B: Shorthand (vu mode only, requires max_vus)
  ramp_up: 30s
  steady_state: 60s
  ramp_down: 30s
  max_vus: 100

http:
  timeout: 30s
  follow_redirects: true
  insecure_skip_verify: false

variables:
  base_url: "https://api.example.com"
  token: "${TOKEN}"       # expands OS env var at load time

endpoints:
  - name: "List Users"
    method: GET
    url: "${base_url}/users"
    headers:
      Authorization: "Bearer ${token}"
    weight: 3             # relative traffic weight (default: 1)
    expect:
      status: 200

  - name: "Create User"
    method: POST
    url: "${base_url}/users"
    headers:
      Content-Type: "application/json"
    body: |
      {"name":"${random.string(8)}","email":"${random.email}","id":"${random.uuid}"}
    weight: 1
    expect:
      status: 201

output:
  format: console         # "console", "json", or "csv"
  interval: 5s
  file: results.json      # optional JSON results export
```

## Data Templating

Use `${...}` tokens in URLs, headers, and request bodies:

| Token | Description | Example Output |
|-------|-------------|----------------|
| `${random.uuid}` | UUID v4 | `550e8400-e29b-41d4-a716-446655440000` |
| `${random.email}` | Random email | `alice1234@example.com` |
| `${random.bool}` | true or false | `true` |
| `${random.int(1,100)}` | Random integer in range | `42` |
| `${random.float(0,1)}` | Random float in range | `0.7351` |
| `${random.string(16)}` | Random alphanumeric | `aB3xZ9kLmN2pQrSt` |
| `${random.choice(a,b,c)}` | Pick from list | `b` |
| `${varname}` | Config variable | value from `variables:` |
| `${var.varname}` | Config variable (explicit) | value from `variables:` |

Environment variables are expanded in `variables` section values only — `${base_url}`
style tokens in URLs and bodies are treated as template variables, not env vars.

## Output Example

```
[ 00:30 ] VUs: 50  RPS: 142.3  Reqs: 4264  Errors: 2 (0.0%)
─────────────────────────────────────────────────────────────────
Endpoint                        Reqs       p50       p90       p99
─────────────────────────────────────────────────────────────────
GET /users                      3198    45.0ms   120.0ms   310.0ms
POST /users                     1066    82.0ms   180.0ms   490.0ms
─────────────────────────────────────────────────────────────────
```

## Example Configs

| File | Description |
|---|---|
| `examples/basic.yaml` | Single endpoint, shorthand ramp |
| `examples/advanced.yaml` | Multi-endpoint with templating and stages |
| `examples/step-ramp.yaml` | Instant VU spawn using `ramp: step` |
| `examples/arrival-rate.yaml` | Fixed RPS dispatch mode |
| `examples/max-rps-cap.yaml` | VU pool with global token-bucket cap |

## Development

```bash
go test ./...                          # run all tests
go build -o perf-test ./cmd/perf-test  # build CLI binary
./perf-test validate examples/basic.yaml
go run ./web/cmd                       # start web UI on localhost:8080
```

## License

MIT — see [LICENSE](LICENSE).
