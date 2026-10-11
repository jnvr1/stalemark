// Package shellx extracts the commands of a fenced code block with a real
// shell parser, mapping each of them to the file line it starts on.
package shellx

import (
	"path"
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/jnvr1/stalemark/internal/markdown"
)

// Command is one simple command found in a code block. FileLine is the file
// line the command starts on. Env holds the FOO=bar prefixes of the call and
// is nil when there are none. Dir is the directory a previous literal cd in
// the same block moved into, "" for the block's root. Placeholder marks a
// command with a <placeholder>, an expansion ($VAR, $(...)) or a YOUR_* value.
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
	case "powershell", "pwsh", "ps1", "cmd", "bat", "batch":
		return extractPS(block)
	}
	return nil
}

// ponytail: zsh is parsed as bash; a cd inside if/for/while bodies sets Dir for
// everything after it, a later issue can mark control-flow commands conditional.
func extractShell(block markdown.CodeBlock) []Command {
	lines := make([]string, len(block.Lines))
	for i, l := range block.Lines {
		lines[i] = l.Text
	}
	src := strings.Join(stripPrompts(lines, block.Lang == "console"), "\n")
	parser := syntax.NewParser(syntax.KeepComments(false), syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(mask(src)), "")
	if err != nil {
		return nil
	}

	var cmds []Command
	dir := ""
	var visit func(syntax.Node) bool
	visit = func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.Subshell: // a cd inside (...) does not leak out
			saved := dir
			for _, s := range n.Stmts {
				syntax.Walk(s, visit)
			}
			dir = saved
			return false
		case *syntax.Redirect: // an unquoted heredoc body may hold $(...); only its opener is a command
			return n.Hdoc == nil
		case *syntax.CallExpr:
			if len(n.Args) == 0 { // FOO=bar alone is not a command
				return true
			}
			// Every dropped line became an empty line, so parser line N is block line N.
			c := Command{Dir: dir, FileLine: block.Lines[n.Pos().Line()-1].FileLine}
			for _, a := range n.Assigns {
				if c.Env == nil {
					c.Env = map[string]string{}
				}
				var ph bool
				c.Env[a.Name.Value], ph = word(src, a.Value)
				c.Placeholder = c.Placeholder || ph
			}
			var ph bool
			c.Name, ph = word(src, n.Args[0])
			c.Placeholder = c.Placeholder || ph
			for _, a := range n.Args[1:] {
				s, ph := word(src, a)
				c.Args = append(c.Args, s)
				c.Placeholder = c.Placeholder || ph
			}
			if c.Name == "cd" && len(c.Args) == 1 && !c.Placeholder {
				dir = chdir(dir, c.Args[0])
			}
			cmds = append(cmds, c)
		}
		return true
	}
	syntax.Walk(file, visit)
	return cmds
}

// chdir applies a literal cd target to dir. Targets are shell text with
// forward slashes, not OS paths, so path (not filepath) keeps results the same
// on every OS. ponytail: ~, - and pushd are not understood.
func chdir(dir, target string) string {
	if path.IsAbs(target) {
		return target
	}
	if d := path.Join(dir, target); d != "." { // back at the root is "", not "."
		return d
	}
	return ""
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

// placeholderRE matches <repo-url> style placeholders. The leading group keeps
// heredoc openers such as `<<EOF > file` from matching.
// ponytail: `cmd <in.txt >out.txt` also matches because spaces are allowed.
var placeholderRE = regexp.MustCompile(`(?:^|[^<])<[A-Za-z][A-Za-z0-9_ .:/-]*>`)

// mask replaces every <placeholder> with underscores of the same length, so the
// parser sees a plain literal and byte offsets still map onto the original text.
func mask(s string) string {
	return placeholderRE.ReplaceAllStringFunc(s, func(m string) string {
		i := strings.LastIndex(m, "<")
		return m[:i] + strings.Repeat("_", len(m)-i)
	})
}

// word returns the unquoted text of a literal word and whether it is a
// placeholder. A word with an expansion ($VAR, $(...), ...) or a <placeholder>
// returns its raw source text instead. A nil word (FOO= cmd) is "".
func word(src string, w *syntax.Word) (string, bool) {
	if w == nil {
		return "", false
	}
	raw := src[w.Pos().Offset():w.End().Offset()]
	s, ok := lit(w)
	if !ok || placeholderRE.MatchString(raw) {
		return raw, true
	}
	return s, strings.Contains(s, "YOUR_") || strings.Contains(s, "your-")
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

// psPlaceholder reports whether a PowerShell line has a <placeholder>, a $
// anywhere or a YOUR_* value.
func psPlaceholder(text string) bool {
	return placeholderRE.MatchString(text) || strings.ContainsAny(text, "$") || strings.Contains(text, "YOUR_") || strings.Contains(text, "your-")
}

// psPromptRE matches "PS> ", "PS C:\dir> " and "C:\dir>" prompts.
var psPromptRE = regexp.MustCompile(`^(?:PS(?: [^>]*)?>|[A-Za-z]:\\[^>]*>) ?`)

// extractPS tokenizes PowerShell and cmd blocks on whitespace. When any line
// carries a prompt the others are output; ">> " and "> " lines continue the
// previous command.
//
// ponytail: naive, no quoting, no ";" splitting, no backtick continuation, no
// Dir tracking, "$" anywhere flags the line; upgrade = a real PowerShell tokenizer.
func extractPS(block markdown.CodeBlock) []Command {
	prompt := false
	for _, l := range block.Lines {
		if psPromptRE.MatchString(l.Text) {
			prompt = true
			break
		}
	}
	var cmds []Command
	for _, l := range block.Lines {
		text := l.Text
		switch {
		case psPromptRE.MatchString(text):
			text = psPromptRE.ReplaceAllString(text, "")
		case strings.HasPrefix(text, "> ") || strings.HasPrefix(text, ">> "):
			if len(cmds) > 0 {
				last := &cmds[len(cmds)-1]
				last.Args = append(last.Args, strings.Fields(strings.TrimLeft(text, "> "))...)
				last.Placeholder = last.Placeholder || psPlaceholder(text)
			}
			continue
		case prompt:
			continue
		}
		f := strings.Fields(text)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		cmds = append(cmds, Command{
			Name:        f[0],
			Args:        append([]string(nil), f[1:]...),
			FileLine:    l.FileLine,
			Placeholder: psPlaceholder(text),
		})
	}
	return cmds
}
