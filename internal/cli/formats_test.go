package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jnvr1/stalemark/internal/cli"
)

func TestRunSARIFClean(t *testing.T) {
	clean := fixture(t, map[string]string{"README.md": goodDoc, "package.json": pkgJSON})
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"check", "--format", "sarif", clean}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{`"version": "2.1.0"`, `"results": []`} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout lacks %s:\n%s", want, stdout.String())
		}
	}
}

func TestRunGitHubDrift(t *testing.T) {
	drift := fixture(t, map[string]string{"README.md": badDoc, "package.json": pkgJSON})
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"check", "--format", "github", drift}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if want := "::error file=README.md,line=4,title=SM-A001::"; !strings.HasPrefix(stdout.String(), want) {
		t.Errorf("stdout = %q, want prefix %q", stdout.String(), want)
	}
}
