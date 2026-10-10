package markdown_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jnvr1/stalemark/internal/markdown"
)

var update = flag.Bool("update", false, "rewrite golden files from the current output")

var fixtures = []string{
	"basic",
	"tilde-fence",
	"list-nested",
	"blockquote",
	"crlf",
	"frontmatter",
	"unclosed-fence",
	"unlabeled-dollar",
}

func fixturePath(name string) string {
	return filepath.Join("..", "..", "testdata", "markdown", name+".md")
}

func TestLoadGolden(t *testing.T) {
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			path := fixturePath(name)

			doc, err := markdown.Load(path)
			if err != nil {
				t.Fatalf("Load(%q) error = %v", path, err)
			}
			if doc.Path != path {
				t.Errorf("Path = %q, want %q", doc.Path, path)
			}

			got, err := json.MarshalIndent(struct {
				Blocks []markdown.CodeBlock
				Spans  []markdown.InlineCode
				Links  []markdown.Link
			}{doc.Blocks, doc.Spans, doc.Links}, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')

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

// TestLoadPositions cross-checks every reported line/col against the raw fixture bytes,
// independently of the goldens.
func TestLoadPositions(t *testing.T) {
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(fixturePath(name))
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(src), "\n")
			at := func(line int) string {
				t.Helper()
				if line < 1 || line > len(lines) {
					t.Fatalf("line %d out of range 1..%d", line, len(lines))
				}
				return strings.TrimSuffix(lines[line-1], "\r")
			}
			from := func(line, col int) string {
				t.Helper()
				s := at(line)
				if col < 1 || col > len(s) {
					t.Fatalf("col %d out of range 1..%d on line %d %q", col, len(s), line, s)
				}
				return s[col-1:]
			}

			doc, err := markdown.Parse(name+".md", src)
			if err != nil {
				t.Fatal(err)
			}
			for _, b := range doc.Blocks {
				if fence := at(b.StartLine); !strings.Contains(fence, "```") && !strings.Contains(fence, "~~~") {
					t.Errorf("block StartLine %d is %q, want a fence", b.StartLine, fence)
				}
				for _, l := range b.Lines {
					if got := at(l.FileLine); !strings.HasSuffix(got, l.Text) {
						t.Errorf("line %d is %q, want suffix %q", l.FileLine, got, l.Text)
					}
				}
			}
			for _, s := range doc.Spans {
				if got := from(s.Line, s.Col); !strings.HasPrefix(got, s.Text) {
					t.Errorf("span at %d:%d is %q, want prefix %q", s.Line, s.Col, got, s.Text)
				}
			}
			for _, l := range doc.Links {
				if got := at(l.Line); !strings.Contains(got, "[") {
					t.Errorf("link %q at %d:%d: line %q has no link syntax", l.Target, l.Line, l.Col, got)
				}
				from(l.Line, l.Col)
			}
		})
	}
}
