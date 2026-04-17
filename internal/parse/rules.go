package parse

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/prometheus/prometheus/model/rulefmt"

	"github.com/donchanee/metricops/internal/model"
)

// helmMarkers are Helm-specific template tokens that break YAML parsing.
// These are disjoint from Prometheus's own annotation templating (which
// uses {{ $labels }}, {{ $value }}, {{ printf }}, etc. — all legal inside
// YAML string values).
//
// We only classify a file as "Helm" if the YAML parser has already failed
// AND one of these markers is present. That avoids false positives on
// Prometheus alerts with templated annotations.
var helmMarkers = []string{
	".Values.",
	".Release.",
	".Chart.",
	"{{- ",
	" -}}",
	"{{/*",
	"{{ include ",
	"{{ tpl ",
	"{{ required ",
}

// ParseRulesDir reads all *.yml and *.yaml files at the top level of path
// and returns the references for alerts and recording rules found.
//
// If path is a single file, only that file is parsed. Per-file parse errors
// are non-fatal: a stderr warning is emitted and parsing continues with
// remaining files. Callers that want strict behavior should watch stderr or
// wrap this and check for any warnings.
//
// Reference construction:
//
//	alert rules     → Source=RefAlert,     Location="<file>#alert:<name>"
//	recording rules → Source=RefRecording, Location="<file>#record:<name>"
//
// The Expr is captured verbatim from the YAML. Metric-name extraction is
// performed later by promqlx.MetricNames on the returned references.
func ParseRulesDir(path string) ([]model.Reference, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.IsDir() {
		return ParseRulesFile(path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", path, err)
	}

	var out []model.Reference
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !isRuleFile(name) {
			continue
		}
		full := filepath.Join(path, name)
		refs, err := ParseRulesFile(full)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", full, err)
			continue
		}
		out = append(out, refs...)
	}
	return out, nil
}

// ParseRulesFile parses a single rule YAML file.
//
// Helm-templated files (containing `{{`) are rejected with a descriptive
// error so callers can document "run helm template first".
//
// Files with zero rules (e.g., only `groups: []`) are valid and return
// an empty slice with no error.
func ParseRulesFile(path string) ([]model.Reference, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	groups, errs := rulefmt.Parse(data)
	if len(errs) > 0 {
		if isHelmTemplated(data) {
			return nil, fmt.Errorf("contains Helm template markers; run `helm template` first")
		}
		// Return the first error; rulefmt typically reports one-per-rule.
		return nil, fmt.Errorf("parse: %w", errs[0])
	}

	var refs []model.Reference
	for _, g := range groups.Groups {
		for _, r := range g.Rules {
			expr := strings.TrimSpace(r.Expr.Value)
			if expr == "" {
				continue
			}
			switch {
			case r.Alert.Value != "":
				refs = append(refs, model.Reference{
					Source:   model.RefAlert,
					Location: fmt.Sprintf("%s#alert:%s", path, r.Alert.Value),
					Expr:     expr,
				})
			case r.Record.Value != "":
				refs = append(refs, model.Reference{
					Source:   model.RefRecording,
					Location: fmt.Sprintf("%s#record:%s", path, r.Record.Value),
					Expr:     expr,
				})
			}
		}
	}
	return refs, nil
}

// isRuleFile reports whether a filename looks like a rule YAML.
func isRuleFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml")
}

// isHelmTemplated reports whether the bytes look like an unrendered Helm chart.
// Called only after rulefmt.Parse already failed, so false-positives are
// bounded to files that are already broken.
func isHelmTemplated(data []byte) bool {
	s := string(data)
	for _, m := range helmMarkers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}
