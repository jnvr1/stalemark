// Package shellx extracts the commands of a fenced code block with a real
// shell parser, mapping each of them to the file line it starts on.
package shellx

import (
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/jnvr1/stalemark/internal/markdown"
)

// Command is one simple command found in a code block. FileLine is the file
// line the command starts on.
type Command struct {
	Name        string
	Args        []string
	Env         map[string]string
	Dir         string
	FileLine    int
	Placeholder bool
}

// Extract returns the commands of block in source order, one per simple
// command: `a && b | c` yields a, b and c, and commands inside subshells,
// control flow and heredoc-opening lines are yielded too. Comments and heredoc
// bodies are ignored. A block whose language is not a shell, or that the
// parser rejects, yields nil: precision over recall, no error.
//
// ponytail: a parse error fails the whole block; upgrade = per-line fallback.
func Extract(block markdown.CodeBlock) []Command {
	switch block.Lang {
	case "bash", "sh", "shell", "console", "zsh", "":
		return extractShell(block)
	}
	return nil
}

// ponytail: zsh is parsed as bash.
func extractShell(block markdown.CodeBlock) []Command {
	lines := make([]string, len(block.Lines))
	for i, l := range block.Lines {
		lines[i] = l.Text
	}
	src := strings.Join(stripPrompts(lines, block.Lang == "console"), "\n")
	parser := syntax.NewParser(syntax.KeepComments(false), syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(src), "")
	if err != nil {
		return nil
	}

	var cmds []Command
	syntax.Walk(file, func(n syntax.Node) bool {
		call, ok := n.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 { // FOO=bar alone is not a command
			return true
		}
		// Every dropped line became an empty line, so parser line N is block line N.
		c := Command{FileLine: block.Lines[call.Pos().Line()-1].FileLine}
		c.Name = word(src, call.Args[0])
		for _, a := range call.Args[1:] {
			c.Args = append(c.Args, word(src, a))
		}
		cmds = append(cmds, c)
		return true
	})
	return cmds
}

var heredocRE = regexp.MustCompile(`(?:^|[^<])<<-?\s*["']?(\w+)`)

// stripPrompts rewrites block lines for the parser, never changing their count.
// A block is in prompt mode when it is a console block or any line starts with
// "$ ". There, "$ " lines are commands, "> " lines right after a backslash
// continuation or inside a heredoc are continuations, and every other line is
// output, replaced by an empty line so line numbers still match.
//
// ponytail: "> " lines are only continuations after "\" or inside a heredoc,
// because npm and friends print "> script" output lines; a "> " continuation
// after "&&", "|" or "do" at the end of a line is dropped and the block then
// fails to parse.
func stripPrompts(lines []string, console bool) []string {
	prompt := console
	for _, l := range lines {
		if l == "$" || strings.HasPrefix(l, "$ ") {
			prompt = true
			break
		}
	}
	out := make([]string, len(lines))
	cont, heredoc := false, ""
	for i, l := range lines {
		switch {
		case !prompt:
			out[i] = l
		case l == "$":
		case strings.HasPrefix(l, "$ "):
			out[i] = l[2:]
		case strings.HasPrefix(l, "> ") && (cont || heredoc != ""):
			out[i] = l[2:]
		case cont || heredoc != "":
			out[i] = l
		}
		cont = strings.HasSuffix(out[i], "\\")
		switch m := heredocRE.FindStringSubmatch(out[i]); {
		case heredoc == "" && m != nil:
			heredoc = m[1]
		case strings.TrimSpace(out[i]) == heredoc:
			heredoc = ""
		}
	}
	return out
}

// word returns the unquoted text of a literal word, or the raw source text of
// a word with expansions such as $VAR or $(...).
func word(src string, w *syntax.Word) string {
	if s, ok := lit(w); ok {
		return s
	}
	return src[w.Pos().Offset():w.End().Offset()]
}

// lit joins the literal parts of w; ok is false when any part is an expansion.
// ponytail: backslash escapes are kept verbatim ("a\ b" stays "a\ b").
func lit(w *syntax.Word) (string, bool) {
	var sb strings.Builder
	for _, p := range w.Parts {
		switch p := p.(type) {
		case *syntax.Lit:
			sb.WriteString(p.Value)
		case *syntax.SglQuoted:
			sb.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, q := range p.Parts {
				l, ok := q.(*syntax.Lit)
				if !ok {
					return "", false
				}
				sb.WriteString(l.Value)
			}
		default:
			return "", false
		}
	}
	return sb.String(), true
}
