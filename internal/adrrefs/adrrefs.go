// Package adrrefs checks the references inside ADRs: links to other ADR files,
// Go import paths of this module, and (through pathref) files and directories.
// Deprecated and superseded ADRs are historical and are downgraded or skipped.
package adrrefs

import (
	"bufio"
	"cmp"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/jnvr1/stalemark/internal/adr"
	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/pathref"
	"github.com/jnvr1/stalemark/internal/runner"
)

// Rule IDs reported by this package.
const (
	// RuleADRLinkMissing: an ADR links to an "NNNN-title.md" that does not exist next to it.
	RuleADRLinkMissing = "SM-D002"
	// RuleModulePathMissing: a Go import path under this module maps to no directory.
	RuleModulePathMissing = "SM-D003"
)

var linkRe = regexp.MustCompile(`^\d{1,5}-[^/]*\.md$`)

// Check adjusts docFindings (the pipeline's findings so far) and adds the ADR
// reference findings. Findings of deprecated and superseded ADRs for the path,
// ADR link and module path rules become warnings, or are dropped when
// historical is "skip". ADRs whose relative path is not in scanned (docs the
// pipeline already ran pathref on) get pathref run here. Input order is kept
// and new findings follow, sorted by path, line and column.
//
// ponytail: only the root go.mod counts (no nested modules, workspaces or
// replace directives) and only exact module-path prefixes, no fuzzy matching.
func Check(ctx runner.Context, ix *pathref.Index, adrs []adr.ADR, scanned map[string]bool, historical string, docFindings []finding.Finding) []finding.Finding {
	old := map[string]bool{}
	for _, a := range adrs {
		if a.Status == "deprecated" || a.Status == "superseded" {
			old[runner.Rel(ctx, a.Path)] = true
		}
	}
	var added []finding.Finding
	mod := moduleName(ctx.Root)
	for _, a := range adrs {
		if !scanned[runner.Rel(ctx, a.Path)] {
			added = append(added, pathref.CheckWith(ctx, ix, a.Doc)...)
		}
		added = append(added, linkFindings(ctx, a)...)
		if mod != "" {
			added = append(added, moduleFindings(ctx, a, mod)...)
		}
	}
	slices.SortStableFunc(added, func(a, b finding.Finding) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Col, b.Col))
	})
	var out []finding.Finding
	for _, f := range slices.Concat(docFindings, added) {
		if old[f.Path] && (f.Rule == pathref.RulePathNotFound || f.Rule == RuleADRLinkMissing || f.Rule == RuleModulePathMissing) {
			if historical == "skip" {
				continue
			}
			f.Severity = finding.Warning
		}
		out = append(out, f)
	}
	return out
}

// linkFindings reports links to sibling ADR files that do not exist.
func linkFindings(ctx runner.Context, a adr.ADR) []finding.Finding {
	var out []finding.Finding
	for _, l := range a.Doc.Links {
		target, _, _ := strings.Cut(l.Target, "#")
		target, _, _ = strings.Cut(target, "?")
		if !linkRe.MatchString(target) {
			continue
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(a.Path), target)); err == nil {
			continue
		}
		out = append(out, finding.Finding{
			Rule: RuleADRLinkMissing, Severity: finding.Error, Path: runner.Rel(ctx, a.Path),
			Line: l.Line, Col: l.Col,
			Message: `ADR link "` + target + `" does not exist`, Subject: target,
		})
	}
	return out
}

// moduleFindings reports code spans and link targets that name a package of
// module mod whose directory does not exist.
func moduleFindings(ctx runner.Context, a adr.ADR, mod string) []finding.Finding {
	type ref struct {
		text      string
		line, col int
	}
	var refs []ref
	for _, s := range a.Doc.Spans {
		refs = append(refs, ref{s.Text, s.Line, s.Col})
	}
	for _, l := range a.Doc.Links {
		refs = append(refs, ref{l.Target, l.Line, l.Col})
	}
	var out []finding.Finding
	seen := map[string]bool{}
	for _, r := range refs {
		s := strings.Trim(strings.TrimPrefix(strings.TrimSpace(r.text), "import "), `" `)
		rest, ok := strings.CutPrefix(s, mod+"/")
		if !ok || strings.ContainsAny(s, " \t*{}<>$") || seen[s+"\x00"+string(rune(r.line))] {
			continue
		}
		seen[s+"\x00"+string(rune(r.line))] = true
		if _, err := os.Stat(filepath.Join(ctx.Root, filepath.FromSlash(rest))); err == nil {
			continue
		}
		out = append(out, finding.Finding{
			Rule: RuleModulePathMissing, Severity: finding.Error, Path: runner.Rel(ctx, a.Path),
			Line: r.line, Col: r.col,
			Message: `module path "` + s + `" does not resolve to a directory`, Subject: s,
		})
	}
	return out
}

// moduleName is the path of the "module" line of root/go.mod, "" when there is none.
func moduleName(root string) string {
	f, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "module"); ok && rest != "" && (rest[0] == ' ' || rest[0] == '\t') {
			rest, _, _ = strings.Cut(rest, "//")
			return strings.Trim(strings.TrimSpace(rest), `"`)
		}
	}
	return ""
}
