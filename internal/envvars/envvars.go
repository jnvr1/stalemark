// Package envvars checks that the environment variables a Markdown document
// names (export lines, FOO=bar prefixes, dotenv snippets, backticked names in
// setup sections) exist in the nearest .env.example. Precision first: every
// heuristic errs on silence, and no example file means no check.
package envvars

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/markdown"
	"github.com/jnvr1/stalemark/internal/runner"
	"github.com/jnvr1/stalemark/internal/shellx"
)

// RuleEnvNotInExample is the rule ID of a documented variable that the nearest
// example file does not list.
//
// ponytail: SM-E002 (example key never documented) needs an opt-in in
// .stalemark.yml; add when a user asks.
const RuleEnvNotInExample = "SM-E001"

var exampleNames = []string{".env.example", ".env.sample", ".env.template"}

// platform holds variables every environment provides or that tooling sets;
// docs may name them without the example file listing them.
var platform = []string{
	"CI", "HOME", "PATH", "NODE_ENV", "PWD", "USER", "SHELL", "TERM", "LANG", "TMPDIR",
	"DEBUG", "NO_COLOR", "FORCE_COLOR", "GITHUB_TOKEN", "GITHUB_ACTIONS",
}

// toolPrefixes mark variables of build toolchains (`CGO_ENABLED=0 go build`,
// `RUST_LOG=debug cargo run`): they configure the tool, not the app, so an
// example file never lists them.
//
// ponytail: fixed list; make it configurable when a repo needs more.
var toolPrefixes = []string{
	"GO", "CGO_", "CARGO_", "RUST", "NODE_", "NPM_", "PNPM_", "YARN_", "PYTHON", "PIP_", "UV_", "POETRY_",
	"DOCKER_", "COMPOSE_", "GIT_", "GITHUB_", "JAVA_", "GRADLE_", "MAVEN_", "DOTNET_", "LC_", "XDG_",
}

// tooling reports whether name belongs to the platform or a toolchain.
func tooling(name string) bool {
	return slices.Contains(platform, name) ||
		slices.ContainsFunc(toolPrefixes, func(p string) bool { return strings.HasPrefix(name, p) })
}

// sections are the heading words that make a backticked UPPER_SNAKE token an
// env var mention.
var sections = []string{"setup", "config", "environment", "env", "getting started", "install", "variables"}

var (
	identRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	snakeRE = regexp.MustCompile(`^[A-Z][A-Z0-9]*(_[A-Z0-9]+)+$`)
)

type mention struct {
	name      string
	line, col int
}

// Check returns a SM-E001 finding for every variable of d that the nearest
// example file lacks, once per name, in document order.
//
// ponytail: the nearest example (doc directory, then each parent up to
// ctx.Root) wins and examples are never merged, so a doc at the root of a
// monorepo with per-service examples only sees the root example. Upgrade =
// merge the examples of the subtree. Backticked names count only under a
// matching heading and need an underscore (`HTTP` stays silent).
func Check(ctx runner.Context, d *markdown.Doc) []finding.Finding {
	example, keys := nearestExample(ctx, filepath.Dir(d.Path))
	if example == "" {
		return nil
	}

	docPath, exampleRel := runner.Rel(ctx, d.Path), runner.Rel(ctx, example)
	var out []finding.Finding
	seen := map[string]bool{}
	for _, m := range mentions(d) {
		if seen[m.name] || !identRE.MatchString(m.name) || tooling(m.name) || slices.Contains(keys, m.name) {
			continue
		}
		seen[m.name] = true
		f := finding.Finding{
			Rule: RuleEnvNotInExample, Severity: finding.Error, Path: docPath,
			Line: m.line, Col: m.col, Subject: m.name,
			Message:    "env var " + `"` + m.name + `"` + " not found in " + exampleRel,
			Suggestion: runner.Closest(m.name, keys),
		}
		if f.Suggestion != "" {
			f.Message += " (did you mean " + `"` + f.Suggestion + `"` + "?)"
		}
		out = append(out, f)
	}
	return out
}

// mentions returns every env var mention of d in document order.
func mentions(d *markdown.Doc) []mention {
	var ms []mention
	for _, b := range d.Blocks {
		if b.Ignored {
			continue
		}
		for _, cmd := range shellx.Extract(b) {
			if cmd.Placeholder {
				continue
			}
			env := make([]string, 0, len(cmd.Env))
			for k := range cmd.Env {
				env = append(env, k)
			}
			sort.Strings(env)
			for _, k := range env {
				ms = append(ms, mention{k, cmd.FileLine, 0})
			}
			if cmd.Name == "export" {
				for _, a := range cmd.Args {
					if !strings.HasPrefix(a, "-") {
						k, _, _ := strings.Cut(a, "=")
						ms = append(ms, mention{k, cmd.FileLine, 0})
					}
				}
			}
		}
		if slices.Contains([]string{"env", "dotenv", "ini", "properties"}, b.Lang) {
			for _, l := range b.Lines {
				// ini and properties keys are often lower case config, not env vars.
				if k, ok := dotenvKey(l.Text); ok && (b.Lang == "env" || b.Lang == "dotenv" || k == strings.ToUpper(k)) {
					ms = append(ms, mention{k, l.FileLine, 0})
				}
			}
		}
	}
	for _, s := range d.Spans {
		if name := strings.TrimSpace(s.Text); snakeRE.MatchString(name) && inSection(d.Headings, s.Line) {
			ms = append(ms, mention{name, s.Line, s.Col})
		}
	}

	sort.SliceStable(ms, func(i, j int) bool {
		return ms[i].line < ms[j].line || ms[i].line == ms[j].line && ms[i].col < ms[j].col
	})
	return ms
}

// Loc is where a variable is first documented.
type Loc struct {
	Path      string
	Line, Col int
}

// Documented returns the app variables d mentions (platform and toolchain
// names excluded), keyed by name, each at its first mention.
func Documented(ctx runner.Context, d *markdown.Doc) map[string]Loc {
	out := map[string]Loc{}
	for _, m := range mentions(d) {
		if _, ok := out[m.name]; !ok && identRE.MatchString(m.name) && !tooling(m.name) {
			out[m.name] = Loc{runner.Rel(ctx, d.Path), m.line, m.col}
		}
	}
	return out
}

// inSection reports whether the nearest heading at or before line names a
// setup-like section; no heading before it means no.
func inSection(headings []markdown.Heading, line int) bool {
	title := ""
	for _, h := range headings {
		if h.Line > line {
			break
		}
		title = strings.ToLower(h.Text)
	}
	return slices.ContainsFunc(sections, func(s string) bool { return strings.Contains(title, s) })
}

// dotenvKey returns the variable name of a dotenv line: comments and blank
// lines are skipped, a leading "export " is dropped, and the key is the text
// before the first "=", which must be an identifier.
func dotenvKey(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", false
	}
	line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
	key, _, ok := strings.Cut(line, "=")
	key = strings.TrimSpace(key)
	return key, ok && identRE.MatchString(key)
}

// nearestExample finds the closest example file from dir up to ctx.Root and
// returns its path and keys, or "" when there is none.
func nearestExample(ctx runner.Context, dir string) (string, []string) {
	for {
		if rel, err := filepath.Rel(ctx.Root, dir); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", nil
		}
		for _, name := range exampleNames {
			p := filepath.Join(dir, name)
			if keys, ok := readKeys(p); ok {
				return p, keys
			}
		}
		parent := filepath.Dir(dir)
		if dir == ctx.Root || parent == dir {
			return "", nil
		}
		dir = parent
	}
}

func readKeys(path string) ([]string, bool) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var keys []string
	for _, l := range strings.Split(string(src), "\n") {
		if k, ok := dotenvKey(l); ok {
			keys = append(keys, k)
		}
	}
	return keys, true
}
