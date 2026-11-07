// Builds the benchmark results page from k6 JSON summaries. Standard library only.
//
//	go run ./cmd/report [-in results] [-php dir] [-out benchmark-report.html]
//
// For each framework it uses <in>/<framework>.json, or else the newest
// <in>/<framework>_*.json. If -php points at the PHP project's summaries
// (fpm.json, swoole.json, franken.json) they are shown as a reference.
package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed page.html
var pageHTML string

type runtime struct{ Key, Label, Detail, Color string }

var goRuntimes = []runtime{
	{"stdlib", "net/http", "Go standard library", "#00ADD8"},
	{"chi", "Chi", "go-chi/chi v5", "#8B5CF6"},
	{"gin", "Gin", "gin-gonic/gin", "#10B981"},
	{"echo", "Echo", "labstack/echo v4", "#F59E0B"},
	{"fiber", "Fiber", "gofiber/fiber v2 (fasthttp)", "#EF4444"},
}

var phpRuntimes = []runtime{
	{"fpm", "PHP-FPM", "Nginx + PHP-FPM 8.3", "#94A3B8"},
	{"swoole", "Swoole", "Laravel Octane + Swoole", "#64748B"},
	{"franken", "FrankenPHP", "Laravel Octane + FrankenPHP", "#475569"},
}

var scenarios = []struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	VUs   string `json:"vus"`
	Trend string `json:"-"`
}{
	{"health_check", "Health check", "5000", "health_duration"},
	{"read_posts", "List 20 posts", "1000 ramp", "list_posts_duration"},
	{"single_post", "Single post", "3000", "single_post_duration"},
	{"write_posts", "Create post", "2000", "create_post_duration"},
	{"heavy_compute", "Heavy compute", "2000", "heavy_duration"},
}

type summary struct {
	Metrics map[string]struct {
		Values map[string]float64 `json:"values"`
	} `json:"metrics"`
}

func (s summary) val(metric, key string) float64 {
	if m, ok := s.Metrics[metric]; ok {
		return m.Values[key]
	}
	return 0
}

// okLatency uses successful responses only; failed connections report ~0 ms.
func (s summary) okLatency(key string) float64 {
	if m, ok := s.Metrics["http_req_duration{expected_response:true}"]; ok {
		return m.Values[key]
	}
	return s.val("http_req_duration", key)
}

type row struct {
	Key     string    `json:"key"`
	Label   string    `json:"label"`
	Detail  string    `json:"detail"`
	Color   string    `json:"color"`
	Source  string    `json:"source"`
	Reqs    float64   `json:"reqs"`
	RPS     float64   `json:"rps"`
	Failed  float64   `json:"failed"`
	Avg     float64   `json:"avg"`
	P95     float64   `json:"p95"`
	ScenAvg []float64 `json:"scenAvg"`
	ScenP95 []float64 `json:"scenP95"`
}

func findSummary(dir, key string) string {
	if p := filepath.Join(dir, key+".json"); fileExists(p) {
		return p
	}
	files, _ := filepath.Glob(filepath.Join(dir, key+"_*.json"))
	if len(files) == 0 {
		return ""
	}
	sort.Strings(files) // ISO timestamps sort chronologically
	return files[len(files)-1]
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func load(dir string, rts []runtime) []row {
	var rows []row
	for _, rt := range rts {
		path := findSummary(dir, rt.Key)
		if path == "" {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			log.Fatal(err)
		}
		var s summary
		if err := json.Unmarshal(raw, &s); err != nil {
			log.Fatalf("%s: %v", path, err)
		}
		r := row{
			Key: rt.Key, Label: rt.Label, Detail: rt.Detail, Color: rt.Color, Source: filepath.Base(path),
			Reqs: s.val("http_reqs", "count"), RPS: s.val("http_reqs", "rate"),
			Failed: s.val("http_req_failed", "rate") * 100,
			Avg:    s.okLatency("avg"), P95: s.okLatency("p(95)"),
		}
		for _, sc := range scenarios {
			r.ScenAvg = append(r.ScenAvg, s.val(sc.Trend, "avg"))
			r.ScenP95 = append(r.ScenP95, s.val(sc.Trend, "p(95)"))
		}
		rows = append(rows, r)
	}
	return rows
}

func main() {
	in := flag.String("in", "results", "directory with the Go k6 summaries")
	php := flag.String("php", "", "optional directory with PHP k6 summaries")
	out := flag.String("out", "benchmark-report.html", "output file")
	date := flag.String("date", time.Now().UTC().Format("2006-01-02"), "run date shown on the page")
	flag.Parse()

	goRows := load(*in, goRuntimes)
	if len(goRows) == 0 {
		log.Fatalf("no k6 summaries found in %s", *in)
	}
	var phpRows []row
	if *php != "" {
		phpRows = load(*php, phpRuntimes)
	}

	data, _ := json.Marshal(map[string]any{
		"go": goRows, "php": phpRows, "scenarios": scenarios, "date": *date,
	})
	// json.Marshal escapes <, > and &, so the data cannot close the script tag.
	page := strings.Replace(pageHTML, "/*DATA*/null", string(data), 1)
	if err := os.WriteFile(*out, []byte(page), 0o644); err != nil {
		log.Fatal(err)
	}

	var names []string
	for _, r := range append(goRows, phpRows...) {
		names = append(names, r.Key)
	}
	fmt.Printf("wrote %s (%s)\n", *out, strings.Join(names, ", "))
}
