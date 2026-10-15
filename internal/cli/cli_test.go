package cli_test

import (
	"bytes"
	"os"
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
