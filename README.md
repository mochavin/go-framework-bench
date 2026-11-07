# Go Framework Benchmark: net/http vs Chi vs Gin vs Echo vs Fiber

**Results page:** https://mochavin.github.io/go-framework-bench/

The same benchmark app as [fpm-vs-swoole-vs-franken](https://github.com/mochavin/fpm-vs-swoole-vs-franken), rewritten in Go and run on five HTTP frameworks. Same endpoints, same Postgres schema, same [k6](https://k6.io/) scenarios, all in Docker Compose.

## Latest Results (2026-11-07)

Run on the same 4 vCPU Azure VM as the PHP benchmark, with the same k6 scenarios ([test machine](#test-machine)). Raw k6 summaries and the HTML report are in [`benchmarks/2026-11-07_azure-d4as-v6/`](benchmarks/2026-11-07_azure-d4as-v6/).

### Overall

| Metric | stdlib | chi | gin | echo | fiber |
|---|---:|---:|---:|---:|---:|
| Total requests | 1,175,373 | 1,211,432 | 1,215,621 | 1,219,796 | **1,254,560** |
| Throughput (req/s) | 5,772 | 5,957 | 5,978 | 6,000 | **6,170** |
| Failed requests | 0.00% | 0.00% | 0.00% | 0.00% | 0.00% |
| Avg latency (ms) | 144.4 | 137.9 | 135.1 | 134.7 | **132.4** |
| p95 latency (ms) | 290.6 | 254.8 | 259.9 | **244.7** | 271.6 |

### Per scenario: avg / p95 latency (ms)

| Scenario | VUs | stdlib | chi | gin | echo | fiber |
|---|---:|---:|---:|---:|---:|---:|
| Health check (no DB) | 5000 | 161 / 346 | 156 / 284 | 149 / 289 | 151 / **284** | **135** / 309 |
| List 20 posts (DB read) | 1000 ramp | **7.9** / 41.6 | 8.8 / 45.7 | 8.2 / **38.5** | 8.2 / 41.7 | 8.4 / 44.6 |
| Single post (DB read) | 3000 | 114 / 238 | 104 / 215 | 104 / 210 | **100 / 200** | 106 / 227 |
| Create post (DB write) | 2000 | 16.1 / 76.9 | 13.6 / **54.7** | 14.3 / 59.5 | **13.1** / 56.5 | 23.8 / 82.5 |
| Heavy compute (CPU) | 2000 | 2,848 / 8,643 | 2,829 / **5,221** | **2,820** / 7,300 | 2,846 / 8,986 | 2,830 / 5,243 |

### Takeaways

- **The frameworks are within ~7% of each other.** Fiber has the highest throughput (6,170 req/s) and stdlib the lowest (5,772), but the gap is small compared to run-to-run noise on a shared 4 vCPU machine.
- **The load generator is the bottleneck.** k6 runs 5000 VUs on the same 4 vCPUs as the server and Postgres, so most of the ~150 ms health-check latency is k6 and the kernel, not the framework.
- **Go vs PHP on the same VM:** every Go framework served ~1.2 M requests with zero failures. The best PHP runtime (FrankenPHP) served 177 k at 771 req/s with 2.1 s average latency, so Go is roughly **8× the throughput** at **~15× lower latency**.
- **Heavy compute** is CPU-bound everywhere (~2.8 s average under 2000 VUs) and the p95 spread between frameworks is scheduling noise. Go still finishes fibonacci(30) far faster than PHP (~30 s average there).
- Fiber was slowest on DB writes (23.8 ms avg vs ~14 ms). It runs on fasthttp, which handles request contexts differently from net/http.

### Test machine

| | |
|---|---|
| Host | Azure `Standard_D4as_v6` |
| CPU | AMD EPYC 9V74, 4 vCPU (2 cores × 2 threads) |
| RAM | 15.6 GiB, no swap |
| OS | Ubuntu 24.04.5 LTS, kernel 7.0.0-1017-azure |
| Docker | Engine 29.1.3, Compose 2.40.3 |
| Go | 1.26 (static binaries, `CGO_ENABLED=0`, distroless image) |
| Versions | gin 1.12.0, chi 5.5.0, echo 4.16.3, fiber 2.52.16, pgx 5.12.0 |
| Database | PostgreSQL 16 (alpine), `max_connections=300` |
| Load generator | k6 v2.3.0, **on the same machine** as the servers |

## Frameworks

| Framework | Host port | Package | Notes |
|---|---|---|---|
| **stdlib** | 8001 | `net/http` | Go 1.22+ `ServeMux` patterns (`GET /api/posts/{id}`) |
| **chi** | 8002 | `github.com/go-chi/chi/v5` | net/http router + `go-chi/render` |
| **gin** | 8003 | `github.com/gin-gonic/gin` | release mode, `gin.New()` + Recovery, no logger |
| **echo** | 8004 | `github.com/labstack/echo/v4` | Recover middleware, no logger |
| **fiber** | 8005 | `github.com/gofiber/fiber/v2` | built on fasthttp, not net/http |

Everything except routing, body parsing and JSON rendering lives in [`internal/app`](internal/app/app.go): the pgx connection pool, SQL queries, validation and the CPU workload. Each `cmd/<framework>/main.go` only wires routes, so the comparison is of the HTTP layer.

All frameworks share one PostgreSQL 16 database. Each app uses a pgx pool of 50 connections (`DB_MAX_CONNS`), and Postgres runs with `max_connections=300`.

## Quick Start

Only Docker with Compose v2 is needed. Go runs inside containers.

```bash
git clone https://github.com/mochavin/go-framework-bench
cd go-framework-bench

docker compose up -d --build
curl http://localhost:8003/api/health
# {"runtime":"gin","status":"ok","timestamp":"..."}
```

Each app creates the `posts` table and seeds 100 posts on startup (an advisory lock makes sure only one of them seeds).

### Run the benchmark

```bash
bash run-bench.sh              # all frameworks, ~20 minutes
bash run-bench.sh gin fiber    # only some
```

The runner builds, starts, smoke-tests, benchmarks each framework in turn with a 10 s cooldown, then writes `benchmark-report.html`.

Manual run for one framework:

```bash
docker compose --profile bench run --rm \
  -e BASE_URL=http://app-gin:8000 -e RUNTIME=gin \
  k6 run /scripts/benchmark.js
```

Rebuild the report from `results/`:

```bash
docker run --rm -v "$PWD:/src" -w /src golang:1.26-alpine go run ./cmd/report
```

Flags: `-in <dir>` (k6 summaries), `-php <dir>` (PHP summaries to show as a reference), `-date`, `-out`. The published page is built with:

```bash
d=benchmarks/2026-11-07_azure-d4as-v6
go run ./cmd/report -in $d -php $d/php -date 2026-11-07 -out $d/benchmark-report.html
```

GitHub Pages serves the `result` branch, whose `index.html` is a copy of that report.

## Endpoints

Same as the Laravel version, with the same JSON shapes.

| Endpoint | Method | Description |
|---|---|---|
| `/api/health` | GET | `{"status":"ok","runtime":...,"timestamp":...}`, no DB |
| `/api/posts` | GET | 20 latest posts (`ORDER BY created_at DESC LIMIT 20`) |
| `/api/posts/{id}` | GET | One post, 404 if missing |
| `/api/posts` | POST | Create a post; Laravel's rules (`title` required, max 255; `body` required), 422 on error |
| `/api/heavy` | GET | Recursive fibonacci(30) + 100 md5 hashes + JSON round trip |

## k6 Scenarios

`k6/benchmark.js` is copied unchanged from the PHP project.

| Scenario | VUs | Duration | Start |
|---|---|---|---|
| `health_check` | 5000 constant | 30s | 0s |
| `read_posts` | 1000 → 500 → 1000 → 0 | 60s | 35s |
| `single_post` | 3000 constant | 30s | 100s |
| `write_posts` | 2000 constant | 30s | 135s |
| `heavy_compute` | 2000 constant | 30s | 170s |

Latency counts successful responses only (`http_req_duration{expected_response:true}`); failed connections report ~0 ms and would make a struggling server look fast.

## Fairness Notes

- Containers get `nofile` 65535, as in the PHP project.
- No request logging on any framework.
- k6 runs on the same machine as the servers and Postgres, so they all share the CPUs. Numbers are comparable to each other and to the PHP project on the same VM, not to a dedicated setup.
- `run-bench.sh` resets `posts` to the 100 seeded rows before each framework, so every framework starts from the same data. Without this, rows from earlier `write_posts` runs would make the list query (no index on `created_at`, same as the Laravel migration) slower for later frameworks.
- Frameworks run one at a time, but all containers stay up during every run.

## Project Structure

```
├── cmd/
│   ├── stdlib/ chi/ gin/ echo/ fiber/   # one main.go per framework
│   └── report/                          # builds the results page (page.html template)
├── internal/app/app.go                  # shared DB, validation, workload
├── k6/benchmark.js, smoke.js
├── benchmarks/                          # committed result summaries
├── Dockerfile                           # --build-arg FRAMEWORK=<name>
├── docker-compose.yml
└── run-bench.sh
```

## Cleanup

```bash
docker compose down -v            # containers + database volume
docker compose down -v --rmi all  # also images
```
