package report_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/report"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

var update = flag.Bool("update", false, "rewrite the golden files from the current output")

// f builds a synthetic finding; the subject is the first quoted token of msg,
// as the resolvers report it, so fingerprints in the goldens are realistic.
func f(rule, path string, line, col int, msg string) finding.Finding {
	subject := ""
	if _, rest, ok := strings.Cut(msg, `"`); ok {
		subject, _, _ = strings.Cut(rest, `"`)
	}
	return finding.Finding{Rule: rule, Severity: finding.Error, Path: path, Line: line, Col: col, Message: msg, Subject: subject}
}

var cases = []struct {
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

func TestText(t *testing.T) {
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

func TestJSON(t *testing.T) {
	for _, tc := range cases {
		name := strings.TrimSuffix(tc.name, "-files")
		t.Run(name, func(t *testing.T) {
			var got bytes.Buffer
			if err := report.JSON(&got, tc.in); err != nil {
				t.Fatal(err)
			}
			golden := filepath.Join("..", "..", "testdata", "report", "json", name+".json")
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

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	sch, err := jsonschema.NewCompiler().Compile(filepath.Join("..", "..", "docs", "schema", "findings-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	return sch
}

func TestJSONSchema(t *testing.T) {
	sch := compileSchema(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := report.JSON(&buf, tc.in); err != nil {
				t.Fatal(err)
			}
			doc, err := jsonschema.UnmarshalJSON(&buf)
			if err != nil {
				t.Fatal(err)
			}
			if err := sch.Validate(doc); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestJSONSchemaRejectsMissingKey(t *testing.T) {
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(`{"version":1,"findings":[{"rule":"SM-A001"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := compileSchema(t).Validate(doc); err == nil {
		t.Error("schema accepted a finding with missing keys")
	}
}
