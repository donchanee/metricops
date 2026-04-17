package model

import "time"

// SchemaVersion is the semantic version of the Report JSON output.
//
// Stability policy:
//   - Adding fields is non-breaking (MINOR bump).
//   - Renaming or removing fields is breaking (MAJOR bump).
//   - Downstream integrators should pin to a major via --schema.
//
// See docs/schema-v1.0.md (to be written in week 4) for the full contract.
const SchemaVersion = "1.0"

// Report is the top-level output of `metricops analyze`.
//
// The JSON marshaling of this type IS the public v1.0 schema. Field order and
// names must not change across v1.x releases. All slice fields are rendered
// in sorted order (see the render package) to guarantee byte-identical output
// for the same input (a CI gate).
type Report struct {
	SchemaVersion       string               `json:"schema_version"`
	GeneratedAt         time.Time            `json:"generated_at"`
	Summary             Summary              `json:"summary"`
	UnusedMetrics       []UnusedMetric       `json:"unused_metrics"`
	CardinalityHotspots []CardinalityHotspot `json:"cardinality_hotspots"`
	Recommendations     []Recommendation     `json:"recommendations"`
}

// Summary is the at-a-glance totals section.
type Summary struct {
	TotalMetrics          int     `json:"total_metrics"`
	UnusedMetrics         int     `json:"unused_metrics"`
	TotalActiveSeries     int     `json:"total_active_series"`
	EstimatedDailyBytes   int64   `json:"estimated_daily_bytes"`
	EstimatedMonthlyBytes int64   `json:"estimated_monthly_bytes"`
	BytesPerSampleAssumed float64 `json:"bytes_per_sample_assumed"`
}

// UnusedMetric is one row in the "unused metrics" list.
type UnusedMetric struct {
	Name                string `json:"name"`
	ActiveSeries        int    `json:"active_series"`
	BytesPerDayEstimate int64  `json:"bytes_per_day_estimate"`
}

// CardinalityHotspot is one row in the "high cardinality" list. Label is
// omitted when the hotspot is about the whole metric, not a specific label.
type CardinalityHotspot struct {
	Metric           string  `json:"metric"`
	Label            string  `json:"label,omitempty"`
	Cardinality      int     `json:"cardinality"`
	PctOfTotalSeries float64 `json:"pct_of_total_series"`
}

// Recommendation is a suggested action derived from findings.
//
// Type is an open enum for forward compatibility:
//
//	drop_metric           — the metric is unused; drop it
//	reduce_label          — a label is exploding cardinality; reduce it
//	convert_to_histogram  — (v2) a counter with many label values could be
//	                        better served by a histogram aggregation
type Recommendation struct {
	Type                        string `json:"type"`
	Target                      string `json:"target"`
	EstimatedSavingsBytesPerDay int64  `json:"estimated_savings_bytes_per_day"`
}
