package npm_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/markdown"
	"github.com/jnvr1/stalemark/internal/runner"
	"github.com/jnvr1/stalemark/internal/runner/npm"
)

var update = flag.Bool("update", false, "rewrite expected.json files from the current output")

var cases = []string{
	"ok",
	"missing-script",
	"workspace-filter",
	"cd-subdir",
	"no-package-json",
	"placeholder",
	"yarn-shorthand",
	"ignore-comment",
}

func TestResolveGolden(t *testing.T) {
	reg := runner.New(npm.New())
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			root, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "npm", name))
			if err != nil {
				t.Fatal(err)
			}
			doc, err := markdown.Load(filepath.Join(root, "README.md"))
			if err != nil {
				t.Fatal(err)
			}

			findings := reg.CheckDoc(runner.Context{Root: root}, doc)
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
