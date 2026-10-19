package jsdoc_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jnvr1/stalemark/internal/config"
	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/jsdoc"
	"github.com/jnvr1/stalemark/internal/runner"
)

const fixtures = "../../testdata/jsdoc"

func cases(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != "node_modules" {
			dirs = append(dirs, filepath.Join(fixtures, e.Name()))
		}
	}
	if len(dirs) < 7 {
		t.Fatalf("fixtures: %v", dirs)
	}
	return dirs
}

func expected(t *testing.T, dir string) []finding.Finding {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want []finding.Finding
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	return want
}

func same(t *testing.T, got, want []finding.Finding) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

// The raw output is what eslint printed when the fixtures were recorded;
// parsing it needs no Node.
func TestParseGoldens(t *testing.T) {
	for _, dir := range cases(t) {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, "eslint.json"))
			if err != nil {
				t.Fatal(err)
			}
			got, err := jsdoc.Parse(dir, raw, finding.Warning)
			if err != nil {
				t.Fatal(err)
			}
			same(t, got, expected(t, dir))
		})
	}
}

func TestParseNormalizesPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	abs, _ := json.Marshal(filepath.Join(root, "src", "m.js"))
	raw := `[
 {"filePath": "<root>/src/b.js", "messages": [{"ruleId": "jsdoc/require-param", "message": "Missing JSDoc @param \"x\" declaration.", "line": 2, "column": 1}]},
 {"filePath": ` + string(abs) + `, "messages": [
  {"ruleId": "jsdoc/check-types", "message": "bad", "line": 9, "column": 4},
  {"ruleId": "jsdoc/check-param-names", "message": "no quote", "line": 9, "column": 4, "suggestions": [{"desc": "fix it"}]},
  {"ruleId": "no-unused-vars", "message": "dropped", "line": 1, "column": 1},
  {"ruleId": null, "message": "parse error", "line": 1, "column": 1}]}
]`
	got, err := jsdoc.Parse(root, []byte(raw), finding.Note)
	if err != nil {
		t.Fatal(err)
	}
	want := []finding.Finding{
		{Rule: "SM-B021", Severity: finding.Note, Path: "src/b.js", Line: 2, Col: 1, Message: `eslint-plugin-jsdoc jsdoc/require-param: Missing JSDoc @param "x" declaration.`, Subject: "x"},
		{Rule: "SM-B020", Severity: finding.Note, Path: "src/m.js", Line: 9, Col: 4, Message: "eslint-plugin-jsdoc jsdoc/check-param-names: no quote", Suggestion: "fix it"},
		{Rule: "SM-B022", Severity: finding.Note, Path: "src/m.js", Line: 9, Col: 4, Message: "eslint-plugin-jsdoc jsdoc/check-types: bad"},
	}
	same(t, got, want)
}

func TestParseInvalidJSON(t *testing.T) {
	if _, err := jsdoc.Parse(t.TempDir(), []byte("not json"), finding.Warning); err == nil {
		t.Error("want an error for output that is not the json formatter's")
	}
}

func cfg() config.JsdocConfig {
	return config.JsdocConfig{Paths: []string{"src"}, MinVersion: "65.0.0", EslintMinVersion: "9.0.0", Severity: "warning"}
}

// withTool skips unless eslint is on PATH and eslint-plugin-jsdoc is installed
// above the fixtures; the tools CI job installs both and requires the PASS.
func withTool(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("eslint"); err != nil {
		t.Skip("eslint not on PATH")
	}
	if _, err := os.Stat("../../testdata/jsdoc/node_modules/eslint-plugin-jsdoc/package.json"); err != nil {
		t.Skip("eslint-plugin-jsdoc not installed under testdata/jsdoc")
	}
}

func TestCheckWithTool(t *testing.T) {
	withTool(t)
	for _, dir := range cases(t) {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			root, err := filepath.Abs(dir)
			if err != nil {
				t.Fatal(err)
			}
			got, err := jsdoc.Check(runner.Context{Root: root}, cfg())
			if err != nil {
				t.Fatal(err)
			}
			same(t, got, expected(t, dir))
		})
	}
}

func TestCheckVersionFloors(t *testing.T) {
	withTool(t)
	root, _ := filepath.Abs(filepath.Join(fixtures, "ok"))
	c := cfg()
	c.EslintMinVersion = "99.0.0"
	if _, err := jsdoc.Check(runner.Context{Root: root}, c); err == nil || !strings.Contains(err.Error(), "eslint") || !strings.Contains(err.Error(), "is older than the required 99.0.0") {
		t.Errorf("eslint floor error = %v", err)
	}
	c = cfg()
	c.MinVersion = "99.0.0"
	if _, err := jsdoc.Check(runner.Context{Root: root}, c); err == nil || !strings.Contains(err.Error(), "eslint-plugin-jsdoc") || !strings.Contains(err.Error(), "is older than the required 99.0.0") {
		t.Errorf("plugin floor error = %v", err)
	}
}

// A broken config makes eslint exit 2; stalemark must surface its message.
func TestCheckEslintExit2(t *testing.T) {
	withTool(t)
	root, err := os.MkdirTemp(fixtures, "broken-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.WriteFile(filepath.Join(root, "eslint.config.mjs"), []byte("export default [ {{{ ;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "a.js"), []byte("export const a = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(root)
	_, err = jsdoc.Check(runner.Context{Root: abs}, cfg())
	if err == nil || !strings.Contains(err.Error(), "eslint exited 2") || !strings.Contains(err.Error(), "SyntaxError") {
		t.Errorf("exit 2 error = %v", err)
	}
}

func TestCheckErrors(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := jsdoc.Check(runner.Context{Root: t.TempDir()}, cfg())
	if err == nil || !strings.Contains(err.Error(), `tool "eslint" not found on PATH`) {
		t.Errorf("missing tool error = %v", err)
	}
}
