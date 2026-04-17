// Package promqlx wraps Prometheus's PromQL parser for metric-name extraction.
//
// The analyzer needs to know which metric names appear in an arbitrary PromQL
// expression. Regex extraction is unsafe because PromQL has label matchers,
// function calls, subqueries, and binary ops that look like identifiers but
// aren't metric names. We use the official parser and walk the AST.
package promqlx

import (
	"errors"
)

// ErrNotImplemented is returned by the stub. Remove once week 2 impl lands.
var ErrNotImplemented = errors.New("promqlx.MetricNames not implemented (week 2 TODO)")

// MetricNames returns the unique metric names referenced by a PromQL
// expression, deduped and sorted.
//
// Supported cases (from eng review test plan):
//   - Simple refs:           rate(http_total[5m])              -> ["http_total"]
//   - label_replace args:    label_replace(foo, "x","y","z","w")-> ["foo"]
//   - label_join args:       similar
//   - Subqueries:            rate(foo[5m:1m])                  -> ["foo"]
//   - @ modifier:            foo @ start()                     -> ["foo"]
//   - Binary ops:            foo / bar                         -> ["bar","foo"]
//   - Aggregations:          sum by (x) (foo)                  -> ["foo"]
//
// Behavior on unparseable input: returns empty slice + error. Caller is
// responsible for logging and deciding whether to fail the whole run.
//
// Implementation: use github.com/prometheus/prometheus/promql/parser.ParseExpr
// and parser.Walk with a visitor that appends VectorSelector.Name values.
func MetricNames(expr string) ([]string, error) {
	return nil, ErrNotImplemented
}
