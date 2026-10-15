// Package e2e runs the built stalemark binary against the fixture repos in testdata/e2e.
package e2e

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/e2e expected.* files from the current output")

// build compiles cmd/stalemark once, without ldflags (so the version is "dev").
func build(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "stalemark")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, filepath.Join("..", "..", "cmd", "stalemark"))
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// run executes the binary in dir and returns stdout, stderr and the exit code.
func run(t *testing.T, bin, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	cmd := exec.Command(bin, args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, &out, &errb
	var exit *exec.ExitError
	if err := cmd.Run(); errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return out.String(), errb.String(), code
}

func TestBinary(t *testing.T) {
	bin := build(t)
	for _, fx := range []struct {
		name string
		code int
	}{{"drifted", 1}, {"clean", 0}} {
		dir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "e2e", fx.name))
		if err != nil {
			t.Fatal(err)
		}
		for format, ext := range map[string]string{"text": "txt", "json": "json", "sarif": "sarif"} {
			t.Run(fx.name+"/"+format, func(t *testing.T) {
				stdout, stderr, code := run(t, bin, dir, "check", "--format", format, dir)
				if code != fx.code {
					t.Errorf("exit code = %d, want %d", code, fx.code)
				}
				if stderr != "" {
					t.Errorf("unexpected stderr: %q", stderr)
				}
				golden := filepath.Join(dir, "expected."+ext)
				if *update {
					if err := os.WriteFile(golden, []byte(stdout), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				want, err := os.ReadFile(golden)
				if err != nil {
					t.Fatalf("read golden: %v (run with -update to create it)", err)
				}
				if stdout != string(want) {
					t.Errorf("stdout differs from %s (run with -update to refresh)\ngot:\n%s\nwant:\n%s", golden, stdout, want)
				}
			})
		}
	}

	t.Run("drifted/github", func(t *testing.T) {
		dir, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "e2e", "drifted"))
		stdout, _, code := run(t, bin, dir, "check", "--format", "github", dir)
		if code != 1 || !strings.HasPrefix(stdout, "::error file=README.md,line=") {
			t.Errorf("exit code = %d, stdout = %q", code, stdout)
		}
	})
}
