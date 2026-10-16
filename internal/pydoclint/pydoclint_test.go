package pydoclint_test

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
	"github.com/jnvr1/stalemark/internal/pydoclint"
	"github.com/jnvr1/stalemark/internal/runner"
)

const fixtures = "../../testdata/pydoclint"

func cases(t *testing.T) []string {
	t.Helper()
	dirs, err := filepath.Glob(filepath.Join(fixtures, "*"))
	if err != nil || len(dirs) < 7 {
		t.Fatalf("fixtures: %v, %v", dirs, err)
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

// The raw output is what pydoclint printed when the fixtures were recorded;
// parsing it needs no tool.
func TestParseGoldens(t *testing.T) {
	for _, dir := range cases(t) {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, "pydoclint.txt"))
			if err != nil {
				t.Fatal(err)
			}
			got := pydoclint.Parse(dir, raw, finding.Warning)
			want := expected(t, dir)
			if len(got) == 0 && len(want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got  %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestParseNormalizesPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	abs := filepath.Join(root, "src", "m.py")
	raw := "src\\a.py:3: DOC101: Function `f`: x\r\n" + abs + ":2: DOC501: Method `C.g`: y\r\nnoise line\r\nsrc/a.py:9: DOC999: dropped\r\n"
	got := pydoclint.Parse(root, []byte(raw), finding.Note)
	want := []finding.Finding{
		{Rule: "SM-B010", Severity: finding.Note, Path: "src/a.py", Line: 3, Message: "pydoclint DOC101: Function `f`: x", Subject: "f"},
		{Rule: "SM-B013", Severity: finding.Note, Path: "src/m.py", Line: 2, Message: "pydoclint DOC501: Method `C.g`: y", Subject: "C.g"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

// TestCheckWithTool runs the real tool on every fixture. It is skipped when
// pydoclint is not on PATH; the tools CI job installs it and requires the PASS.
func TestCheckWithTool(t *testing.T) {
	if _, err := exec.LookPath("pydoclint"); err != nil {
		t.Skip("pydoclint not on PATH")
	}
	for _, dir := range cases(t) {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			root, err := filepath.Abs(dir)
			if err != nil {
				t.Fatal(err)
			}
			got, err := pydoclint.Check(runner.Context{Root: root}, config.PydoclintConfig{Paths: []string{"."}, MinVersion: "0.11.1", Severity: "warning"})
			if err != nil {
				t.Fatal(err)
			}
			want := expected(t, dir)
			if len(got) == 0 && len(want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got  %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestCheckOlderAndBaselineUntouched(t *testing.T) {
	if _, err := exec.LookPath("pydoclint"); err != nil {
		t.Skip("pydoclint not on PATH")
	}
	root := t.TempDir()
	files := map[string]string{
		"pyproject.toml": "[tool.pydoclint]\nstyle = \"google\"\nbaseline = \"base.txt\"\n",
		"base.txt":       "stale baseline content\n",
		"mod.py":         "def f(a, b):\n    \"\"\"Do.\n\n    Args:\n        a: x\n    \"\"\"\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.PydoclintConfig{Paths: []string{"."}, MinVersion: "99.0.0", Severity: "warning"}
	if _, err := pydoclint.Check(runner.Context{Root: root}, cfg); err == nil || !strings.Contains(err.Error(), "is older than the required 99.0.0") {
		t.Errorf("older version error = %v", err)
	}
	cfg.MinVersion = "0.11.1"
	_, _ = pydoclint.Check(runner.Context{Root: root}, cfg)
	for name, body := range files {
		if got, err := os.ReadFile(filepath.Join(root, name)); err != nil || string(got) != body {
			t.Errorf("%s changed: %q, %v", name, got, err)
		}
	}
}

func TestCheckErrors(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := pydoclint.Check(runner.Context{Root: t.TempDir()}, config.PydoclintConfig{Paths: []string{"."}, MinVersion: "0.11.1", Severity: "warning"})
	if err == nil || !strings.Contains(err.Error(), `tool "pydoclint" not found on PATH`) {
		t.Errorf("missing tool error = %v", err)
	}
}
