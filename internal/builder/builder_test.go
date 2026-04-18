package builder

import (
	"testing"

	"github.com/donchanee/metricops/internal/model"
)

func TestBuild_HappyPath(t *testing.T) {
	metrics := []*model.Metric{
		{Name: "foo_total", ActiveSeries: 500},
		{Name: "bar_seconds", ActiveSeries: 300},
		{Name: "baz_bytes", ActiveSeries: 100},
	}
	refs := []model.Reference{
		{Source: model.RefDashboard, Location: "dash#1", Expr: "rate(foo_total[5m])"},
		{Source: model.RefAlert, Location: "alerts#HighFoo", Expr: "foo_total > 100"},
		{Source: model.RefRecording, Location: "rules#agg", Expr: "sum(bar_seconds)"},
	}

	m, r, err := Build(metrics, refs, Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if m.TotalActiveSeries != 900 {
		t.Errorf("TotalActiveSeries: got %d, want 900", m.TotalActiveSeries)
	}

	foo := m.Metrics["foo_total"]
	if foo == nil {
		t.Fatal("foo_total missing from model")
	}
	if len(foo.References) != 2 {
		t.Errorf("foo_total refs: got %d, want 2", len(foo.References))
	}

	bar := m.Metrics["bar_seconds"]
	if bar == nil || len(bar.References) != 1 {
		t.Errorf("bar_seconds: got %v", bar)
	}

	baz := m.Metrics["baz_bytes"]
	if baz == nil {
		t.Fatal("baz_bytes missing")
	}
	if baz.IsUsed() {
		t.Errorf("baz_bytes should be unused, got %d refs", len(baz.References))
	}

	if r.ReferencesAttributed != 3 {
		t.Errorf("ReferencesAttributed: got %d, want 3", r.ReferencesAttributed)
	}
	if r.ReferencesOrphaned != 0 {
		t.Errorf("ReferencesOrphaned: got %d, want 0", r.ReferencesOrphaned)
	}
	if r.ReferencesUnparseable != 0 {
		t.Errorf("ReferencesUnparseable: got %d, want 0", r.ReferencesUnparseable)
	}
}

func TestBuild_OrphanReference(t *testing.T) {
	metrics := []*model.Metric{
		{Name: "foo_total", ActiveSeries: 100},
	}
	refs := []model.Reference{
		{Source: model.RefDashboard, Location: "dash#1", Expr: "rate(foo_total[5m])"},
		{Source: model.RefAlert, Location: "alerts#Ghost", Expr: "rate(deleted_metric[5m])"},
	}
	_, r, err := Build(metrics, refs, Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if r.ReferencesAttributed != 1 {
		t.Errorf("ReferencesAttributed: got %d, want 1", r.ReferencesAttributed)
	}
	if r.ReferencesOrphaned != 1 {
		t.Errorf("ReferencesOrphaned: got %d, want 1", r.ReferencesOrphaned)
	}
}

func TestBuild_UnparseableReference(t *testing.T) {
	metrics := []*model.Metric{
		{Name: "foo_total", ActiveSeries: 100},
	}
	refs := []model.Reference{
		{Source: model.RefDashboard, Location: "dash#1", Expr: "rate(foo_total[5m])"},
		{Source: model.RefAlert, Location: "alerts#Broken", Expr: "this is not promql"},
	}
	_, r, err := Build(metrics, refs, Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if r.ReferencesUnparseable != 1 {
		t.Errorf("ReferencesUnparseable: got %d, want 1", r.ReferencesUnparseable)
	}
	if r.ReferencesAttributed != 1 {
		t.Errorf("ReferencesAttributed: got %d, want 1", r.ReferencesAttributed)
	}
}

func TestBuild_StrictFailsOnUnparseable(t *testing.T) {
	metrics := []*model.Metric{{Name: "foo_total", ActiveSeries: 100}}
	refs := []model.Reference{
		{Source: model.RefAlert, Location: "alerts#Broken", Expr: "this is not promql"},
	}
	_, _, err := Build(metrics, refs, Options{Strict: true})
	if err == nil {
		t.Fatal("Build(Strict=true): want error for unparseable expr, got nil")
	}
}

func TestBuild_MultiMetricExpr(t *testing.T) {
	// Binary op: one ref attributes to BOTH metrics.
	metrics := []*model.Metric{
		{Name: "foo", ActiveSeries: 100},
		{Name: "bar", ActiveSeries: 200},
	}
	refs := []model.Reference{
		{Source: model.RefDashboard, Location: "dash#1", Expr: "foo / bar"},
	}
	m, r, err := Build(metrics, refs, Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(m.Metrics["foo"].References) != 1 || len(m.Metrics["bar"].References) != 1 {
		t.Errorf("both metrics should have 1 ref each; got foo=%d, bar=%d",
			len(m.Metrics["foo"].References), len(m.Metrics["bar"].References))
	}
	// But the ref counts as one attribution (not two), because a single Reference
	// exists — it just touches multiple metrics.
	if r.ReferencesAttributed != 1 {
		t.Errorf("ReferencesAttributed: got %d, want 1 (one ref, even if multi-metric)", r.ReferencesAttributed)
	}
}

func TestBuild_DuplicateMetricName(t *testing.T) {
	// `foo + foo` should attach only ONE reference to foo, not two (promqlx dedupes).
	metrics := []*model.Metric{{Name: "foo", ActiveSeries: 100}}
	refs := []model.Reference{
		{Source: model.RefDashboard, Location: "dash#1", Expr: "foo + foo"},
	}
	m, _, err := Build(metrics, refs, Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := len(m.Metrics["foo"].References); got != 1 {
		t.Errorf("got %d refs after dedup, want 1", got)
	}
}

func TestBuild_EmptyInputs(t *testing.T) {
	m, r, err := Build(nil, nil, Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if m == nil {
		t.Fatal("Build returned nil model for empty inputs")
	}
	if len(m.Metrics) != 0 {
		t.Errorf("got %d metrics, want 0", len(m.Metrics))
	}
	if m.TotalActiveSeries != 0 {
		t.Errorf("TotalActiveSeries: got %d, want 0", m.TotalActiveSeries)
	}
	if r.ReferencesAttributed != 0 || r.ReferencesOrphaned != 0 || r.ReferencesUnparseable != 0 {
		t.Errorf("counters should be zero on empty input: %+v", r)
	}
}

func TestBuild_UnusedMetricPreserved(t *testing.T) {
	// A metric in TSDB with zero references in inputs stays in the Model
	// and is flagged via IsUsed()=false.
	metrics := []*model.Metric{
		{Name: "used_metric", ActiveSeries: 100},
		{Name: "unused_metric", ActiveSeries: 50},
	}
	refs := []model.Reference{
		{Source: model.RefDashboard, Location: "dash#1", Expr: "used_metric"},
	}
	m, _, err := Build(metrics, refs, Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !m.Metrics["used_metric"].IsUsed() {
		t.Error("used_metric should be used")
	}
	if m.Metrics["unused_metric"].IsUsed() {
		t.Error("unused_metric should be unused")
	}
}
