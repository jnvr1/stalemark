package oasdiff_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jnvr1/stalemark/internal/config"
	"github.com/jnvr1/stalemark/internal/extern"
	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/oasdiff"
	"github.com/jnvr1/stalemark/internal/runner"
)

const fixtures = "../../testdata/oasdiff"

func cases(t *testing.T) []string {
	t.Helper()
	dirs, err := filepath.Glob(filepath.Join(fixtures, "*"))
	if err != nil || len(dirs) < 8 {
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

func load(t *testing.T, dir string) config.OasdiffConfig {
	t.Helper()
	cfg, err := config.Load(dir)
	if err != nil || cfg.API.Oasdiff == nil {
		t.Fatalf("config of %s: %+v, %v", dir, cfg.API.Oasdiff, err)
	}
	return *cfg.API.Oasdiff
}

// The raw output is what `oasdiff changelog` printed when the fixtures were
// recorded; parsing it needs no tool. changelog.json, not oasdiff.json: oasdiff
// reads a file of that name in its working directory as its own config.
func TestParseGoldens(t *testing.T) {
	n := 0
	for _, dir := range cases(t) {
		raw, err := os.ReadFile(filepath.Join(dir, "changelog.json"))
		if os.IsNotExist(err) {
			continue // missing-against has no tool output
		}
		if err != nil {
			t.Fatal(err)
		}
		n++
		t.Run(filepath.Base(dir), func(t *testing.T) {
			got, err := oasdiff.Parse(dir, raw, load(t, dir).Spec, finding.Warning)
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
	if n < 7 {
		t.Errorf("only %d recorded fixtures", n)
	}
}

func TestParseLevelsSortingAndPaths(t *testing.T) {
	raw := "[\r\n" +
		`{"id":"i-info","text":"dropped","level":1,"operation":"GET","path":"/a"},` + "\r\n" +
		`{"id":"w-two","text":"a ` + "`query`" + ` request parameter ` + "`q`" + ` gone","level":2,"operation":"GET","path":"/b","baseSource":{"file":"sub\\\\api.yaml","line":9,"column":3}},` + "\r\n" +
		`{"id":"e-three","text":"x","level":3,"operation":"PUT","path":"/a","baseSource":{"file":"sub/api.yaml","line":4,"column":2}},` + "\r\n" +
		`{"id":"endpoint-added","text":"endpoint added","level":1,"operation":"POST","path":"/c","revisionSource":{"file":"gen.yaml","line":2,"column":1}}` + "\r\n]\r\n"
	got, err := oasdiff.Parse(t.TempDir(), []byte(raw), "sub/api.yaml", finding.Error)
	if err != nil {
		t.Fatal(err)
	}
	want := []finding.Finding{
		{Rule: "SM-B003", Severity: finding.Note, Path: "sub/api.yaml", Line: 1, Message: "oasdiff endpoint-added: endpoint added", Subject: "POST /c"},
		{Rule: "SM-B001", Severity: finding.Error, Path: "sub/api.yaml", Line: 4, Col: 2, Message: "oasdiff e-three: x", Subject: "PUT /a"},
		{Rule: "SM-B002", Severity: finding.Error, Path: "sub/api.yaml", Line: 9, Col: 3, Message: "oasdiff w-two: a 'query' request parameter 'q' gone", Subject: "GET /b q"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestParseInvalid(t *testing.T) {
	if _, err := oasdiff.Parse(".", []byte("Error: boom"), "a.yaml", finding.Warning); err == nil {
		t.Error("non-JSON output must be an error")
	}
}

// TestCheckWithTool runs the real tool on every fixture. It is skipped when
// oasdiff is not on PATH; the tools CI job installs it and requires the PASS.
func TestCheckWithTool(t *testing.T) {
	if _, err := exec.LookPath("oasdiff"); err != nil {
		t.Skip("oasdiff not on PATH")
	}
	for _, dir := range cases(t) {
		if _, err := os.Stat(filepath.Join(dir, "expected.json")); err != nil {
			continue
		}
		t.Run(filepath.Base(dir), func(t *testing.T) {
			root, err := filepath.Abs(dir)
			if err != nil {
				t.Fatal(err)
			}
			got, err := oasdiff.Check(runner.Context{Root: root}, load(t, dir))
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

func TestCheckMissingAgainst(t *testing.T) {
	dir := filepath.Join(fixtures, "missing-against")
	want, err := os.ReadFile(filepath.Join(dir, "error.txt"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = oasdiff.Check(runner.Context{Root: dir}, load(t, dir))
	if err == nil || !strings.Contains(err.Error(), strings.TrimSpace(string(want))) {
		t.Errorf("error = %v, want it to mention %q", err, want)
	}
}

func TestCheckMissingTool(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"a.yaml", "b.yaml"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("openapi: 3.0.3\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", t.TempDir())
	_, err := oasdiff.Check(runner.Context{Root: root}, config.OasdiffConfig{Spec: "a.yaml", Against: "b.yaml", MinVersion: "1.33.0", Severity: "warning"})
	if err == nil || !strings.Contains(err.Error(), `tool "oasdiff" not found on PATH`) {
		t.Errorf("missing tool error = %v", err)
	}
}

// A go install build reports "oasdiff version main", which carries no number.
func TestBuildWithoutVersionIsAccepted(t *testing.T) {
	if _, err := exec.LookPath("oasdiff"); err != nil {
		t.Skip("oasdiff not on PATH")
	}
	path, _ := extern.Lookup("oasdiff")
	out, _, _, err := extern.Run("", path, "--version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "version main") {
		t.Skip("release build with a numbered version")
	}
	root := t.TempDir()
	for _, f := range []string{"a.yaml", "b.yaml"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("openapi: 3.0.3\ninfo: {title: t, version: \"1\"}\npaths: {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := oasdiff.Check(runner.Context{Root: root}, config.OasdiffConfig{Spec: "a.yaml", Against: "b.yaml", MinVersion: "99.0.0", Severity: "warning"}); err != nil {
		t.Errorf("unnumbered build: %v", err)
	}
}
