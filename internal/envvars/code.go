package envvars

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/jnvr1/stalemark/internal/finding"
)

// Rules of the code pass. SM-E003 is a note because the scan is a regex: it can
// match strings and comments. SM-E004 is opt-in (env.documented-unused).
const (
	RuleEnvReadUndocumented = "SM-E003"
	RuleEnvDocumentedUnused = "SM-E004"
)

// Read is one literal env var read in a source file; Path is relative to the
// scanned root with forward slashes.
type Read struct {
	Name, Path string
	Line       int
	TestOnly   bool
}

// lang is a source family: the regexes of its literal reads (name in group 1)
// and whether `#` (instead of `//`) starts a line comment.
type lang struct {
	res  []*regexp.Regexp
	hash bool
}

var (
	jsLang = lang{res: []*regexp.Regexp{
		regexp.MustCompile(`process\.env\.([A-Za-z_][A-Za-z0-9_]*)`),
		regexp.MustCompile(`process\.env\[\s*['"]([A-Za-z_][A-Za-z0-9_]*)['"]\s*\]`),
	}}
	pyLang = lang{hash: true, res: []*regexp.Regexp{
		regexp.MustCompile(`os\.environ\[\s*['"]([A-Za-z_]\w*)['"]\s*\]`),
		regexp.MustCompile(`os\.environ\.get\(\s*['"]([A-Za-z_]\w*)['"]`),
		regexp.MustCompile(`os\.getenv\(\s*['"]([A-Za-z_]\w*)['"]`),
	}}
	goLang = lang{res: []*regexp.Regexp{
		regexp.MustCompile(`os\.Getenv\(\s*"([A-Za-z_]\w*)"`),
		regexp.MustCompile(`os\.LookupEnv\(\s*"([A-Za-z_]\w*)"`),
	}}
	rustLang = lang{res: []*regexp.Regexp{regexp.MustCompile(`env::var(?:_os)?\(\s*"([A-Za-z_]\w*)"`)}}

	langs = map[string]lang{
		".js": jsLang, ".mjs": jsLang, ".cjs": jsLang, ".ts": jsLang, ".tsx": jsLang, ".jsx": jsLang,
		".py": pyLang, ".go": goLang, ".rs": rustLang,
	}
	blockRE = regexp.MustCompile(`/\*.*?\*/`)
)

// skipDirs are never entered below the scan root.
//
// ponytail: fixed list, .gitignore is not parsed and testdata holds fixtures,
// not app code; make it configurable (or parse .gitignore) if a repo needs more.
var skipDirs = []string{".git", "node_modules", "vendor", "target", "dist", "build", ".venv", "venv", "testdata"}

// ScanCode returns the literal env reads under root, in path then line order.
// Dynamic keys (`process.env[name]`) have no literal and never match.
//
// ponytail: regex over lines, not an AST. Matches inside strings are reads,
// `//` or `#` before a match hides it (even inside a string), and only
// single-line `/* */` and lines starting with `*` count as comments: multi-line
// block comments and Python docstrings can yield false reads. Upgrade =
// tree-sitter (ADR 0002 allows cgo then).
func ScanCode(root string) ([]Read, error) {
	var reads []Read
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && slices.Contains(skipDirs, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		l, ok := langs[strings.ToLower(filepath.Ext(p))]
		if !ok {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		test := testOnly(rel)
		for i, line := range strings.Split(string(src), "\n") {
			for _, name := range l.names(line) {
				reads = append(reads, Read{name, rel, i + 1, test})
			}
		}
		return nil
	})
	return reads, err
}

// names returns the live literal reads of one line.
func (l lang) names(line string) []string {
	if !l.hash && strings.HasPrefix(strings.TrimSpace(line), "*") {
		return nil // body of a multi-line block comment
	}
	marker := "//"
	if l.hash {
		marker = "#"
	}
	cut := strings.Index(line, marker)
	var blocks [][]int
	if !l.hash {
		blocks = blockRE.FindAllStringIndex(line, -1)
	}
	var out []string
	for _, re := range l.res {
		for _, m := range re.FindAllStringSubmatchIndex(line, -1) {
			start := m[0]
			if cut >= 0 && cut < start || slices.ContainsFunc(blocks, func(b []int) bool { return b[0] <= start && start < b[1] }) {
				continue
			}
			out = append(out, line[m[2]:m[3]])
		}
	}
	return out
}

// testOnly reports whether rel lives under a test directory or is named like a
// test file.
func testOnly(rel string) bool {
	parts := strings.Split(rel, "/")
	for _, dir := range parts[:len(parts)-1] {
		if slices.Contains([]string{"test", "tests", "spec", "__tests__"}, dir) {
			return true
		}
	}
	name := parts[len(parts)-1]
	if strings.HasPrefix(name, "test_") || strings.HasSuffix(name, "_test.go") {
		return true
	}
	for _, mid := range []string{".test.", ".spec."} {
		if strings.Contains(name, mid) {
			return true
		}
	}
	return false
}

// CheckCode compares the documented names with the code reads. SM-E003 (note):
// a name read by non-test code that no doc mentions, once per name at its first
// read; tooling names are exempt. SM-E004 (warning, only when documentedUnused):
// a documented name no file reads, test files included.
func CheckCode(documented map[string]Loc, reads []Read, documentedUnused bool) []finding.Finding {
	var out []finding.Finding
	read, reported := map[string]bool{}, map[string]bool{}
	for _, r := range reads {
		read[r.Name] = true
		if _, ok := documented[r.Name]; ok || r.TestOnly || reported[r.Name] || tooling(r.Name) {
			continue
		}
		reported[r.Name] = true
		out = append(out, finding.Finding{
			Rule: RuleEnvReadUndocumented, Severity: finding.Note, Path: r.Path, Line: r.Line, Subject: r.Name,
			Message: `env var "` + r.Name + `" is read by code but not documented`,
		})
	}
	if documentedUnused {
		names := make([]string, 0, len(documented))
		for n := range documented {
			if !read[n] {
				names = append(names, n)
			}
		}
		slices.SortFunc(names, func(a, b string) int {
			la, lb := documented[a], documented[b]
			if c := strings.Compare(la.Path, lb.Path); c != 0 {
				return c
			}
			if la.Line != lb.Line {
				return la.Line - lb.Line
			}
			return strings.Compare(a, b)
		})
		for _, n := range names {
			l := documented[n]
			out = append(out, finding.Finding{
				Rule: RuleEnvDocumentedUnused, Severity: finding.Warning, Path: l.Path, Line: l.Line, Col: l.Col, Subject: n,
				Message: `env var "` + n + `" is documented but never read by code`,
			})
		}
	}
	return out
}
