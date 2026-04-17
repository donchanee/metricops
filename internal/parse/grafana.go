package parse

import (
	"github.com/donchanee/metricops/internal/model"
)

// ParseGrafanaDir reads all *.json files under dir (non-recursive) and
// returns model.References discovered inside panel targets.
//
// Eng review scope (MVP):
//   - Supported panel types: timeseries, stat, table, bargauge
//   - Extracts PromQL from panel.targets[].expr
//   - Ignores panels whose datasource is not Prometheus
//   - SKIPS panels with template variables in the expr (`${var}`) — records
//     a stderr warning per skip, returns nothing for that target
//   - SKIPS repeat panels (dynamic panel repetition)
//   - Does NOT resolve dashboard variables/queries
//   - Does NOT traverse subdirectories
//
// If path is a single file, parses just that file. If directory, all *.json
// in the top level. Errors on individual files are non-fatal: print warning,
// skip file, continue.
//
// Location format for refs: "<relative-path>#panel:<id>"
func ParseGrafanaDir(path string) ([]model.Reference, error) {
	return nil, ErrNotImplemented
}

// ParseGrafanaFile parses a single dashboard JSON file. Exposed so callers
// with one file need not construct a directory.
func ParseGrafanaFile(path string) ([]model.Reference, error) {
	return nil, ErrNotImplemented
}
