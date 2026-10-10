// Package markdown extracts fenced code blocks, inline code spans and link
// targets from Markdown files, mapping each of them to 1-based file positions.
package markdown

import (
	"bytes"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// Doc is the extracted content of one Markdown file.
type Doc struct {
	Path   string
	Blocks []CodeBlock
	Spans  []InlineCode
	Links  []Link
}

// CodeBlock is a fenced code block. Lang is the lowercased first word of the
// info string and Info the rest of it verbatim. StartLine is the opening fence.
type CodeBlock struct {
	Lang, Info string
	StartLine  int
	Lines      []Line
}

// Line is one line inside a fenced block, without its line ending.
type Line struct {
	Text     string
	FileLine int
}

// InlineCode is a `code span`; Line/Col locate its first content byte.
type InlineCode struct {
	Text      string
	Line, Col int
}

// Link is a link or image target; Line/Col locate the link text.
type Link struct {
	Target    string
	Line, Col int
}

var md = goldmark.New(goldmark.WithExtensions(extension.GFM))

// Load reads path and parses it with Parse.
func Load(path string) (*Doc, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(path, src)
}

// Parse extracts the code blocks, code spans and links of src. All reported
// positions refer to the original src, including any stripped front matter.
func Parse(path string, src []byte) (*Doc, error) {
	body, skipped := stripFrontMatter(src)
	ix := newLineIndex(body, skipped)
	doc := &Doc{Path: path}

	root := md.Parser().Parse(text.NewReader(body))
	err := ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.FencedCodeBlock:
			if b, ok := codeBlock(n, body, ix); ok {
				doc.Blocks = append(doc.Blocks, b)
			}
		case *ast.CodeSpan:
			if s, ok := codeSpan(n, body, ix); ok {
				doc.Spans = append(doc.Spans, s)
			}
		case *ast.Link:
			if l, ok := link(n, n.Destination, body, ix); ok {
				doc.Links = append(doc.Links, l)
			}
		case *ast.Image:
			if l, ok := link(n, n.Destination, body, ix); ok {
				doc.Links = append(doc.Links, l)
			}
		}
		return ast.WalkContinue, nil
	})
	return doc, err
}

func codeSpan(n *ast.CodeSpan, body []byte, ix lineIndex) (InlineCode, bool) {
	first := firstText(n)
	if first == nil {
		return InlineCode{}, false
	}
	var sb strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			sb.Write(t.Segment.Value(body))
		}
	}
	s := InlineCode{Text: strings.ReplaceAll(strings.ReplaceAll(sb.String(), "\r\n", "\n"), "\n", " ")}
	s.Line, s.Col = ix.lineCol(first.Segment.Start)
	return s, true
}

func link(n ast.Node, dest, body []byte, ix lineIndex) (Link, bool) {
	l := Link{Target: string(dest)}
	if t := firstText(n); t != nil {
		l.Line, l.Col = ix.lineCol(t.Segment.Start)
		return l, true
	}
	// ponytail: goldmark v1 keeps no position for a textless link such as ![](img.png),
	// so point at the first occurrence of the target inside the enclosing block.
	// Ceiling: wrong column if the same text appears earlier in that block, and a
	// textless reference link (![][r]) is dropped because its target lives in the
	// definition, not the block. Fix by searching from the previous sibling's segment.
	p := n.Parent()
	for p != nil && p.Type() == ast.TypeInline {
		p = p.Parent()
	}
	if p == nil || p.Lines().Len() == 0 || len(dest) == 0 {
		return l, false
	}
	start := p.Lines().At(0).Start
	end := p.Lines().At(p.Lines().Len() - 1).Stop
	i := bytes.Index(body[start:end], dest)
	if i < 0 {
		return l, false
	}
	l.Line, l.Col = ix.lineCol(start + i)
	return l, true
}

// firstText returns the first ast.Text descendant of n in document order.
func firstText(n ast.Node) *ast.Text {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			return t
		}
		if t := firstText(c); t != nil {
			return t
		}
	}
	return nil
}

func codeBlock(n *ast.FencedCodeBlock, body []byte, ix lineIndex) (CodeBlock, bool) {
	var b CodeBlock
	lines := n.Lines()
	for i := range lines.Len() {
		seg := lines.At(i)
		txt := strings.TrimSuffix(strings.TrimSuffix(string(seg.Value(body)), "\n"), "\r")
		b.Lines = append(b.Lines, Line{Text: txt, FileLine: ix.line(seg.Start)})
	}
	if n.Info == nil {
		if len(b.Lines) == 0 || !strings.HasPrefix(b.Lines[0].Text, "$ ") {
			return b, false
		}
		// The fence is the physical line right before the first content line.
		b.StartLine = b.Lines[0].FileLine - 1
		return b, true
	}
	b.Lang, b.Info = splitInfo(string(n.Info.Segment.Value(body)))
	b.StartLine = ix.line(n.Info.Segment.Start)
	return b, true
}

// splitInfo splits a fence info string into a lowercased language and the
// rest of the string verbatim: `bash title="x"` -> ("bash", `title="x"`).
func splitInfo(info string) (lang, meta string) {
	info = strings.TrimSpace(info)
	i := strings.IndexFunc(info, unicode.IsSpace)
	if i < 0 {
		return strings.ToLower(info), ""
	}
	return strings.ToLower(info[:i]), strings.TrimLeftFunc(info[i:], unicode.IsSpace)
}

// stripFrontMatter drops a leading YAML front matter block ("---" ... "---")
// and returns the rest of src with the number of lines removed.
func stripFrontMatter(src []byte) ([]byte, int) {
	end, n := 0, 0
	for line := range bytes.Lines(src) {
		end += len(line)
		switch trimmed := string(bytes.TrimRight(line, "\r\n")); {
		case n == 0 && trimmed != "---":
			return src, 0
		case n > 0 && trimmed == "---":
			return src[end:], n + 1
		}
		n++
	}
	return src, 0
}

// lineIndex maps byte offsets of the parsed body to 1-based lines and columns
// of the original file.
type lineIndex struct {
	newlines []int // offset of every '\n' in the body
	skipped  int   // lines stripped before the body
}

func newLineIndex(body []byte, skipped int) lineIndex {
	ix := lineIndex{skipped: skipped}
	for i, c := range body {
		if c == '\n' {
			ix.newlines = append(ix.newlines, i)
		}
	}
	return ix
}

func (ix lineIndex) line(off int) int {
	line, _ := ix.lineCol(off)
	return line
}

func (ix lineIndex) lineCol(off int) (line, col int) {
	i := sort.SearchInts(ix.newlines, off) // newlines strictly before off
	prev := -1
	if i > 0 {
		prev = ix.newlines[i-1]
	}
	return 1 + i + ix.skipped, off - prev
}
