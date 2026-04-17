package render

import (
	"encoding/json"
	"io"

	"github.com/donchanee/metricops/internal/model"
)

// JSON writes the v1.0 JSON schema representation of r to w.
//
// Determinism contract (eng review CI gate):
//   - All slices in r are sorted by the analyzer before this function is
//     called. Renderer does NOT re-sort.
//   - All map iteration in r is avoided; any map-derived fields were
//     materialized into sorted slices earlier.
//   - encoding/json preserves struct field order, which matches the v1.0
//     schema ordering in model.Report.
//   - Indent=2 spaces, compact-ish, never trailing whitespace.
//
// 10 successive runs against the same input must produce byte-identical
// output (enforced by a determinism test in week 4).
func JSON(w io.Writer, r *model.Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
