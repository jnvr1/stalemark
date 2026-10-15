// Package adr loads Architecture Decision Records (adr-tools and MADR layouts)
// and checks the links between them.
package adr

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/markdown"
	"github.com/jnvr1/stalemark/internal/runner"
)

// RuleSupersededMissing is reported when an ADR names a successor that does not exist.
const RuleSupersededMissing = "SM-D001"

// DefaultDirs are the directories searched, relative to the root, when the
// config names none.
var DefaultDirs = []string{"docs/adr", "docs/adrs", "adr", "doc/adr", "doc/decisions", "docs/decisions"}

// ADR is one decision record. Path is absolute. Status is lowercase
// ("proposed", "accepted", "deprecated", "superseded"...; the raw lowercased
// first word when unknown, "" when none was found). SupersededBy holds the
// numbers a "superseded by" status names; Links the numbers of the other ADR
// files this one links to.
type ADR struct {
	Number       int
	Title        string
	Status       string
	Path         string
	SupersededBy []int
	Links        []int
	Doc          *markdown.Doc

	supLine int // line to report SupersededBy findings on
}

var (
	nameRe  = regexp.MustCompile(`^(\d{1,5})-[^/]*\.md$`)
	supRe   = regexp.MustCompile(`(?i)superseded\s+by\b`)
	numRe   = regexp.MustCompile(`\b0*(\d{1,5})\b`)
	h1NumRe = regexp.MustCompile(`^\d+\.\s*`)
)

// Load reads every ADR of dirs (relative to root; DefaultDirs when empty).
// Missing directories are skipped, and so are files whose name is not
// "NNNN-title.md". Result is sorted by number, then path.
//
// ponytail: directories are not searched recursively, add it if a repo nests ADRs.
func Load(root string, dirs []string) ([]ADR, error) {
	if len(dirs) == 0 {
		dirs = DefaultDirs
	}
	var out []ADR
	for _, d := range dirs {
		dir := filepath.Join(root, filepath.FromSlash(d))
		ents, err := os.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range ents {
			m := nameRe.FindStringSubmatch(e.Name())
			if m == nil || e.IsDir() {
				continue
			}
			a, err := load(filepath.Join(dir, e.Name()), m[1])
			if err != nil {
				return nil, err
			}
			out = append(out, a)
		}
	}
	slices.SortFunc(out, func(a, b ADR) int {
		if a.Number != b.Number {
			return a.Number - b.Number
		}
		return strings.Compare(a.Path, b.Path)
	})
	return out, nil
}

func load(path, digits string) (ADR, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return ADR{}, err
	}
	doc, err := markdown.Parse(path, src)
	if err != nil {
		return ADR{}, err
	}
	n, _ := strconv.Atoi(digits)
	a := ADR{Number: n, Path: path, Doc: doc, Title: title(doc, filepath.Base(path))}
	for _, l := range doc.Links {
		if ln, ok := linkNum(l.Target); ok && ln != n {
			a.Links = append(a.Links, ln)
		}
	}
	slices.Sort(a.Links)
	a.Links = slices.Compact(a.Links)
	if !a.frontMatterStatus(src) {
		a.sectionStatus(doc)
	}
	slices.Sort(a.SupersededBy)
	a.SupersededBy = slices.Compact(a.SupersededBy)
	return a, nil
}

// title is the first "# " heading without an adr-tools "1. " prefix, else the
// file name after the number.
func title(doc *markdown.Doc, file string) string {
	for _, h := range doc.Headings {
		if h.Level == 1 {
			return strings.TrimSpace(h1NumRe.ReplaceAllString(h.Text, ""))
		}
	}
	stem := strings.TrimSuffix(file, ".md")
	_, rest, _ := strings.Cut(stem, "-")
	return strings.ReplaceAll(rest, "-", " ")
}

// linkNum is the ADR number of a link to a sibling "NNNN-title.md" file.
func linkNum(target string) (int, bool) {
	target, _, _ = strings.Cut(target, "#")
	target = strings.TrimPrefix(target, "./")
	m := nameRe.FindStringSubmatch(target)
	if m == nil {
		return 0, false
	}
	n, _ := strconv.Atoi(m[1])
	return n, true
}

// frontMatterStatus fills Status and SupersededBy from a leading YAML front
// matter block and reports whether it had a status.
//
// ponytail: malformed front matter is ignored, not an error.
func (a *ADR) frontMatterStatus(src []byte) bool {
	fm := frontMatter(src)
	var status, sup string
	for k, v := range fm {
		switch strings.ToLower(k) {
		case "status":
			status = strings.TrimSpace(fmt.Sprint(v))
		case "superseded_by", "superseded-by", "superseded by":
			sup = fmt.Sprint(v)
		}
	}
	if status == "" {
		return false
	}
	a.Status = firstWord(status)
	a.supLine = 1
	if loc := supRe.FindStringIndex(status); loc != nil {
		sup += " " + status[loc[1]:]
	}
	a.SupersededBy = nums(sup)
	return true
}

func frontMatter(src []byte) map[string]any {
	var buf []byte
	n := 0
	for line := range bytes.Lines(src) {
		t := string(bytes.TrimRight(line, "\r\n"))
		switch {
		case n == 0 && t != "---":
			return nil
		case n > 0 && t == "---":
			var m map[string]any
			if yaml.Unmarshal(buf, &m) != nil {
				return nil
			}
			return m
		case n > 0:
			buf = append(buf, line...)
		}
		n++
	}
	return nil
}

// sectionStatus reads the paragraphs between a "Status" heading and the next
// heading: the first word of the first one is the status, and the first one
// saying "superseded by" names the successors, preferring link targets over
// plain numbers.
//
// ponytail: without a link, any number after "superseded by" counts (a year in
// the sentence would too); require a link or an ADR-/0000 shape if that bites.
func (a *ADR) sectionStatus(doc *markdown.Doc) {
	hi := slices.IndexFunc(doc.Headings, func(h markdown.Heading) bool {
		return strings.EqualFold(strings.TrimSpace(h.Text), "status")
	})
	if hi < 0 {
		return
	}
	start, end := doc.Headings[hi].Line, int(^uint(0)>>1)
	if hi+1 < len(doc.Headings) {
		end = doc.Headings[hi+1].Line
	}
	var paras []markdown.Paragraph
	for _, p := range doc.Paragraphs {
		if p.Line > start && p.Line < end {
			paras = append(paras, p)
		}
	}
	if len(paras) == 0 {
		return
	}
	a.Status = firstWord(paras[0].Text)
	for i, p := range paras {
		loc := supRe.FindStringIndex(p.Text)
		if loc == nil {
			continue
		}
		next := end
		if i+1 < len(paras) {
			next = paras[i+1].Line
		}
		for _, l := range doc.Links {
			if ln, ok := linkNum(l.Target); ok && l.Line >= p.Line && l.Line < next {
				a.SupersededBy = append(a.SupersededBy, ln)
			}
		}
		if len(a.SupersededBy) == 0 {
			a.SupersededBy = nums(p.Text[loc[1]:])
		}
		a.supLine = p.Line
		return
	}
}

func firstWord(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimRight(f[0], ".,:;*_"))
}

func nums(s string) []int {
	var out []int
	for _, m := range numRe.FindAllStringSubmatch(s, -1) {
		n, _ := strconv.Atoi(m[1])
		out = append(out, n)
	}
	return out
}

// CheckSupersedes reports every "superseded by N" naming an ADR that was not loaded.
func CheckSupersedes(ctx runner.Context, adrs []ADR) []finding.Finding {
	have := map[int]bool{}
	for _, a := range adrs {
		have[a.Number] = true
	}
	var out []finding.Finding
	for _, a := range adrs {
		for _, n := range a.SupersededBy {
			if have[n] {
				continue
			}
			s := fmt.Sprintf("%04d", n)
			out = append(out, finding.Finding{
				Rule:     RuleSupersededMissing,
				Severity: finding.Error,
				Path:     runner.Rel(ctx, a.Path),
				Line:     a.supLine,
				Message:  "ADR is superseded by " + s + ", which does not exist",
				Subject:  s,
			})
		}
	}
	return out
}
