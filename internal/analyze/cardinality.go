package analyze

import (
	"sort"

	"github.com/donchanee/metricops/internal/model"
)

// HotspotOptions tunes DetectHotspots. Zero values mean "use default".
type HotspotOptions struct {
	// PctThreshold flags a metric when its share of total active series is
	// strictly greater than this fraction (1.0 = 100%). Default 5.0.
	PctThreshold float64
	// NoiseFloor skips metrics whose ActiveSeries is below this count. This
	// avoids reporting hotspots in tiny deployments where one metric with
	// 10 series might trivially exceed 5% of 50 total. Default 100.
	NoiseFloor int
	// TopN caps the output. Zero or negative means unbounded. Default 0.
	TopN int
}

func (o HotspotOptions) resolve() HotspotOptions {
	r := o
	if r.PctThreshold <= 0 {
		r.PctThreshold = 5.0
	}
	if r.NoiseFloor <= 0 {
		r.NoiseFloor = 100
	}
	return r
}

// DetectHotspots returns metrics whose active-series count is disproportionately
// large, as a fraction of the model's total.
//
// Behavior:
//   - Skip metrics under NoiseFloor (avoid noise in small deployments).
//   - Flag if ActiveSeries / TotalActiveSeries * 100 > PctThreshold.
//   - Stable sort: Cardinality desc, then Metric name asc.
//   - Truncate to TopN if > 0.
//
// The Label field on each hotspot is left empty in MVP. Per-label contribution
// needs Metric.LabelCardinality populated, which is not provided by
// `promtool tsdb analyze` at the per-metric level.
func DetectHotspots(m *model.Model, opts HotspotOptions) ([]model.CardinalityHotspot, error) {
	if m == nil || m.TotalActiveSeries <= 0 {
		return nil, nil
	}
	opt := opts.resolve()

	total := float64(m.TotalActiveSeries)
	out := make([]model.CardinalityHotspot, 0)
	for _, metric := range m.Metrics {
		if metric.ActiveSeries < opt.NoiseFloor {
			continue
		}
		pct := float64(metric.ActiveSeries) / total * 100.0
		if pct <= opt.PctThreshold {
			continue
		}
		out = append(out, model.CardinalityHotspot{
			Metric:           metric.Name,
			Cardinality:      metric.ActiveSeries,
			PctOfTotalSeries: round2(pct),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Cardinality != out[j].Cardinality {
			return out[i].Cardinality > out[j].Cardinality
		}
		return out[i].Metric < out[j].Metric
	})

	if opt.TopN > 0 && len(out) > opt.TopN {
		out = out[:opt.TopN]
	}
	return out, nil
}

// round2 rounds to 2 decimals for readable percentage output.
func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}
