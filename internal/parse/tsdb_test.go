package parse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donchanee/metricops/internal/model"
)

func TestParseTSDBAnalyze_Basic(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "tsdb", "basic.txt"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	got, err := ParseTSDBAnalyze(f)
	if err != nil {
		t.Fatalf("ParseTSDBAnalyze: %v", err)
	}

	want := []*model.Metric{
		{Name: "http_request_duration_seconds_bucket", ActiveSeries: 5000},
		{Name: "app_user_actions_total", ActiveSeries: 3000},
		{Name: "grpc_server_handled_total", ActiveSeries: 1500},
		{Name: "node_cpu_seconds_total", ActiveSeries: 1200},
		{Name: "go_memstats_alloc_bytes", ActiveSeries: 800},
		{Name: "process_resident_memory_bytes", ActiveSeries: 500},
		{Name: "go_goroutines", ActiveSeries: 200},
		{Name: "node_load1", ActiveSeries: 100},
		{Name: "go_info", ActiveSeries: 1},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d metrics, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Name != want[i].Name {
			t.Errorf("[%d] Name: got %q, want %q", i, got[i].Name, want[i].Name)
		}
		if got[i].ActiveSeries != want[i].ActiveSeries {
			t.Errorf("[%d] ActiveSeries: got %d, want %d", i, got[i].ActiveSeries, want[i].ActiveSeries)
		}
	}
}

func TestParseTSDBAnalyze_Empty(t *testing.T) {
	got, err := ParseTSDBAnalyze(strings.NewReader(""))
	if err != nil {
		t.Fatalf("ParseTSDBAnalyze: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d metrics, want 0", len(got))
	}
}

func TestParseTSDBAnalyze_HeaderOnly(t *testing.T) {
	input := `Block ID: abc
Total Series: 1000
Label names: 10
`
	got, err := ParseTSDBAnalyze(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseTSDBAnalyze: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d metrics, want 0 (no hotmetrics section)", len(got))
	}
}

func TestParseTSDBAnalyze_Malformed(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "tsdb", "malformed.txt"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	got, err := ParseTSDBAnalyze(f)
	if err != nil {
		t.Fatalf("ParseTSDBAnalyze: %v", err)
	}

	// We expect malformed lines silently skipped and valid ones preserved.
	// Dedup: second "500 http_requests_total" must be ignored.
	wantNames := map[string]int{
		"http_requests_total":  500,
		"good_metric":          1000,
		"trimmed_spaces_ok":    700,
	}
	if len(got) != len(wantNames) {
		names := make([]string, 0, len(got))
		for _, m := range got {
			names = append(names, m.Name)
		}
		t.Fatalf("got %d metrics %v, want %d (%v)", len(got), names, len(wantNames), wantNames)
	}
	for _, m := range got {
		want, ok := wantNames[m.Name]
		if !ok {
			t.Errorf("unexpected metric %q", m.Name)
			continue
		}
		if m.ActiveSeries != want {
			t.Errorf("%s: got %d, want %d", m.Name, m.ActiveSeries, want)
		}
	}
}

func TestParseTSDBAnalyze_UnknownSections(t *testing.T) {
	// Verify the parser is tolerant to new promtool sections it doesn't recognize.
	input := `Block ID: abc
Total Series: 1000

Highest cardinality metric names:
100 foo_total
50 bar_seconds

Some Future Section:
ignore me
also me

Highest cardinality labels:
80 instance

Another Future Bit:
whatever
`
	got, err := ParseTSDBAnalyze(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseTSDBAnalyze: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d metrics, want 2", len(got))
	}
	if got[0].Name != "foo_total" || got[1].Name != "bar_seconds" {
		t.Errorf("unexpected metric names: %v, %v", got[0].Name, got[1].Name)
	}
}

func TestParseTSDBAnalyze_Dedup(t *testing.T) {
	input := `Highest cardinality metric names:
100 foo_total
50 foo_total
25 bar_total
`
	got, err := ParseTSDBAnalyze(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseTSDBAnalyze: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d metrics, want 2", len(got))
	}
	if got[0].Name != "foo_total" || got[0].ActiveSeries != 100 {
		t.Errorf("first occurrence should win: got %+v", got[0])
	}
}

func TestIsSectionHeader(t *testing.T) {
	cases := map[string]bool{
		"Highest cardinality metric names:": true,
		"Highest cardinality labels:":       true,
		"Total Series: 1000":                false,
		"Block ID: abc":                     false,
		"500 foo_total":                     false,
		"":                                  false,
	}
	for input, want := range cases {
		if got := isSectionHeader(input); got != want {
			t.Errorf("isSectionHeader(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestParseCountAndName(t *testing.T) {
	cases := []struct {
		in       string
		count    int
		name     string
		ok       bool
	}{
		{"500 http_requests_total", 500, "http_requests_total", true},
		{"1 go_info", 1, "go_info", true},
		{"0 never_fired", 0, "never_fired", true},
		{"500  with_extra_space", 500, "with_extra_space", true},
		{"noDigits foo", 0, "", false},
		{"500", 0, "", false},
		{"", 0, "", false},
		{"-5 negative", 0, "", false},
	}
	for _, tc := range cases {
		n, nm, ok := parseCountAndName(tc.in)
		if ok != tc.ok || n != tc.count || nm != tc.name {
			t.Errorf("parseCountAndName(%q) = (%d, %q, %v); want (%d, %q, %v)",
				tc.in, n, nm, ok, tc.count, tc.name, tc.ok)
		}
	}
}
