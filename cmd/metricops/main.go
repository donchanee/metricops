// Command metricops is a Prometheus metric governance CLI.
//
// See README.md and `metricops --help` for usage.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/donchanee/metricops/internal/commands"
)

// Build metadata. Populated via -ldflags at release time:
//
//	go build -ldflags "-X main.version=$TAG -X main.commit=$(git rev-parse --short HEAD)"
var (
	version = "0.0.0-dev"
	commit  = "unknown"
)

func main() {
	root := &cobra.Command{
		Use:           "metricops",
		Short:         "Prometheus metric governance CLI",
		Long:          "metricops analyzes a self-hosted Prometheus deployment and reports unused metrics, cardinality hotspots, and estimated cost savings.",
		Version:       fmt.Sprintf("%s (commit %s)", version, commit),
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(commands.NewAnalyzeCommand())
	root.AddCommand(commands.NewValidateCommand())
	root.AddCommand(commands.NewDiffCommand())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitCodeFor(err))
	}
}

// exitCodeFor maps sentinel error types to CLI exit codes, per the eng review:
//
//	0  success
//	1  findings with --fail-on set, or --strict parse warnings
//	2  invalid flags, file not found, permission denied
//
// Sentinel types are defined in internal/commands.
func exitCodeFor(err error) int {
	switch commands.ExitCodeOf(err) {
	case commands.ExitFindings:
		return 1
	case commands.ExitUsage:
		return 2
	default:
		return 1
	}
}
