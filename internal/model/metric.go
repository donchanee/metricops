// Package model defines the core types shared across parse, analyze, and
// render. Types here are the stable internal contract; changes affect every
// other package, so keep them minimal and explicit.
//
// The public JSON schema (v1.0) is defined separately in report.go. That
// schema is external; changing it breaks integrations. The types in this
// file are internal and can evolve freely.
package model

// Metric represents a unique metric name aggregated from TSDB output and all
// reference sources. There is one Metric per unique __name__ value.
//
// A Metric is distinct from a "series": a single metric name can produce
// thousands of series via label combinations. ActiveSeries captures the sum.
type Metric struct {
	// Name is the metric name exactly as it appears in TSDB metadata
	// (e.g., "http_requests_total").
	Name string

	// ActiveSeries is the total number of active series for this metric
	// across all label combinations, from `promtool tsdb analyze`.
	ActiveSeries int

	// LabelCardinality maps label name to the distinct-value count for this
	// metric. May be empty if the tsdb analyze output does not break down
	// per-metric label cardinality.
	LabelCardinality map[string]int

	// References is the list of places this metric is used. Empty references
	// means the metric is unused (zero dashboard, alert, or recording refs).
	References []Reference
}

// IsUsed reports whether the metric is referenced anywhere.
func (m *Metric) IsUsed() bool {
	return len(m.References) > 0
}

// UsedIn returns the distinct reference sources for this metric, sorted.
// Useful for "this metric is used in: dashboard, alert" summaries.
func (m *Metric) UsedIn() []RefSource {
	if len(m.References) == 0 {
		return nil
	}
	seen := make(map[RefSource]struct{}, 3)
	for _, r := range m.References {
		seen[r.Source] = struct{}{}
	}
	// Stable output order: dashboard, alert, recording.
	out := make([]RefSource, 0, len(seen))
	for _, s := range []RefSource{RefDashboard, RefAlert, RefRecording} {
		if _, ok := seen[s]; ok {
			out = append(out, s)
		}
	}
	return out
}

// Model is the complete analyzed state: every metric the TSDB knows about,
// with its references joined in. One Model per `metricops analyze` run.
type Model struct {
	// Metrics is keyed by metric name for O(1) reference joining. Callers
	// that need stable iteration order must sort the keys.
	Metrics map[string]*Metric

	// TotalActiveSeries is the sum of Metrics[*].ActiveSeries. Cached here
	// to avoid repeated recomputation in analyzers.
	TotalActiveSeries int
}
