// Package justfile checks just invocations against a statically parsed
// justfile. just is never executed.
//
// ponytail: conservative mode. Modules ("mod"), non-local imports and
// unreadable imports define recipes we cannot see, so such files report
// nothing; upgrade path is an opt-in "just --dump --dump-format json".
package justfile

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/runner"
	"github.com/jnvr1/stalemark/internal/shellx"
)

// RuleRecipeNotFound is reported when the recipe is not defined in the justfile.
const RuleRecipeNotFound = "SM-A005"

// Resolver implements runner.Resolver for just.
type Resolver struct{}

// New returns the just resolver.
func New() Resolver { return Resolver{} }

// Names lists the tools this resolver handles.
func (Resolver) Names() []string { return []string{"just"} }

// Resolve reports the first recipe of cmd that its justfile lacks. Later
// recipes on the same line are not checked: the arity of the missing one is
// unknown, so where the next recipe starts is too.
func (Resolver) Resolve(ctx runner.Context, doc string, cmd shellx.Command) []finding.Finding {
	inv := parseArgs(cmd.Args)
	if inv.silent || len(inv.args) == 0 {
		return nil
	}
	dir := filepath.Join(runner.WorkDir(ctx, cmd), filepath.FromSlash(inv.dir))
	path := discover(dir, inv.file)
	if path == "" {
		return nil
	}
	jf := parse(path)
	if jf == nil || jf.opaque {
		return nil
	}
	name := jf.missing(inv.args)
	if name == "" {
		return nil
	}
	f := finding.Finding{
		Rule: RuleRecipeNotFound, Severity: finding.Error, Path: doc, Line: cmd.FileLine,
		Subject: name, Suggestion: runner.Closest(name, jf.callable()),
	}
	f.Message = fmt.Sprintf("just recipe %q not found in %s", name, runner.Rel(ctx, path))
	if f.Suggestion != "" {
		f.Message += fmt.Sprintf(" (did you mean %q?)", f.Suggestion)
	}
	return []finding.Finding{f}
}

// invocation is what matters of a just call: the -d directory, the -f file
// and the positional arguments (recipes and their arguments). silent is set
// by flags that never run a recipe (--list, --choose, --show...).
type invocation struct {
	dir, file string
	args      []string
	silent    bool
}

var (
	// options whose value is the next argument (or attached with =).
	valued = map[string]bool{
		"-f": true, "--justfile": true, "-d": true, "--working-directory": true,
		"-s": true, "--show": true, "--completions": true, "-E": true, "--dotenv-path": true,
		"--shell": true, "--shell-arg": true, "--chooser": true, "--color": true, "--command-color": true,
		"--dump-format": true, "--list-heading": true, "--list-prefix": true, "--dotenv-filename": true,
		"--timestamp-format": true,
	}
	// flags after which no recipe of the project justfile is run.
	silentFlags = map[string]bool{
		"-l": true, "--list": true, "--choose": true, "--dump": true, "-e": true, "--edit": true,
		"--evaluate": true, "--fmt": true, "--init": true, "--summary": true, "-s": true, "--show": true,
		"--variables": true, "--changelog": true, "--completions": true, "-h": true, "--help": true,
		"-V": true, "--version": true, "--groups": true, "-g": true, "--global-justfile": true,
		"-c": true, "--command": true, // -c CMD ARGS...: runs CMD, never a recipe
	}
)

// parseArgs walks just's arguments: "--" ends the options, NAME=VALUE tokens
// before the first recipe are variable overrides, other dashed tokens are
// flags and everything else is positional.
//
// ponytail: third copy of the option-value walk (see makefile, taskfile);
// fold into a runner.ParseArgs(valued, silent map[string]bool) when a fourth
// resolver needs it.
func parseArgs(args []string) invocation {
	var inv invocation
	for i := 0; i < len(args); i++ {
		a := args[i]
		opt, val, hasEq := strings.Cut(a, "=")
		switch {
		case a == "--":
			return withArgs(inv, args[i+1:])
		case opt == "--set": // --set NAME VALUE
			i += 2
		case valued[opt]:
			if !hasEq && i+1 < len(args) {
				i++
				val = args[i]
			}
			switch opt {
			case "-f", "--justfile":
				inv.file = val
			case "-d", "--working-directory":
				inv.dir = val
			}
			inv.silent = inv.silent || silentFlags[opt]
		case silentFlags[a]:
			inv.silent = true
		case strings.HasPrefix(a, "-"), hasEq:
		default:
			return withArgs(inv, args[i:])
		}
	}
	return inv
}

// withArgs sets the positional arguments. A first argument with a "/" is a
// search directory ("just sub/build"), which we do not follow.
//
// ponytail: treated as silent; upgrade = resolve against that directory.
func withArgs(inv invocation, args []string) invocation {
	if len(args) > 0 && (strings.Contains(args[0], "/") || args[0] == "." || args[0] == "..") {
		inv.silent = true
	}
	inv.args = args
	return inv
}

// fileNames is just's search order within a directory.
var fileNames = []string{"justfile", "Justfile", ".justfile", ".Justfile", "JUSTFILE"}

// discover returns the justfile just would read in dir: file when given,
// otherwise the first of fileNames. "" when none exists.
//
// ponytail: just also walks up parent directories; we stop at dir because
// docs run from Root.
func discover(dir, file string) string {
	if file == "" {
		return runner.Discover(dir, fileNames...)
	}
	p := filepath.Join(dir, filepath.FromSlash(file))
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		return ""
	}
	return p
}

// recipe is the static view of a recipe header: how many parameters it
// declares (defaults included), whether the last one is variadic and
// whether it is hidden from --list.
type recipe struct {
	params   int
	variadic bool
	private  bool
}

// imp is one import line.
type imp struct {
	path     string
	optional bool
}

// opaque reports whether the import cannot be resolved statically: an
// absolute or home-relative path, or one built from an expression.
func (i imp) opaque() bool {
	return filepath.IsAbs(i.path) || strings.HasPrefix(i.path, "/") ||
		strings.HasPrefix(i.path, "~") || strings.Contains(i.path, "{{")
}

// parsed is the static view of a justfile: recipes by name, aliases by name
// (resolved into recipes by parse) and the imports to follow. opaque means
// a mod or an unresolvable import hides recipes: anything could match.
type parsed struct {
	recipes map[string]recipe
	aliases map[string]string
	imports []imp
	opaque  bool
}

// missing returns the first recipe of args that is not defined, "" when
// every recipe is. Each found recipe consumes its arguments: everything
// after a variadic one, otherwise up to its parameter count. A module path
// ("api::deploy") stops the walk: modules are not inspected.
func (p *parsed) missing(args []string) string {
	for len(args) > 0 {
		name := args[0]
		if strings.Contains(name, "::") {
			return ""
		}
		r, ok := p.recipes[name]
		if !ok {
			return name
		}
		if r.variadic {
			return ""
		}
		args = args[1+min(len(args)-1, r.params):]
	}
	return ""
}

// callable is the sorted list of public names a user may type, for
// suggestions.
func (p *parsed) callable() []string {
	var out []string
	for n, r := range p.recipes {
		if !r.private {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}

// parse loads path with its imports and resolves aliases: an alias is
// callable with the arity of its target, even when the target is missing
// (the justfile's problem, not the document's).
func parse(path string) *parsed {
	p := load(path, 0, map[string]bool{})
	if p == nil {
		return nil
	}
	for a, target := range p.aliases {
		p.recipes[a] = p.recipes[target]
	}
	return p
}

// load parses path and its local imports, which contribute their recipes
// unprefixed. nil when the file is unreadable or nested too deep. seen
// holds the files already loaded: just dedupes imports, so a second import
// of the same file adds nothing.
func load(path string, depth int, seen map[string]bool) *parsed {
	abs, err := filepath.Abs(path)
	if err != nil || depth > 8 {
		return nil
	}
	if seen[abs] {
		return &parsed{}
	}
	seen[abs] = true
	src, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	p := parseSource(src)
	for _, im := range p.imports {
		var sub *parsed
		if !im.opaque() {
			full := filepath.Join(filepath.Dir(path), filepath.FromSlash(im.path))
			if _, err := os.Stat(full); err != nil && im.optional {
				continue
			}
			sub = load(full, depth+1, seen)
		}
		if sub == nil || sub.opaque {
			p.opaque = true
			return p
		}
		maps.Copy(p.recipes, sub.recipes)
		maps.Copy(p.aliases, sub.aliases)
	}
	return p
}

// name is a recipe, alias or parameter identifier; param is one parameter:
// optional +, * or $ prefix, name and optional default: a quoted string, a
// backtick command, a parenthesized expression or function call (which may
// contain spaces), or a bare word.
//
// ponytail: one level of parentheses; a header with nested ones falls back
// to looseRE and is recorded as variadic (callable, arity unknown).
const (
	ident = `[A-Za-z_][A-Za-z0-9_-]*`
	param = `([+*$]*)` + ident + `(?:=(?:"[^"]*"|'[^']*'|` + "`[^`]*`" + `|(?:` + ident + `)?\([^)]*\)|[^\s:]+))?`
)

var (
	// headerRE matches a recipe header at column 0: optional @, the name,
	// parameters and a ":" that is not ":=".
	headerRE = regexp.MustCompile(`^@?(` + ident + `)((?:\s+` + param + `)*)\s*:(?:[^=]|$)`)
	// looseRE catches headers whose defaults headerRE cannot parse (nested
	// parentheses). [^:]* stops at ":=", so assignments never match.
	looseRE  = regexp.MustCompile(`^@?(` + ident + `)\s[^:]*:(?:[^=]|$)`)
	paramRE  = regexp.MustCompile(param)
	aliasRE  = regexp.MustCompile(`^alias\s+(` + ident + `)\s*:=\s*(` + ident + `)`)
	attrRE   = regexp.MustCompile(`^\[([^\]]*)\]`)
	modRE    = regexp.MustCompile(`^mod\??\s`)
	importRE = regexp.MustCompile(`^import(\??)\s+(?:"([^"]*)"|'([^']*)')`)
)

// parseSource reads the recipe headers, aliases, imports and mod lines of
// one justfile. Indented lines are recipe bodies and are skipped, as are
// comments, settings and assignments.
//
// ponytail: line-based parse; a [private] attribute on an alias line is not
// tracked (aliases take their target's visibility).
func parseSource(src []byte) *parsed {
	p := &parsed{recipes: map[string]recipe{}, aliases: map[string]string{}}
	private := false
	for line := range strings.SplitSeq(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n") {
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}
		if m := attrRE.FindStringSubmatch(line); m != nil {
			for attr := range strings.SplitSeq(m[1], ",") {
				private = private || strings.TrimSpace(attr) == "private"
			}
			continue
		}
		if modRE.MatchString(line) {
			p.opaque = true
		} else if m := importRE.FindStringSubmatch(line); m != nil {
			p.imports = append(p.imports, imp{path: m[2] + m[3], optional: m[1] == "?"})
		} else if m := aliasRE.FindStringSubmatch(line); m != nil {
			p.aliases[m[1]] = m[2]
		} else if m := headerRE.FindStringSubmatch(line); m != nil {
			r := recipe{private: private || strings.HasPrefix(m[1], "_")}
			for _, pm := range paramRE.FindAllStringSubmatch(m[2], -1) {
				r.params++
				r.variadic = r.variadic || strings.ContainsAny(pm[1], "+*")
			}
			p.recipes[m[1]] = r
		} else if m := looseRE.FindStringSubmatch(line); m != nil {
			p.recipes[m[1]] = recipe{variadic: true, private: private || strings.HasPrefix(m[1], "_")}
		}
		private = false
	}
	return p
}
