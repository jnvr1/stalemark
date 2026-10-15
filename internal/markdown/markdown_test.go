package markdown_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
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
	"ignore-comment",
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
				Blocks   []markdown.CodeBlock
				Spans    []markdown.InlineCode
				Links    []markdown.Link
				Headings []markdown.Heading
			}{doc.Blocks, doc.Spans, doc.Links, doc.Headings}, "", "  ")
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

func TestParseSpansAndLinks(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantSpans []markdown.InlineCode
		wantLinks []markdown.Link
	}{
		{
			name:      "code span position",
			src:       "Use `cmd` here.",
			wantSpans: []markdown.InlineCode{{Text: "cmd", Line: 1, Col: 6}},
		},
		{
			name:      "code span after CRLF line",
			src:       "a\r\n`b`\r\n",
			wantSpans: []markdown.InlineCode{{Text: "b", Line: 2, Col: 2}},
		},
		{
			name:      "multi-line code span joins with spaces",
			src:       "`a\nb`",
			wantSpans: []markdown.InlineCode{{Text: "a b", Line: 1, Col: 2}},
		},
		{
			name:      "inline link points at link text",
			src:       "see [t](a.md)",
			wantLinks: []markdown.Link{{Target: "a.md", Line: 1, Col: 6}},
		},
		{
			name:      "image with empty alt points at target",
			src:       "![](img.png)",
			wantLinks: []markdown.Link{{Target: "img.png", Line: 1, Col: 5}},
		},
		{
			name:      "reference link resolves definition",
			src:       "x\n\n[t][r]\n\n[r]: b.md",
			wantLinks: []markdown.Link{{Target: "b.md", Line: 3, Col: 2}},
		},
		{
			name:      "front matter offsets positions",
			src:       "---\na: 1\n---\n`c` [t](d.md)",
			wantSpans: []markdown.InlineCode{{Text: "c", Line: 4, Col: 2}},
			wantLinks: []markdown.Link{{Target: "d.md", Line: 4, Col: 6}},
		},
		{
			name: "textless reference image outside its block is dropped",
			src:  "![][r]\n\n[r]: img.png",
		},
		{
			name: "autolinks are ignored",
			src:  "<https://x.y> https://x.y",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := markdown.Parse("t.md", []byte(tt.src))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(doc.Spans, tt.wantSpans) {
				t.Errorf("Spans = %+v, want %+v", doc.Spans, tt.wantSpans)
			}
			if !reflect.DeepEqual(doc.Links, tt.wantLinks) {
				t.Errorf("Links = %+v, want %+v", doc.Links, tt.wantLinks)
			}
		})
	}
}

func TestParseHeadings(t *testing.T) {
	src := "---\na: 1\n---\n# Top `x`\n\ntext\n\n## Getting *started*\n\nSetup\n=====\n"
	want := []markdown.Heading{
		{Level: 1, Text: "Top x", Line: 4},
		{Level: 2, Text: "Getting started", Line: 8},
		{Level: 1, Text: "Setup", Line: 10},
	}
	doc, err := markdown.Parse("t.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc.Headings, want) {
		t.Errorf("Headings = %+v, want %+v", doc.Headings, want)
	}
}

func TestParseIgnored(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []bool
	}{
		{"no comment", "```sh\na\n```\n", []bool{false}},
		{"html block before fence", "<!-- stalemark-ignore -->\n```sh\na\n```\n", []bool{true}},
		{"blank line between", "<!-- stalemark-ignore -->\n\n```sh\na\n```\n", []bool{true}},
		{"inline raw html", "skip <!-- stalemark-ignore --> this:\n\n```sh\na\n```\n", []bool{true}},
		{"only the next block", "<!-- stalemark-ignore -->\n```sh\na\n```\n```sh\nb\n```\n", []bool{true, false}},
		{"other comment", "<!-- todo -->\n```sh\na\n```\n", []bool{false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := markdown.Parse("t.md", []byte(tt.src))
			if err != nil {
				t.Fatal(err)
			}
			var got []bool
			for _, b := range doc.Blocks {
				got = append(got, b.Ignored)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Ignored = %v, want %v", got, tt.want)
			}
		})
	}
}

func FuzzParse(f *testing.F) {
	for _, name := range fixtures {
		src, err := os.ReadFile(fixturePath(name))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(src)
	}

	f.Fuzz(func(t *testing.T, src []byte) {
		doc, err := markdown.Parse("fuzz.md", src)
		if err != nil {
			t.Fatal(err)
		}
		lineCount := bytes.Count(src, []byte("\n")) + 1
		check := func(what string, line, col int) {
			t.Helper()
			if line < 1 || line > lineCount || col < 1 {
				t.Errorf("%s at %d:%d outside 1..%d", what, line, col, lineCount)
			}
		}
		for _, b := range doc.Blocks {
			check("block start", b.StartLine, 1)
			prev := b.StartLine
			for _, l := range b.Lines {
				check("block line", l.FileLine, 1)
				if l.FileLine < prev {
					t.Errorf("block line %d after line %d", l.FileLine, prev)
				}
				prev = l.FileLine
			}
		}
		for _, s := range doc.Spans {
			check("span", s.Line, s.Col)
		}
		for _, l := range doc.Links {
			check("link", l.Line, l.Col)
		}
	})
}
