# Go Framework Benchmark: net/http vs Chi vs Gin vs Echo vs Fiber

The same benchmark app as [fpm-vs-swoole-vs-franken](https://github.com/mochavin/fpm-vs-swoole-vs-franken), rewritten in Go and run on five HTTP frameworks. Same endpoints, same Postgres schema, same [k6](https://k6.io/) scenarios, all in Docker Compose.

<!-- RESULTS -->

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
│   └── report/                          # builds benchmark-report.html
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
