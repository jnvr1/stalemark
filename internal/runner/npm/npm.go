// Package npm checks npm, pnpm, yarn and bun script invocations against the
// "scripts" of the package.json they would run in.
//
// ponytail: npx is not resolved. npx fetches missing packages from the
// registry, so a bin absent from node_modules is never a precise finding.
package npm

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/runner"
	"github.com/jnvr1/stalemark/internal/shellx"
)

// RuleScriptNotFound is reported when a script is missing from package.json.
const RuleScriptNotFound = "SM-A001"

// Resolver implements runner.Resolver for the npm family of tools.
type Resolver struct{}

// New returns the npm resolver.
func New() Resolver { return Resolver{} }

// Names lists the tools this resolver handles.
func (Resolver) Names() []string { return []string{"npm", "pnpm", "yarn", "bun"} }

// Resolve reports a finding when cmd runs a script that package.json lacks.
// Anything ambiguous (no package.json, unreadable JSON, unknown workspace,
// a bare word that may be a built-in) yields nothing: precision first.
func (Resolver) Resolve(ctx runner.Context, doc string, cmd shellx.Command) []finding.Finding {
	inv := parse(cmd.Name, cmd.Args)
	if inv.script == "" {
		return nil
	}
	dir := runner.WorkDir(ctx, cmd)
	pkgPath := filepath.Join(dir, "package.json")
	if inv.selector != "" {
		pkgPath = workspace(dir, inv.selector)
	}
	if pkgPath == "" {
		return nil
	}
	pkg, ok := readPackage(pkgPath)
	if !ok {
		return nil
	}
	if _, has := pkg.Scripts[inv.script]; has {
		return nil
	}
	if inv.bare && len(pkg.Scripts) == 0 {
		return nil
	}
	if cmd.Name == "npm" && inv.script == "start" && exists(filepath.Join(dir, "server.js")) {
		return nil // npm start falls back to node server.js
	}

	rel, err := filepath.Rel(ctx.Root, pkgPath)
	if err != nil {
		rel = pkgPath
	}
	names := slices.Sorted(maps.Keys(pkg.Scripts))
	f := finding.Finding{
		Rule:       RuleScriptNotFound,
		Severity:   finding.Error,
		Path:       doc,
		Line:       cmd.FileLine,
		Subject:    inv.script,
		Suggestion: runner.Closest(inv.script, names),
	}
	f.Message = fmt.Sprintf("%s script %q not found in %s", cmd.Name, inv.script, filepath.ToSlash(rel))
	if f.Suggestion != "" {
		f.Message += fmt.Sprintf(" (did you mean %q?)", f.Suggestion)
	}
	return []finding.Finding{f}
}

// invocation is what matters of a tool call: the script it runs, the
// workspace it targets and whether the script came from a bare "yarn X".
type invocation struct {
	script, selector string
	bare             bool
}

// npmBuiltins are the npm commands that run the script of the same name.
var npmBuiltins = map[string]bool{"test": true, "start": true, "stop": true, "restart": true}

// shorthandBuiltins are words after a bare "yarn X" / "pnpm X" that are tool
// commands, not scripts. Conservative on purpose: when in doubt, no finding.
var shorthandBuiltins = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`install i add remove rm init create up upgrade update dlx exec link unlink
		publish pack info list ls why outdated audit cache config set get bin workspace workspaces dedupe
		import patch patch-commit store env setup help version login logout test start build dev lint node run`) {
		shorthandBuiltins[w] = true
	}
}

// parse locates the script name in args, consuming workspace selectors
// wherever they appear and skipping other options. Parsing stops at "--".
func parse(tool string, args []string) invocation {
	var inv invocation
	run := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return inv
		case a == "-w" || a == "--workspace" || a == "--filter" || a == "-F":
			if i+1 < len(args) {
				inv.selector = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--workspace=") || strings.HasPrefix(a, "--filter="):
			inv.selector = a[strings.Index(a, "=")+1:]
		case strings.HasPrefix(a, "-"):
		case inv.script != "": // positional after the script: an argument to it
		case !run && (a == "run" || a == "run-script"):
			run = true
		case run:
			inv.script = a
		case tool == "npm" && npmBuiltins[a]:
			inv.script = a
		case (tool == "yarn" || tool == "pnpm") && !shorthandBuiltins[a]:
			inv.script, inv.bare = a, true
		default:
			return inv // a tool command such as install, or bun's own commands
		}
	}
	return inv
}

type pkgJSON struct {
	Name    string            `json:"name"`
	Scripts map[string]string `json:"scripts"`
}

func readPackage(path string) (pkgJSON, bool) {
	var pkg pkgJSON
	src, err := os.ReadFile(path)
	if err != nil {
		return pkg, false
	}
	return pkg, json.Unmarshal(src, &pkg) == nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// workspace returns the package.json the selector points at: a path when the
// selector looks like one, otherwise the first package (max depth 4, skipping
// node_modules and .git) whose "name" equals it. "" when not found.
func workspace(dir, selector string) string {
	if strings.ContainsAny(selector, "/.") {
		p := filepath.Join(dir, filepath.FromSlash(selector), "package.json")
		if exists(p) {
			return p
		}
		return ""
	}
	found := ""
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".git" || strings.Count(rel, string(filepath.Separator)) >= 4 {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() != "package.json" {
			return nil
		}
		if pkg, ok := readPackage(path); ok && pkg.Name == selector {
			found = path
			return fs.SkipAll
		}
		return nil
	})
	return found
}
