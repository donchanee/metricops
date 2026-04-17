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

// runAnalyze is the orchestrator. Week 1 note: each step below gets its own
// function as the parsers land.
//
// Pipeline (all in internal/):
//
//   parse.TSDBAnalyze(tsdbPath)              -> []Metric with cardinality
//   parse.GrafanaDir(grafanaPath) +          -> []Reference
//   parse.RulesDir(rulesPath)
//   model.Build(metrics, refs)               -> *Model
//   analyze.DetectUnused(model)              -> UnusedMetric list
//   analyze.DetectHotspots(model)            -> CardinalityHotspot list
//   analyze.EstimateCost(model, bytesPerSample) -> Summary + per-metric costs
//   render.Markdown(report) | render.JSON(report) -> stdout
//
// See the eng review test plan artifact for detection rules and edge cases.
func runAnalyze(f *analyzeFlags) error {
	// TODO(week-1): implement the pipeline. Until then, return a clear
	// not-implemented error with usage exit code.
	fmt.Fprintln(os.Stderr, "analyze: pipeline not yet implemented (week 1 TODO)")
	fmt.Fprintf(os.Stderr, "  parsed flags: tsdb=%q grafana=%q rules=%q format=%q\n",
		f.tsdbPath, f.grafanaPath, f.rulesPath, f.format)
	return WithExitCode(errors.New("not implemented"), ExitFindings)
}
