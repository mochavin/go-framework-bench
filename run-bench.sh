#!/bin/bash
# Benchmarks every Go framework in turn with k6, then builds benchmark-report.html.
# Usage: bash run-bench.sh [framework ...]   (default: all)
set -e
cd "$(dirname "$0")"

ALL=(stdlib chi gin echo fiber)
FRAMEWORKS=("${@:-${ALL[@]}}")
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
RESULTS_DIR="./results/${TIMESTAMP}"

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; CYAN='\033[0;36m'; NC='\033[0m'

port_of() { for i in "${!ALL[@]}"; do [ "${ALL[$i]}" = "$1" ] && echo $((8001 + i)); done; }

mkdir -p "$RESULTS_DIR"
chmod -R a+rwX ./results  # k6 container runs as non-root and must write here

echo -e "${BLUE}[1/4] Building images...${NC}"
docker compose build --parallel

echo -e "${BLUE}[2/4] Starting services...${NC}"
docker compose up -d db $(printf 'app-%s ' "${ALL[@]}")

echo -e "${BLUE}[3/4] Waiting for services + smoke tests...${NC}"
for fw in "${FRAMEWORKS[@]}"; do
    port=$(port_of "$fw")
    for _ in $(seq 60); do
        curl -sf "http://localhost:${port}/api/health" >/dev/null && break
        sleep 1
    done
    if docker compose --profile bench run --rm -e BASE_URL="http://app-${fw}:8000" -e K6_OUT= \
        k6 run --quiet /scripts/smoke.js >/dev/null 2>&1; then
        echo -e "  ${GREEN}✓ ${fw} (port ${port})${NC}"
    else
        echo -e "  ${RED}✗ ${fw} smoke test failed${NC}"; exit 1
    fi
done

echo -e "${BLUE}[4/4] Running benchmarks...${NC}"
for i in "${!FRAMEWORKS[@]}"; do
    fw="${FRAMEWORKS[$i]}"
    # Start every framework from the same 100 seeded posts. Otherwise rows
    # written by earlier runs make the list query slower for later frameworks.
    docker compose exec -T db psql -q -U bench -d go_bench \
        -c "DELETE FROM posts WHERE id > 100" -c "VACUUM ANALYZE posts" >/dev/null
    echo -e "${CYAN}━━━ Benchmarking: ${fw^^} (http://app-${fw}:8000) ━━━${NC}"
    docker compose --profile bench run --rm \
        -e BASE_URL="http://app-${fw}:8000" \
        -e RUNTIME="${fw}" \
        -e K6_OUT="${K6_RAW:+json=/results/${TIMESTAMP}/${fw}_raw.json}" \
        k6 run /scripts/benchmark.js 2>&1 | tee "${RESULTS_DIR}/${fw}_output.txt"
    if [ $i -lt $((${#FRAMEWORKS[@]} - 1)) ]; then
        echo -e "${YELLOW}  Cooling down for 10 seconds...${NC}"; sleep 10
    fi
done

echo -e "${YELLOW}[Report] Generating benchmark-report.html...${NC}"
docker run --rm -v "$PWD:/src" -w /src golang:1.26-alpine go run ./cmd/report
echo -e "${GREEN}Done.${NC} Results in ${RESULTS_DIR}/, report in benchmark-report.html"
