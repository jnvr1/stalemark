package ports_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/markdown"
	"github.com/jnvr1/stalemark/internal/ports"
	"github.com/jnvr1/stalemark/internal/runner"
)

var update = flag.Bool("update", false, "rewrite expected.json files from the current output")

// TestCheckGolden mirrors envvars' loop: ports consumes a whole Doc, so it is
// not a runner.Resolver and cannot reuse runnertest.Golden.
func TestCheckGolden(t *testing.T) {
	casesDir := filepath.Join("..", "..", "testdata", "ports")
	entries, err := os.ReadDir(casesDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			root, err := filepath.Abs(filepath.Join(casesDir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			doc, err := markdown.Load(filepath.Join(root, "README.md"))
			if err != nil {
				t.Fatal(err)
			}
			findings := ports.Check(runner.Context{Root: root}, doc)
			if findings == nil {
				findings = []finding.Finding{}
			}
			got, err := json.MarshalIndent(findings, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')

			golden := filepath.Join(root, "expected.json")
			if *update {
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden: %v (run with -update to create it)", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("findings differ from %s (run with -update to refresh)\ngot:\n%s", golden, got)
			}
		})
	}
}

// TestInterpolatedDefault: ${PORT:-3000} publishes 3000 and, with no other
// unknown source, 7777 is still reported.
func TestInterpolatedDefault(t *testing.T) {
	root := t.TempDir()
	compose := "services:\n  web:\n    ports:\n      - \"${PORT:-3000}:3000\"\n"
	if err := os.WriteFile(filepath.Join(root, "compose.yml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := markdown.Parse(filepath.Join(root, "README.md"), []byte("Open `localhost:3000` or `localhost:7777`.\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := ports.Check(runner.Context{Root: root}, doc)
	if len(got) != 1 || got[0].Subject != "7777" {
		t.Fatalf("findings = %+v, want exactly 7777", got)
	}
}

// TestNoPublishedPortsIsSilent: a Dockerfile without EXPOSE declares nothing
// to compare against, so the check stays quiet.
func TestNoPublishedPortsIsSilent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := markdown.Parse(filepath.Join(root, "README.md"), []byte("Open `localhost:3000`.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := ports.Check(runner.Context{Root: root}, doc); got != nil {
		t.Fatalf("findings = %+v, want none", got)
	}
}
