// Package cli runs the Swale commands.
package cli

import (
	"flag"
	"fmt"
	"io"
)

const rootUsage = `Swale generates Terragrunt units and stacks that reference modules from a catalog.

Usage:
  swale [command]

Commands:
  init      Prepare a repository for Swale.
  scaffold  Generate Terragrunt units and stacks.
`

// Execute runs one Swale command and returns the exit code.
func Execute(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && !isFlag(args[0]) {
		return runCommand(args[0], args[1:], stdout, stderr)
	}
	help, rest, err := parseArgs("swale", args, stderr)
	if err != nil {
		return 1
	}
	if help || len(rest) == 0 {
		fmt.Fprint(stdout, rootUsage)
		return 0
	}
	fmt.Fprintf(stderr, "unknown command %q\n", rest[0])
	return 1
}

func runCommand(name string, args []string, stdout, stderr io.Writer) int {
	switch name {
	case "init", "scaffold":
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", name)
		return 1
	}
	help, rest, err := parseArgs(name, args, stderr)
	if err != nil {
		return 1
	}
	if help {
		fmt.Fprint(stdout, commandUsage(name))
		return 0
	}
	if len(rest) > 0 {
		fmt.Fprintf(stderr, "unexpected argument %q\n", rest[0])
		return 1
	}
	fmt.Fprint(stderr, commandUsage(name))
	fmt.Fprintln(stderr, "not implemented yet")
	return 1
}

func parseArgs(name string, args []string, stderr io.Writer) (bool, []string, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	short := fs.Bool("h", false, "Show usage")
	long := fs.Bool("help", false, "Show usage")
	fs.Usage = func() {}
	if err := fs.Parse(args); err != nil {
		return false, nil, err
	}
	return *short || *long, fs.Args(), nil
}

func commandUsage(name string) string {
	switch name {
	case "init":
		return "Prepare a repository for Swale.\n\nUsage:\n  swale init\n"
	case "scaffold":
		return "Generate Terragrunt units and stacks.\n\nUsage:\n  swale scaffold\n"
	default:
		return ""
	}
}

func isFlag(arg string) bool {
	return len(arg) > 0 && arg[0] == '-'
}
