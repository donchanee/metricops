package promqlx

import (
	"reflect"
	"strings"
	"testing"
)

func TestMetricNames(t *testing.T) {
	cases := []struct {
		name string
		expr string
		want []string
	}{
		// Simple forms.
		{"plain selector", `foo`, []string{"foo"}},
		{"with label matcher", `foo{job="x"}`, []string{"foo"}},
		{"selector with regex label", `foo{status=~"5.."}`, []string{"foo"}},

		// Range / rate / subqueries.
		{"rate", `rate(foo_total[5m])`, []string{"foo_total"}},
		{"irate", `irate(foo_total[1m])`, []string{"foo_total"}},
		{"increase", `increase(foo_total[1h])`, []string{"foo_total"}},
		{"subquery", `rate(foo[5m:1m])`, []string{"foo"}},

		// Modifiers.
		{"offset", `foo offset 5m`, []string{"foo"}},
		{"at modifier start", `foo @ start()`, []string{"foo"}},
		{"at modifier end", `foo @ end()`, []string{"foo"}},
		{"at modifier literal", `foo @ 1609459200`, []string{"foo"}},

		// Aggregations.
		{"sum by", `sum by (x) (foo)`, []string{"foo"}},
		{"sum without", `sum without (x) (foo)`, []string{"foo"}},
		{"avg", `avg(foo)`, []string{"foo"}},
		{"topk", `topk(10, foo)`, []string{"foo"}},
		{"count_values", `count_values("version", foo)`, []string{"foo"}},
		{"quantile", `quantile(0.5, foo)`, []string{"foo"}},

		// Binary ops.
		{"binary divide", `foo / bar`, []string{"bar", "foo"}},
		{"binary with ignoring", `foo / ignoring(x) bar`, []string{"bar", "foo"}},
		{"binary with on", `foo / on(job) bar`, []string{"bar", "foo"}},
		{"arithmetic with scalar", `foo * 2`, []string{"foo"}},
		{"comparison", `foo > 0.5`, []string{"foo"}},

		// Functions that take vector args.
		{"histogram_quantile", `histogram_quantile(0.99, rate(foo_bucket[5m]))`, []string{"foo_bucket"}},
		{"absent", `absent(up)`, []string{"up"}},
		{"absent_over_time", `absent_over_time(foo[5m])`, []string{"foo"}},
		{"label_replace", `label_replace(foo, "x","y","z","w")`, []string{"foo"}},
		{"label_join", `label_join(foo, "x", ",", "y", "z")`, []string{"foo"}},
		{"clamp", `clamp(foo, 0, 100)`, []string{"foo"}},
		{"vector", `vector(5)`, nil},
		{"scalar", `scalar(foo)`, []string{"foo"}},
		{"predict_linear", `predict_linear(foo[1h], 3600)`, []string{"foo"}},
		{"deriv", `deriv(foo[5m])`, []string{"foo"}},

		// Nested / composite.
		{"nested binary", `rate(foo[5m]) / rate(bar[5m])`, []string{"bar", "foo"}},
		{"dedup same metric", `foo + foo`, []string{"foo"}},
		{"three metrics", `foo + bar - baz`, []string{"bar", "baz", "foo"}},
		{"sum of rate", `sum by (job) (rate(foo_total[5m]))`, []string{"foo_total"}},
		{"hq over sum of rate", `histogram_quantile(0.99, sum by (le, job) (rate(http_request_duration_seconds_bucket[5m])))`, []string{"http_request_duration_seconds_bucket"}},

		// __name__ matchers.
		{"explicit __name__ equal", `{__name__="foo", job="x"}`, []string{"foo"}},
		{"explicit __name__ equal no labels", `{__name__="foo"}`, []string{"foo"}},

		// Pure literals: no metric names.
		{"number literal", `5`, nil},
		{"string literal", `"hello"`, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MetricNames(tc.expr)
			if err != nil {
				t.Fatalf("MetricNames(%q): %v", tc.expr, err)
			}
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("MetricNames(%q) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestMetricNames_RegexNameMatcher(t *testing.T) {
	// Regex on __name__ is not resolvable to specific metrics. Expected:
	// no metric names returned, warning emitted. We don't capture stderr
	// here — just verify it doesn't crash and returns empty.
	got, err := MetricNames(`{__name__=~"http.*", job="x"}`)
	if err != nil {
		t.Fatalf("MetricNames: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("regex __name__: got %v, want empty", got)
	}
}

func TestMetricNames_NegatedNameMatcher(t *testing.T) {
	got, err := MetricNames(`{__name__!="foo", job="x"}`)
	if err != nil {
		t.Fatalf("MetricNames: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("negated __name__: got %v, want empty", got)
	}
}

func TestMetricNames_InvalidExpr(t *testing.T) {
	cases := []string{
		"",
		"this is not promql",
		"rate(foo[",
		"foo +",
		// NOTE: `foo +++ bar` is VALID PromQL — parses as `foo + (+(+bar))`
		// via unary + operators, so it is not in this list.
	}
	for _, expr := range cases {
		t.Run(expr, func(t *testing.T) {
			_, err := MetricNames(expr)
			if err == nil {
				t.Errorf("MetricNames(%q): want error, got nil", expr)
			}
			if err != nil && !strings.Contains(err.Error(), "parse promql") {
				t.Errorf("MetricNames(%q): error should mention 'parse promql', got: %v", expr, err)
			}
		})
	}
}

func TestMetricNames_SortedOutput(t *testing.T) {
	// Output must be sorted for deterministic downstream processing.
	got, err := MetricNames(`zeta / beta + alpha`)
	if err != nil {
		t.Fatalf("MetricNames: %v", err)
	}
	want := []string{"alpha", "beta", "zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sorted output: got %v, want %v", got, want)
	}
}
