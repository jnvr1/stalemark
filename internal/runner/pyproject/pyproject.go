// Package pyproject checks "poetry run", "uv run", "pdm run" and "hatch run"
// invocations against the script tables of the pyproject.toml they would run
// in: [project.scripts], [project.gui-scripts], [tool.poetry.scripts],
// [tool.pdm.scripts] and [tool.hatch.envs.*.scripts].
//
// ponytail: all four tools also run any program installed in the
// environment, so a name missing from the script tables is only a finding on
// typo evidence (a similar script exists) and when it is not a declared
// dependency either. Bins whose name differs from their package's ("dotenv"
// from python-dotenv) are a ceiling; an alias map like npm's binPackages is
// the upgrade path once a probe shows a need.
package pyproject

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/runner"
	"github.com/jnvr1/stalemark/internal/shellx"
)

// RuleScriptNotFound is reported when a script is missing from pyproject.toml.
const RuleScriptNotFound = "SM-A006"

// Resolver implements runner.Resolver for the Python project managers.
type Resolver struct{}

// New returns the pyproject resolver.
func New() Resolver { return Resolver{} }

// Names lists the tools this resolver handles.
func (Resolver) Names() []string { return []string{"poetry", "uv", "pdm", "hatch"} }

// Resolve reports a finding when cmd runs a script that pyproject.toml lacks.
// Anything ambiguous (no or invalid pyproject.toml, a file or module, an
// unknown hatch env, no similar script, a declared dependency) yields
// nothing: precision first.
func (Resolver) Resolve(ctx runner.Context, doc string, cmd shellx.Command) []finding.Finding {
	inv := parse(cmd.Name, cmd.Args)
	if inv.script == "" {
		return nil
	}
	path := runner.Discover(filepath.Join(runner.WorkDir(ctx, cmd), filepath.FromSlash(inv.dir)), "pyproject.toml")
	if path == "" {
		return nil
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var py map[string]any
	if toml.Unmarshal(src, &py) != nil {
		return nil
	}
	env, name := inv.env, inv.script
	if cmd.Name == "hatch" {
		if e, s, ok := strings.Cut(name, ":"); ok {
			env, name = e, s
		}
	}
	names, ok := scripts(py, cmd.Name, env)
	if !ok || slices.Contains(names, name) {
		return nil
	}
	if strings.ContainsAny(name, `/\.`) || name == "python" || strings.HasPrefix(name, "python3") {
		return nil // a file, a module path or the interpreter, never a script
	}
	suggestion := runner.Closest(name, names)
	if suggestion == "" || dependsOn(py, name) {
		return nil
	}
	f := finding.Finding{
		Rule:       RuleScriptNotFound,
		Severity:   finding.Error,
		Path:       doc,
		Line:       cmd.FileLine,
		Subject:    inv.script,
		Suggestion: inv.script[:len(inv.script)-len(name)] + suggestion, // keep the "env:" prefix as typed
	}
	f.Message = fmt.Sprintf("%s script %q not found in %s (did you mean %q?)", cmd.Name, f.Subject, runner.Rel(ctx, path), f.Suggestion)
	return []finding.Finding{f}
}

// invocation is what matters of a tool call: the directory a project option
// points at, the hatch env selected with -e and the first positional after
// "run" (for hatch possibly "env:script").
type invocation struct {
	dir, env, script string
}

// valued lists, per tool, the options whose value is the next argument (or
// attached with "="), with the role of that value: "dir" moves the lookup,
// "env" picks a hatch env. Accepted before and after "run" alike.
//
// ponytail: an unknown valued option ("uv run --foo bar serve") makes "bar"
// the script; the dependency and typo guards keep that from a false finding
// unless "bar" is a near miss of a real script.
var valued = map[string]map[string]string{
	"poetry": {"-C": "dir", "--directory": "dir", "-P": "dir", "--project": "dir"},
	"uv": {
		"--directory": "dir", "--project": "dir",
		"-p": "", "--python": "", "--with": "", "--with-requirements": "", "--with-editable": "",
		"--env-file": "", "--index": "", "--default-index": "", "--index-url": "", "--extra-index-url": "",
		"--find-links": "", "--extra": "", "--group": "", "--only-group": "", "--no-group": "",
		"--exclude-newer": "", "--package": "", "--config-file": "", "--cache-dir": "", "--color": "",
		"--resolution": "", "--prerelease": "", "--link-mode": "", "--index-strategy": "",
		"--keyring-provider": "", "--python-preference": "", "--refresh-package": "",
		"--reinstall-package": "", "--upgrade-package": "", "--no-binary-package": "", "--no-build-package": "",
	},
	"pdm":   {"-p": "dir", "--project": "dir"},
	"hatch": {"-e": "env", "--env": "env", "-p": "", "--python": "", "--data-dir": "", "--cache-dir": "", "--config": "", "--color": ""},
}

// hatchBuiltins are hatch's own commands; "hatch run fmt" names the command
// mistyped as a script, never a script typo.
var hatchBuiltins = map[string]bool{
	"build": true, "clean": true, "config": true, "dep": true, "env": true, "fmt": true, "new": true,
	"project": true, "publish": true, "python": true, "run": true, "self": true, "shell": true,
	"status": true, "test": true, "version": true,
}

// parse locates the script name: the first positional after "run", skipping
// options (with their value when valued) and hatch "+key=value" matrix
// selectors. "--" ends the options; the next token is the script. Any other
// subcommand (install, tool, fmt...) and uv's -m MODULE / --script FILE
// yield an empty invocation.
func parse(tool string, args []string) invocation {
	var inv invocation
	opts := valued[tool]
	run := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		opt, val, hasEq := strings.Cut(a, "=")
		role, isValued := opts[opt]
		switch {
		case a == "--":
			if run && i+1 < len(args) {
				inv.script = args[i+1]
			}
			return inv
		case isValued:
			if !hasEq && i+1 < len(args) {
				i++
				val = args[i]
			}
			switch role {
			case "dir":
				inv.dir = val
			case "env":
				inv.env = val
			}
		case tool == "uv" && (a == "-m" || a == "--module" || a == "--script"):
			return invocation{}
		case run && tool == "hatch" && strings.HasPrefix(a, "+"):
		case run && tool == "hatch" && hatchBuiltins[a]:
			return invocation{}
		case strings.HasPrefix(a, "-"):
		case !run && a == "run":
			run = true
		case !run:
			return invocation{}
		default:
			inv.script = a
			return inv
		}
	}
	return inv
}

// scripts is the sorted union of the script names tool can run. For hatch,
// env's scripts (plus the default env's, which every env inherits) are
// added; ok is false when env is not defined. A bare hatch lookup uses the
// default env, or every env when none is named "default".
//
// ponytail: hatch templates other than default and the "detached" flag are
// ignored.
func scripts(py map[string]any, tool, env string) (names []string, ok bool) {
	set := map[string]bool{}
	add := func(m map[string]any) {
		for k := range m {
			if k != "_" { // pdm's settings key
				set[k] = true
			}
		}
	}
	add(table(py, "project", "scripts"))
	add(table(py, "project", "gui-scripts"))
	add(table(py, "tool", "poetry", "scripts"))
	add(table(py, "tool", "pdm", "scripts"))
	if tool == "hatch" {
		envs := table(py, "tool", "hatch", "envs")
		_, hasDefault := envs["default"]
		switch {
		case env != "":
			if _, ok := envs[env]; !ok {
				return nil, false
			}
			add(table(envs, env, "scripts"))
			add(table(envs, "default", "scripts"))
		case hasDefault:
			add(table(envs, "default", "scripts"))
		default:
			for e := range envs {
				add(table(envs, e, "scripts"))
			}
		}
	}
	return slices.Sorted(maps.Keys(set)), true
}

// dependsOn reports whether name, normalized PEP 503 style, is declared as
// a dependency anywhere in py.
func dependsOn(py map[string]any, name string) bool {
	return slices.Contains(dependencies(py), normalize(name))
}

// dependencies collects every declared dependency name, normalized:
// PEP 621 dependencies and optional-dependencies, PEP 735 dependency
// groups, poetry's dependency tables, pdm's and uv's dev-dependencies and
// the build-system requires.
func dependencies(py map[string]any) []string {
	var out []string
	reqs := func(v any) {
		for _, s := range strs(v) { // {include-group = ...} tables are skipped
			out = append(out, normalize(requirementName(s)))
		}
	}
	keys := func(m map[string]any) {
		for k := range m {
			out = append(out, normalize(k))
		}
	}
	reqs(table(py, "project")["dependencies"])
	for _, v := range table(py, "project", "optional-dependencies") {
		reqs(v)
	}
	for _, v := range table(py, "dependency-groups") {
		reqs(v)
	}
	keys(table(py, "tool", "poetry", "dependencies"))
	keys(table(py, "tool", "poetry", "dev-dependencies"))
	for g := range table(py, "tool", "poetry", "group") {
		keys(table(py, "tool", "poetry", "group", g, "dependencies"))
	}
	for _, v := range table(py, "tool", "pdm", "dev-dependencies") {
		reqs(v)
	}
	reqs(table(py, "tool", "uv")["dev-dependencies"])
	reqs(table(py, "build-system")["requires"])
	return out
}

// requirementName is the project name of a PEP 508 requirement string: the
// text before any extras, version specifier, marker, URL or parenthesis.
func requirementName(req string) string {
	if i := strings.IndexAny(req, " [<>=!~;(@"); i >= 0 {
		req = req[:i]
	}
	return strings.TrimSpace(req)
}

var separators = regexp.MustCompile(`[-_.]+`)

// normalize applies PEP 503 name normalization: lowercase, runs of "-", "_"
// and "." become "-".
func normalize(name string) string {
	return separators.ReplaceAllString(strings.ToLower(name), "-")
}

// table walks keys into nested TOML tables; nil when any step is missing or
// not a table.
func table(m map[string]any, keys ...string) map[string]any {
	for _, k := range keys {
		m, _ = m[k].(map[string]any)
	}
	return m
}

// strs is the string items of a TOML array.
func strs(v any) []string {
	var out []string
	items, _ := v.([]any)
	for _, it := range items {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
