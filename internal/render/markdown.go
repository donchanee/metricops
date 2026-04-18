// Package render turns a model.Report into human- or machine-readable
// output. Renderers are pure functions: given a Report they produce bytes.
// They must not allocate non-deterministic state; all ordering comes from
// the caller (analyzers sort their outputs before placing them in Report).
package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/donchanee/metricops/internal/model"
)

// Markdown writes a GitHub-flavored markdown report of r to w.
//
// Section order (locked by eng review):
//
//	# metricops Report
//	Generated: ISO8601 · Schema: X.Y
//
//	## Summary
//	key/value table
//
//	## Unused Metrics
//	table, or "No unused metrics detected." when empty
//
//	## Cardinality Hotspots
//	table, or "No cardinality hotspots detected." when empty
//
//	## Recommendations
//	bullet list, or "No recommendations." when empty
//
// All table ordering comes from the already-sorted slices in Report.
func Markdown(w io.Writer, r *model.Report) error {
	if r == nil {
		return fmt.Errorf("render: nil report")
	}

	b := &strings.Builder{}

	// Header.
	fmt.Fprintln(b, "# metricops Report")
	fmt.Fprintln(b)
	fmt.Fprintf(b, "Generated: %s · Schema: %s\n",
		r.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z"), r.SchemaVersion)
	fmt.Fprintln(b)

	// Summary.
	fmt.Fprintln(b, "## Summary")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "| Metric | Value |")
	fmt.Fprintln(b, "|--------|-------|")
	fmt.Fprintf(b, "| Total metrics | %s |\n", thousands(int64(r.Summary.TotalMetrics)))
	fmt.Fprintf(b, "| Unused metrics | %s |\n", thousands(int64(r.Summary.UnusedMetrics)))
	fmt.Fprintf(b, "| Total active series | %s |\n", thousands(int64(r.Summary.TotalActiveSeries)))
	fmt.Fprintf(b, "| Estimated daily bytes | %s (%s) |\n",
		thousands(r.Summary.EstimatedDailyBytes), humanBytes(r.Summary.EstimatedDailyBytes))
	fmt.Fprintf(b, "| Estimated monthly bytes | %s (%s) |\n",
		thousands(r.Summary.EstimatedMonthlyBytes), humanBytes(r.Summary.EstimatedMonthlyBytes))
	fmt.Fprintf(b, "| Bytes per sample assumed | %.2f |\n", r.Summary.BytesPerSampleAssumed)
	fmt.Fprintln(b)

	// Unused Metrics.
	fmt.Fprintln(b, "## Unused Metrics")
	fmt.Fprintln(b)
	if len(r.UnusedMetrics) == 0 {
		fmt.Fprintln(b, "No unused metrics detected.")
	} else {
		fmt.Fprintf(b, "%d metrics appear in TSDB but are not referenced by any dashboard, alert, or recording rule.\n",
			len(r.UnusedMetrics))
		fmt.Fprintln(b)
		fmt.Fprintln(b, "| Metric | Active Series | Bytes/day |")
		fmt.Fprintln(b, "|--------|--------------:|----------:|")
		for _, u := range r.UnusedMetrics {
			fmt.Fprintf(b, "| `%s` | %s | %s |\n",
				u.Name, thousands(int64(u.ActiveSeries)), thousands(u.BytesPerDayEstimate))
		}
	}
	fmt.Fprintln(b)

	// Cardinality Hotspots.
	fmt.Fprintln(b, "## Cardinality Hotspots")
	fmt.Fprintln(b)
	if len(r.CardinalityHotspots) == 0 {
		fmt.Fprintln(b, "No cardinality hotspots detected.")
	} else {
		fmt.Fprintf(b, "%d metrics each account for a disproportionate share of total active series.\n",
			len(r.CardinalityHotspots))
		fmt.Fprintln(b)
		fmt.Fprintln(b, "| Metric | Cardinality | % of Total |")
		fmt.Fprintln(b, "|--------|------------:|-----------:|")
		for _, h := range r.CardinalityHotspots {
			fmt.Fprintf(b, "| `%s` | %s | %.2f%% |\n",
				h.Metric, thousands(int64(h.Cardinality)), h.PctOfTotalSeries)
		}
	}
	fmt.Fprintln(b)

	// Recommendations.
	fmt.Fprintln(b, "## Recommendations")
	fmt.Fprintln(b)
	if len(r.Recommendations) == 0 {
		fmt.Fprintln(b, "No recommendations.")
	} else {
		var totalSavings int64
		for _, rec := range r.Recommendations {
			totalSavings += rec.EstimatedSavingsBytesPerDay
		}
		fmt.Fprintf(b, "%d actions identified. Total estimated daily savings: %s (%s).\n",
			len(r.Recommendations), thousands(totalSavings), humanBytes(totalSavings))
		fmt.Fprintln(b)
		for _, rec := range r.Recommendations {
			fmt.Fprintf(b, "- `%s` **`%s`** — saves ~%s/day (%s)\n",
				rec.Type, rec.Target,
				thousands(rec.EstimatedSavingsBytesPerDay),
				humanBytes(rec.EstimatedSavingsBytesPerDay))
		}
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// thousands formats an integer with comma separators, e.g. 1234567 → "1,234,567".
func thousands(n int64) string {
	if n < 0 {
		return "-" + thousands(-n)
	}
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
		if len(s) > pre {
			b.WriteByte(',')
		}
	}
	for i := pre; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteByte(',')
		}
	}
	return b.String()
}

// humanBytes returns a human-friendly byte size string like "8.42 MiB".
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return fmt.Sprintf("%.2f %s", float64(n)/float64(div), units[exp])
}
