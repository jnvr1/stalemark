package report_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/report"
)

var update = flag.Bool("update", false, "rewrite the golden files from the current output")

func f(rule, path string, line, col int, msg string) finding.Finding {
	return finding.Finding{Rule: rule, Severity: finding.Error, Path: path, Line: line, Col: col, Message: msg}
}

func TestText(t *testing.T) {
	cases := []struct {
		name string
		in   []finding.Finding
	}{
		{"none", nil},
		{"one", []finding.Finding{
			f("SM-A001", "README.md", 7, 0, `npm script "buidl" not found in package.json (did you mean "build"?)`),
		}},
		// Deliberately shuffled: mixed files, same file different lines, same
		// line different rules, one with a column.
		{"many-files", []finding.Finding{
			f("SM-A001", "docs/guide.md", 12, 0, `npm script "tset" not found in package.json (did you mean "test"?)`),
			f("SM-A002", "README.md", 20, 0, `make target "bild" not found in Makefile`),
			f("SM-A001", "README.md", 7, 3, `npm script "buidl" not found in package.json`),
			f("SM-A001", "README.md", 20, 0, `npm script "lint" not found in package.json`),
			f("SM-A006", "docs/guide.md", 4, 0, `poetry script "serve" not found in pyproject.toml`),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got bytes.Buffer
			if err := report.Text(&got, tc.in); err != nil {
				t.Fatal(err)
			}
			golden := filepath.Join("..", "..", "testdata", "report", "text", tc.name+".txt")
			if *update {
				if err := os.WriteFile(golden, got.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Bytes(), want) {
				t.Errorf("output differs from %s\ngot:\n%s", golden, got.Bytes())
			}
		})
	}
}

func TestTextDoesNotReorderInput(t *testing.T) {
	in := []finding.Finding{f("SM-A001", "b.md", 1, 0, "x"), f("SM-A001", "a.md", 1, 0, "x")}
	if err := report.Text(&bytes.Buffer{}, in); err != nil {
		t.Fatal(err)
	}
	if in[0].Path != "b.md" {
		t.Error("Text sorted the caller's slice")
	}
}
