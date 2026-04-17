package parse

import (
	"github.com/donchanee/metricops/internal/model"
)

// ParseRulesDir reads all *.yml and *.yaml rule files under path (non-recursive)
// and returns references for alert and recording rules.
//
// Implementation (eng review):
//   - Use github.com/prometheus/prometheus/model/rulefmt for YAML parsing
//   - Helm-templated files (containing `{{`) produce a stderr warning and are
//     skipped. Users should pipe `helm template | metricops -` if needed.
//   - Alert rules emit references with Source=RefAlert, Location=
//     "<path>#alert:<name>"
//   - Recording rules emit Source=RefRecording, Location=
//     "<path>#record:<record-name>"
//   - Rule files with no rules (empty groups) are valid and return []
//
// If path is a single file, parses just that file. If directory, all
// *.yml and *.yaml in the top level. Errors on individual files are
// non-fatal by default; --strict promotes them to a returned error.
func ParseRulesDir(path string) ([]model.Reference, error) {
	return nil, ErrNotImplemented
}

// ParseRulesFile parses a single rule YAML file.
func ParseRulesFile(path string) ([]model.Reference, error) {
	return nil, ErrNotImplemented
}
