package cli_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jnvr1/stalemark/internal/cli"
)

const (
	badDoc   = "# x\n\n```bash\nnpm run buidl\n```\n"
	goodDoc  = "# x\n\n```bash\nnpm run build\n```\n"
	pkgJSON  = `{"scripts": {"build": "echo build"}}`
	finding1 = `README.md:4: SM-A001 npm script "buidl" not found in package.json (did you mean "build"?)` + "\n"
)

// fixture writes files (slash-separated relative paths) into a fresh temp dir.
func fixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRun(t *testing.T) {
	empty := t.TempDir()
	drift := fixture(t, map[string]string{"README.md": badDoc, "package.json": pkgJSON})
	clean := fixture(t, map[string]string{"README.md": goodDoc, "package.json": pkgJSON})
	goDrift := fixture(t, map[string]string{
		"README.md":       "# x\n\n```bash\ngo run ./cmd/missing\n```\n",
		"go.mod":          "module example.com/x\n\ngo 1.22\n",
		"cmd/app/main.go": "package main\n\nfunc main() {}\n",
	})
	nested := fixture(t, map[string]string{
		"README.md":        goodDoc,
		"package.json":     pkgJSON,
		"docs/guide.md":    badDoc,
		"docs/a/deep.md":   badDoc,
		"docs/notes.txt":   badDoc,
		"other/ignored.md": badDoc,
	})

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string // exact match when non-empty
		wantStderr string // substring match when non-empty
	}{
		{
			name:       "version flag prints version and exits 0",
			args:       []string{"--version"},
			wantCode:   0,
			wantStdout: "stalemark dev\n",
		},
		{
			name:       "check on empty dir exits 0",
			args:       []string{"check", empty},
			wantCode:   0,
			wantStdout: "no findings\n",
		},
		{
			name:       "check on non-existent path exits 2",
			args:       []string{"check", filepath.Join(empty, "does-not-exist")},
			wantCode:   2,
			wantStderr: "does-not-exist",
		},
		{
			name:       "unknown subcommand exits 2 with usage",
			args:       []string{"frobnicate"},
			wantCode:   2,
			wantStderr: "Usage:",
		},
		{
			name:       "drifted README exits 1 with text report",
			args:       []string{"check", drift},
			wantCode:   1,
			wantStdout: finding1 + "\n1 finding in 1 file\n",
		},
		{
			name:       "clean README exits 0",
			args:       []string{"check", clean},
			wantCode:   0,
			wantStdout: "no findings\n",
		},
		{
			name:       "go run of a missing package is reported",
			args:       []string{"check", goDrift},
			wantCode:   1,
			wantStdout: `README.md:4: SM-A007 go package "./cmd/missing" not found (no directory cmd/missing)` + "\n\n1 finding in 1 file\n",
		},
		{
			name:       "explicit --format text is the default",
			args:       []string{"check", "--format", "text", drift},
			wantCode:   1,
			wantStdout: finding1 + "\n1 finding in 1 file\n",
		},
		{
			name:       "--format json on a clean dir renders an empty findings array",
			args:       []string{"check", "--format", "json", clean},
			wantCode:   0,
			wantStdout: "{\n  \"version\": 1,\n  \"findings\": []\n}\n",
		},
		{
			name:       "unknown format exits 2",
			args:       []string{"check", "--format", "xml", drift},
			wantCode:   2,
			wantStderr: `unknown format "xml"`,
		},
		{
			name:     "unknown flag exits 2",
			args:     []string{"check", "--bogus", drift},
			wantCode: 2,
		},
		{
			name:       "single file path checks only that file",
			args:       []string{"check", filepath.Join(drift, "README.md")},
			wantCode:   1,
			wantStdout: finding1 + "\n1 finding in 1 file\n",
		},
		{
			name:     "docs folder is included recursively, other folders and non-md files are not",
			args:     []string{"check", nested},
			wantCode: 1,
			wantStdout: `docs/a/deep.md:4: SM-A001 npm script "buidl" not found in package.json (did you mean "build"?)` + "\n" +
				"\n" +
				`docs/guide.md:4: SM-A001 npm script "buidl" not found in package.json (did you mean "build"?)` + "\n" +
				"\n2 findings in 2 files\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := cli.Run(tt.args, &stdout, &stderr)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stderr: %q)", code, tt.wantCode, stderr.String())
			}
			if tt.wantStdout != "" && stdout.String() != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestRunJSONFindings(t *testing.T) {
	drift := fixture(t, map[string]string{"README.md": badDoc, "package.json": pkgJSON})
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"check", "--format", "json", drift}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr: %q)", code, stderr.String())
	}
	var doc struct {
		Findings []struct{ Rule, File string }
	}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Findings) != 1 || doc.Findings[0].File != "README.md" || doc.Findings[0].Rule != "SM-A001" {
		t.Errorf("findings = %+v", doc.Findings)
	}
}

func TestRunPathRefs(t *testing.T) {
	dir := fixture(t, map[string]string{"README.md": "# x\n\nSee `cmd/nope/main.go`.\n"})
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"check", dir}, &stdout, &stderr); code != 1 {
		t.Errorf("exit code = %d, want 1 (stderr: %q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "SM-P001") {
		t.Errorf("stdout = %q, want it to contain SM-P001", stdout.String())
	}
}

func TestRunEnvVars(t *testing.T) {
	dir := fixture(t, map[string]string{".env.example": "A=1\n", "README.md": "# x\n\n```bash\nexport B=2\n```\n"})
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"check", dir}, &stdout, &stderr); code != 1 {
		t.Errorf("exit code = %d, want 1 (stderr: %q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "SM-E001") {
		t.Errorf("stdout = %q, want it to contain SM-E001", stdout.String())
	}
}

func TestRunPorts(t *testing.T) {
	dir := fixture(t, map[string]string{"compose.yml": "services:\n  web:\n    ports:\n      - \"8080:80\"\n", "README.md": "# x\n\nOpen `localhost:3000`.\n"})
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"check", dir}, &stdout, &stderr); code != 1 {
		t.Errorf("exit code = %d, want 1 (stderr: %q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "SM-N001") {
		t.Errorf("stdout = %q, want it to contain SM-N001", stdout.String())
	}
}

func TestRunPydoclintMissingTool(t *testing.T) {
	dir := fixture(t, map[string]string{".stalemark.yml": "api:\n  pydoclint: {}\n", "README.md": "# x\n"})
	t.Setenv("PATH", t.TempDir())
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"check", dir}, &stdout, &stderr); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), `tool "pydoclint" not found on PATH`) {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestRunOasdiffMissingTool(t *testing.T) {
	dir := fixture(t, map[string]string{
		".stalemark.yml": "api:\n  oasdiff: {spec: a.yaml, against: b.yaml}\n", "README.md": "# x\n",
		"a.yaml": "openapi: 3.0.3\n", "b.yaml": "openapi: 3.0.3\n",
	})
	t.Setenv("PATH", t.TempDir())
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"check", dir}, &stdout, &stderr); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), `tool "oasdiff" not found on PATH`) {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestRunOasdiffDrift(t *testing.T) {
	if _, err := exec.LookPath("oasdiff"); err != nil {
		t.Skip("oasdiff not on PATH")
	}
	spec := "openapi: 3.0.3\ninfo: {title: t, version: \"1\"}\npaths:\n  /a:\n    get:\n      responses: {\"200\": {description: ok}}\n  /b:\n    get:\n      responses: {\"200\": {description: ok}}\n"
	dir := fixture(t, map[string]string{
		".stalemark.yml": "api:\n  oasdiff: {spec: a.yaml, against: b.yaml}\n", "README.md": "# x\n",
		"a.yaml": spec, "b.yaml": strings.Replace(spec, "  /b:\n    get:\n      responses: {\"200\": {description: ok}}\n", "", 1),
	})
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"check", dir}, &stdout, &stderr); code != 1 {
		t.Errorf("exit code = %d, want 1 (stderr: %q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "a.yaml:8:5: SM-B001") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestRunToolVersions(t *testing.T) {
	dir := fixture(t, map[string]string{".nvmrc": "22\n", "README.md": "# x\n\n## Setup\n\nNode 18+\n"})
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"check", dir}, &stdout, &stderr); code != 1 {
		t.Errorf("exit code = %d, want 1 (stderr: %q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "SM-V001") {
		t.Errorf("stdout = %q, want it to contain SM-V001", stdout.String())
	}
}

func TestRunADRSupersede(t *testing.T) {
	dir := fixture(t, map[string]string{"docs/adr/0001-a.md": "# 1. A\n\n## Status\n\nSuperseded by 0002\n"})
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"check", dir}, &stdout, &stderr); code != 1 {
		t.Errorf("exit code = %d, want 1 (stderr: %q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "SM-D001") {
		t.Errorf("stdout = %q, want it to contain SM-D001", stdout.String())
	}
}

func TestRunADRLink(t *testing.T) {
	dir := fixture(t, map[string]string{"docs/adr/0001-x.md": "# 1. X\n\n## Status\n\nAccepted\n\nSee [y](0009-y.md).\n"})
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"check", dir}, &stdout, &stderr); code != 1 {
		t.Errorf("exit code = %d, want 1 (stderr: %q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "SM-D002") {
		t.Errorf("stdout = %q, want it to contain SM-D002", stdout.String())
	}
}

func TestRunADRLibs(t *testing.T) {
	dir := fixture(t, map[string]string{
		"docs/adr/0001-a.md": "---\nstatus: accepted\nuses: [redis]\n---\n\n# 1. A\n",
		"package.json":       `{"dependencies": {}}`,
	})
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"check", dir}, &stdout, &stderr); code != 1 {
		t.Errorf("exit code = %d, want 1 (stderr: %q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "SM-D004") {
		t.Errorf("stdout = %q, want it to contain SM-D004", stdout.String())
	}
}

func TestRunDeterministic(t *testing.T) {
	dir := fixture(t, map[string]string{
		"README.md": badDoc, "package.json": pkgJSON,
		"docs/b.md": badDoc, "docs/a.md": badDoc, "CONTRIBUTING.md": badDoc,
	})
	var first string
	for i := range 3 {
		var out, errb bytes.Buffer
		cli.Run([]string{"check", dir}, &out, &errb)
		if i == 0 {
			first = out.String()
		} else if out.String() != first {
			t.Fatalf("run %d differs:\n%s\nvs\n%s", i, out.String(), first)
		}
	}
}

var update = flag.Bool("update", false, "rewrite testdata/config expected.txt files from the current output")

func TestConfigFixtures(t *testing.T) {
	cases := map[string]struct {
		code       int
		wantStderr string
	}{
		"defaults":            {1, ""},
		"exclude":             {1, ""},
		"disable-rule":        {1, ""},
		"baseline-hit":        {0, ""},
		"baseline-moved-line": {0, ""},
		"baseline-stale":      {0, "notice: baseline entry"},
		"invalid-yaml":        {2, ".stalemark.yml"},
		"unknown-key":         {2, "includes"},
	}
	entries, err := os.ReadDir(filepath.Join("..", "..", "testdata", "config"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		want, ok := cases[e.Name()]
		if !ok {
			t.Errorf("fixture %s has no expected exit code", e.Name())
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			dir := filepath.Join("..", "..", "testdata", "config", e.Name())
			var stdout, stderr bytes.Buffer
			if code := cli.Run([]string{"check", dir}, &stdout, &stderr); code != want.code {
				t.Errorf("exit code = %d, want %d (stderr: %q)", code, want.code, stderr.String())
			}
			if !strings.Contains(stderr.String(), want.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), want.wantStderr)
			}
			if want.wantStderr == "" && stderr.Len() > 0 {
				t.Errorf("unexpected stderr: %q", stderr.String())
			}
			golden := filepath.Join(dir, "expected.txt")
			if *update {
				if err := os.WriteFile(golden, stdout.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			exp, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden: %v (run with -update to create it)", err)
			}
			if got := stdout.String(); got != string(exp) {
				t.Errorf("stdout differs from %s (run with -update to refresh)\ngot:\n%s\nwant:\n%s", golden, got, exp)
			}
		})
	}
}

func TestBaselineCommand(t *testing.T) {
	dir := fixture(t, map[string]string{"README.md": badDoc, "package.json": pkgJSON})
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"baseline", dir}, &stdout, &stderr); code != 0 {
		t.Fatalf("baseline exit = %d (stderr: %q)", code, stderr.String())
	}
	if want := "wrote 1 entry to .stalemark-baseline.json\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".stalemark-baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Errorf("baseline is not valid JSON: %s", raw)
	}

	stdout.Reset()
	stderr.Reset()
	if code := cli.Run([]string{"check", dir}, &stdout, &stderr); code != 0 || stdout.String() != "no findings\n" {
		t.Errorf("check after baseline: exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestEnvCodePass(t *testing.T) {
	src := "const x = process.env.X;\n"
	noteOnly := fixture(t, map[string]string{"README.md": "# x\n", "src/app.js": src})
	unused := fixture(t, map[string]string{
		"README.md":      "# x\n\n## Setup\n\n```bash\nexport Y=1\n```\n",
		".env.example":   "Y=1\n",
		"src/app.js":     src,
		".stalemark.yml": "env:\n  documented-unused: true\n",
	})
	tests := []struct {
		name     string
		dir      string
		wantCode int
		wantOut  string
	}{
		{"a note alone does not fail the run", noteOnly, 0, "SM-E003"},
		{"documented-unused warns and fails", unused, 1, "SM-E004"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := cli.Run([]string{"check", tt.dir}, &stdout, &stderr)
			if code != tt.wantCode || !strings.Contains(stdout.String(), tt.wantOut) {
				t.Errorf("exit = %d, stdout = %q (stderr %q), want exit %d with %s", code, stdout.String(), stderr.String(), tt.wantCode, tt.wantOut)
			}
		})
	}
}

func TestBaselineCommandCounts(t *testing.T) {
	two := fixture(t, map[string]string{"README.md": badDoc + "\n```bash\nnpm run buidl2\n```\n", "package.json": pkgJSON})
	zero := fixture(t, map[string]string{"README.md": goodDoc, "package.json": pkgJSON})
	for dir, want := range map[string]string{
		two:  "wrote 2 entries to .stalemark-baseline.json\n",
		zero: "wrote 0 entries to .stalemark-baseline.json\n",
	} {
		var stdout, stderr bytes.Buffer
		if code := cli.Run([]string{"baseline", dir}, &stdout, &stderr); code != 0 {
			t.Fatalf("exit = %d (stderr: %q)", code, stderr.String())
		}
		if stdout.String() != want {
			t.Errorf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunJsdocMissingTool(t *testing.T) {
	dir := fixture(t, map[string]string{".stalemark.yml": "api:\n  jsdoc: {}\n", "README.md": "# x\n"})
	t.Setenv("PATH", t.TempDir())
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"check", dir}, &stdout, &stderr); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), `tool "eslint" not found on PATH`) {
		t.Errorf("stderr = %q", stderr.String())
	}
}
