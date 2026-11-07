// Builds benchmark-report.html from the newest results/<framework>_*.json k6
// summary of each framework. Standard library only.
//
//	go run ./cmd/report [-in results] [-out benchmark-report.html]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var frameworks = []string{"stdlib", "chi", "gin", "echo", "fiber"}

var colors = map[string]string{
	"stdlib": "#00ADD8", "chi": "#8B5CF6", "gin": "#10B981", "echo": "#F59E0B", "fiber": "#EF4444",
}

var scenarios = []struct{ Key, Label, Trend string }{
	{"health_check", "Health check (no DB)", "health_duration"},
	{"read_posts", "List 20 posts (DB read)", "list_posts_duration"},
	{"single_post", "Single post (DB read)", "single_post_duration"},
	{"write_posts", "Create post (DB write)", "create_post_duration"},
	{"heavy_compute", "Heavy compute (CPU)", "heavy_duration"},
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
	Name, Color, Source         string
	Reqs, RPS, Failed, Avg, P95 float64
	ScenAvg, ScenP95            []float64
}

func main() {
	in := flag.String("in", "results", "directory with k6 JSON summaries")
	out := flag.String("out", "benchmark-report.html", "output file")
	flag.Parse()

	var rows []row
	for _, fw := range frameworks {
		files, _ := filepath.Glob(filepath.Join(*in, fw+"_*.json"))
		if _, err := os.Stat(filepath.Join(*in, fw+".json")); err == nil {
			files = append(files, filepath.Join(*in, fw+".json")) // benchmarks/ naming
		}
		if len(files) == 0 {
			continue
		}
		sort.Strings(files) // ISO timestamps sort chronologically
		path := files[len(files)-1]
		raw, err := os.ReadFile(path)
		if err != nil {
			log.Fatal(err)
		}
		var s summary
		if err := json.Unmarshal(raw, &s); err != nil {
			log.Fatalf("%s: %v", path, err)
		}
		r := row{
			Name: fw, Color: colors[fw], Source: filepath.Base(path),
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
	if len(rows) == 0 {
		log.Fatalf("no k6 summaries found in %s", *in)
	}

	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	labels := []string{}
	for _, sc := range scenarios {
		labels = append(labels, sc.Label)
	}
	chartJSON, _ := json.Marshal(map[string]any{"labels": labels, "rows": rows})

	err = page.Execute(f, map[string]any{
		"Rows": rows, "Scenarios": scenarios, "Generated": time.Now().UTC().Format("2006-01-02 15:04 UTC"),
		"Chart": template.JS(chartJSON),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s (%s)\n", *out, strings.Join(names(rows), ", "))
}

func names(rows []row) []string {
	var n []string
	for _, r := range rows {
		n = append(n, r.Name)
	}
	return n
}

var page = template.Must(template.New("report").Funcs(template.FuncMap{
	"n0":  func(v float64) string { return commas(fmt.Sprintf("%.0f", v)) },
	"n2":  func(v float64) string { return fmt.Sprintf("%.2f", v) },
	"idx": func(s []float64, i int) float64 { return s[i] },
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Go Framework Benchmark</title>
<script src="https://cdnjs.cloudflare.com/ajax/libs/Chart.js/4.4.1/chart.umd.min.js"></script>
<style>
:root{--bg:#f8fafc;--card:#fff;--fg:#0f172a;--muted:#64748b;--line:#e2e8f0}
@media (prefers-color-scheme:dark){:root{--bg:#0b1120;--card:#111827;--fg:#e5e7eb;--muted:#94a3b8;--line:#1f2937}}
body{margin:0;background:var(--bg);color:var(--fg);font:15px/1.5 system-ui,sans-serif}
main{max-width:1100px;margin:auto;padding:24px 16px}
h1{margin:0 0 4px}.muted{color:var(--muted)}
.card{background:var(--card);border:1px solid var(--line);border-radius:12px;padding:16px;margin:16px 0;overflow-x:auto}
table{border-collapse:collapse;width:100%}th,td{padding:8px 10px;border-bottom:1px solid var(--line);text-align:right;white-space:nowrap}
th:first-child,td:first-child{text-align:left}.dot{display:inline-block;width:10px;height:10px;border-radius:50%;margin-right:6px}
</style></head><body><main>
<h1>Go Framework Benchmark</h1>
<p class="muted">net/http vs Chi vs Gin vs Echo vs Fiber · same app, same Postgres, same k6 scenarios · generated {{.Generated}}</p>

<div class="card"><h2>Overall</h2><table>
<tr><th>Framework</th><th>Requests</th><th>Req/s</th><th>Failed</th><th>Avg (ms)</th><th>p95 (ms)</th></tr>
{{range .Rows}}<tr><td><span class="dot" style="background:{{.Color}}"></span>{{.Name}}</td>
<td>{{n0 .Reqs}}</td><td>{{n0 .RPS}}</td><td>{{n2 .Failed}}%</td><td>{{n2 .Avg}}</td><td>{{n2 .P95}}</td></tr>{{end}}
</table><p class="muted">Latency counts successful responses only.</p></div>

<div class="card"><h2>Per scenario: avg / p95 (ms)</h2><table>
<tr><th>Scenario</th>{{range .Rows}}<th>{{.Name}}</th>{{end}}</tr>
{{range $i, $s := .Scenarios}}<tr><td>{{$s.Label}}</td>{{range $.Rows}}<td>{{n2 (idx .ScenAvg $i)}} / {{n2 (idx .ScenP95 $i)}}</td>{{end}}</tr>{{end}}
</table></div>

<div class="card"><h2>Throughput (req/s)</h2><canvas id="rps" height="90"></canvas></div>
<div class="card"><h2>p95 latency per scenario (ms)</h2><canvas id="p95" height="140"></canvas></div>

<p class="muted">Sources: {{range .Rows}}{{.Source}} {{end}}</p>
</main><script>
const d = {{.Chart}};
const grid = getComputedStyle(document.documentElement).getPropertyValue('--line');
const opts = {plugins:{legend:{display:false}},scales:{x:{grid:{color:grid}},y:{grid:{color:grid},beginAtZero:true}}};
new Chart(rps,{type:'bar',data:{labels:d.rows.map(r=>r.Name),datasets:[{data:d.rows.map(r=>r.RPS),backgroundColor:d.rows.map(r=>r.Color)}]},options:opts});
new Chart(p95,{type:'bar',data:{labels:d.labels,datasets:d.rows.map(r=>({label:r.Name,data:r.ScenP95,backgroundColor:r.Color}))},
  options:{...opts,plugins:{legend:{display:true}}}});
</script></body></html>`))

func commas(s string) string {
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		return "-" + s
	}
	return s
}
