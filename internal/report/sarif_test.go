package report_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/report"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type namedCase = struct {
	name string
	in   []finding.Finding
}

// windowsCase has backslash paths. Its fingerprint in the golden is computed
// from the ORIGINAL path (docs\guide.md): Fingerprint hashes Finding.Path as-is.
var windowsCase = []finding.Finding{
	f("SM-A001", `docs\guide.md`, 3, 0, `npm script "x" not found in package.json`),
}

var escapingCase = []finding.Finding{
	f("SM-A001", "a,b:c/x.md", 2, 7, "100% sure\r\nnpm script \"x\": not found"),
}

func sarifCases() []namedCase {
	out := []namedCase{{"windows-paths", windowsCase}}
	for _, tc := range cases {
		out = append(out, namedCase{strings.TrimSuffix(tc.name, "-files"), tc.in})
	}
	return out
}

func golden(t *testing.T, got []byte, rel string) {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "report", filepath.FromSlash(rel))
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output differs from %s\ngot:\n%s", path, got)
	}
}

func many(n int) []finding.Finding {
	in := make([]finding.Finding, n)
	for i := range in {
		in[i] = f("SM-A001", "README.md", i+1, 0, `npm script "x" not found`)
	}
	return in
}

func TestSARIF(t *testing.T) {
	for _, tc := range sarifCases() {
		t.Run(tc.name, func(t *testing.T) {
			var got bytes.Buffer
			if err := report.SARIF(&got, tc.in); err != nil {
				t.Fatal(err)
			}
			golden(t, got.Bytes(), "sarif/"+tc.name+".sarif")
		})
	}
}

func compileSARIFSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	sch, err := jsonschema.NewCompiler().Compile(filepath.Join("..", "..", "testdata", "report", "sarif", "sarif-schema-2.1.0.json"))
	if err != nil {
		t.Fatal(err)
	}
	return sch
}

func TestSARIFSchema(t *testing.T) {
	sch := compileSARIFSchema(t)
	for _, tc := range append(sarifCases(), namedCase{"truncated", many(25001)}) {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := report.SARIF(&buf, tc.in); err != nil {
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

func TestSARIFSchemaRejectsWrongVersion(t *testing.T) {
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(`{"version":"2.0.0","runs":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := compileSARIFSchema(t).Validate(doc); err == nil {
		t.Error("schema accepted version 2.0.0")
	}
}

func TestSARIFTruncates(t *testing.T) {
	var buf bytes.Buffer
	if err := report.SARIF(&buf, many(25001)); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Runs []struct {
			Results     []json.RawMessage
			Invocations []struct {
				ToolExecutionNotifications []struct{ Message struct{ Text string } }
			}
		}
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	run := doc.Runs[0]
	if len(run.Results) != 25000 {
		t.Errorf("results = %d, want 25000", len(run.Results))
	}
	if got := run.Invocations[0].ToolExecutionNotifications[0].Message.Text; !strings.Contains(got, "1 findings truncated to 25000") {
		t.Errorf("notification = %q", got)
	}
}

func TestSARIFUnknownRuleKeepsOutput(t *testing.T) {
	var buf bytes.Buffer
	if err := report.SARIF(&buf, []finding.Finding{f("SM-X999", "a.md", 1, 0, "x")}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"text": "SM-X999"`) {
		t.Errorf("missing fallback description:\n%s", buf.String())
	}
}

func TestGitHub(t *testing.T) {
	for _, tc := range []namedCase{{"one", cases[1].in}, {"escaping", escapingCase}} {
		t.Run(tc.name, func(t *testing.T) {
			var got bytes.Buffer
			if err := report.GitHub(&got, tc.in); err != nil {
				t.Fatal(err)
			}
			golden(t, got.Bytes(), "github/"+tc.name+".txt")
		})
	}
}

func TestGitHubEmptyWritesNothing(t *testing.T) {
	var got bytes.Buffer
	if err := report.GitHub(&got, nil); err != nil || got.Len() != 0 {
		t.Errorf("got %q, %v", got.String(), err)
	}
}

func TestGitHubTruncates(t *testing.T) {
	var got bytes.Buffer
	if err := report.GitHub(&got, many(25002)); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(got.String(), "\n"), "\n")
	if len(lines) != 25001 {
		t.Fatalf("lines = %d, want 25001", len(lines))
	}
	if want := "::warning title=stalemark::2 findings truncated to 25000 (GitHub annotations limit)"; lines[25000] != want {
		t.Errorf("last line = %q", lines[25000])
	}
}
