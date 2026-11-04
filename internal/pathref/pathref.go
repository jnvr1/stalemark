// Package pathref checks that the file and directory references of a Markdown
// document (backticked paths, link targets and path arguments of commands)
// exist on disk. Precision first: every heuristic errs on silence.
package pathref

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/markdown"
	"github.com/jnvr1/stalemark/internal/runner"
	"github.com/jnvr1/stalemark/internal/shellx"
)

// RulePathNotFound is the rule ID of a referenced path that does not exist.
const RulePathNotFound = "SM-P001"

// ponytail: fixed list of directories that builds and installs create, so docs
// may legitimately mention them before they exist. Upgrade = .stalemark.yml.
var generated = []string{"node_modules", "dist", "build", "target", "out", "coverage", "vendor", ".git", "bin", "venv", ".venv", "env"}

// ponytail: a fixed list of extensions. Inline code needs one (or a trailing or
// leading slash) so that `owner/repo` and `net/http.Client` stay silent.
// Upgrade = a per-repo allowlist, or stat-probing the sibling files.
var knownExt = []string{
	".go", ".md", ".json", ".yaml", ".yml", ".toml", ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx",
	".py", ".rs", ".sh", ".ps1", ".bat", ".rb", ".php", ".java", ".kt", ".c", ".h", ".cpp",
	".txt", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".pdf", ".html", ".css", ".sql", ".xml",
	".csv", ".lock", ".cfg", ".ini", ".env", ".mod", ".sum",
}

var interpreters = []string{"python", "python3", "node", "bash", "sh", "zsh", "source", "ruby", "php", "perl", "deno"}

type ref struct {
	tok       string
	line, col int
}

// Check returns a SM-P001 finding for every candidate path of d that exists
// neither next to the document nor under ctx.Root, in document order.
//
// ponytail: single-segment names (package.json, ./run.sh, internal/) are never
// checked, docs name files of other people's repos and build outputs all the
// time. Upgrade = check known-extension single segments inside command
// arguments only. Redirect targets (> file) are not exposed by shellx, so a
// file created by a redirect is not known as created.
func Check(ctx runner.Context, d *markdown.Doc) []finding.Finding {
	return CheckWith(ctx, nil, d)
}

// CheckWith is Check with a last resort: a candidate that matches the tail of
// a path in ix exists. A nil ix disables the tail match.
func CheckWith(ctx runner.Context, ix *Index, d *markdown.Doc) []finding.Finding {
	var refs []ref
	for _, s := range d.Spans {
		if tok, ok := normalize(strings.TrimSpace(s.Text), true); ok {
			refs = append(refs, ref{tok, s.Line, s.Col})
		}
	}
	for _, l := range d.Links {
		target, _, _ := strings.Cut(l.Target, "#")
		target, _, _ = strings.Cut(target, "?")
		if tok, ok := normalize(target, false); ok {
			refs = append(refs, ref{tok, l.Line, l.Col})
		}
	}
	made := createdPaths(d)
	for _, b := range d.Blocks {
		if b.Ignored {
			continue
		}
		for _, cmd := range shellx.Extract(b) {
			if cmd.Placeholder {
				continue
			}
			for _, t := range commandRefs(cmd) {
				if tok, ok := normalize(t, false); ok {
					refs = append(refs, ref{tok, cmd.FileLine, 0})
				}
			}
		}
	}

	sort.SliceStable(refs, func(i, j int) bool {
		return refs[i].line < refs[j].line || refs[i].line == refs[j].line && refs[i].col < refs[j].col
	})
	docDir, docPath := filepath.Dir(d.Path), runner.Rel(ctx, d.Path)
	var out []finding.Finding
	type key struct {
		tok  string
		line int
	}
	seen := map[key]bool{}
	for _, r := range refs {
		k := key{r.tok, r.line}
		if seen[k] || isCreated(made, r.tok) || !missing(ctx.Root, docDir, r.tok) || ix != nil && ix.HasTail(r.tok) {
			continue
		}
		seen[k] = true
		out = append(out, finding.Finding{
			Rule: RulePathNotFound, Severity: finding.Error, Path: docPath,
			Line: r.line, Col: r.col,
			Message: "path " + `"` + r.tok + `"` + " not found", Subject: r.tok,
		})
	}
	return out
}

// normalize reports whether tok is a path candidate and returns it without
// trailing punctuation. inline is true for backticked text, which is stricter.
func normalize(tok string, inline bool) (string, bool) {
	if strings.Contains(tok, "...") {
		return "", false
	}
	tok = strings.TrimRight(tok, ".:,)")
	for _, r := range tok {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("._-/", r) {
			return "", false // spaces, globs, placeholders, expansions, schemes, drives, ~ ...
		}
	}
	if strings.HasPrefix(tok, "/") ||
		strings.HasPrefix(tok, "path/to") || strings.Contains(tok, "/path/to") ||
		strings.HasPrefix(tok, "your-") || strings.HasPrefix(tok, "my-") {
		return "", false
	}
	// tokens are slash-separated text, not OS paths: path, not filepath.
	segs := strings.Split(path.Clean(tok), "/")
	if len(segs) < 2 {
		return "", false
	}
	if i := strings.Index(segs[0], "."); i > 0 || slices.Contains(generated, segs[0]) { // github.com/x/y
		return "", false
	}
	if inline && !strings.HasSuffix(tok, "/") && !strings.HasPrefix(tok, "./") && !strings.HasPrefix(tok, "../") &&
		!slices.Contains(knownExt, strings.ToLower(path.Ext(tok))) {
		return "", false
	}
	return tok, true
}

// missing is true when tok exists neither under docDir nor under root. A path
// that only resolves outside root, or into a generated directory, is never
// missing.
func missing(root, docDir, tok string) bool {
	checked := false
	for _, base := range []string{docDir, root} {
		p := filepath.Join(base, filepath.FromSlash(tok))
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if first, _, _ := strings.Cut(filepath.ToSlash(rel), "/"); slices.Contains(generated, first) {
			continue
		}
		checked = true
		if _, err := os.Stat(p); err == nil {
			return false
		}
	}
	return checked
}

// commandRefs returns the path arguments of cmd worth checking.
func commandRefs(cmd shellx.Command) []string {
	switch {
	case strings.HasPrefix(cmd.Name, "./") || strings.HasPrefix(cmd.Name, "../"):
		return []string{cmd.Name}
	case slices.Contains(interpreters, cmd.Name):
		for _, a := range cmd.Args {
			if !strings.HasPrefix(a, "-") {
				return []string{a}
			}
			if slices.Contains([]string{"-c", "-m", "-e", "-p"}, a) { // inline code or a module, not a file
				return nil
			}
		}
	case cmd.Name == "docker-compose" || cmd.Name == "docker" && len(cmd.Args) > 0 && cmd.Args[0] == "compose":
		return flagValues(cmd.Args, "-f", "--file")
	case cmd.Name == "kubectl" && len(cmd.Args) > 0 && (cmd.Args[0] == "apply" || cmd.Args[0] == "delete"):
		return flagValues(cmd.Args, "-f")
	}
	return nil
}

// flagValues returns the values of "-f x" and "--file=x" style flags.
func flagValues(args []string, flags ...string) []string {
	var out []string
	for i, a := range args {
		if slices.Contains(flags, a) && i+1 < len(args) {
			out = append(out, args[i+1])
		} else if v, ok := strings.CutPrefix(a, "--file="); ok && slices.Contains(flags, "--file") {
			out = append(out, v)
		}
	}
	return out
}

// positional drops the options of args.
func positional(args []string) []string {
	var out []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			out = append(out, a)
		}
	}
	return out
}

// createdPaths lists, in slash form, every path a shell block of d creates.
//
// ponytail: the set is per document, not per position: a path created anywhere
// in the doc silences mentions anywhere in it. Upgrade = compare block order.
func createdPaths(d *markdown.Doc) []string {
	var made []string
	for _, b := range d.Blocks {
		if b.Ignored {
			continue
		}
		for _, cmd := range shellx.Extract(b) {
			if cmd.Placeholder {
				continue
			}
			var names []string
			pos := positional(cmd.Args)
			switch {
			case cmd.Name == "mkdir" || cmd.Name == "touch":
				names = pos
			case (cmd.Name == "cp" || cmd.Name == "mv") && len(pos) >= 2:
				names = pos[len(pos)-1:]
			case cmd.Name == "git" && len(cmd.Args) > 0 && cmd.Args[0] == "clone":
				if p := positional(cmd.Args[1:]); len(p) >= 2 {
					names = p[1:2]
				} else if len(p) == 1 {
					base := strings.TrimSuffix(strings.TrimSuffix(p[0], "/"), ".git")
					names = []string{base[strings.LastIndexAny(base, "/:")+1:]}
				}
			case cmd.Name == "cargo" && len(cmd.Args) > 0 && cmd.Args[0] == "new":
				if p := positional(cmd.Args[1:]); len(p) > 0 {
					names = p[:1]
				}
			case cmd.Name == "npx" && len(cmd.Args) > 1 && strings.HasPrefix(cmd.Args[0], "create-"):
				names = cmd.Args[1:2]
			case slices.Contains([]string{"npm", "pnpm", "yarn"}, cmd.Name) && len(cmd.Args) > 0 && (cmd.Args[0] == "create" || cmd.Args[0] == "init"):
				if p := positional(cmd.Args[1:]); len(p) >= 2 {
					names = p[1:2]
				}
			}
			for _, n := range names {
				for _, c := range []string{path.Clean(n), path.Join(cmd.Dir, n)} {
					if c != "." && c != ".." && !path.IsAbs(c) && !strings.HasPrefix(c, "../") {
						made = append(made, c)
					}
				}
			}
		}
	}
	return made
}

// isCreated reports whether tok is, is inside, or contains a created path.
func isCreated(made []string, tok string) bool {
	tok = path.Clean(tok)
	for _, c := range made {
		if tok == c || strings.HasPrefix(tok, c+"/") || strings.HasPrefix(c, tok+"/") {
			return true
		}
	}
	return false
}
