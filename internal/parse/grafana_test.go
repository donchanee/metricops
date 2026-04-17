package parse

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donchanee/metricops/internal/model"
)

func TestParseGrafanaFile_Basic(t *testing.T) {
	path := filepath.Join("testdata", "grafana", "basic.json")
	refs, err := ParseGrafanaFile(path)
	if err != nil {
		t.Fatalf("ParseGrafanaFile: %v", err)
	}
	// 2 targets on panel 1 + 1 target on panel 2 + 1 target on panel 3 = 4 refs.
	if len(refs) != 4 {
		t.Fatalf("got %d refs, want 4", len(refs))
	}
	for _, r := range refs {
		if r.Source != model.RefDashboard {
			t.Errorf("Source: got %q, want %q", r.Source, model.RefDashboard)
		}
		if !strings.HasPrefix(r.Location, path+"#panel:") {
			t.Errorf("Location %q should start with %q", r.Location, path+"#panel:")
		}
		if r.Expr == "" {
			t.Errorf("empty Expr on %s", r.Location)
		}
	}
}

func TestParseGrafanaFile_MixedDatasources(t *testing.T) {
	path := filepath.Join("testdata", "grafana", "mixed.json")
	refs, err := ParseGrafanaFile(path)
	if err != nil {
		t.Fatalf("ParseGrafanaFile: %v", err)
	}

	// Expected:
	//   panel 1 (prom object)       → 1 ref
	//   panel 2 (loki)              → 0
	//   panel 3 (string Prometheus) → 1 ref
	//   panel 4 (string CloudWatch) → 0
	//   panel 5 (template var)      → 0 (skipped due to ${})
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2 (see comment)", len(refs))
	}

	exprs := make(map[string]bool, len(refs))
	for _, r := range refs {
		exprs[r.Expr] = true
	}
	if !exprs["rate(http_requests_total[5m])"] {
		t.Error("missing prom object datasource ref")
	}
	if !exprs["go_goroutines"] {
		t.Error("missing legacy string Prometheus ref")
	}
}

func TestParseGrafanaFile_Rows(t *testing.T) {
	path := filepath.Join("testdata", "grafana", "rows.json")
	refs, err := ParseGrafanaFile(path)
	if err != nil {
		t.Fatalf("ParseGrafanaFile: %v", err)
	}
	// Row (id=10) has 2 nested panels, each with 1 target → 2 refs.
	// Repeat panel (id=20) is skipped → 0 refs.
	// No-target panel (id=30) contributes 0 refs.
	if len(refs) != 2 {
		for _, r := range refs {
			t.Logf("got: %+v", r)
		}
		t.Fatalf("got %d refs, want 2", len(refs))
	}
	// Verify refs came from nested panels (ids 11 and 12), not row (10) or repeat (20).
	for _, r := range refs {
		if strings.HasSuffix(r.Location, "#panel:10") {
			t.Errorf("should not emit ref for row panel itself")
		}
		if strings.HasSuffix(r.Location, "#panel:20") {
			t.Errorf("should not emit ref for repeat panel")
		}
	}
}

func TestParseGrafanaFile_Corrupt(t *testing.T) {
	_, err := ParseGrafanaFile(filepath.Join("testdata", "grafana", "corrupt.json"))
	if err == nil {
		t.Fatal("expected error for corrupt JSON")
	}
}

func TestParseGrafanaFile_NotFound(t *testing.T) {
	_, err := ParseGrafanaFile(filepath.Join("testdata", "grafana", "nope.json"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParseGrafanaDir(t *testing.T) {
	refs, err := ParseGrafanaDir(filepath.Join("testdata", "grafana"))
	if err != nil {
		t.Fatalf("ParseGrafanaDir: %v", err)
	}
	// basic.json=4 + mixed.json=2 + rows.json=2 + corrupt.json=skipped = 8 refs
	if len(refs) != 8 {
		t.Errorf("got %d refs across dir, want 8", len(refs))
	}
}

func TestParseGrafanaDir_SingleFile(t *testing.T) {
	refs, err := ParseGrafanaDir(filepath.Join("testdata", "grafana", "basic.json"))
	if err != nil {
		t.Fatalf("ParseGrafanaDir: %v", err)
	}
	if len(refs) != 4 {
		t.Errorf("got %d refs, want 4", len(refs))
	}
}

func TestIsPrometheusDatasource(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{"prom object", `{"type":"prometheus","uid":"abc"}`, true},
		{"prom object upper", `{"type":"Prometheus","uid":"abc"}`, true},
		{"loki object", `{"type":"loki","uid":"loki"}`, false},
		{"cloudwatch object", `{"type":"cloudwatch"}`, false},
		{"string prom", `"Prometheus"`, true},
		{"string prom lower", `"prometheus"`, true},
		{"string loki", `"Loki"`, false},
		{"string mixed", `"-- Mixed --"`, true},
		{"empty raw", ``, true},
		{"null", `null`, true},
		{"string empty", `""`, true},
		{"unknown type object", `{"type":"customplugin"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isPrometheusDatasource(json.RawMessage(tc.raw))
			if got != tc.want {
				t.Errorf("isPrometheusDatasource(%s) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestParseGrafanaFile_PreservesExprs(t *testing.T) {
	refs, err := ParseGrafanaFile(filepath.Join("testdata", "grafana", "basic.json"))
	if err != nil {
		t.Fatalf("ParseGrafanaFile: %v", err)
	}
	// Spot-check: the histogram_quantile expression was preserved verbatim.
	found := false
	for _, r := range refs {
		if strings.Contains(r.Expr, "histogram_quantile") &&
			strings.Contains(r.Expr, "http_request_duration_seconds_bucket") {
			found = true
		}
	}
	if !found {
		t.Error("histogram_quantile expression not preserved in refs")
	}
}
