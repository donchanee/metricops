// Package analyze applies detection rules to a model.Model. Analyzers never
// do I/O; they are pure functions over the in-memory model, so tests are
// fast and deterministic.
package analyze

import (
	"errors"

	"github.com/donchanee/metricops/internal/model"
)

// ErrNotImplemented is returned by stubs until week 3 impl lands.
var ErrNotImplemented = errors.New("analyzer not implemented (week 3 TODO)")

// DetectUnused returns metrics that appear in TSDB but have zero references
// from any dashboard, alert, or recording rule.
//
// Detection rule (eng review):
//   - A metric is "used" if ANY expr in any dashboard panel, alert rule, or
//     recording rule references it by name
//   - Recording-rule chains are NOT traversed in MVP (documented limitation)
//   - Output is sorted by ActiveSeries descending, then by Name ascending,
//     for deterministic JSON output
//
// Returned slice is safe to place directly into Report.UnusedMetrics after
// computing BytesPerDayEstimate via the cost package.
func DetectUnused(m *model.Model) ([]model.UnusedMetric, error) {
	return nil, ErrNotImplemented
}
