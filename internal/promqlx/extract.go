// Package promqlx wraps the Prometheus PromQL parser for metric-name
// extraction.
//
// The analyzer needs to know which metric names appear in an arbitrary
// PromQL expression. Regex extraction is unsafe — PromQL has label
// matchers, function calls, subqueries, binary ops, and modifiers that
// look like identifiers but are not metric names. We use the official
// parser and walk the AST instead.
package promqlx

import (
	"fmt"
	"os"
	"sort"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

// MetricNames returns the unique metric names referenced by expr, sorted.
//
// Supported patterns:
//
//	rate(foo_total[5m])                               -> ["foo_total"]
//	foo{job="x"}                                      -> ["foo"]
//	{__name__="foo", job="x"}                         -> ["foo"]
//	foo / bar                                         -> ["bar", "foo"]
//	sum by (x) (foo)                                  -> ["foo"]
//	rate(foo[5m:1m])                                  -> ["foo"]  (subquery)
//	foo @ start()                                     -> ["foo"]
//	foo offset 5m                                     -> ["foo"]
//	label_replace(foo, "y","z","w","x")               -> ["foo"]
//	histogram_quantile(0.99, rate(foo_bucket[5m]))    -> ["foo_bucket"]
//	absent(up)                                        -> ["up"]
//	foo + foo                                         -> ["foo"]  (deduped)
//
// Limitations:
//
//   - Regex / negation matchers on __name__ ({__name__=~"http.*"} and
//     friends) cannot be resolved to specific metric names. A warning is
//     emitted on stderr and the selector contributes nothing to the
//     returned slice. This is a known false-positive source for the
//     unused-metric detector: a metric referenced ONLY via a regex
//     __name__ matcher will appear unused.
//
// On parse failure, the parser error is wrapped and returned; no metric
// names are returned.
func MetricNames(expr string) ([]string, error) {
	parsed, err := parser.ParseExpr(expr)
	if err != nil {
		return nil, fmt.Errorf("parse promql: %w", err)
	}

	c := &collector{
		names: make(map[string]struct{}),
		expr:  expr,
	}
	// parser.Walk never returns an error unless the visitor does; our
	// visitor returns nil, so this call cannot fail.
	_ = parser.Walk(c, parsed, nil)

	out := make([]string, 0, len(c.names))
	for name := range c.names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

// collector implements parser.Visitor by accumulating unique metric names
// into the names set.
type collector struct {
	names map[string]struct{}
	expr  string // retained for warning messages
}

// Visit extracts metric names from VectorSelector nodes. For all other
// node types it just keeps walking.
func (c *collector) Visit(n parser.Node, _ []parser.Node) (parser.Visitor, error) {
	if n == nil {
		return c, nil
	}
	vs, ok := n.(*parser.VectorSelector)
	if !ok {
		return c, nil
	}

	// Common case: vs.Name is populated from the lexed token, e.g.
	// `http_requests_total{...}` or `http_requests_total`.
	if vs.Name != "" {
		c.names[vs.Name] = struct{}{}
		return c, nil
	}

	// Fallback: metric name supplied via explicit __name__ matcher, e.g.
	// `{__name__="http_requests_total", job="foo"}`. Only MatchEqual is
	// resolvable; regex and negation matchers cannot be mapped to a single
	// name and are flagged but otherwise ignored.
	for _, m := range vs.LabelMatchers {
		if m.Name != labels.MetricName {
			continue
		}
		switch m.Type {
		case labels.MatchEqual:
			c.names[m.Value] = struct{}{}
		case labels.MatchRegexp, labels.MatchNotRegexp, labels.MatchNotEqual:
			fmt.Fprintf(os.Stderr,
				"warning: promqlx: unresolvable __name__ matcher in %q; skipping\n", c.expr)
		}
	}
	return c, nil
}
