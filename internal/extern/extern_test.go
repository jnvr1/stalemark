package extern_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/jnvr1/stalemark/internal/extern"
	"github.com/jnvr1/stalemark/internal/finding"
)

func TestOlder(t *testing.T) {
	for _, tc := range []struct {
		have, min string
		want      bool
	}{
		{"0.9.0", "0.11.1", true},
		{"0.11.1", "0.11.1", false},
		{"0.11.2", "0.11.1", false},
		{"1.0", "0.11.1", false},
		{"0.11", "0.11.1", true},
		{"0.10.9", "0.11.0", true},
	} {
		if got := extern.Older(tc.have, tc.min); got != tc.want {
			t.Errorf("Older(%q, %q) = %v, want %v", tc.have, tc.min, got, tc.want)
		}
	}
}

func TestSeverity(t *testing.T) {
	for in, want := range map[string]finding.Severity{"": finding.Warning, "warning": finding.Warning, "note": finding.Note, "error": finding.Error} {
		if got, err := extern.Severity(in); err != nil || got != want {
			t.Errorf("Severity(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := extern.Severity("fatal"); err == nil {
		t.Error("Severity(fatal) must fail")
	}
}

// The go tool exists wherever these tests run, so it stands in for a wrapped tool.
func TestLookupVersionRun(t *testing.T) {
	path, err := extern.Lookup("go")
	if err != nil {
		t.Fatal(err)
	}
	v, err := extern.Version(path, "version")
	if err != nil || !strings.HasPrefix(v, "1.") {
		t.Errorf("Version = %q, %v", v, err)
	}
	out, _, code, err := extern.Run(t.TempDir(), path, "env", "GOOS")
	if err != nil || code != 0 || strings.TrimSpace(string(out)) != runtime.GOOS {
		t.Errorf("Run = %q, %d, %v", out, code, err)
	}
	_, _, code, err = extern.Run(t.TempDir(), path, "nonexistent-subcommand")
	if err != nil || code == 0 {
		t.Errorf("a non-zero exit must come back in the code: %d, %v", code, err)
	}
	if _, err := extern.Lookup("stalemark-no-such-tool"); err == nil || !strings.Contains(err.Error(), `tool "stalemark-no-such-tool" not found on PATH`) {
		t.Errorf("Lookup error = %v", err)
	}
}
