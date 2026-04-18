// Package analyze applies detection rules to a model.Model. Analyzers are
// pure functions over the in-memory model; they never do I/O. This makes
// them fast and deterministic to test.
package analyze

import (
	"sort"

	"github.com/donchanee/metricops/internal/model"
)

// DetectUnused returns the metrics in m that are present in TSDB but have
// zero references from any dashboard, alert, or recording rule.
//
// Detection rule (locked by eng review):
//
//   - A metric is "used" if ANY dashboard panel target, alert rule, or
//     recording rule references it by name (Reference.Source × Expr).
//   - Recording-rule chains are NOT traversed in MVP (documented
//     limitation in promqlx + builder — a metric referenced only via
//     another recording rule output will appear used; a metric referenced
//     only through that chain's output will not).
//
// Sorting (for deterministic JSON output):
//
//   - Primary:   ActiveSeries descending (bigger cost first).
//   - Secondary: Name ascending (stable tiebreak).
//
// BytesPerDayEstimate is computed via EstimateBytesPerDay using the
// bytesPerSample argument. Callers typically pass the CLI's
// --bytes-per-sample value or DefaultBytesPerSample.
func DetectUnused(m *model.Model, bytesPerSample float64) ([]model.UnusedMetric, error) {
	if m == nil {
		return nil, nil
	}

	out := make([]model.UnusedMetric, 0)
	for _, metric := range m.Metrics {
		if metric.IsUsed() {
			continue
		}
		out = append(out, model.UnusedMetric{
			Name:                metric.Name,
			ActiveSeries:        metric.ActiveSeries,
			BytesPerDayEstimate: EstimateBytesPerDay(metric.ActiveSeries, bytesPerSample),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].ActiveSeries != out[j].ActiveSeries {
			return out[i].ActiveSeries > out[j].ActiveSeries
		}
		return out[i].Name < out[j].Name
	})

	return out, nil
}
