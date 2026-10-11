// Package makefile checks make target invocations against a statically
// parsed Makefile. make is never executed.
//
// ponytail: conservative mode. A Makefile with pattern rules, $(eval,
// $(foreach, $(call or non-literal includes defines targets we cannot see,
// so such files report nothing; upgrade path is an opt-in "make -qp" listing.
package makefile

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/runner"
	"github.com/jnvr1/stalemark/internal/shellx"
)

// RuleTargetNotFound is reported when a target is missing from the Makefile.
const RuleTargetNotFound = "SM-A002"

// Resolver implements runner.Resolver for make.
type Resolver struct{}

// New returns the make resolver.
func New() Resolver { return Resolver{} }

// Names lists the tools this resolver handles.
func (Resolver) Names() []string { return []string{"make", "gmake"} }

// Resolve reports one finding per target of cmd that its Makefile lacks.
func (Resolver) Resolve(ctx runner.Context, doc string, cmd shellx.Command) []finding.Finding {
	inv := parseArgs(cmd.Args)
	if len(inv.targets) == 0 {
		return nil
	}
	dir := filepath.Join(runner.WorkDir(ctx, cmd), filepath.FromSlash(inv.dir))
	path := discover(dir, inv.file)
	if path == "" {
		return nil
	}
	p := parseFile(path)
	if p.dynamic {
		return nil
	}
	rel := runner.Rel(ctx, path)
	var out []finding.Finding
	for _, target := range inv.targets {
		if slices.Contains(p.targets, target) {
			continue
		}
		f := finding.Finding{
			Rule:       RuleTargetNotFound,
			Severity:   finding.Error,
			Path:       doc,
			Line:       cmd.FileLine,
			Subject:    target,
			Suggestion: runner.Closest(target, p.targets),
		}
		f.Message = fmt.Sprintf("make target %q not found in %s", target, rel)
		if f.Suggestion != "" {
			f.Message += fmt.Sprintf(" (did you mean %q?)", f.Suggestion)
		}
		out = append(out, f)
	}
	return out
}

// invocation is what matters of a make call: the -C directory, the -f file
// and the targets. Variable assignments and other options are dropped.
type invocation struct {
	dir, file string
	targets   []string
}

// cd applies one -C: relative directories nest (make semantics), absolute
// ones replace.
func (inv *invocation) cd(d string) {
	switch {
	case d == "":
	case inv.dir == "" || strings.HasPrefix(d, "/") || filepath.IsAbs(d):
		inv.dir = d
	default:
		inv.dir += "/" + d
	}
}

var (
	numRe = regexp.MustCompile(`^[0-9.]+$`)
	// bundleRe is a run of short flags such as -k or -sC.
	bundleRe = regexp.MustCompile(`^-[a-zA-Z]+$`)
	// long options whose value is the next argument.
	valueLong = []string{"--include-dir", "--old-file", "--what-if", "--assume-old", "--assume-new", "--new-file", "--eval"}
	// long options with an optional numeric value as the next argument.
	numLong = []string{"--jobs", "--load-average", "--max-load"}
)

const (
	shortValue    = "CfIoWE"           // short options that require a value
	shortAttached = shortValue + "jlO" // letters that may start an attached value: -Csub, -j4, -Otarget
)

// parseArgs walks make's arguments, consuming option values so they are not
// mistaken for targets. -O alone is a flag: its value is only ever attached.
func parseArgs(args []string) invocation {
	var inv invocation
	next := func(i *int) string { // consumes the following argument, if any
		if *i+1 < len(args) {
			*i++
			return args[*i]
		}
		return ""
	}
	skipNum := func(i *int) { // consumes the following argument when it is a number
		if *i+1 < len(args) && numRe.MatchString(args[*i+1]) {
			*i++
		}
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--directory":
			inv.cd(next(&i))
		case strings.HasPrefix(a, "--directory="):
			inv.cd(strings.TrimPrefix(a, "--directory="))
		case a == "--file" || a == "--makefile":
			if v := next(&i); v != "" {
				inv.file = v
			}
		case strings.HasPrefix(a, "--file="), strings.HasPrefix(a, "--makefile="):
			inv.file = a[strings.Index(a, "=")+1:]
		case slices.Contains(valueLong, a):
			next(&i)
		case slices.Contains(numLong, a):
			skipNum(&i)
		case len(a) > 2 && a[0] == '-' && strings.IndexByte(shortAttached, a[1]) >= 0:
			// attached value: -Csub, -fx.mk, -Iinc, -j4
			switch a[1] {
			case 'C':
				inv.cd(a[2:])
			case 'f':
				inv.file = a[2:]
			}
		case bundleRe.MatchString(a):
			// the last letter of a bundle (-C, -sC, -kf, -j) owns the next argument
			switch last := a[len(a)-1]; last {
			case 'C':
				inv.cd(next(&i))
			case 'f':
				if v := next(&i); v != "" {
					inv.file = v
				}
			case 'j', 'l':
				skipNum(&i)
			default:
				if strings.IndexByte(shortValue, last) >= 0 {
					next(&i)
				}
			}
		case strings.HasPrefix(a, "-"), strings.Contains(a, "="):
			// other flags and variable assignments carry no target
		default:
			inv.targets = append(inv.targets, a)
		}
	}
	return inv
}

// discover returns the makefile make would read in dir: file when given,
// otherwise the first of GNUmakefile, makefile, Makefile. "" when none exists.
// Names are matched against the directory listing so that case-insensitive
// filesystems do not turn Makefile into "makefile" in findings.
func discover(dir, file string) string {
	if file != "" {
		p := filepath.Join(dir, filepath.FromSlash(file))
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
		return ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, n := range []string{"GNUmakefile", "makefile", "Makefile"} {
		for _, e := range entries {
			if e.Name() == n && !e.IsDir() {
				return filepath.Join(dir, n)
			}
		}
	}
	return ""
}

// parsed is the static view of a makefile: the literal targets it defines
// and whether it also defines targets we cannot enumerate.
type parsed struct {
	targets []string
	dynamic bool
}

var (
	assignRe  = regexp.MustCompile(`^[^:#=]+(=|:=|::=|\?=|\+=|!=)`)
	includeRe = regexp.MustCompile(`^-?s?include\s+(.+)$`)
	dynamicRe = regexp.MustCompile(`\$\((eval|foreach|call)\b`)
)

// specialTargets are GNU make's built-in targets; they are never invoked.
// Other dot-prefixed names (.env, .venv) are ordinary targets.
var specialTargets = map[string]bool{
	".PHONY": true, ".SUFFIXES": true, ".PRECIOUS": true, ".INTERMEDIATE": true,
	".NOTINTERMEDIATE": true, ".SECONDARY": true, ".SECONDEXPANSION": true,
	".DELETE_ON_ERROR": true, ".IGNORE": true, ".LOW_RESOLUTION_TIME": true,
	".SILENT": true, ".EXPORT_ALL_VARIABLES": true, ".NOTPARALLEL": true,
	".ONESHELL": true, ".POSIX": true, ".WAIT": true,
}

func parseFile(path string) parsed {
	var p parsed
	seen := map[string]bool{}
	parseInto(&p, path, seen, 0)
	slices.Sort(p.targets)
	p.targets = slices.Compact(p.targets)
	return p
}

func parseInto(p *parsed, path string, seen map[string]bool, depth int) {
	abs, err := filepath.Abs(path)
	if err != nil || seen[abs] || depth > 8 {
		return
	}
	seen[abs] = true
	src, err := os.ReadFile(path)
	if err != nil {
		p.dynamic = true
		return
	}
	parseSource(p, string(src), func(inc string) {
		parseInto(p, filepath.Join(filepath.Dir(path), filepath.FromSlash(inc)), seen, depth+1)
	})
}

// parseSource scans one makefile; include calls back for every literal
// included path that exists, anything else marks the result dynamic.
//
// ponytail: ifeq/ifdef/else/endif are not evaluated, both branches are
// parsed; over-inclusive is safe for a not-found check.
func parseSource(p *parsed, src string, include func(string)) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\\\n", " ")
	inDefine := false
	for line := range strings.SplitSeq(src, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case inDefine:
			kw, _, _ := strings.Cut(trimmed, "#")
			inDefine = strings.TrimSpace(kw) != "endef"
			continue
		case strings.HasPrefix(line, "\t"), trimmed == "", strings.HasPrefix(trimmed, "#"):
			continue
		case strings.HasPrefix(trimmed, "define "), trimmed == "define":
			inDefine = true
			continue
		}
		if m := includeRe.FindStringSubmatch(trimmed); m != nil {
			paths, _, _ := strings.Cut(m[1], "#")
			for _, inc := range strings.Fields(paths) {
				if strings.ContainsAny(inc, "$*?[") {
					p.dynamic = true
					continue
				}
				include(inc)
			}
			continue
		}
		if dynamicRe.MatchString(trimmed) {
			p.dynamic = true
			continue
		}
		if assignRe.MatchString(trimmed) {
			continue
		}
		head, rest, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		names := strings.Fields(head)
		if strings.TrimSpace(head) == ".PHONY" {
			deps, _, _ := strings.Cut(rest, "#")
			deps, _, _ = strings.Cut(deps, ";")
			names = strings.Fields(deps)
		}
		for _, name := range names {
			name = strings.TrimSuffix(name, "&") // grouped-target marker: "a b &:" or "a b&:"
			switch {
			case name == "":
			case strings.Contains(name, "%"), strings.ContainsAny(name, "$"), name == ".DEFAULT":
				p.dynamic = true
			case specialTargets[name]:
			default:
				p.targets = append(p.targets, name)
			}
		}
	}
}
