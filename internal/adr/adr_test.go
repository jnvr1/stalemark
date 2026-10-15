package adr_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jnvr1/stalemark/internal/adr"
	"github.com/jnvr1/stalemark/internal/config"
	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/runner"
)

var update = flag.Bool("update", false, "rewrite adrs.json and expected.json files from the current output")

// view is the golden shape of an ADR: Doc is left out and Path is relative.
type view struct {
	Number       int
	Title        string
	Status       string
	Path         string
	SupersededBy []int    `json:",omitempty"`
	Links        []int    `json:",omitempty"`
	Uses         []string `json:",omitempty"`
}

func golden(t *testing.T, path string, v any) {
	t.Helper()
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (run with -update to create it)", err)
	}
	if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))) {
		t.Errorf("output differs from %s (run with -update to refresh)\ngot:\n%s", path, got)
	}
}

func TestGolden(t *testing.T) {
	casesDir := filepath.Join("..", "..", "testdata", "adr")
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
			ctx := runner.Context{Root: root}
			views := []view{}
			for _, a := range adrs {
				if a.Doc == nil {
					t.Errorf("ADR %d has no Doc", a.Number)
				}
				views = append(views, view{a.Number, a.Title, a.Status, runner.Rel(ctx, a.Path), a.SupersededBy, a.Links, a.Uses})
			}
			golden(t, filepath.Join(root, "adrs.json"), views)

			findings := adr.CheckSupersedes(ctx, adrs)
			if findings == nil {
				findings = []finding.Finding{}
			}
			golden(t, filepath.Join(root, "expected.json"), findings)
		})
	}
}

func TestMADRSupersededStatusText(t *testing.T) {
	dir := t.TempDir()
	body := "---\nstatus: \"superseded by ADR-0003\"\n---\n\n# 1. X\n"
	if err := os.WriteFile(filepath.Join(dir, "0001-x.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	adrs, err := adr.Load(dir, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	if len(adrs) != 1 || adrs[0].Status != "superseded" || !reflect.DeepEqual(adrs[0].SupersededBy, []int{3}) {
		t.Errorf("got %+v", adrs)
	}
}

func TestUsesCommaString(t *testing.T) {
	dir := t.TempDir()
	body := "---\nstatus: accepted\nuses: redis, `pg`,\n---\n\n# 1. X\n"
	if err := os.WriteFile(filepath.Join(dir, "0001-x.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	adrs, err := adr.Load(dir, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(adrs[0].Uses, []string{"redis", "pg"}) || !reflect.DeepEqual(adrs[0].UsesLines, []int{3, 3}) {
		t.Errorf("got %+v %+v", adrs[0].Uses, adrs[0].UsesLines)
	}
}
