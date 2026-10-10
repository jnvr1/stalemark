package shellx_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jnvr1/stalemark/internal/markdown"
	"github.com/jnvr1/stalemark/internal/shellx"
)

var update = flag.Bool("update", false, "rewrite golden files from the current output")

var fixtures = []string{
	"prompts",
	"continuation",
	"heredoc",
	"comments",
	"pipes-and-and",
	"console-output",
	"placeholders",
	"cd-chain",
	"powershell",
	"invalid-syntax",
}

func fixturePath(name string) string {
	return filepath.Join("..", "..", "testdata", "shell", name+".md")
}

func TestExtractGolden(t *testing.T) {
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			path := fixturePath(name)

			doc, err := markdown.Load(path)
			if err != nil {
				t.Fatalf("Load(%q) error = %v", path, err)
			}

			type block struct {
				Lang      string
				StartLine int
				Commands  []shellx.Command
			}
			var blocks []block
			for _, b := range doc.Blocks {
				blocks = append(blocks, block{b.Lang, b.StartLine, shellx.Extract(b)})
			}

			var buf bytes.Buffer
			enc := json.NewEncoder(&buf) // keeps <placeholders> readable instead of <
			enc.SetEscapeHTML(false)
			enc.SetIndent("", "  ")
			if err := enc.Encode(blocks); err != nil {
				t.Fatal(err)
			}
			got := buf.Bytes()

			golden := strings.TrimSuffix(path, ".md") + ".golden.json"
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
				t.Errorf("output differs from %s (run with -update to refresh)\ngot:\n%s", golden, got)
			}
		})
	}
}

// TestExtractFileLines cross-checks every FileLine against the raw fixture bytes,
// independently of the goldens: the line must belong to the block and contain the name.
func TestExtractFileLines(t *testing.T) {
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			path := fixturePath(name)
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(src), "\n")

			doc, err := markdown.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, b := range doc.Blocks {
				first, last := b.Lines[0].FileLine, b.Lines[len(b.Lines)-1].FileLine
				for _, c := range shellx.Extract(b) {
					if c.FileLine < first || c.FileLine > last {
						t.Errorf("%s at line %d outside block %d..%d", c.Name, c.FileLine, first, last)
						continue
					}
					if got := lines[c.FileLine-1]; !strings.Contains(got, c.Name) {
						t.Errorf("line %d is %q, want it to contain %q", c.FileLine, got, c.Name)
					}
				}
			}
		})
	}
}
