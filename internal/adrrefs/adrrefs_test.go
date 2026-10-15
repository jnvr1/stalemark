package adrrefs_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jnvr1/stalemark/internal/adr"
	"github.com/jnvr1/stalemark/internal/adrrefs"
	"github.com/jnvr1/stalemark/internal/config"
	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/markdown"
	"github.com/jnvr1/stalemark/internal/pathref"
	"github.com/jnvr1/stalemark/internal/runner"
)

// TestGolden mirrors the pipeline: pathref on every doc, then adrrefs.Check.
func TestGolden(t *testing.T) {
	casesDir := filepath.Join("..", "..", "testdata", "adrrefs")
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
			docs, err := cfg.Docs(root)
			if err != nil {
				t.Fatal(err)
			}
			ctx := runner.Context{Root: root}
			scanned := map[string]bool{}
			var docFindings []finding.Finding
			for _, p := range docs {
				d, err := markdown.Load(p)
				if err != nil {
					t.Fatal(err)
				}
				scanned[runner.Rel(ctx, p)] = true
				docFindings = append(docFindings, pathref.Check(ctx, d)...)
			}
			adrs, err := adr.Load(root, cfg.ADR.Dirs)
			if err != nil {
				t.Fatal(err)
			}
			got := adrrefs.Check(ctx, adrs, scanned, cfg.ADR.Historical, docFindings)
			if got == nil {
				got = []finding.Finding{}
			}
			raw, err := os.ReadFile(filepath.Join(root, "expected.json"))
			if err != nil {
				t.Fatal(err)
			}
			want := []finding.Finding{}
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				g, _ := json.MarshalIndent(got, "", "  ")
				t.Errorf("got:\n%s\nwant: %+v", g, want)
			}
		})
	}
}
