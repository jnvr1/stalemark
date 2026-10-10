// Package cli implements the stalemark command-line interface.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// version is injected at build time via -ldflags "-X github.com/jnvr1/stalemark/internal/cli.version=...".
var version = "dev"

const usage = `Usage:
  stalemark check [path]   check Markdown docs against the repository (default path: .)
  stalemark --version      print version and exit

Exit codes: 0 clean, 1 findings, 2 tool error.
`

// Run executes the CLI with the given arguments and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("stalemark", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	showVersion := fs.Bool("version", false, "print version and exit")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if *showVersion {
		fmt.Fprintf(stdout, "stalemark %s\n", version)
		return 0
	}

	// ponytail: manual dispatch over stdlib flag; move to a subcommand table when there are more than ~3 commands.
	switch fs.Arg(0) {
	case "check":
		return check(fs.Args()[1:], stderr)
	default:
		fs.Usage()
		return 2
	}
}

func check(args []string, stderr io.Writer) int {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	if _, err := os.Stat(path); err != nil {
		fmt.Fprintln(stderr, "stalemark:", err)
		return 2
	}

	// ponytail: stub until the first check (v0.1 commands & paths) lands; always clean.
	return 0
}
