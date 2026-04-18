// Package commands wires cobra subcommands for metricops.
//
// Each subcommand file owns flag parsing and orchestration; business logic
// lives in internal/parse, internal/analyze, internal/render.
package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/donchanee/metricops/internal/builder"
	"github.com/donchanee/metricops/internal/model"
	"github.com/donchanee/metricops/internal/parse"
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

// runAnalyze is the pipeline orchestrator.
//
// Stage status (W2):
//
//	parse  → DONE (tsdb + grafana + rules)
//	build  → DONE (references attributed via promqlx extraction)
//	analyze → TODO (week 3: unused + cardinality + cost)
//	render → TODO (week 3: markdown + json)
//
// For W2, after the model is built we print a raw summary to stderr so the
// full parse→build pipeline can be exercised end-to-end. The markdown/JSON
// report lands in W3.
func runAnalyze(f *analyzeFlags) error {
	// 1. TSDB metrics (required).
	metrics, err := parse.ParseTSDBAnalyzeFile(f.tsdbPath)
	if err != nil {
		return WithExitCode(fmt.Errorf("tsdb: %w", err), ExitUsage)
	}

	// 2. References from dashboards and/or rules. Both optional, at least
	//    one recommended. In strict mode, a parse error is fatal; otherwise
	//    warn + continue.
	var refs []model.Reference
	if f.grafanaPath != "" {
		r, err := parse.ParseGrafanaDir(f.grafanaPath)
		if err != nil {
			if f.strict {
				return WithExitCode(fmt.Errorf("grafana: %w", err), ExitUsage)
			}
			fmt.Fprintf(os.Stderr, "warning: grafana parse: %v\n", err)
		}
		refs = append(refs, r...)
	}
	if f.rulesPath != "" {
		r, err := parse.ParseRulesDir(f.rulesPath)
		if err != nil {
			if f.strict {
				return WithExitCode(fmt.Errorf("rules: %w", err), ExitUsage)
			}
			fmt.Fprintf(os.Stderr, "warning: rules parse: %v\n", err)
		}
		refs = append(refs, r...)
	}
	if f.grafanaPath == "" && f.rulesPath == "" {
		fmt.Fprintln(os.Stderr,
			"warning: no --grafana or --rules provided; every metric will be reported as unused")
	}

	// 3. Build the model by attributing references to metrics via promqlx.
	m, br, err := builder.Build(metrics, refs, builder.Options{Strict: f.strict})
	if err != nil {
		return WithExitCode(err, ExitFindings)
	}

	// 4. W2 summary (W3 replaces this with a real rendered report).
	unused := 0
	for _, metric := range m.Metrics {
		if !metric.IsUsed() {
			unused++
		}
	}
	fmt.Fprintln(os.Stderr, "=== metricops analyze (W2 summary — real report in W3) ===")
	fmt.Fprintf(os.Stderr, "  metrics (TSDB):            %d\n", len(m.Metrics))
	fmt.Fprintf(os.Stderr, "  total active series:       %d\n", m.TotalActiveSeries)
	fmt.Fprintf(os.Stderr, "  references parsed:         %d\n", len(refs))
	fmt.Fprintf(os.Stderr, "    attributed to metric:    %d\n", br.ReferencesAttributed)
	fmt.Fprintf(os.Stderr, "    orphaned (metric gone):  %d\n", br.ReferencesOrphaned)
	fmt.Fprintf(os.Stderr, "    unparseable expr:        %d\n", br.ReferencesUnparseable)
	fmt.Fprintf(os.Stderr, "  metrics flagged unused:    %d / %d\n", unused, len(m.Metrics))

	// Week 3 will replace this with render.Markdown / render.JSON.
	return nil
}
