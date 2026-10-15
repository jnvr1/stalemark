// Package gocargo checks the cheap, explicit targets of "go run|build|test"
// and "cargo run|build|test|bench|check|clippy|doc" invocations: a local
// package directory or .go file that does not exist, a "go run" of a
// directory that is not a main package, and a cargo --bin, --example or -p
// that names nothing in the Cargo.toml they would run against. Subcommands and
// flags are never validated.
//
// ponytail: verify-tool-presence mode. Without a go.mod/go.work (or
// Cargo.toml) between the command's directory and the check root nothing is
// reported. Cargo's autobins/autoexamples = false, [lib] and target tables
// beyond [[bin]]/[[example]] are ignored, which only causes false negatives.
package gocargo

import (
	"errors"
	"fmt"
	"io/fs"
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

// Rules reported by this resolver.
const (
	RuleGoPackageNotFound   = "SM-A007" // a local package directory or .go file does not exist
	RuleGoRunNotMain        = "SM-A008" // "go run" of a directory without a main package
	RuleCargoTargetNotFound = "SM-A009" // --bin, --example or -p names nothing in the Cargo.toml
)

// Resolver implements runner.Resolver for go and cargo.
type Resolver struct{}

// New returns the go/cargo resolver.
func New() Resolver { return Resolver{} }

// Names lists the tools this resolver handles.
func (Resolver) Names() []string { return []string{"go", "cargo"} }

// Resolve reports the explicit targets of cmd that do not exist. Anything
// ambiguous (no manifest, an import path, a flag value, a glob, an
// unparsable Cargo.toml) yields nothing: precision first.
func (Resolver) Resolve(ctx runner.Context, doc string, cmd shellx.Command) []finding.Finding {
	if cmd.Name == "go" {
		return resolveGo(ctx, doc, cmd)
	}
	return resolveCargo(ctx, doc, cmd)
}

// walkUp calls visit for start and each parent up to root, stopping when it
// returns true. It never goes above root and does nothing when start is not an
// existing directory inside root.
func walkUp(start, root string, visit func(dir string) bool) bool {
	if st, err := os.Stat(start); err != nil || !st.IsDir() {
		return false
	}
	if rel, err := filepath.Rel(root, start); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	root = filepath.Clean(root)
	for dir := filepath.Clean(start); ; dir = filepath.Dir(dir) {
		if visit(dir) {
			return true
		}
		if dir == root || dir == filepath.Dir(dir) {
			return false
		}
	}
}

// ---- go ----

// goCall is what matters of a go invocation: the subcommand (run, build or
// test, "" for anything else), the -C directory and the positionals that are
// package patterns or .go files.
type goCall struct {
	sub, dir string
	targets  []string
}

// goValued lists the flags whose value is the next argument when written
// without "=". Every other dashed token is a boolean flag.
//
// ponytail: an unknown valued flag whose value looks like "./x" is read as a
// package and can be reported; add the flag here when a real README hits it.
var goValued = map[string]bool{
	"o": true, "p": true, "C": true, "tags": true, "ldflags": true, "gcflags": true, "asmflags": true,
	"gccgoflags": true, "mod": true, "modfile": true, "overlay": true, "pgo": true, "pkgdir": true,
	"toolexec": true, "installsuffix": true, "buildmode": true, "compiler": true, "exec": true,
	"run": true, "bench": true, "benchtime": true, "count": true, "cpu": true, "timeout": true,
	"coverprofile": true, "covermode": true, "coverpkg": true, "cpuprofile": true, "memprofile": true,
	"memprofilerate": true, "blockprofile": true, "blockprofilerate": true, "mutexprofile": true,
	"mutexprofilefraction": true, "trace": true, "outputdir": true, "parallel": true, "skip": true,
	"fuzz": true, "fuzztime": true, "fuzzminimizetime": true, "list": true, "shuffle": true, "vet": true,
}

// cutFlag splits a go flag: dashes and the "test." prefix are dropped.
func cutFlag(a string) (name, val string, hasEq bool) {
	name, val, hasEq = strings.Cut(strings.TrimLeft(a, "-"), "=")
	return strings.TrimPrefix(name, "test."), val, hasEq
}

// parseGo walks the arguments after "go". For run only the leading
// positionals matter (the package, or a chain of .go files) because
// everything after them belongs to the program; for build and test every
// positional is a pattern, up to "-args".
func parseGo(args []string) goCall {
	var c goCall
	flagsDone := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case !flagsDone && a == "--":
			flagsDone = true
		case !flagsDone && len(a) > 1 && a[0] == '-':
			name, val, hasEq := cutFlag(a)
			if c.sub == "" && name != "C" || c.sub == "run" && len(c.targets) > 0 || c.sub == "test" && name == "args" {
				if c.sub == "" {
					return goCall{}
				}
				return c
			}
			if goValued[name] && !hasEq && i+1 < len(args) {
				i++
				val = args[i]
			}
			if name == "C" {
				c.dir = val
			}
		case c.sub == "":
			if a != "run" && a != "build" && a != "test" {
				return goCall{}
			}
			c.sub = a
		case c.sub == "run" && len(c.targets) > 0 && (!strings.HasSuffix(c.targets[0], ".go") || !strings.HasSuffix(a, ".go")):
			return c
		default:
			c.targets = append(c.targets, a)
		}
	}
	return c
}

// local reports whether p is a relative package pattern or a .go file; import
// paths, "std", "cmd", "all" and pkg@version are not.
func local(p string) bool {
	if strings.Contains(p, "@") || filepath.IsAbs(p) {
		return false
	}
	return p == "." || p == ".." || strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") || strings.HasSuffix(p, ".go")
}

// mainRE matches the package clause of a main package.
//
// ponytail: a "package main" line inside a block comment also matches; use
// go/parser.ParseFile(PackageClauseOnly) if a probe finds one.
var mainRE = regexp.MustCompile(`(?m)^package\s+main\b`)

// isMain reports whether a non-test .go file of dir declares package main,
// and whether dir has any such file at all.
func isMain(dir string) (isMain, anyGo bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, false
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") || n[0] == '_' || n[0] == '.' {
			continue
		}
		anyGo = true
		if src, err := os.ReadFile(filepath.Join(dir, n)); err == nil && mainRE.Match(src) {
			return true, true
		}
	}
	return false, anyGo
}

// findGo reports whether a go.mod or go.work exists from start up to root.
func findGo(start, root string) bool {
	return walkUp(start, root, func(dir string) bool {
		for _, n := range []string{"go.mod", "go.work"} {
			if st, err := os.Stat(filepath.Join(dir, n)); err == nil && !st.IsDir() {
				return true
			}
		}
		return false
	})
}

func resolveGo(ctx runner.Context, doc string, cmd shellx.Command) []finding.Finding {
	call := parseGo(cmd.Args)
	if len(call.targets) == 0 {
		return nil
	}
	work := filepath.Join(runner.WorkDir(ctx, cmd), filepath.FromSlash(call.dir))
	if !findGo(work, ctx.Root) {
		return nil
	}
	var out []finding.Finding
	for _, t := range call.targets {
		if !local(t) {
			continue
		}
		pat := strings.TrimSuffix(t, "/...")
		wild := pat != t
		if strings.Contains(pat, "...") {
			continue
		}
		path := filepath.Join(work, filepath.FromSlash(pat))
		st, err := os.Stat(path)
		f := finding.Finding{Severity: finding.Error, Path: doc, Line: cmd.FileLine, Subject: t}
		switch {
		case errors.Is(err, fs.ErrNotExist):
			f.Rule = RuleGoPackageNotFound
			if strings.HasSuffix(pat, ".go") {
				f.Message = fmt.Sprintf("go file %q not found", t)
			} else {
				f.Message = fmt.Sprintf("go package %q not found (no directory %s)", t, runner.Rel(ctx, path))
			}
		case err == nil && call.sub == "run" && !wild && st.IsDir():
			if m, anyGo := isMain(path); m || !anyGo {
				continue // a dir without .go files may just parent packages
			}
			f.Rule = RuleGoRunNotMain
			f.Message = fmt.Sprintf("go run %q is not a main package", t)
		default:
			continue
		}
		out = append(out, f)
	}
	return out
}

// ---- cargo ----

// cargoCall is what matters of a cargo invocation.
type cargoCall struct {
	manifest             string
	bins, examples, pkgs []string
}

var cargoSubs = map[string]bool{"run": true, "build": true, "test": true, "bench": true, "check": true, "clippy": true, "doc": true}

// cargoValued lists the options whose value is the next argument when written
// without "=". Every other dashed token is a boolean flag.
var cargoValued = map[string]bool{
	"--bin": true, "--example": true, "-p": true, "--package": true, "--manifest-path": true,
	"--target-dir": true, "--target": true, "--features": true, "-F": true, "--profile": true, "-j": true,
	"--jobs": true, "--config": true, "-Z": true, "--color": true, "--message-format": true, "--test": true,
	"--bench": true, "--exclude": true, "--artifact-dir": true, "--lockfile-path": true,
}

// parseCargo walks the arguments after "cargo": "+toolchain" and global
// options may precede the subcommand, "--" ends the options. Values that are
// globs, paths, URLs or name:version specs are dropped; -p NAME@VERSION keeps
// NAME.
func parseCargo(args []string) cargoCall {
	var c cargoCall
	sub := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return c
		case sub == "" && strings.HasPrefix(a, "+"):
		case !strings.HasPrefix(a, "-"):
			if sub == "" {
				if !cargoSubs[a] {
					return cargoCall{}
				}
				sub = a
			}
		default:
			opt, val, hasEq := strings.Cut(a, "=")
			if cargoValued[opt] && !hasEq && i+1 < len(args) {
				i++
				val = args[i]
			}
			if sub == "" || val == "" || strings.ContainsAny(val, "*?[") {
				continue
			}
			switch opt {
			case "--bin":
				c.bins = append(c.bins, val)
			case "--example":
				c.examples = append(c.examples, val)
			case "-p", "--package":
				if !strings.ContainsAny(val, `/:\`) {
					name, _, _ := strings.Cut(val, "@")
					c.pkgs = append(c.pkgs, name)
				}
			case "--manifest-path":
				c.manifest = val
			}
		}
	}
	return c
}

// readManifest parses a Cargo.toml; nil when unreadable or invalid.
func readManifest(path string) map[string]any {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m map[string]any
	if toml.Unmarshal(src, &m) != nil {
		return nil
	}
	return m
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

// findCargo returns the Cargo.toml a cargo command in start resolves to: the
// nearest workspace root up to root, else the nearest manifest. "" when none.
func findCargo(start, root string) string {
	nearest := ""
	walkUp(start, root, func(dir string) bool {
		m := runner.Discover(dir, "Cargo.toml")
		if m == "" {
			return false
		}
		if nearest == "" {
			nearest = m
		}
		if _, ok := readManifest(m)["workspace"].(map[string]any); ok {
			nearest = m
			return true
		}
		return false
	})
	return nearest
}

// pkg is a cargo package: its name and the directory of its Cargo.toml.
type pkg struct{ name, dir string }

// packages lists the root package of manifest plus its workspace members:
// "members" globs minus "exclude" globs, each needing a Cargo.toml with a
// [package] name.
func packages(manifest string) []pkg {
	m := readManifest(manifest)
	if m == nil {
		return nil
	}
	dir := filepath.Dir(manifest)
	var out []pkg
	seen := map[string]bool{}
	add := func(d string, m map[string]any) {
		if name, _ := table(m, "package")["name"].(string); name != "" && !seen[d] {
			seen[d] = true
			out = append(out, pkg{name, d})
		}
	}
	glob := func(pat string) []string {
		matches, _ := filepath.Glob(filepath.Join(dir, filepath.FromSlash(pat)))
		return matches
	}
	excluded := map[string]bool{}
	for _, pat := range strs(table(m, "workspace")["exclude"]) {
		for _, d := range glob(pat) {
			excluded[d] = true
		}
	}
	add(dir, m)
	for _, pat := range strs(table(m, "workspace")["members"]) {
		for _, d := range glob(pat) {
			if !excluded[d] {
				add(d, readManifest(filepath.Join(d, "Cargo.toml")))
			}
		}
	}
	return out
}

// depNames lists the direct dependencies declared in m: the table keys of
// every dependencies, dev-dependencies and build-dependencies table (also
// under [target.*]) and [workspace.dependencies], plus the real crate name of
// renamed entries (package = "...").
func depNames(m map[string]any) []string {
	var out []string
	add := func(deps map[string]any) {
		for k, v := range deps {
			out = append(out, k)
			if d, ok := v.(map[string]any); ok {
				if orig, _ := d["package"].(string); orig != "" {
					out = append(out, orig)
				}
			}
		}
	}
	for _, kind := range []string{"dependencies", "dev-dependencies", "build-dependencies"} {
		add(table(m, kind))
		for _, tgt := range table(m, "target") {
			tm, _ := tgt.(map[string]any)
			add(table(tm, kind))
		}
	}
	add(table(m, "workspace", "dependencies"))
	return out
}

// knownPackages is every name "cargo -p" accepts here: the workspace
// packages, the direct dependencies of the root manifest and of every
// package, and every [[package]] of a Cargo.lock next to the manifest.
//
// ponytail: without a Cargo.lock, transitive dependencies are unknown, so
// "-p" of one is reported; run cargo generate-lockfile in the repo to fix.
func knownPackages(manifest string, pkgs []pkg) []string {
	var out []string
	for _, p := range pkgs {
		out = append(out, p.name)
		out = append(out, depNames(readManifest(filepath.Join(p.dir, "Cargo.toml")))...)
	}
	out = append(out, depNames(readManifest(manifest))...)
	locked, _ := readManifest(filepath.Join(filepath.Dir(manifest), "Cargo.lock"))["package"].([]any)
	for _, it := range locked {
		m, _ := it.(map[string]any)
		if name, _ := m["name"].(string); name != "" {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// targets lists the names of one kind of cargo target of p: the [[key]]
// entries, sub/NAME.rs files and sub/NAME/main.rs directories, plus extra.
func targets(p pkg, key, sub string, extra ...string) []string {
	out := extra
	items, _ := readManifest(filepath.Join(p.dir, "Cargo.toml"))[key].([]any)
	for _, it := range items {
		m, _ := it.(map[string]any)
		if name, _ := m["name"].(string); name != "" {
			out = append(out, name)
		}
	}
	entries, _ := os.ReadDir(filepath.Join(p.dir, filepath.FromSlash(sub)))
	for _, e := range entries {
		switch n := e.Name(); {
		case !e.IsDir() && strings.HasSuffix(n, ".rs"):
			out = append(out, strings.TrimSuffix(n, ".rs"))
		case e.IsDir():
			if _, err := os.Stat(filepath.Join(p.dir, filepath.FromSlash(sub), n, "main.rs")); err == nil {
				out = append(out, n)
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// bins lists the binaries of p: [[bin]], src/bin and the package itself when
// it has a src/main.rs.
func bins(p pkg) []string {
	var extra []string
	if _, err := os.Stat(filepath.Join(p.dir, "src", "main.rs")); err == nil {
		extra = append(extra, p.name)
	}
	return targets(p, "bin", "src/bin", extra...)
}

// examples lists the examples of p: [[example]] and examples/.
func examples(p pkg) []string { return targets(p, "example", "examples") }

func resolveCargo(ctx runner.Context, doc string, cmd shellx.Command) []finding.Finding {
	call := parseCargo(cmd.Args)
	if len(call.bins)+len(call.examples)+len(call.pkgs) == 0 {
		return nil
	}
	start := runner.WorkDir(ctx, cmd)
	if call.manifest != "" {
		path := filepath.Join(start, filepath.FromSlash(call.manifest))
		if st, err := os.Stat(path); filepath.Base(path) != "Cargo.toml" || err != nil || st.IsDir() {
			return nil
		}
		start = filepath.Dir(path)
	}
	manifest := findCargo(start, ctx.Root)
	if manifest == "" {
		return nil
	}
	pkgs := packages(manifest)
	if len(pkgs) == 0 {
		return nil
	}
	rel := runner.Rel(ctx, manifest)
	missing := func(kind, name string, candidates []string) finding.Finding {
		f := finding.Finding{
			Rule: RuleCargoTargetNotFound, Severity: finding.Error, Path: doc, Line: cmd.FileLine,
			Subject: name, Suggestion: runner.Closest(name, candidates),
		}
		f.Message = fmt.Sprintf("cargo %s %q not found in %s", kind, name, rel)
		if f.Suggestion != "" {
			f.Message += fmt.Sprintf(" (did you mean %q?)", f.Suggestion)
		}
		return f
	}

	var out []finding.Finding
	scope := pkgs
	if len(call.pkgs) > 0 {
		scope = nil
		for _, name := range call.pkgs {
			i := slices.IndexFunc(pkgs, func(p pkg) bool { return p.name == name })
			if i >= 0 {
				scope = append(scope, pkgs[i])
				continue
			}
			if slices.Contains(knownPackages(manifest, pkgs), name) {
				continue // a dependency: valid for -p, contributes no bins or examples
			}
			var all []string
			for _, p := range pkgs {
				all = append(all, p.name)
			}
			out = append(out, missing("package", name, all))
		}
		if len(scope) == 0 {
			return out
		}
	}
	for _, t := range []struct {
		kind  string
		names []string
		list  func(pkg) []string
	}{{"bin", call.bins, bins}, {"example", call.examples, examples}} {
		var known []string
		for _, p := range scope {
			known = append(known, t.list(p)...)
		}
		for _, name := range t.names {
			if !slices.Contains(known, name) {
				out = append(out, missing(t.kind, name, known))
			}
		}
	}
	return out
}
