package commands

import (
	"fmt"
	"io"
	"time"
)

// progress is a tiny stage-based progress reporter for the analyze pipeline.
//
// Enabled via --progress. When disabled, every method is a no-op. Output is
// stage-oriented (not a spinner or percentage bar) because our phases are
// discrete and short; seeing "parsing Grafana dashboards... done (188 refs)"
// is more useful to a 3AM on-call than a wheel spinning.
type progress struct {
	w         io.Writer
	enabled   bool
	stageStart time.Time
	stageName  string
}

// newProgress returns a progress reporter writing to w. When enabled is
// false all operations are no-ops.
func newProgress(w io.Writer, enabled bool) *progress {
	return &progress{w: w, enabled: enabled}
}

// stage announces the start of a named pipeline phase. Any in-flight stage
// is implicitly completed (with an empty detail) so callers don't have to
// pair every stage() with a done().
func (p *progress) stage(name string) {
	if !p.enabled {
		return
	}
	// Close any open stage without extra detail.
	if p.stageName != "" {
		p.finishNoDetail()
	}
	p.stageName = name
	p.stageStart = time.Now()
	fmt.Fprintf(p.w, "[progress] %s...\n", name)
}

// done closes the current stage, printing a final line with optional
// detail (e.g., counts). Safe to call when no stage is active.
func (p *progress) done(detail string) {
	if !p.enabled || p.stageName == "" {
		return
	}
	elapsed := time.Since(p.stageStart).Round(time.Millisecond)
	if detail != "" {
		fmt.Fprintf(p.w, "[progress] %s done in %s\n%s\n", p.stageName, elapsed, detail)
	} else {
		fmt.Fprintf(p.w, "[progress] %s done in %s\n", p.stageName, elapsed)
	}
	p.stageName = ""
}

// finishNoDetail is an internal helper that closes a stage without detail.
func (p *progress) finishNoDetail() {
	elapsed := time.Since(p.stageStart).Round(time.Millisecond)
	fmt.Fprintf(p.w, "[progress] %s done in %s\n", p.stageName, elapsed)
	p.stageName = ""
}
