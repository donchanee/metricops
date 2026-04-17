// Package parse reads inputs from disk and returns model types. Each source
// (tsdb, grafana, rules) lives in its own file. Parsers never run analysis;
// they strictly transform text/JSON/YAML into model values.
package parse

import (
	"errors"
	"io"

	"github.com/donchanee/metricops/internal/model"
)

// ErrNotImplemented is returned by stub parsers until week 1 lands.
var ErrNotImplemented = errors.New("parser not implemented (week 1 TODO)")

// ParseTSDBAnalyze reads the output of `promtool tsdb analyze` and returns
// metrics with their cardinality counts.
//
// Format reference:
//
//	https://github.com/prometheus/prometheus/blob/main/cmd/promtool/tsdb.go
//
// Sections to extract:
//
//	Block ID: ...
//	Duration: ...
//	Total Series: N                 — populates total series count
//	Label names: N
//
//	Highest cardinality metric names:
//	  <count> <metric_name>
//	  ...                           — the primary data source
//
//	Highest cardinality labels:
//	  <count> <label>               — per-label cardinality context
//
// Implementation notes (eng review):
//   - Stream via bufio.Scanner; do not slurp into memory
//   - Be tolerant to version drift (v2.48, v2.51, v2.54). Unknown sections
//     should be skipped with a stderr warning, not an error
//   - Return best-effort partial results when a section is malformed
//   - Do NOT attempt to parse "Postings" sections; out of scope for MVP
func ParseTSDBAnalyze(r io.Reader) ([]*model.Metric, error) {
	return nil, ErrNotImplemented
}
