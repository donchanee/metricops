package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/donchanee/metricops/internal/model"
)

// fixedReport returns a small deterministic Report for render tests.
func fixedReport() *model.Report {
	return &model.Report{
		SchemaVersion: "1.0",
		GeneratedAt:   time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC),
		Summary: model.Summary{
			TotalMetrics:          80,
			UnusedMetrics:         21,
			TotalActiveSeries:     76475,
			EstimatedDailyBytes:   8812800,
			EstimatedMonthlyBytes: 264384000,
			BytesPerSampleAssumed: 2.0,
		},
		UnusedMetrics: []model.UnusedMetric{
			{Name: "legacy_worker_queue_depth", ActiveSeries: 512, BytesPerDayEstimate: 5898240},
			{Name: "api_v1_requests_total", ActiveSeries: 320, BytesPerDayEstimate: 3686400},
		},
		CardinalityHotspots: []model.CardinalityHotspot{
			{Metric: "app_user_actions_total", Cardinality: 15432, PctOfTotalSeries: 20.18},
			{Metric: "http_request_duration_seconds_bucket", Cardinality: 12000, PctOfTotalSeries: 15.69},
		},
		Recommendations: []model.Recommendation{
			{Type: "drop_metric", Target: "legacy_worker_queue_depth", EstimatedSavingsBytesPerDay: 5898240},
			{Type: "drop_metric", Target: "api_v1_requests_total", EstimatedSavingsBytesPerDay: 3686400},
		},
	}
}

// --- Markdown ---

func TestMarkdown_Header(t *testing.T) {
	var buf bytes.Buffer
	if err := Markdown(&buf, fixedReport()); err != nil {
		t.Fatalf("Markdown: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"# metricops Report",
		"Generated: 2026-04-17T12:00:00Z",
		"Schema: 1.0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown missing %q", want)
		}
	}
}

func TestMarkdown_Sections(t *testing.T) {
	var buf bytes.Buffer
	if err := Markdown(&buf, fixedReport()); err != nil {
		t.Fatalf("Markdown: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"## Summary",
		"## Unused Metrics",
		"## Cardinality Hotspots",
		"## Recommendations",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing section: %s", want)
		}
	}

	// Section ordering: Summary → Unused → Hotspots → Recommendations.
	iSummary := strings.Index(out, "## Summary")
	iUnused := strings.Index(out, "## Unused Metrics")
	iHotspots := strings.Index(out, "## Cardinality Hotspots")
	iRecs := strings.Index(out, "## Recommendations")
	if !(iSummary < iUnused && iUnused < iHotspots && iHotspots < iRecs) {
		t.Errorf("section order wrong: summary=%d unused=%d hotspots=%d recs=%d",
			iSummary, iUnused, iHotspots, iRecs)
	}
}

func TestMarkdown_ContentPresent(t *testing.T) {
	var buf bytes.Buffer
	_ = Markdown(&buf, fixedReport())
	out := buf.String()
	for _, want := range []string{
		"`legacy_worker_queue_depth`",
		"`api_v1_requests_total`",
		"`app_user_actions_total`",
		"20.18%",
		"drop_metric",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing content: %q", want)
		}
	}
}

func TestMarkdown_ThousandsFormatting(t *testing.T) {
	var buf bytes.Buffer
	_ = Markdown(&buf, fixedReport())
	out := buf.String()
	// Total series 76475 should render with comma.
	if !strings.Contains(out, "76,475") {
		t.Errorf("expected comma-formatted 76,475 in output")
	}
}

func TestMarkdown_HumanBytes(t *testing.T) {
	var buf bytes.Buffer
	_ = Markdown(&buf, fixedReport())
	out := buf.String()
	// 8812800 bytes → 8.40 MiB (not exact but close).
	if !strings.Contains(out, "MiB") {
		t.Errorf("expected MiB in human-readable output; got:\n%s", out)
	}
}

func TestMarkdown_EmptyReport(t *testing.T) {
	empty := &model.Report{
		SchemaVersion: "1.0",
		GeneratedAt:   time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC),
		Summary:       model.Summary{TotalMetrics: 0, BytesPerSampleAssumed: 2.0},
	}
	var buf bytes.Buffer
	if err := Markdown(&buf, empty); err != nil {
		t.Fatalf("Markdown: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"No unused metrics detected.",
		"No cardinality hotspots detected.",
		"No recommendations.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("empty report missing %q", want)
		}
	}
}

func TestMarkdown_NilReport(t *testing.T) {
	var buf bytes.Buffer
	if err := Markdown(&buf, nil); err == nil {
		t.Error("want error on nil report")
	}
}

func TestMarkdown_Deterministic(t *testing.T) {
	var a, b bytes.Buffer
	_ = Markdown(&a, fixedReport())
	_ = Markdown(&b, fixedReport())
	if a.String() != b.String() {
		t.Error("markdown output differs between identical calls")
	}
}

// --- JSON ---

func TestJSON_Roundtrip(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, fixedReport()); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	var decoded model.Report
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("roundtrip unmarshal: %v", err)
	}
	if decoded.SchemaVersion != "1.0" {
		t.Errorf("SchemaVersion: got %q, want 1.0", decoded.SchemaVersion)
	}
	if decoded.Summary.TotalMetrics != 80 {
		t.Errorf("TotalMetrics roundtrip: got %d, want 80", decoded.Summary.TotalMetrics)
	}
	if len(decoded.UnusedMetrics) != 2 {
		t.Errorf("UnusedMetrics roundtrip: got %d, want 2", len(decoded.UnusedMetrics))
	}
}

func TestJSON_Deterministic10Runs(t *testing.T) {
	// The CI determinism gate (10 byte-identical runs) enforced in v4 is
	// checked here at unit level too. Report has no map fields so this is
	// stable purely by struct ordering.
	first := &bytes.Buffer{}
	if err := JSON(first, fixedReport()); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	for i := 0; i < 9; i++ {
		var b bytes.Buffer
		if err := JSON(&b, fixedReport()); err != nil {
			t.Fatalf("JSON[%d]: %v", i, err)
		}
		if !bytes.Equal(first.Bytes(), b.Bytes()) {
			t.Errorf("run %d differs from run 0", i+1)
		}
	}
}

func TestJSON_FieldOrder(t *testing.T) {
	// Verify top-level field order matches model.Report definition.
	var buf bytes.Buffer
	_ = JSON(&buf, fixedReport())
	out := buf.String()
	order := []string{
		`"schema_version"`,
		`"generated_at"`,
		`"summary"`,
		`"unused_metrics"`,
		`"cardinality_hotspots"`,
		`"recommendations"`,
	}
	last := 0
	for _, key := range order {
		i := strings.Index(out, key)
		if i < 0 {
			t.Errorf("missing field %s", key)
			continue
		}
		if i < last {
			t.Errorf("%s appears before previous field (order broken)", key)
		}
		last = i
	}
}

func TestJSON_NilReport(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, nil); err == nil {
		t.Error("want error on nil report")
	}
}

func TestJSON_EmptyReport(t *testing.T) {
	empty := &model.Report{SchemaVersion: "1.0"}
	var buf bytes.Buffer
	if err := JSON(&buf, empty); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	// Should not crash on empty/zero slices and maps; should produce valid JSON.
	var back model.Report
	if err := json.Unmarshal(buf.Bytes(), &back); err != nil {
		t.Errorf("empty report: unmarshal failed: %v", err)
	}
}

// --- Helpers ---

func TestThousands(t *testing.T) {
	cases := map[int64]string{
		0:         "0",
		1:         "1",
		100:       "100",
		999:       "999",
		1000:      "1,000",
		12345:     "12,345",
		1234567:   "1,234,567",
		-1234:     "-1,234",
		-1000000:  "-1,000,000",
	}
	for in, want := range cases {
		if got := thousands(in); got != want {
			t.Errorf("thousands(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1023, "1023 B"},
		{1024, "1.00 KiB"},
		{1536, "1.50 KiB"},
		{1048576, "1.00 MiB"},
		{8812800, "8.40 MiB"},
		{1073741824, "1.00 GiB"},
	}
	for _, c := range cases {
		if got := humanBytes(c.in); got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
