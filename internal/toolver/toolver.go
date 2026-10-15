// Package toolver checks that the node, go, python and rust versions a
// Markdown document states in its setup sections agree with the machine
// readable version files of the repository (.nvmrc, go.mod, pyproject.toml...).
// Precision first: every heuristic errs on silence.
package toolver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/markdown"
	"github.com/jnvr1/stalemark/internal/runner"
)

// RuleVersionDrift is the rule ID of a documented tool version that differs
// from the one a version file pins.
const RuleVersionDrift = "SM-V001"

// sections are the heading words that make a paragraph a setup-like place.
var sections = []string{
	"setup", "prerequisite", "requirement", "install", "getting started",
	"environment", "toolchain", "dependencies",
}

var (
	// groups: 1 tool, 2 optional ">=" or "v", 3 number, 4 optional "+" or ".x".
	mentionRE = regexp.MustCompile(`(?i)\b(node(?:\.js)?|go(?:lang)?|python|rust)\b\s*(?:version\s*)?(>=|v)?\s*(\d+(?:\.\d+){0,2})(\+|\.x)?`)
	numberRE  = regexp.MustCompile(`\d+(?:\.\d+){0,2}`)
	versionRE = regexp.MustCompile(`^\d+(?:\.\d+){0,2}$`)
	pyBoundRE = regexp.MustCompile(`(?:>=|~=|==)\s*(\d+(?:\.\d+)*)`)
)

// toolNames maps a spelling in prose or in .tool-versions to the canonical tool.
var toolNames = map[string]string{
	"node": "node", "node.js": "node", "nodejs": "node",
	"go": "go", "golang": "go", "python": "python", "rust": "rust",
}

// source is one machine readable version of a tool; advisory ones (package.json
// engines) only warn.
type source struct {
	version, file string
	advisory      bool
}

// Check returns a SM-V001 finding for the first setup-section mention of each
// tool whose version differs, at the written precision, from the version file.
// Tools whose non-advisory files disagree with each other are skipped.
//
// ponytail: sources come from ctx.Root only, with no per-directory monorepo
// lookup; only the first mention of a tool per document is judged ("Node 18
// or 20" reports 18); conflicting files skip the tool silently because checks
// have no stderr channel for the notice the issue asks for. Upgrade = resolve
// sources from the doc directory up, compare every mention, and surface notices.
func Check(ctx runner.Context, d *markdown.Doc) []finding.Finding {
	docPath := runner.Rel(ctx, d.Path)
	srcs := map[string][]source{}
	seen := map[string]bool{}
	var out []finding.Finding
	for _, p := range d.Paragraphs {
		if !inSection(d.Headings, p.Line) {
			continue
		}
		for _, m := range mentionRE.FindAllStringSubmatchIndex(p.Text, -1) {
			if next, _ := utf8.DecodeRuneInString(p.Text[m[1]:]); unicode.IsLetter(next) || next == '-' || next == '/' {
				continue // "Node 18x", "Node 18-20", "Node 18/20": not a plain version
			}
			tool := toolNames[strings.ToLower(p.Text[m[2]:m[3]])]
			if seen[tool] {
				continue
			}
			seen[tool] = true
			if _, ok := srcs[tool]; !ok {
				srcs[tool] = sources(ctx.Root, tool)
			}
			number := p.Text[m[6]:m[7]]
			written := p.Text[m[6]:m[1]] // number plus any "+" or ".x"
			if m[4] >= 0 {
				written = p.Text[m[4]:m[1]] // plus the ">=" or "v" before it
			}
			n := strings.Count(number, ".") + 1
			src, ok := pick(srcs[tool], n)
			if !ok {
				continue
			}
			want := prefix(src.version, n)
			if prefix(number, len(strings.Split(want, "."))) == want {
				continue
			}
			sev := finding.Error
			if src.advisory {
				sev = finding.Warning
			}
			out = append(out, finding.Finding{
				Rule: RuleVersionDrift, Severity: sev, Path: docPath, Line: p.Line,
				Message:    "documented " + tool + " " + written + " but " + src.file + " says " + src.version,
				Subject:    tool + " " + written,
				Suggestion: want,
			})
		}
	}
	return out
}

// prefix is the first n dot components of v (all of them when v has fewer).
func prefix(v string, n int) string {
	parts := strings.Split(v, ".")
	return strings.Join(parts[:min(n, len(parts))], ".")
}

// pick chooses the source to compare against at precision n: the non-advisory
// ones must agree with each other at that precision, otherwise the tool is
// skipped; with none of them, an advisory source stands in.
func pick(all []source, n int) (source, bool) {
	var hard []source
	for _, s := range all {
		if !s.advisory {
			hard = append(hard, s)
		}
	}
	if len(hard) == 0 {
		if len(all) == 0 {
			return source{}, false
		}
		return all[0], true
	}
	for _, s := range hard[1:] {
		if prefix(s.version, n) != prefix(hard[0].version, n) {
			return source{}, false
		}
	}
	return hard[0], true
}

// inSection reports whether the nearest heading at or
// before line names a setup-like section; no heading before it means no.
//
// ponytail: copied from envvars; share it when a third check needs it.
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

// sources lists the version files of tool found in root, in precedence order.
func sources(root, tool string) []source {
	var out []source
	add := func(file, version string, advisory bool) {
		if versionRE.MatchString(version) {
			out = append(out, source{version, file, advisory})
		}
	}
	read := func(file string) string {
		b, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			return ""
		}
		return string(b)
	}
	plain := func(file string) {
		add(file, strings.TrimPrefix(strings.TrimSpace(read(file)), "v"), false)
	}

	switch tool {
	case "node":
		plain(".nvmrc")
		plain(".node-version")
	case "python":
		plain(".python-version")
	case "go":
		for _, l := range strings.Split(read("go.mod"), "\n") {
			if f := strings.Fields(l); len(f) == 2 && f[0] == "go" {
				add("go.mod", f[1], false)
				break
			}
		}
	case "rust":
		var t struct{ Toolchain struct{ Channel string } }
		if toml.Unmarshal([]byte(read("rust-toolchain.toml")), &t) == nil {
			add("rust-toolchain.toml", t.Toolchain.Channel, false)
		}
		plain("rust-toolchain")
	}
	for _, l := range strings.Split(read(".tool-versions"), "\n") {
		if f := strings.Fields(l); len(f) >= 2 && toolNames[f[0]] == tool {
			add(".tool-versions", f[1], false)
		}
	}
	switch tool {
	case "node":
		var pkg struct{ Engines struct{ Node string } }
		if json.Unmarshal([]byte(read("package.json")), &pkg) == nil {
			add("package.json", numberRE.FindString(pkg.Engines.Node), true)
		}
	case "python":
		var pp struct {
			Project struct {
				RequiresPython string `toml:"requires-python"`
			}
		}
		if toml.Unmarshal([]byte(read("pyproject.toml")), &pp) == nil {
			if m := pyBoundRE.FindStringSubmatch(pp.Project.RequiresPython); m != nil {
				add("pyproject.toml", m[1], false)
			}
		}
	}
	return out
}
