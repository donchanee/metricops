//go:build ignore

// Command generate produces synthetic Prometheus-like fixtures for testing metricops.
//
// Output layout under -out:
//
//	tsdb-analyze.txt       mimics `promtool tsdb analyze` output
//	dashboards/*.json      Grafana v10+ dashboard JSON files
//	rules/*.yml            Prometheus alert + recording rule files
//	MANIFEST.json          ground truth for assertion-based tests
//
// Usage:
//
//	go run testdata/fixtures/generate.go -out ./testdata/fixtures
//
// Determinism: the same -seed produces byte-identical output across runs and OSes.
//
// Ground truth: MANIFEST.json lists which metrics are intentionally unused and
// which are deliberate cardinality hotspots, so test assertions can verify the
// analyzer's output against known expected findings.
//
// The fixture is intentionally small enough to read by eye (80 metrics by default)
// but structurally realistic (power-law cardinality, multi-theme dashboards,
// mixed alert and recording rules, deprecated-metric sprawl).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ----------------------------- Config ---------------------------------------

// Config controls the shape of the generated fixture. Every knob is flag-driven
// so tests can build fixtures with specific characteristics (tiny, huge, all-unused,
// no-hotspots, and so on) without touching this file.
type Config struct {
	OutDir                   string  `json:"out_dir"`
	Seed                     int64   `json:"seed"`
	NumMetrics               int     `json:"num_metrics"`
	NumDashboards            int     `json:"num_dashboards"`
	PanelsPerDashboard       int     `json:"panels_per_dashboard"`
	NumRuleFiles             int     `json:"num_rule_files"`
	RulesPerFile             int     `json:"rules_per_file"`
	UnusedRatio              float64 `json:"unused_ratio"`
	CardinalityHotspotCount  int     `json:"cardinality_hotspot_count"`
	CardinalityHotspotSeries int     `json:"cardinality_hotspot_series"`
}

func parseFlags() Config {
	var cfg Config
	flag.StringVar(&cfg.OutDir, "out", "./testdata/fixtures", "output directory")
	flag.Int64Var(&cfg.Seed, "seed", 42, "RNG seed for deterministic output")
	flag.IntVar(&cfg.NumMetrics, "metrics", 80, "total unique metric names (must be <= pool size, 80)")
	flag.IntVar(&cfg.NumDashboards, "dashboards", 12, "number of Grafana dashboards")
	flag.IntVar(&cfg.PanelsPerDashboard, "panels", 8, "panels per dashboard")
	flag.IntVar(&cfg.NumRuleFiles, "rulefiles", 4, "number of rule YAML files")
	flag.IntVar(&cfg.RulesPerFile, "rules", 10, "rules per file")
	flag.Float64Var(&cfg.UnusedRatio, "unused", 0.25, "fraction of metrics with zero references")
	flag.IntVar(&cfg.CardinalityHotspotCount, "hotspots", 3, "number of high-cardinality metrics")
	flag.IntVar(&cfg.CardinalityHotspotSeries, "hotspot-series", 15000, "active series per hotspot metric")
	flag.Parse()
	return cfg
}

// ---------------------------- Metric pool -----------------------------------

type MetricType string

const (
	Counter   MetricType = "counter"
	Gauge     MetricType = "gauge"
	Histogram MetricType = "histogram"
	Summary   MetricType = "summary"
)

type Metric struct {
	Name        string
	Type        MetricType
	Help        string
	Labels      []string
	Cardinality int
	Unused      bool
	Theme       string
}

// Themes group metrics into realistic dashboard clusters.
const (
	themeHTTP    = "http"
	themeGoRT    = "go_runtime"
	themeProcess = "process"
	themeNode    = "node_exporter"
	themeGRPC    = "grpc"
	themeDB      = "database"
	themeQueue   = "queue"
	themeApp     = "app"
	themeLegacy  = "legacy"
)

// baseMetricPool is the hand-curated pool of 80 realistic metric definitions.
// Order is stable; callers must not re-sort. Each theme corresponds to a common
// observability surface:
//
//	http           Prometheus HTTP server middleware
//	go_runtime     Go runtime instrumentation (go_* family)
//	process        process_exporter
//	node_exporter  node_exporter standard metrics
//	grpc           go-grpc-prometheus or equivalent
//	database       generic DB client library metrics
//	queue          generic queue/broker instrumentation
//	app            custom application metrics (some high-cardinality)
//	legacy         deprecated metrics still being scraped (sprawl source)
func baseMetricPool() []Metric {
	return []Metric{
		// HTTP (10)
		{Name: "http_requests_total", Type: Counter, Help: "HTTP requests received.", Labels: []string{"job", "instance", "method", "status_code", "handler"}, Theme: themeHTTP},
		{Name: "http_request_duration_seconds_bucket", Type: Histogram, Help: "HTTP request latency distribution.", Labels: []string{"job", "instance", "method", "handler", "le"}, Theme: themeHTTP},
		{Name: "http_request_duration_seconds_count", Type: Counter, Help: "HTTP request duration observation count.", Labels: []string{"job", "instance", "method", "handler"}, Theme: themeHTTP},
		{Name: "http_request_duration_seconds_sum", Type: Counter, Help: "HTTP request duration observation sum.", Labels: []string{"job", "instance", "method", "handler"}, Theme: themeHTTP},
		{Name: "http_request_size_bytes", Type: Histogram, Help: "HTTP request size.", Labels: []string{"job", "instance", "method"}, Theme: themeHTTP},
		{Name: "http_response_size_bytes", Type: Histogram, Help: "HTTP response size.", Labels: []string{"job", "instance", "method", "status_code"}, Theme: themeHTTP},
		{Name: "http_errors_total", Type: Counter, Help: "HTTP errors by code.", Labels: []string{"job", "instance", "status_code"}, Theme: themeHTTP},
		{Name: "http_active_connections", Type: Gauge, Help: "HTTP connections currently open.", Labels: []string{"job", "instance"}, Theme: themeHTTP},
		{Name: "http_retries_total", Type: Counter, Help: "HTTP retry attempts.", Labels: []string{"job", "instance", "reason"}, Theme: themeHTTP},
		{Name: "http_upstream_errors_total", Type: Counter, Help: "Errors from upstream services.", Labels: []string{"job", "instance", "upstream"}, Theme: themeHTTP},

		// Go runtime (10)
		{Name: "go_goroutines", Type: Gauge, Help: "Current goroutine count.", Labels: []string{"job", "instance"}, Theme: themeGoRT},
		{Name: "go_memstats_alloc_bytes", Type: Gauge, Help: "Bytes allocated and in use.", Labels: []string{"job", "instance"}, Theme: themeGoRT},
		{Name: "go_memstats_heap_alloc_bytes", Type: Gauge, Help: "Heap bytes allocated and in use.", Labels: []string{"job", "instance"}, Theme: themeGoRT},
		{Name: "go_memstats_heap_inuse_bytes", Type: Gauge, Help: "Heap bytes in use.", Labels: []string{"job", "instance"}, Theme: themeGoRT},
		{Name: "go_gc_duration_seconds", Type: Summary, Help: "GC invocation duration summary.", Labels: []string{"job", "instance", "quantile"}, Theme: themeGoRT},
		{Name: "go_threads", Type: Gauge, Help: "OS threads used.", Labels: []string{"job", "instance"}, Theme: themeGoRT},
		{Name: "go_memstats_gc_sys_bytes", Type: Gauge, Help: "GC metadata size.", Labels: []string{"job", "instance"}, Theme: themeGoRT},
		{Name: "go_memstats_mallocs_total", Type: Counter, Help: "Total mallocs.", Labels: []string{"job", "instance"}, Theme: themeGoRT},
		{Name: "go_memstats_frees_total", Type: Counter, Help: "Total frees.", Labels: []string{"job", "instance"}, Theme: themeGoRT},
		{Name: "go_info", Type: Gauge, Help: "Go version info.", Labels: []string{"job", "instance", "version"}, Theme: themeGoRT},

		// Process (8)
		{Name: "process_cpu_seconds_total", Type: Counter, Help: "CPU seconds consumed.", Labels: []string{"job", "instance"}, Theme: themeProcess},
		{Name: "process_resident_memory_bytes", Type: Gauge, Help: "Resident memory.", Labels: []string{"job", "instance"}, Theme: themeProcess},
		{Name: "process_virtual_memory_bytes", Type: Gauge, Help: "Virtual memory.", Labels: []string{"job", "instance"}, Theme: themeProcess},
		{Name: "process_open_fds", Type: Gauge, Help: "Open file descriptors.", Labels: []string{"job", "instance"}, Theme: themeProcess},
		{Name: "process_max_fds", Type: Gauge, Help: "Max file descriptors allowed.", Labels: []string{"job", "instance"}, Theme: themeProcess},
		{Name: "process_start_time_seconds", Type: Gauge, Help: "Process start time (unix seconds).", Labels: []string{"job", "instance"}, Theme: themeProcess},
		{Name: "process_network_receive_bytes_total", Type: Counter, Help: "Bytes received by process.", Labels: []string{"job", "instance"}, Theme: themeProcess},
		{Name: "process_network_transmit_bytes_total", Type: Counter, Help: "Bytes transmitted by process.", Labels: []string{"job", "instance"}, Theme: themeProcess},

		// Node exporter (14)
		{Name: "node_cpu_seconds_total", Type: Counter, Help: "CPU seconds per mode.", Labels: []string{"instance", "cpu", "mode"}, Theme: themeNode},
		{Name: "node_memory_MemFree_bytes", Type: Gauge, Help: "Free memory.", Labels: []string{"instance"}, Theme: themeNode},
		{Name: "node_memory_MemTotal_bytes", Type: Gauge, Help: "Total memory.", Labels: []string{"instance"}, Theme: themeNode},
		{Name: "node_memory_MemAvailable_bytes", Type: Gauge, Help: "Available memory.", Labels: []string{"instance"}, Theme: themeNode},
		{Name: "node_filesystem_free_bytes", Type: Gauge, Help: "Free filesystem bytes.", Labels: []string{"instance", "device", "mountpoint", "fstype"}, Theme: themeNode},
		{Name: "node_filesystem_size_bytes", Type: Gauge, Help: "Total filesystem bytes.", Labels: []string{"instance", "device", "mountpoint", "fstype"}, Theme: themeNode},
		{Name: "node_filesystem_avail_bytes", Type: Gauge, Help: "Available filesystem bytes.", Labels: []string{"instance", "device", "mountpoint", "fstype"}, Theme: themeNode},
		{Name: "node_network_receive_bytes_total", Type: Counter, Help: "Network bytes received.", Labels: []string{"instance", "device"}, Theme: themeNode},
		{Name: "node_network_transmit_bytes_total", Type: Counter, Help: "Network bytes transmitted.", Labels: []string{"instance", "device"}, Theme: themeNode},
		{Name: "node_disk_read_bytes_total", Type: Counter, Help: "Disk bytes read.", Labels: []string{"instance", "device"}, Theme: themeNode},
		{Name: "node_disk_written_bytes_total", Type: Counter, Help: "Disk bytes written.", Labels: []string{"instance", "device"}, Theme: themeNode},
		{Name: "node_load1", Type: Gauge, Help: "1-minute load average.", Labels: []string{"instance"}, Theme: themeNode},
		{Name: "node_load5", Type: Gauge, Help: "5-minute load average.", Labels: []string{"instance"}, Theme: themeNode},
		{Name: "node_load15", Type: Gauge, Help: "15-minute load average.", Labels: []string{"instance"}, Theme: themeNode},

		// gRPC (6)
		{Name: "grpc_server_handled_total", Type: Counter, Help: "Total gRPC RPCs completed on the server.", Labels: []string{"job", "instance", "grpc_service", "grpc_method", "grpc_code"}, Theme: themeGRPC},
		{Name: "grpc_server_handling_seconds_bucket", Type: Histogram, Help: "gRPC server-side latency distribution.", Labels: []string{"job", "instance", "grpc_service", "grpc_method", "le"}, Theme: themeGRPC},
		{Name: "grpc_server_handling_seconds_count", Type: Counter, Help: "gRPC server-side latency count.", Labels: []string{"job", "instance", "grpc_service", "grpc_method"}, Theme: themeGRPC},
		{Name: "grpc_server_msg_received_total", Type: Counter, Help: "gRPC messages received.", Labels: []string{"job", "instance", "grpc_service", "grpc_method"}, Theme: themeGRPC},
		{Name: "grpc_server_msg_sent_total", Type: Counter, Help: "gRPC messages sent.", Labels: []string{"job", "instance", "grpc_service", "grpc_method"}, Theme: themeGRPC},
		{Name: "grpc_server_started_total", Type: Counter, Help: "gRPC RPCs started.", Labels: []string{"job", "instance", "grpc_service", "grpc_method"}, Theme: themeGRPC},

		// Database (8)
		{Name: "db_connections_active", Type: Gauge, Help: "Active database connections.", Labels: []string{"job", "instance", "database"}, Theme: themeDB},
		{Name: "db_connections_idle", Type: Gauge, Help: "Idle database connections.", Labels: []string{"job", "instance", "database"}, Theme: themeDB},
		{Name: "db_query_duration_seconds_bucket", Type: Histogram, Help: "Database query latency.", Labels: []string{"job", "instance", "database", "operation", "le"}, Theme: themeDB},
		{Name: "db_queries_total", Type: Counter, Help: "Total database queries.", Labels: []string{"job", "instance", "database", "operation"}, Theme: themeDB},
		{Name: "db_errors_total", Type: Counter, Help: "Database errors.", Labels: []string{"job", "instance", "database", "error_code"}, Theme: themeDB},
		{Name: "db_slow_queries_total", Type: Counter, Help: "Slow queries detected.", Labels: []string{"job", "instance", "database"}, Theme: themeDB},
		{Name: "db_pool_wait_seconds_bucket", Type: Histogram, Help: "Connection pool wait time.", Labels: []string{"job", "instance", "database", "le"}, Theme: themeDB},
		{Name: "db_transactions_total", Type: Counter, Help: "Total transactions.", Labels: []string{"job", "instance", "database", "outcome"}, Theme: themeDB},

		// Queue (5)
		{Name: "queue_depth", Type: Gauge, Help: "Current queue depth.", Labels: []string{"job", "instance", "queue"}, Theme: themeQueue},
		{Name: "queue_messages_published_total", Type: Counter, Help: "Total messages published.", Labels: []string{"job", "instance", "queue"}, Theme: themeQueue},
		{Name: "queue_messages_consumed_total", Type: Counter, Help: "Total messages consumed.", Labels: []string{"job", "instance", "queue"}, Theme: themeQueue},
		{Name: "queue_processing_duration_seconds_bucket", Type: Histogram, Help: "Message processing latency.", Labels: []string{"job", "instance", "queue", "le"}, Theme: themeQueue},
		{Name: "queue_dead_letter_total", Type: Counter, Help: "Dead-letter events.", Labels: []string{"job", "instance", "queue", "reason"}, Theme: themeQueue},

		// Custom app (10)
		{Name: "app_sessions_active", Type: Gauge, Help: "Active user sessions.", Labels: []string{"job", "instance", "region"}, Theme: themeApp},
		{Name: "app_user_actions_total", Type: Counter, Help: "User actions by type (high-card user_id).", Labels: []string{"job", "instance", "action_type", "user_id"}, Theme: themeApp},
		{Name: "app_feature_flag_evaluations_total", Type: Counter, Help: "Feature flag evaluations.", Labels: []string{"job", "instance", "flag", "variant"}, Theme: themeApp},
		{Name: "app_checkout_attempts_total", Type: Counter, Help: "Checkout attempts.", Labels: []string{"job", "instance", "region", "outcome"}, Theme: themeApp},
		{Name: "app_payment_latency_seconds_bucket", Type: Histogram, Help: "Payment processing latency.", Labels: []string{"job", "instance", "provider", "le"}, Theme: themeApp},
		{Name: "app_cache_hits_total", Type: Counter, Help: "Cache hits.", Labels: []string{"job", "instance", "cache"}, Theme: themeApp},
		{Name: "app_cache_misses_total", Type: Counter, Help: "Cache misses.", Labels: []string{"job", "instance", "cache"}, Theme: themeApp},
		{Name: "app_background_jobs_duration_seconds", Type: Histogram, Help: "Background job duration.", Labels: []string{"job", "instance", "job_type", "le"}, Theme: themeApp},
		{Name: "app_rate_limit_hits_total", Type: Counter, Help: "Rate limit hits (high-card user_id).", Labels: []string{"job", "instance", "endpoint", "user_id"}, Theme: themeApp},
		{Name: "app_websocket_connections_active", Type: Gauge, Help: "Active websocket connections.", Labels: []string{"job", "instance", "region"}, Theme: themeApp},

		// Legacy / deprecated (9) — high-probability unused candidates
		{Name: "api_v1_requests_total", Type: Counter, Help: "Deprecated: v1 API requests (migrate to v2).", Labels: []string{"job", "instance"}, Theme: themeLegacy},
		{Name: "api_v1_errors_total", Type: Counter, Help: "Deprecated: v1 API errors.", Labels: []string{"job", "instance"}, Theme: themeLegacy},
		{Name: "legacy_worker_queue_depth", Type: Gauge, Help: "Old worker queue (decommissioned).", Labels: []string{"job"}, Theme: themeLegacy},
		{Name: "abandoned_experiment_conversions", Type: Counter, Help: "Abandoned A/B test.", Labels: []string{"variant"}, Theme: themeLegacy},
		{Name: "beta_feature_opt_in_total", Type: Counter, Help: "Beta feature that was removed.", Labels: []string{"feature"}, Theme: themeLegacy},
		{Name: "prototype_latency_milliseconds", Type: Gauge, Help: "Prototype metric (intern project 2023).", Labels: []string{"endpoint"}, Theme: themeLegacy},
		{Name: "temp_debug_counter", Type: Counter, Help: "Temporary debug counter (never removed).", Labels: []string{"job"}, Theme: themeLegacy},
		{Name: "old_billing_reconciliation_errors", Type: Counter, Help: "Old billing system (migrated 2024).", Labels: []string{"job"}, Theme: themeLegacy},
		{Name: "deprecated_auth_flow_hits", Type: Counter, Help: "Auth flow replaced Q4 2024.", Labels: []string{"flow"}, Theme: themeLegacy},
	}
}

// ---------------------------- Distribution ----------------------------------

// assignCardinality writes a realistic active-series count to every metric.
//
// Distribution philosophy:
//   - base count scales with label set size (each label adds combinations)
//   - histograms get an 8x multiplier because le buckets explode series
//   - hotspot candidates (metrics with user_id or other high-card labels by
//     convention) are force-set to cfg.CardinalityHotspotSeries + jitter
//
// Real Prometheus cardinality follows a heavy-tailed distribution: most metrics
// have < 1k series while a handful dominate. We approximate that.
func assignCardinality(cfg Config, metrics []Metric, rng *rand.Rand) {
	for i := range metrics {
		m := &metrics[i]

		base := 20 + rng.Intn(200)
		if m.Type == Histogram {
			base *= 8
		}

		labelMult := 1
		for range m.Labels {
			labelMult *= 2
		}
		if labelMult > 32 {
			labelMult = 32
		}

		m.Cardinality = base * labelMult / 8
		if m.Cardinality < 1 {
			m.Cardinality = 1
		}
	}

	// Force-apply hotspot values to named candidates. Order matters for
	// determinism; the candidate list is fixed.
	candidates := []string{
		"app_user_actions_total",
		"app_rate_limit_hits_total",
		"http_request_duration_seconds_bucket",
	}
	forced := 0
	for _, name := range candidates {
		if forced >= cfg.CardinalityHotspotCount {
			break
		}
		for i := range metrics {
			if metrics[i].Name == name {
				metrics[i].Cardinality = cfg.CardinalityHotspotSeries + rng.Intn(3000)
				forced++
				break
			}
		}
	}
}

// assignReferences marks a target fraction of metrics as "unused" (no references
// in any dashboard, alert, or recording rule). Legacy-themed metrics are
// preferred candidates, since that mirrors production sprawl where deprecated
// metrics are the common source of dead weight.
func assignReferences(cfg Config, metrics []Metric, rng *rand.Rand) {
	target := int(float64(len(metrics)) * cfg.UnusedRatio)

	legacyIdx, otherIdx := []int{}, []int{}
	for i, m := range metrics {
		if m.Theme == themeLegacy {
			legacyIdx = append(legacyIdx, i)
		} else {
			otherIdx = append(otherIdx, i)
		}
	}
	rng.Shuffle(len(legacyIdx), func(i, j int) { legacyIdx[i], legacyIdx[j] = legacyIdx[j], legacyIdx[i] })
	rng.Shuffle(len(otherIdx), func(i, j int) { otherIdx[i], otherIdx[j] = otherIdx[j], otherIdx[i] })

	marked := 0
	for _, i := range legacyIdx {
		if marked >= target {
			break
		}
		metrics[i].Unused = true
		marked++
	}
	for _, i := range otherIdx {
		if marked >= target {
			break
		}
		metrics[i].Unused = true
		marked++
	}
}

// ---------------------------- Output: TSDB analyze --------------------------

// writeTSDBAnalyze produces a file in `promtool tsdb analyze` format. The
// parser in internal/parse/tsdb must be resilient to minor format variance
// across Prometheus versions, so this fixture matches the v2.51+ layout
// closely but does not pretend to be byte-identical to real promtool output.
func writeTSDBAnalyze(cfg Config, metrics []Metric, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	totalSeries := 0
	for _, m := range metrics {
		totalSeries += m.Cardinality
	}
	labelSet := map[string]bool{}
	for _, m := range metrics {
		for _, l := range m.Labels {
			labelSet[l] = true
		}
	}

	fmt.Fprintf(f, "Block ID: 01HW5KZSYNTHETICTESTFIXTUREAA\n")
	fmt.Fprintf(f, "Duration: 2h0m0s\n")
	fmt.Fprintf(f, "Total Series: %d\n", totalSeries)
	fmt.Fprintf(f, "Label names: %d\n", len(labelSet))
	fmt.Fprintf(f, "Postings (unique label pairs): %d\n", totalSeries/4)
	fmt.Fprintf(f, "Postings entries (total label pairs): %d\n", totalSeries*3)
	fmt.Fprintf(f, "\n")

	byCard := make([]Metric, len(metrics))
	copy(byCard, metrics)
	sort.Slice(byCard, func(i, j int) bool {
		if byCard[i].Cardinality != byCard[j].Cardinality {
			return byCard[i].Cardinality > byCard[j].Cardinality
		}
		return byCard[i].Name < byCard[j].Name
	})

	fmt.Fprintf(f, "Highest cardinality metric names:\n")
	for _, m := range byCard {
		fmt.Fprintf(f, "%d %s\n", m.Cardinality, m.Name)
	}
	fmt.Fprintf(f, "\n")

	labelCards := map[string]int{}
	for _, m := range metrics {
		if len(m.Labels) == 0 {
			continue
		}
		per := m.Cardinality / len(m.Labels)
		for _, l := range m.Labels {
			labelCards[l] += per
		}
	}
	type lc struct {
		Name  string
		Count int
	}
	lcs := make([]lc, 0, len(labelCards))
	for n, c := range labelCards {
		lcs = append(lcs, lc{n, c})
	}
	sort.Slice(lcs, func(i, j int) bool {
		if lcs[i].Count != lcs[j].Count {
			return lcs[i].Count > lcs[j].Count
		}
		return lcs[i].Name < lcs[j].Name
	})
	fmt.Fprintf(f, "Highest cardinality labels:\n")
	for _, x := range lcs {
		fmt.Fprintf(f, "%d %s\n", x.Count, x.Name)
	}

	return nil
}

// ---------------------------- Output: Grafana dashboards --------------------

type grafanaDashboard struct {
	ID            any            `json:"id"`
	UID           string         `json:"uid"`
	Title         string         `json:"title"`
	SchemaVersion int            `json:"schemaVersion"`
	Panels        []grafanaPanel `json:"panels"`
}

type grafanaPanel struct {
	ID         int             `json:"id"`
	Type       string          `json:"type"`
	Title      string          `json:"title"`
	Datasource grafanaDS       `json:"datasource"`
	Targets    []grafanaTarget `json:"targets"`
}

type grafanaTarget struct {
	RefID      string    `json:"refId"`
	Expr       string    `json:"expr"`
	Datasource grafanaDS `json:"datasource"`
}

type grafanaDS struct {
	Type string `json:"type"`
	UID  string `json:"uid"`
}

var panelTypes = []string{"timeseries", "stat", "table", "bargauge"}

// promQLTemplates is intentionally mixed: simple, aggregated, histogram, and
// binary-op expressions, to exercise the production PromQL metric-name
// extractor across common patterns. Fixtures do not validate semantic
// correctness (a histogram_quantile on a counter compiles fine, it just
// returns nothing at runtime), so it's OK to apply any template to any metric.
var promQLTemplates = []string{
	"rate({{m}}[5m])",
	"sum by (job) (rate({{m}}[5m]))",
	"sum by (instance) (rate({{m}}[5m]))",
	"histogram_quantile(0.99, sum by (le, job) (rate({{m}}[5m])))",
	"{{m}}",
	"avg({{m}})",
	"max by (instance) ({{m}})",
	"topk(10, {{m}})",
	"increase({{m}}[1h])",
	"irate({{m}}[1m])",
}

func pickExpression(metric string, rng *rand.Rand) string {
	t := promQLTemplates[rng.Intn(len(promQLTemplates))]
	return strings.ReplaceAll(t, "{{m}}", metric)
}

func writeDashboards(cfg Config, metrics []Metric, outdir string, rng *rand.Rand) ([]string, error) {
	if err := os.MkdirAll(outdir, 0755); err != nil {
		return nil, err
	}

	byTheme := map[string][]string{}
	for _, m := range metrics {
		if m.Unused {
			continue
		}
		byTheme[m.Theme] = append(byTheme[m.Theme], m.Name)
	}
	themes := make([]string, 0, len(byTheme))
	for t := range byTheme {
		themes = append(themes, t)
	}
	sort.Strings(themes)
	if len(themes) == 0 {
		return nil, fmt.Errorf("no themes have usable metrics; reduce -unused")
	}

	referenced := map[string]bool{}

	for i := 0; i < cfg.NumDashboards; i++ {
		theme := themes[i%len(themes)]
		dash := grafanaDashboard{
			ID:            nil,
			UID:           fmt.Sprintf("synthetic-%02d", i+1),
			Title:         fmt.Sprintf("%s Dashboard %02d", titleCase(theme), i+1),
			SchemaVersion: 39,
		}

		for p := 0; p < cfg.PanelsPerDashboard; p++ {
			panel := grafanaPanel{
				ID:         p + 1,
				Type:       panelTypes[rng.Intn(len(panelTypes))],
				Title:      fmt.Sprintf("Panel %d", p+1),
				Datasource: grafanaDS{Type: "prometheus", UID: "prom"},
			}
			numTargets := 1 + rng.Intn(3)
			for t := 0; t < numTargets; t++ {
				var metric string
				if rng.Float64() < 0.85 {
					metric = byTheme[theme][rng.Intn(len(byTheme[theme]))]
				} else {
					cross := themes[rng.Intn(len(themes))]
					metric = byTheme[cross][rng.Intn(len(byTheme[cross]))]
				}
				referenced[metric] = true
				panel.Targets = append(panel.Targets, grafanaTarget{
					RefID:      string(rune('A' + t)),
					Expr:       pickExpression(metric, rng),
					Datasource: grafanaDS{Type: "prometheus", UID: "prom"},
				})
			}
			dash.Panels = append(dash.Panels, panel)
		}

		data, err := json.MarshalIndent(dash, "", "  ")
		if err != nil {
			return nil, err
		}
		path := filepath.Join(outdir, fmt.Sprintf("%s-%02d.json", theme, i+1))
		if err := os.WriteFile(path, data, 0644); err != nil {
			return nil, err
		}
	}

	out := make([]string, 0, len(referenced))
	for m := range referenced {
		out = append(out, m)
	}
	sort.Strings(out)
	return out, nil
}

func titleCase(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// ---------------------------- Output: rule files ----------------------------

// writeRules emits a mix of alert and recording rules across NumRuleFiles
// files. Approximately 60% of rules are alerts, 40% recording. Each rule
// pulls from the pool of usable (non-unused) metrics. The generator does NOT
// make the recording-rule output names referenceable from dashboards (chain
// traversal is out of scope for MVP and the analyzer does not follow chains).
func writeRules(cfg Config, metrics []Metric, outdir string, rng *rand.Rand) (alertRefs, recRefs []string, err error) {
	if err := os.MkdirAll(outdir, 0755); err != nil {
		return nil, nil, err
	}

	usable := []string{}
	for _, m := range metrics {
		if !m.Unused {
			usable = append(usable, m.Name)
		}
	}
	sort.Strings(usable)
	if len(usable) == 0 {
		return nil, nil, fmt.Errorf("no usable metrics for rules; reduce -unused")
	}

	alertSet := map[string]bool{}
	recSet := map[string]bool{}

	for f := 0; f < cfg.NumRuleFiles; f++ {
		path := filepath.Join(outdir, fmt.Sprintf("rules-%02d.yml", f+1))
		file, cerr := os.Create(path)
		if cerr != nil {
			return nil, nil, cerr
		}

		fmt.Fprintf(file, "groups:\n")
		fmt.Fprintf(file, "  - name: group_%02d\n", f+1)
		fmt.Fprintf(file, "    interval: 30s\n")
		fmt.Fprintf(file, "    rules:\n")

		for r := 0; r < cfg.RulesPerFile; r++ {
			metric := usable[rng.Intn(len(usable))]
			if rng.Float64() < 0.6 {
				alertName := fmt.Sprintf("Alert_%02d_%02d", f+1, r+1)
				expr := fmt.Sprintf("rate(%s[5m]) > %0.2f", metric, rng.Float64()*10)
				fmt.Fprintf(file, "      - alert: %s\n", alertName)
				fmt.Fprintf(file, "        expr: %s\n", expr)
				fmt.Fprintf(file, "        for: 5m\n")
				fmt.Fprintf(file, "        labels:\n")
				fmt.Fprintf(file, "          severity: warning\n")
				fmt.Fprintf(file, "        annotations:\n")
				fmt.Fprintf(file, "          summary: %q\n", fmt.Sprintf("Synthetic alert %s", alertName))
				alertSet[metric] = true
			} else {
				recName := fmt.Sprintf("synthetic:rule_%02d_%02d:rate5m", f+1, r+1)
				expr := fmt.Sprintf("sum by (job) (rate(%s[5m]))", metric)
				fmt.Fprintf(file, "      - record: %s\n", recName)
				fmt.Fprintf(file, "        expr: %s\n", expr)
				recSet[metric] = true
			}
		}
		file.Close()
	}

	for m := range alertSet {
		alertRefs = append(alertRefs, m)
	}
	for m := range recSet {
		recRefs = append(recRefs, m)
	}
	sort.Strings(alertRefs)
	sort.Strings(recRefs)
	return alertRefs, recRefs, nil
}

// ---------------------------- Output: MANIFEST ------------------------------

type Manifest struct {
	Version     string      `json:"version"`
	Seed        int64       `json:"seed"`
	Config      Config      `json:"config"`
	GroundTruth GroundTruth `json:"ground_truth"`
}

type GroundTruth struct {
	TotalMetrics         int            `json:"total_metrics"`
	TotalActiveSeries    int            `json:"total_active_series"`
	UnusedMetrics        []string       `json:"unused_metrics"`
	CardinalityHotspots  []HotspotEntry `json:"cardinality_hotspots"`
	DashboardReferenced  []string       `json:"dashboard_referenced"`
	AlertReferenced      []string       `json:"alert_referenced"`
	RecordingReferenced  []string       `json:"recording_referenced"`
}

type HotspotEntry struct {
	Name        string `json:"name"`
	Cardinality int    `json:"cardinality"`
}

func writeManifest(cfg Config, metrics []Metric, dashRefs, alertRefs, recRefs []string, outdir string) error {
	gt := GroundTruth{TotalMetrics: len(metrics)}
	for _, m := range metrics {
		gt.TotalActiveSeries += m.Cardinality
		if m.Unused {
			gt.UnusedMetrics = append(gt.UnusedMetrics, m.Name)
		}
		if m.Cardinality >= cfg.CardinalityHotspotSeries {
			gt.CardinalityHotspots = append(gt.CardinalityHotspots, HotspotEntry{
				Name:        m.Name,
				Cardinality: m.Cardinality,
			})
		}
	}
	sort.Strings(gt.UnusedMetrics)
	sort.Slice(gt.CardinalityHotspots, func(i, j int) bool {
		if gt.CardinalityHotspots[i].Cardinality != gt.CardinalityHotspots[j].Cardinality {
			return gt.CardinalityHotspots[i].Cardinality > gt.CardinalityHotspots[j].Cardinality
		}
		return gt.CardinalityHotspots[i].Name < gt.CardinalityHotspots[j].Name
	})
	gt.DashboardReferenced = dashRefs
	gt.AlertReferenced = alertRefs
	gt.RecordingReferenced = recRefs

	m := Manifest{Version: "1.0", Seed: cfg.Seed, Config: cfg, GroundTruth: gt}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outdir, "MANIFEST.json"), data, 0644)
}

// ---------------------------- Main ------------------------------------------

func main() {
	cfg := parseFlags()

	pool := baseMetricPool()
	if cfg.NumMetrics > len(pool) {
		log.Fatalf("-metrics=%d exceeds built-in pool size %d", cfg.NumMetrics, len(pool))
	}
	if cfg.NumMetrics < 1 {
		log.Fatalf("-metrics must be >= 1")
	}
	metrics := pool[:cfg.NumMetrics]

	rng := rand.New(rand.NewSource(cfg.Seed))
	assignCardinality(cfg, metrics, rng)
	assignReferences(cfg, metrics, rng)

	if err := os.MkdirAll(cfg.OutDir, 0755); err != nil {
		log.Fatalf("mkdir %s: %v", cfg.OutDir, err)
	}

	if err := writeTSDBAnalyze(cfg, metrics, filepath.Join(cfg.OutDir, "tsdb-analyze.txt")); err != nil {
		log.Fatalf("tsdb-analyze.txt: %v", err)
	}

	dashRefs, err := writeDashboards(cfg, metrics, filepath.Join(cfg.OutDir, "dashboards"), rng)
	if err != nil {
		log.Fatalf("dashboards: %v", err)
	}

	alertRefs, recRefs, err := writeRules(cfg, metrics, filepath.Join(cfg.OutDir, "rules"), rng)
	if err != nil {
		log.Fatalf("rules: %v", err)
	}

	if err := writeManifest(cfg, metrics, dashRefs, alertRefs, recRefs, cfg.OutDir); err != nil {
		log.Fatalf("MANIFEST.json: %v", err)
	}

	totalSeries := 0
	unusedCount := 0
	for _, m := range metrics {
		totalSeries += m.Cardinality
		if m.Unused {
			unusedCount++
		}
	}

	log.Printf("wrote fixture to %s", cfg.OutDir)
	log.Printf("  metrics:            %d total, %d intentionally unused", len(metrics), unusedCount)
	log.Printf("  total active series: %d", totalSeries)
	log.Printf("  dashboards:         %d files in dashboards/", cfg.NumDashboards)
	log.Printf("  rule files:         %d files in rules/", cfg.NumRuleFiles)
	log.Printf("  dashboard refs:     %d unique metrics", len(dashRefs))
	log.Printf("  alert refs:         %d unique metrics", len(alertRefs))
	log.Printf("  recording refs:     %d unique metrics", len(recRefs))
	log.Printf("  ground truth:       MANIFEST.json")
}
