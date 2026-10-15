package adrlibs_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/jnvr1/stalemark/internal/adr"
	"github.com/jnvr1/stalemark/internal/adrlibs"
	"github.com/jnvr1/stalemark/internal/config"
	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/runner"
)

var update = flag.Bool("update", false, "rewrite expected.json files from the current output")

func TestGolden(t *testing.T) {
	casesDir := filepath.Join("..", "..", "testdata", "adrlibs")
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
			cfg, err := config.Load(root)
			if err != nil {
				t.Fatal(err)
			}
			adrs, err := adr.Load(root, cfg.ADR.Dirs)
			if err != nil {
				t.Fatal(err)
			}
			out := adrlibs.Check(runner.Context{Root: root}, adrs, cfg.ADR.Freetext)
			if out == nil {
				out = []finding.Finding{}
			}
			got, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			path := filepath.Join(root, "expected.json")
			if *update {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))) {
				t.Errorf("output differs from %s\ngot:\n%s", path, got)
			}
		})
	}
}
