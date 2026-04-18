package analyze

import (
	"sort"

	"github.com/donchanee/metricops/internal/model"
)

// BuildRecommendations derives actionable recommendations from the detector
// outputs. In MVP this is:
//
//	drop_metric      — one per UnusedMetric with nonzero cost
//
// Cardinality-hotspot "reduce_label" recommendations are deferred to v2,
// since a confident savings estimate requires per-label cardinality data
// that promtool tsdb analyze does not supply at the per-metric level.
//
// Output is sorted by EstimatedSavingsBytesPerDay desc, then Target asc.
func BuildRecommendations(unused []model.UnusedMetric, hotspots []model.CardinalityHotspot) ([]model.Recommendation, error) {
	_ = hotspots // reserved for v2 reduce_label recommendations

	out := make([]model.Recommendation, 0, len(unused))
	for _, u := range unused {
		if u.BytesPerDayEstimate <= 0 {
			// Skip zero-cost drops: technically still a cleanup but not
			// worth surfacing in a ranked savings list.
			continue
		}
		out = append(out, model.Recommendation{
			Type:                        "drop_metric",
			Target:                      u.Name,
			EstimatedSavingsBytesPerDay: u.BytesPerDayEstimate,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].EstimatedSavingsBytesPerDay != out[j].EstimatedSavingsBytesPerDay {
			return out[i].EstimatedSavingsBytesPerDay > out[j].EstimatedSavingsBytesPerDay
		}
		return out[i].Target < out[j].Target
	})

	return out, nil
}
