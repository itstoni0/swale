// Package cli builds the Swale command tree.
package cli

import (
	"errors"
	"io"

	"github.com/spf13/cobra"
)

// errNotImplemented is the error for a stub command.
var errNotImplemented = errors.New("not implemented yet")

// Execute runs one Swale command and returns the exit code.
func Execute(args []string, stdout, stderr io.Writer) int {
	cmd := newRoot()
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		return 1
	}
	return 0
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "swale",
		Short: "Generate Terragrunt units and stacks",
		Long:  "Swale generates Terragrunt units and stacks that reference modules from a catalog.",
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.AddCommand(newInit(), newScaffold())
	return root
}

func newInit() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Prepare a repository for Swale",
		RunE:  notImplemented,
	}
}

func newScaffold() *cobra.Command {
	return &cobra.Command{
		Use:   "scaffold",
		Short: "Generate Terragrunt units and stacks",
		RunE:  notImplemented,
	}
}

func notImplemented(*cobra.Command, []string) error {
	return errNotImplemented
}
