package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// NewValidateCommand is a v2 placeholder. It exists in v1 so the subcommand
// surface is stable: users scripting `metricops validate` do not get "unknown
// command" once it ships.
func NewValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate rule files and naming conventions (placeholder for v2)",
		Long: `Validate runs linting checks against Prometheus rule files and
dashboard metric naming. Not yet implemented; reserved for v2.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(os.Stderr, "validate: not yet implemented (v2)")
			return WithExitCode(errors.New("validate not implemented"), ExitFindings)
		},
	}
}

// NewDiffCommand is a v2 placeholder for comparing two governance reports.
func NewDiffCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "diff",
		Short: "Compare two governance reports (placeholder for v2)",
		Long: `Diff compares two JSON reports and highlights regressions:
newly unused metrics, newly appearing hotspots, cost deltas.
Not yet implemented; reserved for v2.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(os.Stderr, "diff: not yet implemented (v2)")
			return WithExitCode(errors.New("diff not implemented"), ExitFindings)
		},
	}
}
