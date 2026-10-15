package toolver_test

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
	"github.com/jnvr1/stalemark/internal/toolver"
)

var update = flag.Bool("update", false, "rewrite expected.json files from the current output")

// TestCheckGolden mirrors envvars: toolver consumes a whole Doc, so it is not
// a runner.Resolver and cannot reuse runnertest.Golden.
func TestCheckGolden(t *testing.T) {
	casesDir := filepath.Join("..", "..", "testdata", "versions")
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
			findings := toolver.Check(runner.Context{Root: root}, doc)
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

// TestCheckInline covers the spellings the fixtures do not: v prefix, >=, .x,
// Node.js, a source with fewer components than the doc, and range-looking prose.
func TestCheckInline(t *testing.T) {
	tests := []struct {
		name, nvmrc, doc string
		want             []string // subjects
	}{
		{"v prefix equal", "v20.1.0", "Node v20", nil},
		{"v prefix drift", "22", "Node v20", []string{"node v20"}},
		{"gte drift", "22", "Node >=18", []string{"node >=18"}},
		{"dot x equal", "20.5.1", "Node 20.x", nil},
		{"minor precision drift", "20.5.1", "Node.js 20.4", []string{"node 20.4"}},
		{"source shorter than doc", "20", "Node 20.4.1", nil},
		{"range prose", "22", "Node 18-20 works", nil},
		{"alternatives prose", "22", "Node 18/20 works", nil},
		{"word suffix", "22", "Node 18x works", nil},
		{"mongo is not go", "22", "MongoDB 7 and mongo 5", nil},
		{"non numeric source", "lts/*", "Node 18+", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, ".nvmrc"), []byte(tt.nvmrc+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			doc, err := markdown.Parse(filepath.Join(root, "README.md"), []byte("# P\n\n## Setup\n\n"+tt.doc+"\n"))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, f := range toolver.Check(runner.Context{Root: root}, doc) {
				got = append(got, f.Subject)
			}
			if len(got) != len(tt.want) || len(got) > 0 && got[0] != tt.want[0] {
				t.Errorf("subjects = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSourcesGoToolVersionsAndRust checks the sources the fixtures leave out.
func TestSourcesGoToolVersionsAndRust(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("rust-toolchain.toml", "[toolchain]\nchannel = \"1.80.1\"\n")
	write(".tool-versions", "# comment\nrust 1.80.1\ngolang 1.23.4\npython 3.12.1\n")
	doc, err := markdown.Parse(filepath.Join(root, "README.md"),
		[]byte("# P\n\n## Install\n\nRust 1.79+ and Go 1.22+ and Python 3.12.\n"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range toolver.Check(runner.Context{Root: root}, doc) {
		got = append(got, f.Subject+"="+f.Suggestion)
	}
	want := []string{"rust 1.79+=1.80", "go 1.22+=1.23"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v, want %v", got, want)
	}
}
