package envvars_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/jnvr1/stalemark/internal/envvars"
	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/markdown"
	"github.com/jnvr1/stalemark/internal/runner"
)

var update = flag.Bool("update", false, "rewrite expected.json files from the current output")

// TestCheckGolden mirrors pathref's loop: envvars consumes a whole Doc, so it
// is not a runner.Resolver and cannot reuse runnertest.Golden.
func TestCheckGolden(t *testing.T) {
	casesDir := filepath.Join("..", "..", "testdata", "env")
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
			findings := envvars.Check(runner.Context{Root: root}, doc)
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

// TestNearestExampleWins: a doc next to services/api/.env.example resolves
// against that file only; the root example is not merged in.
func TestNearestExampleWins(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "env", "monorepo-two-examples"))
	if err != nil {
		t.Fatal(err)
	}
	src := "```bash\nexport API_VAR=1\nexport ROOT_VAR=2\n```\n"
	doc, err := markdown.Parse(filepath.Join(root, "services", "api", "README.md"), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	got := envvars.Check(runner.Context{Root: root}, doc)
	if len(got) != 1 || got[0].Subject != "ROOT_VAR" || got[0].Path != "services/api/README.md" {
		t.Fatalf("findings = %+v, want exactly ROOT_VAR in services/api/README.md", got)
	}
	if want := `env var "ROOT_VAR" not found in services/api/.env.example`; got[0].Message[:len(want)] != want {
		t.Errorf("message = %q, want prefix %q", got[0].Message, want)
	}
}
