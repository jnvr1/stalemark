package pathref_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/markdown"
	"github.com/jnvr1/stalemark/internal/pathref"
	"github.com/jnvr1/stalemark/internal/runner"
)

var update = flag.Bool("update", false, "rewrite expected.json files from the current output")

// TestCheckGolden mirrors runnertest.Golden: pathref consumes a whole Doc, so
// it is not a runner.Resolver and cannot reuse that loop.
func TestCheckGolden(t *testing.T) {
	casesDir := filepath.Join("..", "..", "testdata", "paths")
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
			findings := pathref.CheckWith(runner.Context{Root: root}, pathref.NewIndex(root), doc)
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
