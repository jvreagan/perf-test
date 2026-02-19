# perf-test

A production-quality open-source CLI performance testing tool for HTTP APIs.

## Features

- **Config-file driven** — YAML-based test configuration
- **Stage-based load profiles** — Linear ramp up/down with arbitrary stages
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

# Validate a config file
perf-test validate examples/advanced.yaml

# Show version
perf-test version
```

## Config Format

```yaml
name: "My API Test"
description: "Load test my endpoints"

load:
  # Option A: Stage-based (explicit phases)
  stages:
    - duration: 30s
      target: 10      # linear ramp from 0 to 10 VUs
    - duration: 60s
      target: 100     # hold at 100 VUs
    - duration: 30s
      target: 0       # ramp down

  # Option B: Simple shorthand
  ramp_up: 30s
  steady_state: 60s
  ramp_down: 30s
  max_vus: 100

  think_time: 100ms   # delay between requests per VU
  max_rps: 500        # global rate cap (optional)

http:
  timeout: 30s
  follow_redirects: true
  insecure_skip_verify: false

variables:
  base_url: "https://api.example.com"
  token: "${TOKEN}"   # env var expansion

endpoints:
  - name: "List Users"
    method: GET
    url: "${base_url}/users"
    headers:
      Authorization: "Bearer ${token}"
    weight: 3
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
  format: console    # console | json | csv
  interval: 5s
  file: results.json
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

Environment variables are expanded at parse time: `${MY_ENV_VAR}` → `$MY_ENV_VAR`.

## Output Example

```
[ 00:00:30 ] VUs: 50  RPS: 142.3  Reqs: 4264  Errors: 2 (0.0%)
─────────────────────────────────────────────────────────────────
Endpoint                        Reqs       p50       p90       p99
─────────────────────────────────────────────────────────────────
GET /users                      3198    45.0ms   120.0ms   310.0ms
POST /users                     1066    82.0ms   180.0ms   490.0ms
─────────────────────────────────────────────────────────────────
```

## Development

```bash
# Run tests
go test ./...

# Build binary
go build -o perf-test ./cmd/perf-test

# Run against examples (requires a local server)
./perf-test run examples/basic.yaml
```

## License

MIT — see [LICENSE](LICENSE).
