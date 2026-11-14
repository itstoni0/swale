// Command swale is the Swale CLI.
package main

import (
	"os"

	"github.com/itstoni0/swale/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
