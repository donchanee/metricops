package analyze

import (
	"github.com/donchanee/metricops/internal/model"
)

// HotspotOptions tunes DetectHotspots. Zero values mean defaults:
//
//	PctThreshold:  5.0   (flag metrics producing > 5% of total series)
//	NoiseFloor:    100   (ignore metrics with fewer active series than this)
//	TopN:          0 = unbounded
type HotspotOptions struct {
	PctThreshold float64
	NoiseFloor   int
	TopN         int
}

// DetectHotspots returns metrics whose active-series count is disproportionately
// large, as a fraction of the model's total active series.
//
// Behavior (eng review):
//   - Metrics under NoiseFloor are skipped (avoid noise in small deployments)
//   - A metric is flagged if ActiveSeries / TotalActiveSeries > PctThreshold
//   - Stable sort: by Cardinality descending, then Metric name ascending
//   - If multiple metrics tie on cardinality, tie-break deterministically
//
// The Label field in CardinalityHotspot is left empty in MVP. In a future
// iteration the analyzer can break down per-label contributions using
// Metric.LabelCardinality.
func DetectHotspots(m *model.Model, opts HotspotOptions) ([]model.CardinalityHotspot, error) {
	return nil, ErrNotImplemented
}
