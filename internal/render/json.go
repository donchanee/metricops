package render

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/donchanee/metricops/internal/model"
)

// JSON writes the v1.0 JSON schema representation of r to w.
//
// Determinism contract (enforced by json_test.go):
//
//   - All slices in r are pre-sorted by the analyzer.
//   - The Report struct has no map fields, so there is no map-iteration
//     nondeterminism at the render stage.
//   - encoding/json emits struct fields in declaration order, which
//     matches the v1.0 schema field ordering in model.Report.
//   - Two successive calls with the same input produce byte-identical
//     output.
//
// Output ends with a trailing newline (encoder default) for tool friendliness
// (shells, jq pipes).
func JSON(w io.Writer, r *model.Report) error {
	if r == nil {
		return fmt.Errorf("render: nil report")
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
