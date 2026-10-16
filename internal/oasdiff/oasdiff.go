// Package oasdiff wraps the external oasdiff tool (ADR 0003, amendment of
// 2026-10-16): it runs it on PATH, checks its version and maps its JSON
// changelog of the committed OpenAPI spec against the generated one to findings.
package oasdiff

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/jnvr1/stalemark/internal/config"
	"github.com/jnvr1/stalemark/internal/extern"
	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/report"
	"github.com/jnvr1/stalemark/internal/runner"
)

// Rules: the oasdiff level of the change picks the rule.
const (
	RuleBreaking     = "SM-B001" // level ERR
	RulePossible     = "SM-B002" // level WARN
	RuleUndocumented = "SM-B003" // endpoint-added: in the generated spec only
	toolName         = "oasdiff"
	endpointAdded    = "endpoint-added"
)

// oasdiff levels in its JSON output.
const (
	levelWarn = 2
	levelErr  = 3
)

type source struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type change struct {
	ID             string  `json:"id"`
	Text           string  `json:"text"`
	Level          int     `json:"level"`
	Operation      string  `json:"operation"`
	Path           string  `json:"path"`
	BaseSource     *source `json:"baseSource"`
	RevisionSource *source `json:"revisionSource"`
}

// "request parameter `limit`", "the required property `items/name`".
var nameRE = regexp.MustCompile("(?:parameter|property) `([^`]+)`")

// Parse maps the JSON array printed by `oasdiff changelog <spec> <against>
// --format json` to findings, sorted by path, line, column, rule and subject.
// Level ERR is SM-B001, WARN SM-B002 (both with severity sev); the INFO change
// endpoint-added is SM-B003 (note); other INFO changes are dropped. Line and
// column come from baseSource when it names spec (YAML only), else 1 and 0.
// Backslashes and \r\n are tolerated so recorded output is portable.
//
// ponytail: Subject takes the first parameter or property named in the text; a change naming two of them shares a baseline fingerprint with its sibling.
func Parse(root string, raw []byte, spec string, sev finding.Severity) ([]finding.Finding, error) {
	var changes []change
	if err := json.Unmarshal(raw, &changes); err != nil {
		return nil, fmt.Errorf("%s: output is not a JSON changelog: %w", toolName, err)
	}
	spec = path.Clean(filepath.ToSlash(spec))
	prefix := strings.TrimSuffix(strings.ReplaceAll(root, `\`, "/"), "/") + "/"
	var out []finding.Finding
	for _, c := range changes {
		f := finding.Finding{Severity: sev, Path: spec, Line: 1}
		switch {
		case c.Level == levelErr:
			f.Rule = RuleBreaking
		case c.Level == levelWarn:
			f.Rule = RulePossible
		case c.ID == endpointAdded:
			f.Rule, f.Severity = RuleUndocumented, finding.Note
		default:
			continue
		}
		if s := c.BaseSource; s != nil && s.Line > 0 &&
			path.Clean(strings.TrimPrefix(strings.ReplaceAll(s.File, `\`, "/"), prefix)) == spec {
			f.Line, f.Col = s.Line, s.Column
		}
		f.Message = "oasdiff " + c.ID + ": " + strings.ReplaceAll(c.Text, "`", "'")
		f.Subject = c.Operation + " " + c.Path
		if m := nameRE.FindStringSubmatch(c.Text); m != nil {
			f.Subject += " " + m[1]
		}
		out = append(out, f)
	}
	slices.SortStableFunc(out, func(a, b finding.Finding) int {
		if c := strings.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		if a.Line != b.Line {
			return a.Line - b.Line
		}
		if a.Col != b.Col {
			return a.Col - b.Col
		}
		if c := strings.Compare(a.Rule, b.Rule); c != 0 {
			return c
		}
		return strings.Compare(a.Subject, b.Subject)
	})
	return out, nil
}

// Check runs oasdiff in ctx.Root on the committed and the generated spec. The
// tool must be on PATH and not older than cfg.MinVersion; anything unexpected
// is an error.
func Check(ctx runner.Context, cfg config.OasdiffConfig) ([]finding.Finding, error) {
	sev, err := extern.Severity(cfg.Severity)
	if err != nil {
		return nil, fmt.Errorf("api.oasdiff: %w", err)
	}
	for _, k := range [][2]string{{"spec", cfg.Spec}, {"against", cfg.Against}} {
		if _, err := os.Stat(filepath.Join(ctx.Root, filepath.FromSlash(k[1]))); err != nil {
			return nil, fmt.Errorf("api.oasdiff.%s: %w", k[0], err)
		}
	}
	tool, err := extern.Lookup(toolName)
	if err != nil {
		return nil, err
	}
	version, err := extern.Version(tool, "--version")
	if err != nil {
		// ponytail: `go install` builds print "oasdiff version main", with no number to compare; accepted unchecked, pin the release binary in CI for a real floor.
		out, _, _, rerr := extern.Run("", tool, "--version")
		if rerr != nil || !strings.Contains(string(out), "version main") {
			return nil, fmt.Errorf("%s: %w", toolName, err)
		}
		version = "main"
	} else if extern.Older(version, cfg.MinVersion) {
		return nil, fmt.Errorf("%s %s is older than the required %s", toolName, version, cfg.MinVersion)
	}
	report.ToolExtensions = append(report.ToolExtensions, report.ToolExtension{Name: toolName, Version: version})

	stdout, stderr, code, err := extern.Run(ctx.Root, tool, "changelog", cfg.Spec, cfg.Against, "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", toolName, err)
	}
	if code != 0 {
		return nil, fmt.Errorf("%s exited %d: %s", toolName, code, firstLine(stderr))
	}
	return Parse(ctx.Root, stdout, cfg.Spec, sev)
}

func firstLine(b []byte) string {
	s, _, _ := strings.Cut(strings.TrimSpace(string(b)), "\n")
	return strings.TrimSpace(s)
}
