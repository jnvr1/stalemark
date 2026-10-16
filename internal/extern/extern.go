// Package extern runs external tools that a wrapper (pydoclint, ...) depends
// on: PATH lookup, version check and a subprocess call with no shell.
package extern

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/jnvr1/stalemark/internal/finding"
)

// Lookup finds name on PATH only (no virtualenv or node_modules probing).
func Lookup(name string) (string, error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("tool %q not found on PATH: %w", name, err)
	}
	return p, nil
}

var versionRE = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?`)

// Version runs path with args and returns the first dotted number of its output.
func Version(path string, args ...string) (string, error) {
	stdout, stderr, code, err := Run("", path, args...)
	if err != nil {
		return "", err
	}
	if v := versionRE.Find(append(stdout, stderr...)); v != nil && code == 0 {
		return string(v), nil
	}
	return "", fmt.Errorf("%s %s: no version found in its output (exit %d)", path, strings.Join(args, " "), code)
}

// Older reports whether version have is lower than floor, comparing numeric
// components; a missing component counts as 0 except that "0.11" < "0.11.1".
func Older(have, floor string) bool {
	h, m := strings.Split(have, "."), strings.Split(floor, ".")
	for i := 0; i < max(len(h), len(m)); i++ {
		a, b := part(h, i), part(m, i)
		if a != b {
			return a < b
		}
	}
	return false
}

func part(s []string, i int) int {
	if i >= len(s) {
		return 0
	}
	n, _ := strconv.Atoi(s[i])
	return n
}

// Run executes path with args in dir (the current directory when empty),
// without a shell. A non-zero exit is not an error: it comes back in exitCode.
func Run(dir, path string, args ...string) (stdout, stderr []byte, exitCode int, err error) {
	cmd := exec.Command(path, args...)
	cmd.Dir = dir
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err = cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return o.Bytes(), e.Bytes(), ee.ExitCode(), nil
	}
	return o.Bytes(), e.Bytes(), 0, err
}

// Severity validates a configured severity; "" means warning.
func Severity(s string) (finding.Severity, error) {
	switch s {
	case "", "warning":
		return finding.Warning, nil
	case "note":
		return finding.Note, nil
	case "error":
		return finding.Error, nil
	}
	return "", fmt.Errorf("severity must be note, warning or error, got %q", s)
}
