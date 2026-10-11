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
	dir := filepath.Join(runner.WorkDir(ctx, cmd), filepath.FromSlash(inv.dir))
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
	names := slices.Sorted(maps.Keys(pkg.Scripts))
	suggestion := runner.Closest(inv.script, names)
	if cmd.Name == "yarn" || cmd.Name == "bun" || inv.bare {
		// "yarn X", "yarn run X", "pnpm X" and "bun run X" fall through to
		// node_modules/.bin/X, so a missing script is only a finding on typo
		// evidence (a similar script exists) and when X is no declared dependency.
		if suggestion == "" || dependsOn(ctx.Root, pkgPath, inv.script) {
			return nil
		}
	}
	if cmd.Name == "npm" && (inv.script == "start" || inv.script == "restart") {
		// npm restart runs stop + start; npm start falls back to node server.js.
		if _, has := pkg.Scripts["start"]; has || exists(filepath.Join(filepath.Dir(pkgPath), "server.js")) {
			return nil
		}
	}

	f := finding.Finding{
		Rule:       RuleScriptNotFound,
		Severity:   finding.Error,
		Path:       doc,
		Line:       cmd.FileLine,
		Subject:    inv.script,
		Suggestion: suggestion,
	}
	f.Message = fmt.Sprintf("%s script %q not found in %s", cmd.Name, inv.script, runner.Rel(ctx, pkgPath))
	if f.Suggestion != "" {
		f.Message += fmt.Sprintf(" (did you mean %q?)", f.Suggestion)
	}
	return []finding.Finding{f}
}

// invocation is what matters of a tool call: the script it runs, the
// workspace it targets, the directory a --cwd/-C/--dir/--prefix option
// points at and whether the script came from a bare "yarn X".
type invocation struct {
	script, selector, dir string
	bare                  bool
}

// dirOptions take a directory the tool switches to before running.
var dirOptions = map[string]bool{"--cwd": true, "-C": true, "--dir": true, "--prefix": true}

// npmBuiltins are the npm commands that run the script of the same name.
var npmBuiltins = map[string]bool{"test": true, "start": true, "stop": true, "restart": true}

// shorthandBuiltins are words after a bare "yarn X" / "pnpm X" that are tool
// commands, not scripts. Conservative on purpose: when in doubt, no finding.
var shorthandBuiltins = map[string]bool{
	"install": true, "i": true, "add": true, "remove": true, "rm": true, "init": true, "create": true,
	"up": true, "upgrade": true, "update": true, "dlx": true, "exec": true, "link": true, "unlink": true,
	"publish": true, "pack": true, "info": true, "list": true, "ls": true, "why": true, "outdated": true,
	"audit": true, "cache": true, "config": true, "set": true, "get": true, "bin": true, "workspace": true,
	"workspaces": true, "dedupe": true, "import": true, "patch": true, "patch-commit": true, "store": true,
	"env": true, "setup": true, "help": true, "version": true, "login": true, "logout": true, "test": true,
	"start": true, "build": true, "dev": true, "lint": true, "node": true, "run": true,
}

// multiTarget flags run the script in every workspace or tolerate a missing
// one, so the package.json at hand need not define it: never a finding.
var multiTarget = map[string]bool{
	"-r": true, "--recursive": true, "--parallel": true, "--workspaces": true, "-ws": true, "--if-present": true,
}

// parse locates the script name in args, consuming workspace selectors
// wherever they appear and skipping other options. Parsing stops at "--".
func parse(tool string, args []string) invocation {
	var inv invocation
	run, sawOpt := false, false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return inv
		case multiTarget[a]:
			return invocation{}
		case dirOptions[a]:
			if i+1 < len(args) {
				inv.dir = args[i+1]
				i++
			}
		case a == "-w" || a == "--workspace" || a == "--filter" || a == "-F":
			if i+1 < len(args) {
				inv.selector = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--workspace=") || strings.HasPrefix(a, "--filter="):
			inv.selector = a[strings.Index(a, "=")+1:]
		case strings.HasPrefix(a, "-"):
			sawOpt = true
		case inv.script != "": // positional after the script: an argument to it
		case !run && (a == "run" || a == "run-script"):
			run = true
		case run && tool == "bun" && strings.ContainsAny(a, "./"):
			return inv // bun run <file>: a path, not a script
		case run:
			inv.script = a
		case tool == "npm" && npmBuiltins[a]:
			inv.script = a
		case (tool == "yarn" || tool == "pnpm") && !shorthandBuiltins[a]:
			if sawOpt {
				// ponytail: an unknown option (--loglevel x) may have consumed
				// this word as its value; a bare shorthand after it is a guess.
				return inv
			}
			inv.script, inv.bare = a, true
		default:
			return inv // a tool command such as install, or bun's own commands
		}
	}
	return inv
}

type pkgJSON struct {
	Name            string                     `json:"name"`
	Scripts         map[string]string          `json:"scripts"`
	Dependencies    map[string]json.RawMessage `json:"dependencies"`
	DevDependencies map[string]json.RawMessage `json:"devDependencies"`
	OptionalDeps    map[string]json.RawMessage `json:"optionalDependencies"`
	PeerDeps        map[string]json.RawMessage `json:"peerDependencies"`
}

// binPackages maps bins whose name differs from their package's to it.
// ponytail: bins with unrelated names and a near-miss script are still
// flagged; upgrade path is reading node_modules/.bin when present.
var binPackages = map[string]string{
	"tsc": "typescript", "ng": "@angular/cli", "nest": "@nestjs/cli",
	"playwright": "@playwright/test", "biome": "@biomejs/biome",
}

// provides reports whether a dependency named dep ships the bin name.
func provides(dep, name string) bool {
	return dep == name || strings.HasSuffix(dep, "/"+name) || dep == name+"-cli" || dep == binPackages[name]
}

// dependsOn reports whether name is a dependency of the package.json at
// pkgPath or of any ancestor one up to root: hoisted bins resolve from there.
func dependsOn(root, pkgPath, name string) bool {
	for dir := filepath.Dir(pkgPath); ; dir = filepath.Dir(dir) {
		if pkg, ok := readPackage(filepath.Join(dir, "package.json")); ok {
			for _, deps := range []map[string]json.RawMessage{pkg.Dependencies, pkg.DevDependencies, pkg.OptionalDeps, pkg.PeerDeps} {
				for dep := range deps {
					if provides(dep, name) {
						return true
					}
				}
			}
		}
		if rel, err := filepath.Rel(root, dir); err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return false
		}
	}
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
	if !strings.HasPrefix(selector, "@") && strings.ContainsAny(selector, "/.") { // @scope/name is a name
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
