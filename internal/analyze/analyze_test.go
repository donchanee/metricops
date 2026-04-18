package analyze

import (
	"testing"

	"github.com/donchanee/metricops/internal/model"
)

// newTestModel creates a small deterministic model for shared use across
// analyzer tests.
//
// Layout:
//
//	foo      ActiveSeries=800, used (dashboard ref)     → not unused
//	bar      ActiveSeries=500, used (alert ref)         → not unused
//	baz      ActiveSeries=200, no refs                  → unused
//	qux      ActiveSeries=80,  no refs                  → unused (below default noise floor)
//	zero     ActiveSeries=0,   no refs                  → unused, zero cost
//	dominant ActiveSeries=9000, used                    → cardinality hotspot
//
// TotalActiveSeries = 10580.
func newTestModel() *model.Model {
	metrics := map[string]*model.Metric{
		"foo": {
			Name:         "foo",
			ActiveSeries: 800,
			References: []model.Reference{
				{Source: model.RefDashboard, Location: "dash#1", Expr: "foo"},
			},
		},
		"bar": {
			Name:         "bar",
			ActiveSeries: 500,
			References: []model.Reference{
				{Source: model.RefAlert, Location: "alert#x", Expr: "bar > 5"},
			},
		},
		"baz":      {Name: "baz", ActiveSeries: 200},
		"qux":      {Name: "qux", ActiveSeries: 80},
		"zero":     {Name: "zero", ActiveSeries: 0},
		"dominant": {
			Name:         "dominant",
			ActiveSeries: 9000,
			References: []model.Reference{
				{Source: model.RefDashboard, Location: "dash#2", Expr: "dominant"},
			},
		},
	}
	total := 0
	for _, m := range metrics {
		total += m.ActiveSeries
	}
	return &model.Model{Metrics: metrics, TotalActiveSeries: total}
}

// --- DetectUnused ---

func TestDetectUnused_Basic(t *testing.T) {
	m := newTestModel()
	got, err := DetectUnused(m, DefaultBytesPerSample)
	if err != nil {
		t.Fatalf("DetectUnused: %v", err)
	}
	wantNames := []string{"baz", "qux", "zero"}
	if len(got) != len(wantNames) {
		t.Fatalf("got %d unused, want %d", len(got), len(wantNames))
	}
	// Sort check: by ActiveSeries desc, then Name asc.
	if got[0].Name != "baz" || got[1].Name != "qux" || got[2].Name != "zero" {
		t.Errorf("sort order wrong: %v", []string{got[0].Name, got[1].Name, got[2].Name})
	}
}

func TestDetectUnused_CostEstimated(t *testing.T) {
	m := newTestModel()
	got, _ := DetectUnused(m, DefaultBytesPerSample)
	for _, u := range got {
		if u.Name == "zero" {
			if u.BytesPerDayEstimate != 0 {
				t.Errorf("zero-series unused should have 0 cost, got %d", u.BytesPerDayEstimate)
			}
		} else if u.BytesPerDayEstimate <= 0 {
			t.Errorf("%s should have nonzero cost, got %d", u.Name, u.BytesPerDayEstimate)
		}
	}
}

func TestDetectUnused_EmptyModel(t *testing.T) {
	got, err := DetectUnused(&model.Model{Metrics: map[string]*model.Metric{}}, DefaultBytesPerSample)
	if err != nil {
		t.Fatalf("DetectUnused: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty model: got %d unused, want 0", len(got))
	}
}

func TestDetectUnused_NilModel(t *testing.T) {
	got, err := DetectUnused(nil, DefaultBytesPerSample)
	if err != nil {
		t.Fatalf("DetectUnused(nil): %v", err)
	}
	if got != nil {
		t.Errorf("nil model: got %v, want nil", got)
	}
}

func TestDetectUnused_StableTiebreak(t *testing.T) {
	m := &model.Model{
		Metrics: map[string]*model.Metric{
			"zz": {Name: "zz", ActiveSeries: 500},
			"aa": {Name: "aa", ActiveSeries: 500},
			"mm": {Name: "mm", ActiveSeries: 500},
		},
		TotalActiveSeries: 1500,
	}
	got, _ := DetectUnused(m, DefaultBytesPerSample)
	if got[0].Name != "aa" || got[1].Name != "mm" || got[2].Name != "zz" {
		t.Errorf("tie broken wrong (want aa,mm,zz): got %v %v %v",
			got[0].Name, got[1].Name, got[2].Name)
	}
}

// --- DetectHotspots ---

func TestDetectHotspots_Basic(t *testing.T) {
	m := newTestModel()
	got, err := DetectHotspots(m, HotspotOptions{})
	if err != nil {
		t.Fatalf("DetectHotspots: %v", err)
	}
	// total=10580, >5% threshold requires > 529 series.
	// dominant(9000) = 85% → hit
	// foo(800)       = 7.6% → hit
	// bar(500)       = 4.7% → miss (below threshold)
	if len(got) != 2 {
		for _, h := range got {
			t.Logf("got: %+v", h)
		}
		t.Fatalf("got %d hotspots, want 2", len(got))
	}
	if got[0].Metric != "dominant" || got[1].Metric != "foo" {
		t.Errorf("sort order wrong: %v, %v", got[0].Metric, got[1].Metric)
	}
	if got[0].PctOfTotalSeries < 80 || got[0].PctOfTotalSeries > 90 {
		t.Errorf("dominant pct wrong: %v", got[0].PctOfTotalSeries)
	}
}

func TestDetectHotspots_NoiseFloor(t *testing.T) {
	// Tiny deployment: one metric hits 20% but is below default NoiseFloor.
	m := &model.Model{
		Metrics: map[string]*model.Metric{
			"small": {Name: "small", ActiveSeries: 40},
			"large": {Name: "large", ActiveSeries: 160},
		},
		TotalActiveSeries: 200,
	}
	got, _ := DetectHotspots(m, HotspotOptions{})
	// large(160) > default NoiseFloor(100), 80% of total → hotspot
	// small(40) < NoiseFloor → skipped despite 20%
	if len(got) != 1 || got[0].Metric != "large" {
		t.Errorf("noise-floor filter failed: got %+v", got)
	}
}

func TestDetectHotspots_TopN(t *testing.T) {
	m := newTestModel()
	got, _ := DetectHotspots(m, HotspotOptions{TopN: 1})
	if len(got) != 1 {
		t.Errorf("TopN=1: got %d, want 1", len(got))
	}
	if got[0].Metric != "dominant" {
		t.Errorf("TopN should preserve top entry, got %s", got[0].Metric)
	}
}

func TestDetectHotspots_CustomThreshold(t *testing.T) {
	m := newTestModel()
	got, _ := DetectHotspots(m, HotspotOptions{PctThreshold: 1.0}) // > 1%
	// With 1% threshold: dominant(85%), foo(7.6%), bar(4.7%), baz(1.9%) all hit.
	// qux(0.75%) and zero(0%) miss.
	// But NoiseFloor=100 excludes qux(80) and zero(0). baz(200) passes.
	if len(got) != 4 {
		names := []string{}
		for _, h := range got {
			names = append(names, h.Metric)
		}
		t.Errorf("threshold=1%%: got %d (%v), want 4", len(got), names)
	}
}

func TestDetectHotspots_NilModel(t *testing.T) {
	got, err := DetectHotspots(nil, HotspotOptions{})
	if err != nil {
		t.Fatalf("nil: %v", err)
	}
	if got != nil {
		t.Errorf("nil model: got %v, want nil", got)
	}
}

func TestDetectHotspots_ZeroTotalSeries(t *testing.T) {
	m := &model.Model{
		Metrics:           map[string]*model.Metric{"x": {Name: "x", ActiveSeries: 0}},
		TotalActiveSeries: 0,
	}
	got, _ := DetectHotspots(m, HotspotOptions{})
	if got != nil {
		t.Errorf("zero total: got %v, want nil (avoid div by zero)", got)
	}
}

// --- EstimateBytesPerDay ---

func TestEstimateBytesPerDay(t *testing.T) {
	// 1 series * 1/15 samples/s * 2 bytes/sample * 86400 s/day = 11520 bytes/day
	got := EstimateBytesPerDay(1, DefaultBytesPerSample)
	if got != 11520 {
		t.Errorf("1 series: got %d, want 11520", got)
	}

	// Linear scaling.
	got2 := EstimateBytesPerDay(1000, DefaultBytesPerSample)
	if got2 != 11520*1000 {
		t.Errorf("1000 series: got %d, want %d", got2, 11520*1000)
	}

	// Zero / negative inputs → 0.
	if got := EstimateBytesPerDay(0, DefaultBytesPerSample); got != 0 {
		t.Errorf("0 series: got %d, want 0", got)
	}
	if got := EstimateBytesPerDay(-5, DefaultBytesPerSample); got != 0 {
		t.Errorf("negative series: got %d, want 0", got)
	}
	if got := EstimateBytesPerDay(100, 0); got != 0 {
		t.Errorf("0 bytesPerSample: got %d, want 0", got)
	}
}

// --- EstimateSummary ---

func TestEstimateSummary(t *testing.T) {
	m := newTestModel()
	s, err := EstimateSummary(m, DefaultBytesPerSample)
	if err != nil {
		t.Fatalf("EstimateSummary: %v", err)
	}
	if s.TotalMetrics != 6 {
		t.Errorf("TotalMetrics: got %d, want 6", s.TotalMetrics)
	}
	if s.UnusedMetrics != 3 {
		t.Errorf("UnusedMetrics: got %d, want 3", s.UnusedMetrics)
	}
	if s.TotalActiveSeries != 10580 {
		t.Errorf("TotalActiveSeries: got %d, want 10580", s.TotalActiveSeries)
	}
	if s.BytesPerSampleAssumed != DefaultBytesPerSample {
		t.Errorf("BytesPerSampleAssumed: got %v, want %v", s.BytesPerSampleAssumed, DefaultBytesPerSample)
	}
	// Daily = sum of per-metric daily; 30 * daily = monthly.
	if s.EstimatedMonthlyBytes != s.EstimatedDailyBytes*30 {
		t.Errorf("monthly != daily*30: %d vs %d", s.EstimatedMonthlyBytes, s.EstimatedDailyBytes*30)
	}
	// Sanity bound.
	if s.EstimatedDailyBytes <= 0 {
		t.Errorf("EstimatedDailyBytes should be positive, got %d", s.EstimatedDailyBytes)
	}
}

func TestEstimateSummary_NilModel(t *testing.T) {
	s, err := EstimateSummary(nil, DefaultBytesPerSample)
	if err != nil {
		t.Fatalf("nil: %v", err)
	}
	if s != (model.Summary{}) {
		t.Errorf("nil model: got %+v, want zero Summary", s)
	}
}

// --- BuildRecommendations ---

func TestBuildRecommendations(t *testing.T) {
	unused := []model.UnusedMetric{
		{Name: "big", ActiveSeries: 500, BytesPerDayEstimate: 500_000},
		{Name: "mid", ActiveSeries: 200, BytesPerDayEstimate: 200_000},
		{Name: "zero", ActiveSeries: 0, BytesPerDayEstimate: 0},
	}
	recs, err := BuildRecommendations(unused, nil)
	if err != nil {
		t.Fatalf("BuildRecommendations: %v", err)
	}
	// zero-cost drop is excluded.
	if len(recs) != 2 {
		t.Fatalf("got %d recs, want 2", len(recs))
	}
	if recs[0].Target != "big" || recs[0].Type != "drop_metric" {
		t.Errorf("first rec wrong: %+v", recs[0])
	}
	if recs[0].EstimatedSavingsBytesPerDay != 500_000 {
		t.Errorf("savings wrong: %d", recs[0].EstimatedSavingsBytesPerDay)
	}
}

func TestBuildRecommendations_SortBySavings(t *testing.T) {
	unused := []model.UnusedMetric{
		{Name: "a", BytesPerDayEstimate: 100},
		{Name: "b", BytesPerDayEstimate: 300},
		{Name: "c", BytesPerDayEstimate: 200},
	}
	recs, _ := BuildRecommendations(unused, nil)
	if recs[0].Target != "b" || recs[1].Target != "c" || recs[2].Target != "a" {
		t.Errorf("sort wrong: %s %s %s", recs[0].Target, recs[1].Target, recs[2].Target)
	}
}

func TestBuildRecommendations_SortTiebreakByName(t *testing.T) {
	unused := []model.UnusedMetric{
		{Name: "zz", BytesPerDayEstimate: 100},
		{Name: "aa", BytesPerDayEstimate: 100},
		{Name: "mm", BytesPerDayEstimate: 100},
	}
	recs, _ := BuildRecommendations(unused, nil)
	if recs[0].Target != "aa" || recs[1].Target != "mm" || recs[2].Target != "zz" {
		t.Errorf("tiebreak wrong: %s %s %s", recs[0].Target, recs[1].Target, recs[2].Target)
	}
}

func TestBuildRecommendations_Empty(t *testing.T) {
	recs, err := BuildRecommendations(nil, nil)
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("got %d recs, want 0", len(recs))
	}
}
