// Package adrlibs checks the dependencies accepted ADRs say they use against
// the manifests at the check root.
package adrlibs

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"go.yaml.in/yaml/v3"

	"github.com/jnvr1/stalemark/internal/adr"
	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/runner"
)

// Rules: a claim whose alias is in no manifest is an error (a warning when it
// came from prose); a claim with no alias and no manifest match is a notice.
const (
	RuleMissing = "SM-D004"
	RuleUnknown = "SM-D005"
)

// aliases.yml sits next to this code, not in internal/adr as the issue says,
// because this package is the one that reads it.
//
//go:embed aliases.yml
var aliasesYML []byte

var aliases = func() map[string][]string {
	var m map[string][]string
	if err := yaml.Unmarshal(aliasesYML, &m); err != nil {
		panic("adrlibs: aliases.yml: " + err.Error())
	}
	return m
}()

// ponytail: single-word claims only; "we use Apache Kafka" claims "apache".
var (
	freeRe = regexp.MustCompile(`(?i)\bwe\s+(?:use|chose|adopt(?:ed)?|picked|rely on)\s+([A-Za-z][A-Za-z0-9.+_-]{1,40})`)
	stop   = []string{"the", "a", "an", "it", "this", "that", "our", "only", "both"}
)

type claim struct {
	name  string
	line  int
	prose bool
}

// Check reports, for every accepted ADR, each declared dependency that no
// manifest at ctx.Root lists. With freetext, "we use X" sentences count too,
// as warnings.
//
// ponytail: only root manifests are read; walk subdirectories if monorepos need it.
func Check(ctx runner.Context, adrs []adr.ADR, freetext bool) []finding.Finding {
	var have map[string]bool
	var out []finding.Finding
	for _, a := range adrs {
		if a.Status != "accepted" {
			continue
		}
		var claims []claim
		for i, n := range a.Uses {
			claims = append(claims, claim{n, a.UsesLines[i], false})
		}
		if freetext {
			for _, p := range a.Doc.Paragraphs {
				for _, m := range freeRe.FindAllStringSubmatch(p.Text, -1) {
					n := strings.TrimRight(m[1], ".-_")
					if !slices.Contains(stop, strings.ToLower(n)) {
						claims = append(claims, claim{n, p.Line, true})
					}
				}
			}
		}
		if len(claims) == 0 {
			continue
		}
		if have == nil {
			have = manifestNames(ctx.Root)
		}
		seen := map[string]bool{}
		for _, c := range claims {
			key := norm(c.name)
			if seen[key] {
				continue
			}
			seen[key] = true
			f := finding.Finding{Path: runner.Rel(ctx, a.Path), Line: c.line, Subject: c.name}
			vals, known := aliases[key]
			switch {
			case have[key] || slices.ContainsFunc(vals, func(v string) bool { return have[norm(v)] }):
				continue
			case known:
				f.Rule, f.Severity = RuleMissing, finding.Error
				if c.prose {
					f.Severity = finding.Warning
				}
				f.Message = fmt.Sprintf("ADR says it uses %q but no manifest lists it (%s)", c.name, strings.Join(vals[:min(4, len(vals))], ", "))
			default:
				f.Rule, f.Severity = RuleUnknown, finding.Note
				f.Message = fmt.Sprintf("ADR says it uses %q, which is not in any manifest and has no alias; add it to internal/adrlibs/aliases.yml if it is a dependency", c.name)
			}
			out = append(out, f)
		}
	}
	return out
}

func norm(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.Trim(strings.TrimSpace(s), "`")), "_", "-")
}

// manifestNames is every dependency name the root manifests declare,
// normalized. Unreadable or unparsable manifests are ignored.
func manifestNames(root string) map[string]bool {
	have := map[string]bool{}
	add := func(names ...string) {
		for _, n := range names {
			if n = norm(n); n != "" {
				have[n] = true
			}
		}
	}
	read := func(name string) []byte {
		b, _ := os.ReadFile(filepath.Join(root, name))
		return b
	}

	// go.mod: full path and last segment.
	inBlock := false
	for line := range strings.Lines(string(read("go.mod"))) {
		line, _, _ = strings.Cut(line, "//")
		f := strings.Fields(line)
		switch {
		case inBlock && len(f) == 1 && f[0] == ")":
			inBlock = false
		case len(f) == 2 && f[0] == "require" && f[1] == "(":
			inBlock = true
		case inBlock && len(f) >= 1:
			add(f[0], f[0][strings.LastIndex(f[0], "/")+1:])
		case len(f) >= 2 && f[0] == "require":
			add(f[1], f[1][strings.LastIndex(f[1], "/")+1:])
		}
	}

	// package.json: full name and the part after the scope.
	var pkg map[string]any
	if json.Unmarshal(read("package.json"), &pkg) == nil {
		for _, k := range []string{"dependencies", "devDependencies", "peerDependencies"} {
			deps, _ := pkg[k].(map[string]any)
			for n := range deps {
				add(n)
				if _, after, ok := strings.Cut(n, "/"); ok && strings.HasPrefix(n, "@") {
					add(after)
				}
			}
		}
	}

	var py map[string]any
	if toml.Unmarshal(read("pyproject.toml"), &py) == nil {
		reqs := func(v any) {
			items, _ := v.([]any)
			for _, it := range items {
				if s, ok := it.(string); ok {
					add(requirement(s))
				}
			}
		}
		keys := func(m map[string]any) {
			for k := range m {
				add(k)
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
		for g := range table(py, "tool", "poetry", "group") {
			keys(table(py, "tool", "poetry", "group", g, "dependencies"))
		}
	}

	var cargo map[string]any
	if toml.Unmarshal(read("Cargo.toml"), &cargo) == nil {
		crates := func(deps map[string]any) {
			for k, v := range deps {
				add(k)
				if d, ok := v.(map[string]any); ok {
					if orig, _ := d["package"].(string); orig != "" {
						add(orig)
					}
				}
			}
		}
		for _, kind := range []string{"dependencies", "dev-dependencies", "build-dependencies"} {
			crates(table(cargo, kind))
			for _, tgt := range table(cargo, "target") {
				tm, _ := tgt.(map[string]any)
				crates(table(tm, kind))
			}
		}
		crates(table(cargo, "workspace", "dependencies"))
	}

	files := []string{"requirements.txt"}
	for _, pat := range []string{"requirements-*.txt", "requirements/*.txt"} {
		m, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(pat)))
		for _, p := range m {
			if r, err := filepath.Rel(root, p); err == nil {
				files = append(files, r)
			}
		}
	}
	for _, f := range files {
		for line := range strings.Lines(string(read(f))) {
			line, _, _ = strings.Cut(line, " #")
			if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "-") {
				add(requirement(line))
			}
		}
	}
	return have
}

// requirement is the project name of a PEP 508 requirement: the text before
// extras, a version specifier, a marker or a URL.
func requirement(req string) string {
	if i := strings.IndexAny(req, " [<>=!~;(@"); i >= 0 {
		req = req[:i]
	}
	return req
}

func table(m map[string]any, keys ...string) map[string]any {
	for _, k := range keys {
		m, _ = m[k].(map[string]any)
	}
	return m
}
