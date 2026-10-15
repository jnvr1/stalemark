// Package report renders findings for humans and machines.
package report

import (
	"cmp"
	"fmt"
	"io"
	"slices"

	"github.com/jnvr1/stalemark/internal/finding"
)

// Text writes findings as "path:line[:col]: RULE message", grouped by file in
// (path, line, col, rule, subject) order, followed by a summary line. The
// caller's slice is left untouched.
//
// ponytail: a function instead of a Reporter interface until the JSON reporter (#12) is the second implementation.
// ponytail: no color; add it only for a TTY and when NO_COLOR is unset (needs an isatty check) when someone asks.
func Text(w io.Writer, findings []finding.Finding) error {
	if len(findings) == 0 {
		_, err := io.WriteString(w, "no findings\n")
		return err
	}

	sorted := slices.Clone(findings)
	slices.SortStableFunc(sorted, func(a, b finding.Finding) int {
		return cmp.Or(
			cmp.Compare(a.Path, b.Path),
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Col, b.Col),
			cmp.Compare(a.Rule, b.Rule),
			cmp.Compare(a.Subject, b.Subject),
		)
	})

	files := 0
	for i, f := range sorted {
		if i > 0 && f.Path != sorted[i-1].Path {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
		if i == 0 || f.Path != sorted[i-1].Path {
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

	_, err := fmt.Fprintf(w, "\n%s in %s\n", plural(len(sorted), "finding"), plural(files, "file"))
	return err
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
