package parse

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/donchanee/metricops/internal/model"
)

func TestParseRulesFile_Basic(t *testing.T) {
	path := filepath.Join("testdata", "rules", "basic.yml")
	refs, err := ParseRulesFile(path)
	if err != nil {
		t.Fatalf("ParseRulesFile: %v", err)
	}

	// 3 alerts + 2 recording rules = 5 refs.
	if len(refs) != 5 {
		t.Fatalf("got %d refs, want 5", len(refs))
	}

	// Index refs by location for easier assertion.
	byLoc := make(map[string]model.Reference, len(refs))
	for _, r := range refs {
		byLoc[r.Location] = r
	}

	wantAlerts := []string{
		path + "#alert:HighErrorRate",
		path + "#alert:SlowRequests",
		path + "#alert:HighGoroutines",
	}
	wantRecords := []string{
		path + "#record:job:http_requests:rate5m",
		path + "#record:job:http_errors:rate5m",
	}

	for _, loc := range wantAlerts {
		r, ok := byLoc[loc]
		if !ok {
			t.Errorf("missing alert at %q", loc)
			continue
		}
		if r.Source != model.RefAlert {
			t.Errorf("%s: Source=%q, want %q", loc, r.Source, model.RefAlert)
		}
		if r.Expr == "" {
			t.Errorf("%s: empty Expr", loc)
		}
	}
	for _, loc := range wantRecords {
		r, ok := byLoc[loc]
		if !ok {
			t.Errorf("missing recording at %q", loc)
			continue
		}
		if r.Source != model.RefRecording {
			t.Errorf("%s: Source=%q, want %q", loc, r.Source, model.RefRecording)
		}
	}
}

func TestParseRulesFile_PreservesExpr(t *testing.T) {
	refs, err := ParseRulesFile(filepath.Join("testdata", "rules", "basic.yml"))
	if err != nil {
		t.Fatalf("ParseRulesFile: %v", err)
	}
	for _, r := range refs {
		if !strings.Contains(r.Expr, "(") && !strings.Contains(r.Expr, ">") {
			// Spot-check: every expr in our fixture uses at least one of these.
			t.Errorf("expr %q looks malformed/truncated", r.Expr)
		}
	}
}

func TestParseRulesFile_Empty(t *testing.T) {
	refs, err := ParseRulesFile(filepath.Join("testdata", "rules", "empty.yml"))
	if err != nil {
		t.Fatalf("ParseRulesFile: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("got %d refs, want 0", len(refs))
	}
}

func TestParseRulesFile_Helm(t *testing.T) {
	_, err := ParseRulesFile(filepath.Join("testdata", "rules", "helm.yml"))
	if err == nil {
		t.Fatal("expected error for Helm-templated file")
	}
	if !strings.Contains(err.Error(), "Helm") {
		t.Errorf("error should mention Helm, got: %v", err)
	}
}

func TestParseRulesFile_Invalid(t *testing.T) {
	_, err := ParseRulesFile(filepath.Join("testdata", "rules", "invalid.yml"))
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestParseRulesFile_NotFound(t *testing.T) {
	_, err := ParseRulesFile(filepath.Join("testdata", "rules", "does-not-exist.yml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParseRulesDir(t *testing.T) {
	refs, err := ParseRulesDir(filepath.Join("testdata", "rules"))
	if err != nil {
		t.Fatalf("ParseRulesDir: %v", err)
	}
	// helm.yml and invalid.yml should be skipped with warning (nothing returned).
	// basic.yml contributes 5 refs, empty.yml contributes 0.
	if len(refs) != 5 {
		t.Errorf("got %d refs across dir, want 5", len(refs))
	}
}

func TestParseRulesDir_SingleFile(t *testing.T) {
	// When path is a file, delegate to ParseRulesFile.
	refs, err := ParseRulesDir(filepath.Join("testdata", "rules", "basic.yml"))
	if err != nil {
		t.Fatalf("ParseRulesDir: %v", err)
	}
	if len(refs) != 5 {
		t.Errorf("got %d refs, want 5", len(refs))
	}
}

func TestParseRulesDir_SortedDeterministic(t *testing.T) {
	// Parse twice; location set must match. (We don't guarantee output order
	// is stable within a file, but the set of refs is.)
	a, err := ParseRulesDir(filepath.Join("testdata", "rules"))
	if err != nil {
		t.Fatalf("ParseRulesDir(1): %v", err)
	}
	b, err := ParseRulesDir(filepath.Join("testdata", "rules"))
	if err != nil {
		t.Fatalf("ParseRulesDir(2): %v", err)
	}
	locs := func(refs []model.Reference) []string {
		out := make([]string, len(refs))
		for i, r := range refs {
			out[i] = r.Location
		}
		sort.Strings(out)
		return out
	}
	la, lb := locs(a), locs(b)
	if len(la) != len(lb) {
		t.Fatalf("ref set size differs: %d vs %d", len(la), len(lb))
	}
	for i := range la {
		if la[i] != lb[i] {
			t.Errorf("ref set differs at [%d]: %q vs %q", i, la[i], lb[i])
		}
	}
}

func TestIsRuleFile(t *testing.T) {
	cases := map[string]bool{
		"rules.yml":       true,
		"rules.yaml":      true,
		"rules.YML":       true,
		"rules.json":      false,
		"rules":           false,
		"rules.yml.bak":   false,
		".hidden.yml":     true,
		"":                false,
	}
	for input, want := range cases {
		if got := isRuleFile(input); got != want {
			t.Errorf("isRuleFile(%q) = %v, want %v", input, got, want)
		}
	}
}
