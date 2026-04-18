// Package commands wires cobra subcommands for metricops.
//
// Each subcommand file owns flag parsing and orchestration; business logic
// lives in internal/parse, internal/analyze, internal/render.
package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/donchanee/metricops/internal/analyze"
	"github.com/donchanee/metricops/internal/builder"
	"github.com/donchanee/metricops/internal/model"
	"github.com/donchanee/metricops/internal/parse"
	"github.com/donchanee/metricops/internal/render"
)

// Exit code sentinels. The main package maps these to process exit codes per
// the eng review:
//
//	0  success
//	1  ExitFindings  — analysis ran, but --fail-on matched OR --strict warnings
//	2  ExitUsage     — invalid flags, missing files, permission denied
type ExitCode int

const (
	ExitSuccess ExitCode = iota
	ExitFindings
	ExitUsage
)

// Error type that carries an ExitCode.
type codedError struct {
	err  error
	code ExitCode
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

// WithExitCode wraps err with an ExitCode for the process to emit.
func WithExitCode(err error, code ExitCode) error {
	if err == nil {
		return nil
	}
	return &codedError{err: err, code: code}
}

// ExitCodeOf extracts the ExitCode from an error, defaulting to ExitFindings.
func ExitCodeOf(err error) ExitCode {
	var ce *codedError
	if errors.As(err, &ce) {
		return ce.code
	}
	return ExitFindings
}

// analyzeFlags holds all flags for `metricops analyze`.
type analyzeFlags struct {
	tsdbPath       string
	grafanaPath    string
	rulesPath      string
	format         string
	schema         string
	strict         bool
	bytesPerSample float64
	failOn         string
	timeout        string
	progress       bool
}

// NewAnalyzeCommand constructs the `analyze` subcommand.
//
// Flag design is locked by the eng review:
//   - --tsdb, --grafana, --rules each accept file or directory (auto-detect)
//   - --format: markdown (default) or json
//   - --schema: JSON schema version (default 1.0)
//   - --strict: promote per-file parse warnings to exit 1
//   - --bytes-per-sample: cost estimation constant (default 2.0)
//   - --fail-on: CI gating (e.g., "findings" exits 1 on any findings)
//   - --timeout: analysis deadline (default 5m)
//   - --progress: show progress on stderr (for large inputs)
func NewAnalyzeCommand() *cobra.Command {
	f := &analyzeFlags{}

	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze a Prometheus deployment for metric governance issues",
		Long: `Analyze produces a governance report listing unused metrics,
cardinality hotspots, and estimated cost savings.

Inputs:
  --tsdb      output of 'promtool tsdb analyze' (file, or '-' for stdin)
  --grafana   Grafana dashboard JSON (file or directory, auto-detected)
  --rules     Prometheus alert/recording rule YAML (file or directory)

Output format: markdown (default) or json, via --format.
For CI integration, see --fail-on and --strict.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAnalyze(f)
		},
	}

	cmd.Flags().StringVar(&f.tsdbPath, "tsdb", "", "promtool tsdb analyze output (file or '-' for stdin)")
	cmd.Flags().StringVar(&f.grafanaPath, "grafana", "", "Grafana dashboard JSON file or directory")
	cmd.Flags().StringVar(&f.rulesPath, "rules", "", "Prometheus rule YAML file or directory")
	cmd.Flags().StringVar(&f.format, "format", "markdown", "output format: markdown or json")
	cmd.Flags().StringVar(&f.schema, "schema", "1.0", "JSON schema version")
	cmd.Flags().BoolVar(&f.strict, "strict", false, "promote per-file parse warnings to exit 1")
	cmd.Flags().Float64Var(&f.bytesPerSample, "bytes-per-sample", 2.0, "assumed bytes per sample (compressed TSDB)")
	cmd.Flags().StringVar(&f.failOn, "fail-on", "", "exit 1 if findings of this type exist (e.g. 'findings')")
	cmd.Flags().StringVar(&f.timeout, "timeout", "5m", "analysis timeout (e.g., '5m', '30s')")
	cmd.Flags().BoolVar(&f.progress, "progress", false, "show progress on stderr")

	_ = cmd.MarkFlagRequired("tsdb")

	return cmd
}

// runAnalyze is the entry point for the analyze subcommand. It parses and
// validates flags, then runs the pipeline under an optional timeout.
//
// Timeout strategy: since our per-phase operations are CPU-bound and don't
// hit the network, we wrap the entire pipeline in a goroutine and use a
// context-deadline select. On timeout the main thread returns; the inner
// goroutine may linger briefly but the process exits and frees resources.
// For a short-lived CLI this is acceptable and keeps the implementation
// simple (no ctx plumbing through parsers/analyzers).
func runAnalyze(f *analyzeFlags) error {
	if f.schema != "" && f.schema != model.SchemaVersion {
		return WithExitCode(fmt.Errorf("unsupported --schema=%s (this build supports %s)",
			f.schema, model.SchemaVersion), ExitUsage)
	}

	timeout, err := time.ParseDuration(f.timeout)
	if err != nil {
		return WithExitCode(fmt.Errorf("invalid --timeout=%q: %w", f.timeout, err), ExitUsage)
	}
	if timeout <= 0 {
		return WithExitCode(fmt.Errorf("--timeout must be positive, got %s", timeout), ExitUsage)
	}

	p := newProgress(os.Stderr, f.progress)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runPipeline(p, f)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return WithExitCode(
			fmt.Errorf("analysis timed out after %s (raise --timeout to allow more time)", timeout),
			ExitFindings,
		)
	}
}

// runPipeline executes the actual parse → build → analyze → render stages.
// Split from runAnalyze so the timeout wrapper can stay trivial.
//
// Stage contracts (eng-review locked):
//   - parse/ returns raw metrics and references; never analyzes.
//   - builder/ joins them via promqlx into a *model.Model.
//   - analyze/ runs detectors (unused, hotspots, cost, recommendations).
//   - render/ writes the final Report to stdout in markdown or JSON.
//
// Pipeline-health warnings from builder are written to stderr regardless
// of output format so automation consumers still see them.
func runPipeline(p *progress, f *analyzeFlags) error {
	// 1. TSDB metrics (required).
	p.stage("parsing TSDB analyze output")
	metrics, err := parse.ParseTSDBAnalyzeFile(f.tsdbPath)
	if err != nil {
		return WithExitCode(fmt.Errorf("tsdb: %w", err), ExitUsage)
	}
	p.done(fmt.Sprintf("  %d metrics", len(metrics)))

	// 2. References from dashboards and/or rules. Both optional.
	var refs []model.Reference
	if f.grafanaPath != "" {
		p.stage("parsing Grafana dashboards")
		r, perr := parse.ParseGrafanaDir(f.grafanaPath)
		if perr != nil {
			if f.strict {
				return WithExitCode(fmt.Errorf("grafana: %w", perr), ExitUsage)
			}
			fmt.Fprintf(os.Stderr, "warning: grafana parse: %v\n", perr)
		}
		p.done(fmt.Sprintf("  %d refs", len(r)))
		refs = append(refs, r...)
	}
	if f.rulesPath != "" {
		p.stage("parsing Prometheus rules")
		r, perr := parse.ParseRulesDir(f.rulesPath)
		if perr != nil {
			if f.strict {
				return WithExitCode(fmt.Errorf("rules: %w", perr), ExitUsage)
			}
			fmt.Fprintf(os.Stderr, "warning: rules parse: %v\n", perr)
		}
		p.done(fmt.Sprintf("  %d refs", len(r)))
		refs = append(refs, r...)
	}
	if f.grafanaPath == "" && f.rulesPath == "" {
		fmt.Fprintln(os.Stderr,
			"warning: no --grafana or --rules provided; every metric will be reported as unused")
	}

	// 3. Build model.
	p.stage("building model (PromQL extraction)")
	m, br, err := builder.Build(metrics, refs, builder.Options{Strict: f.strict})
	if err != nil {
		return WithExitCode(err, ExitFindings)
	}
	p.done(fmt.Sprintf("  %d refs attributed, %d orphaned, %d unparseable",
		br.ReferencesAttributed, br.ReferencesOrphaned, br.ReferencesUnparseable))

	// 4. Pipeline-health notes.
	if br.ReferencesOrphaned > 0 {
		fmt.Fprintf(os.Stderr,
			"note: %d references point at metrics not in TSDB (dangling).\n",
			br.ReferencesOrphaned)
	}
	if br.ReferencesUnparseable > 0 && !f.strict {
		fmt.Fprintf(os.Stderr,
			"note: %d references have unparseable PromQL and were skipped. Rerun with --strict to fail fast.\n",
			br.ReferencesUnparseable)
	}

	// 5. Analyze.
	bps := f.bytesPerSample
	if bps <= 0 {
		bps = analyze.DefaultBytesPerSample
	}

	p.stage("running detectors")
	unused, err := analyze.DetectUnused(m, bps)
	if err != nil {
		return WithExitCode(fmt.Errorf("detect unused: %w", err), ExitFindings)
	}
	hotspots, err := analyze.DetectHotspots(m, analyze.HotspotOptions{})
	if err != nil {
		return WithExitCode(fmt.Errorf("detect hotspots: %w", err), ExitFindings)
	}
	summary, err := analyze.EstimateSummary(m, bps)
	if err != nil {
		return WithExitCode(fmt.Errorf("summary: %w", err), ExitFindings)
	}
	recs, err := analyze.BuildRecommendations(unused, hotspots)
	if err != nil {
		return WithExitCode(fmt.Errorf("recommendations: %w", err), ExitFindings)
	}
	p.done(fmt.Sprintf("  %d unused, %d hotspots, %d recommendations",
		len(unused), len(hotspots), len(recs)))

	// 6. Assemble final Report.
	report := &model.Report{
		SchemaVersion:       model.SchemaVersion,
		GeneratedAt:         time.Now().UTC(),
		Summary:             summary,
		UnusedMetrics:       unused,
		CardinalityHotspots: hotspots,
		Recommendations:     recs,
	}

	// 7. Render to stdout.
	p.stage("rendering report")
	switch f.format {
	case "", "markdown", "md":
		if err := render.Markdown(os.Stdout, report); err != nil {
			return WithExitCode(fmt.Errorf("render markdown: %w", err), ExitFindings)
		}
	case "json":
		if err := render.JSON(os.Stdout, report); err != nil {
			return WithExitCode(fmt.Errorf("render json: %w", err), ExitFindings)
		}
	default:
		return WithExitCode(fmt.Errorf("unknown --format=%q (want: markdown|json)", f.format), ExitUsage)
	}
	p.done("")

	// 8. CI gating via --fail-on.
	if f.failOn == "findings" && (len(unused) > 0 || len(hotspots) > 0) {
		return WithExitCode(
			fmt.Errorf("findings present (%d unused, %d hotspots)", len(unused), len(hotspots)),
			ExitFindings,
		)
	}

	return nil
}
