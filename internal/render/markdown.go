// Package render turns a model.Report into human or machine-readable output.
//
// Renderers are pure functions: given a Report, they produce bytes. They must
// never allocate non-deterministic state; all ordering comes from the caller.
package render

import (
	"errors"
	"io"

	"github.com/donchanee/metricops/internal/model"
)

// ErrNotImplemented is returned by the stub. Week 3 deletes it.
var ErrNotImplemented = errors.New("renderer not implemented (week 3 TODO)")

// Markdown writes a human-readable report to w.
//
// Output sections (locked by eng review):
//
//	# metricops Report
//	Generated: <ISO8601>
//
//	## Summary
//	- Total metrics: N
//	- Unused metrics: N
//	- Total active series: N
//	- Estimated daily bytes: N (Xmo: N)
//
//	## Unused Metrics
//	| Metric | Active Series | Bytes/day |
//
//	## Cardinality Hotspots
//	| Metric | Cardinality | % of Total |
//
//	## Recommendations
//	- drop_metric `foo` — saves ~N bytes/day
//
// Table ordering comes from the already-sorted slices in Report. Markdown
// tables use pipe syntax, compatible with GitHub Flavored Markdown.
func Markdown(w io.Writer, r *model.Report) error {
	return ErrNotImplemented
}
