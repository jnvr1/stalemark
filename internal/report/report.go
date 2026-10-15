// Package report renders findings for humans and machines.
package report

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/jnvr1/stalemark/internal/finding"
)

// Formats maps --format names to reporters.
//
// ponytail: a function type is the interface; a Reporter interface with options appears when SARIF (#13) needs configuration.
var Formats = map[string]func(io.Writer, []finding.Finding) error{"text": Text, "json": JSON, "sarif": SARIF, "github": GitHub}

// Text writes findings as "path:line[:col]: RULE message", grouped by file in
// (path, line, col, rule, subject) order, followed by a summary line. The
// caller's slice is left untouched.
//
// ponytail: no color; add it only for a TTY and when NO_COLOR is unset (needs an isatty check) when someone asks.
func Text(w io.Writer, findings []finding.Finding) error {
	if len(findings) == 0 {
		_, err := io.WriteString(w, "no findings\n")
		return err
	}

	rows := sorted(findings)
	files := 0
	for i, f := range rows {
		if i > 0 && f.Path != rows[i-1].Path {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
		if i == 0 || f.Path != rows[i-1].Path {
			files++
		}
		loc := fmt.Sprintf("%s:%d", f.Path, f.Line)
		if f.Col > 0 {
			loc += fmt.Sprintf(":%d", f.Col)
		}
		if _, err := fmt.Fprintf(w, "%s: %s %s\n", loc, f.Rule, f.Message); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintf(w, "\n%s in %s\n", plural(len(rows), "finding"), plural(files, "file"))
	return err
}

// sorted returns a copy of findings in (path, line, col, rule, subject) order.
func sorted(findings []finding.Finding) []finding.Finding {
	s := slices.Clone(findings)
	slices.SortStableFunc(s, func(a, b finding.Finding) int {
		return cmp.Or(
			cmp.Compare(a.Path, b.Path),
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Col, b.Col),
			cmp.Compare(a.Rule, b.Rule),
			cmp.Compare(a.Subject, b.Subject),
		)
	})
	return s
}

type jsonFinding struct {
	Rule        string           `json:"rule"`
	Severity    finding.Severity `json:"severity"`
	File        string           `json:"file"`
	Line        int              `json:"line"`
	Column      int              `json:"column"`
	Message     string           `json:"message"`
	Subject     string           `json:"subject"`
	Fingerprint string           `json:"fingerprint"`
}

// JSON writes findings as the version 1 document described by
// docs/schema/findings-v1.json, in the same order as Text. The caller's slice
// is left untouched.
func JSON(w io.Writer, findings []finding.Finding) error {
	doc := struct {
		Version  int           `json:"version"`
		Findings []jsonFinding `json:"findings"`
	}{Version: 1, Findings: []jsonFinding{}}
	for _, f := range sorted(findings) {
		doc.Findings = append(doc.Findings, jsonFinding{
			f.Rule, f.Severity, f.Path, f.Line, f.Col, f.Message, f.Subject, f.Fingerprint(),
		})
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
