// Package builder assembles a model.Model from parsed inputs.
//
// Pipeline position: the builder sits between parsers (which produce raw
// []*Metric and []Reference) and analyzers (which operate over a Model).
// It is the only component that knows how to extract metric names from
// PromQL expressions, so it lives in its own package — model stays pure
// data types with no external dependencies.
package builder

import (
	"fmt"
	"os"

	"github.com/donchanee/metricops/internal/model"
	"github.com/donchanee/metricops/internal/promqlx"
)

// Options controls builder behavior.
type Options struct {
	// Strict promotes per-reference parse failures from stderr warnings
	// to a returned error. Defaults to false (tolerant).
	Strict bool
}

// Result captures build counters useful for summary output and diagnostics.
type Result struct {
	// ReferencesAttributed is the count of References that extracted at least
	// one metric name that matched a known metric in the TSDB pool.
	ReferencesAttributed int

	// ReferencesOrphaned is the count of References whose extracted metric
	// names did NOT match any TSDB metric. These are "dangling" usages —
	// dashboards/rules still referencing metrics that have been deleted.
	ReferencesOrphaned int

	// ReferencesUnparseable is the count of References whose Expr could not
	// be parsed by promqlx. Silently skipped in non-strict mode.
	ReferencesUnparseable int
}

// Build assembles a model.Model from parsed TSDB metrics and parsed References.
//
// For every Reference, the Expr is parsed to extract metric names. The
// Reference is then attached to each matching Metric in the pool. References
// naming metrics not in the TSDB pool (orphans) are counted but dropped —
// they cannot contribute to unused/cardinality analysis.
//
// Counting semantics (to keep summary output deterministic):
//   - A Reference may attribute to multiple metrics (e.g., `foo / bar`
//     attributes once each to foo and bar). ReferencesAttributed counts
//     the Reference once if any attribution succeeded.
//   - A Reference whose names are all orphans is counted once under
//     ReferencesOrphaned.
//   - A Reference that is unparseable is counted under
//     ReferencesUnparseable and not under the other two.
//
// Returned Model has Metrics keyed by name; iteration order is not stable
// without sorting.
func Build(metrics []*model.Metric, refs []model.Reference, opts Options) (*model.Model, Result, error) {
	byName := make(map[string]*model.Metric, len(metrics))
	total := 0
	for _, m := range metrics {
		byName[m.Name] = m
		total += m.ActiveSeries
	}

	var r Result
	for _, ref := range refs {
		names, err := promqlx.MetricNames(ref.Expr)
		if err != nil {
			r.ReferencesUnparseable++
			if opts.Strict {
				return nil, r, fmt.Errorf("unparseable expr at %s: %w", ref.Location, err)
			}
			fmt.Fprintf(os.Stderr, "warning: unparseable expr at %s: %v\n", ref.Location, err)
			continue
		}
		if len(names) == 0 {
			// Expr parsed but yielded no metric names (e.g., vector(5),
			// or regex __name__ matcher that promqlx chose not to resolve).
			// Not orphaned, just informational; do not count either way.
			continue
		}

		attributed := false
		for _, name := range names {
			if m, ok := byName[name]; ok {
				m.References = append(m.References, ref)
				attributed = true
			}
		}
		if attributed {
			r.ReferencesAttributed++
		} else {
			r.ReferencesOrphaned++
		}
	}

	m := &model.Model{
		Metrics:           byName,
		TotalActiveSeries: total,
	}
	return m, r, nil
}
