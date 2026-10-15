// Package cli implements the stalemark command-line interface.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/jnvr1/stalemark/internal/adr"
	"github.com/jnvr1/stalemark/internal/adrrefs"
	"github.com/jnvr1/stalemark/internal/config"
	"github.com/jnvr1/stalemark/internal/envvars"
	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/markdown"
	"github.com/jnvr1/stalemark/internal/pathref"
	"github.com/jnvr1/stalemark/internal/ports"
	"github.com/jnvr1/stalemark/internal/report"
	"github.com/jnvr1/stalemark/internal/runner"
	"github.com/jnvr1/stalemark/internal/runner/gocargo"
	"github.com/jnvr1/stalemark/internal/runner/justfile"
	"github.com/jnvr1/stalemark/internal/runner/makefile"
	"github.com/jnvr1/stalemark/internal/runner/npm"
	"github.com/jnvr1/stalemark/internal/runner/pyproject"
	"github.com/jnvr1/stalemark/internal/runner/taskfile"
	"github.com/jnvr1/stalemark/internal/toolver"
)

// version is injected at build time via -ldflags "-X github.com/jnvr1/stalemark/internal/cli.version=...".
var version = "dev"

const usage = `Usage:
  stalemark check [--format text|json|sarif|github] [path]
                           check Markdown docs against the repository (default path: .)
  stalemark baseline [path]
                           write the baseline file from the current findings
  stalemark --version      print version and exit

Exit codes: 0 clean (notes do not fail the run), 1 findings, 2 tool error.
`

// Run executes the CLI with the given arguments and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	report.ToolVersion = version
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
	case "baseline":
		return baseline(fs.Args()[1:], stdout, stderr)
	default:
		fs.Usage()
		return 2
	}
}

func check(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("stalemark check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, usage) }
	format := flags.String("format", "text", "output format (text|json|sarif|github)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	render, ok := report.Formats[*format]
	if !ok {
		fmt.Fprintf(stderr, "stalemark: unknown format %q\n", *format)
		return 2
	}

	root, cfg, all, err := pipeline(flags.Arg(0))
	var base map[string]config.Entry
	if err == nil {
		base, err = config.ReadBaseline(filepath.Join(root, cfg.Baseline))
	}
	if err != nil {
		fmt.Fprintln(stderr, "stalemark:", err)
		return 2
	}

	var reported []finding.Finding
	matched := map[string]bool{}
	for _, f := range all {
		if _, ok := base[f.Fingerprint()]; ok {
			matched[f.Fingerprint()] = true
		} else {
			reported = append(reported, f)
		}
	}
	// ponytail: checking a single file reports every other file's baseline entry as stale; scope the notice to the checked docs if that gets noisy.
	for _, fp := range slices.Sorted(maps.Keys(base)) {
		if !matched[fp] {
			e := base[fp]
			fmt.Fprintf(stderr, "stalemark: notice: baseline entry %s (%s %s %s) no longer matches\n", fp, e.Rule, e.File, e.Subject)
		}
	}

	if err := render(stdout, reported); err != nil {
		fmt.Fprintln(stderr, "stalemark:", err)
		return 2
	}
	// Notes are reported but do not fail the run.
	if slices.ContainsFunc(reported, func(f finding.Finding) bool { return f.Severity != finding.Note }) {
		return 1
	}
	return 0
}

// baseline writes every current finding (after rule disabling, before any
// baseline filtering) to the configured baseline file.
func baseline(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("stalemark baseline", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, usage) }
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	root, cfg, all, err := pipeline(flags.Arg(0))
	var entries map[string]config.Entry
	if err == nil {
		path := filepath.Join(root, cfg.Baseline)
		if err = config.WriteBaseline(path, all); err == nil {
			entries, err = config.ReadBaseline(path)
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, "stalemark:", err)
		return 2
	}
	noun := "entries"
	if len(entries) == 1 {
		noun = "entry"
	}
	fmt.Fprintf(stdout, "wrote %d %s to %s\n", len(entries), noun, filepath.ToSlash(cfg.Baseline))
	return 0
}

// pipeline loads the config, finds the docs of path ("" means ".") and runs
// every check on them, dropping the findings of disabled rules. A file path
// checks only that file, with its directory as root.
func pipeline(path string) (root string, cfg config.Config, all []finding.Finding, err error) {
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", cfg, nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", cfg, nil, err
	}
	root, docs := abs, []string(nil)
	if !info.IsDir() {
		root, docs = filepath.Dir(abs), []string{abs}
	}
	if cfg, err = config.Load(root); err != nil {
		return "", cfg, nil, err
	}
	if info.IsDir() {
		if docs, err = cfg.Docs(root); err != nil {
			return "", cfg, nil, err
		}
	}

	reg := runner.New(npm.New(), makefile.New(), taskfile.New(), justfile.New(), pyproject.New(), gocargo.New())
	ctx := runner.Context{Root: root}
	documented := map[string]envvars.Loc{}
	for _, p := range docs {
		d, err := markdown.Load(p)
		if err != nil {
			return "", cfg, nil, err
		}
		all = append(all, reg.CheckDoc(ctx, d)...)
		all = append(all, pathref.Check(ctx, d)...)
		all = append(all, envvars.Check(ctx, d)...)
		all = append(all, ports.Check(ctx, d)...)
		all = append(all, toolver.Check(ctx, d)...)
		for name, loc := range envvars.Documented(ctx, d) {
			if _, ok := documented[name]; !ok {
				documented[name] = loc
			}
		}
	}
	// ponytail: the code pass runs only on a directory check; a single-file check sees one doc, so "undocumented" would be noise.
	if info.IsDir() {
		reads, err := envvars.ScanCode(root)
		if err != nil {
			return "", cfg, nil, err
		}
		all = append(all, envvars.CheckCode(documented, reads, cfg.Env.DocumentedUnused)...)
	}
	if info.IsDir() {
		adrs, err := adr.Load(root, cfg.ADR.Dirs)
		if err != nil {
			return "", cfg, nil, err
		}
		all = append(all, adr.CheckSupersedes(ctx, adrs)...)
		scanned := map[string]bool{}
		for _, p := range docs {
			scanned[runner.Rel(ctx, p)] = true
		}
		all = adrrefs.Check(ctx, adrs, scanned, cfg.ADR.Historical, all)
	}
	all = slices.DeleteFunc(all, func(f finding.Finding) bool { return slices.Contains(cfg.Disable, f.Rule) })
	return root, cfg, all, nil
}
