# perf-test

A production-quality open-source CLI performance testing tool for HTTP APIs.

## Features

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
go build -o perf-test ./cmd/perf-test  # build binary
./perf-test validate examples/basic.yaml
```

## License

MIT — see [LICENSE](LICENSE).
