// Package cli implements the stalemark command-line interface.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/markdown"
	"github.com/jnvr1/stalemark/internal/report"
	"github.com/jnvr1/stalemark/internal/runner"
	"github.com/jnvr1/stalemark/internal/runner/gocargo"
	"github.com/jnvr1/stalemark/internal/runner/justfile"
	"github.com/jnvr1/stalemark/internal/runner/makefile"
	"github.com/jnvr1/stalemark/internal/runner/npm"
	"github.com/jnvr1/stalemark/internal/runner/pyproject"
	"github.com/jnvr1/stalemark/internal/runner/taskfile"
)

// version is injected at build time via -ldflags "-X github.com/jnvr1/stalemark/internal/cli.version=...".
var version = "dev"

const usage = `Usage:
  stalemark check [--format text] [path]
                           check Markdown docs against the repository (default path: .)
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
		return check(fs.Args()[1:], stdout, stderr)
	default:
		fs.Usage()
		return 2
	}
}

func check(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("stalemark check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, usage) }
	format := flags.String("format", "text", "output format (text)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *format != "text" {
		fmt.Fprintf(stderr, "stalemark: unknown format %q\n", *format)
		return 2
	}

	path := "."
	if flags.NArg() > 0 {
		path = flags.Arg(0)
	}
	root, docs, err := discover(path)
	if err != nil {
		fmt.Fprintln(stderr, "stalemark:", err)
		return 2
	}

	reg := runner.New(npm.New(), makefile.New(), taskfile.New(), justfile.New(), pyproject.New(), gocargo.New())
	ctx := runner.Context{Root: root}
	var all []finding.Finding
	for _, p := range docs {
		d, err := markdown.Load(p)
		if err != nil {
			fmt.Fprintln(stderr, "stalemark:", err)
			return 2
		}
		all = append(all, reg.CheckDoc(ctx, d)...)
	}

	if err := report.Text(stdout, all); err != nil {
		fmt.Fprintln(stderr, "stalemark:", err)
		return 2
	}
	if len(all) > 0 {
		return 1
	}
	return 0
}

// discover returns the absolute check root and the sorted Markdown files to
// check: path itself when it is a file, otherwise README.md, CONTRIBUTING.md
// and docs/**/*.md under it.
//
// ponytail: hardcoded include set (README.md, CONTRIBUTING.md, docs/**/*.md) until .stalemark.yml (#14) makes it configurable.
func discover(path string) (root string, docs []string, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", nil, err
	}
	if !info.IsDir() {
		return filepath.Dir(abs), []string{abs}, nil
	}

	for _, name := range []string{"README.md", "CONTRIBUTING.md"} {
		if p := runner.Discover(abs, name); p != "" {
			docs = append(docs, p)
		}
	}
	docsDir := filepath.Join(abs, "docs")
	if _, err := os.Stat(docsDir); err == nil {
		err = filepath.WalkDir(docsDir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && filepath.Ext(p) == ".md" {
				docs = append(docs, p)
			}
			return err
		})
		if err != nil {
			return "", nil, err
		}
	}
	slices.Sort(docs)
	return abs, docs, nil
}
