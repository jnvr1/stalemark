// Package runner dispatches the commands found in a document to the Resolver
// that knows how to check them against the project on disk.
package runner

import (
	"path/filepath"
	"strings"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/markdown"
	"github.com/jnvr1/stalemark/internal/shellx"
)

// Context is what every check runs against. Root is the absolute directory
// being checked; commands are assumed to run from it, with cmd.Dir joined on
// top.
//
// ponytail: docs in subfolders still resolve from Root; revisit if real
// READMEs disagree.
type Context struct {
	Root string
}

// Resolver checks the commands whose Name is one of Names. doc is the value
// to use as Finding.Path.
type Resolver interface {
	Names() []string
	Resolve(ctx Context, doc string, cmd shellx.Command) []finding.Finding
}

// Registry maps command names to the Resolver that checks them.
type Registry struct {
	byName map[string]Resolver
}

// New builds a Registry; a later resolver claiming the same name wins.
func New(resolvers ...Resolver) *Registry {
	r := &Registry{byName: map[string]Resolver{}}
	for _, res := range resolvers {
		for _, name := range res.Names() {
			r.byName[name] = res
		}
	}
	return r
}

// CheckDoc extracts the commands of every non-ignored block of d and returns
// the findings of their resolvers in document order. Placeholder commands and
// commands without a resolver are skipped.
func (r *Registry) CheckDoc(ctx Context, d *markdown.Doc) []finding.Finding {
	rel, err := filepath.Rel(ctx.Root, d.Path)
	if err != nil {
		rel = d.Path
	}
	doc := filepath.ToSlash(rel)

	var out []finding.Finding
	for _, b := range d.Blocks {
		if b.Ignored {
			continue
		}
		for _, cmd := range shellx.Extract(b) {
			if cmd.Placeholder {
				continue
			}
			if res, ok := r.byName[cmd.Name]; ok {
				out = append(out, res.Resolve(ctx, doc, cmd)...)
			}
		}
	}
	return out
}

// WorkDir is the directory cmd runs in: ctx.Root plus the shell-relative
// cmd.Dir.
func WorkDir(ctx Context, cmd shellx.Command) string {
	return filepath.Join(ctx.Root, filepath.FromSlash(cmd.Dir))
}

// Closest returns the candidate most similar to name: the smallest Levenshtein
// distance among candidates within distance 3 or sharing a prefix with name.
// Ties keep the first candidate. "" when nothing is close enough.
func Closest(name string, candidates []string) string {
	if name == "" {
		return ""
	}
	best, bestDist := "", -1
	for _, c := range candidates {
		d := levenshtein(name, c)
		if d > 3 && !strings.HasPrefix(c, name) && !strings.HasPrefix(name, c) {
			continue
		}
		if bestDist < 0 || d < bestDist {
			best, bestDist = c, d
		}
	}
	return best
}

// levenshtein is the plain edit distance over runes.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
