package cli_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jnvr1/stalemark/internal/cli"
)

func TestRun(t *testing.T) {
	tmp := t.TempDir()

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
			name:     "check on empty dir exits 0",
			args:     []string{"check", tmp},
			wantCode: 0,
		},
		{
			name:       "check on non-existent path exits 2",
			args:       []string{"check", filepath.Join(tmp, "does-not-exist")},
			wantCode:   2,
			wantStderr: "does-not-exist",
		},
		{
			name:       "unknown subcommand exits 2 with usage",
			args:       []string{"frobnicate"},
			wantCode:   2,
			wantStderr: "Usage:",
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
