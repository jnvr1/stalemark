// Package pydoclint wraps the external pydoclint tool (ADR 0003): it runs it
// on PATH, checks its version and maps its native output to findings.
package pydoclint

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jnvr1/stalemark/internal/config"
	"github.com/jnvr1/stalemark/internal/extern"
	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/report"
	"github.com/jnvr1/stalemark/internal/runner"
)

// Rules, one per family of pydoclint codes. Codes outside rules are dropped.
const (
	RuleArgs     = "SM-B010"
	RuleArgOrder = "SM-B011"
	RuleReturn   = "SM-B012"
	RuleRaises   = "SM-B013"
	toolName     = "pydoclint"
)

var rules = map[string]string{
	"DOC101": RuleArgs, "DOC102": RuleArgs, "DOC103": RuleArgs,
	"DOC104": RuleArgOrder, "DOC105": RuleArgOrder,
	"DOC201": RuleReturn, "DOC202": RuleReturn, "DOC203": RuleReturn,
	"DOC501": RuleRaises, "DOC502": RuleRaises, "DOC503": RuleRaises,
}

var (
	// `file:line: DOCnnn: message`; file may be absolute (C:\x.py:3: ...).
	lineRE = regexp.MustCompile(`^(.+):(\d+): (DOC\d+): (.*)$`)
	// "Function `name`", "Method `Class.name`" at the start of the message.
	subjectRE = regexp.MustCompile("^(?:Function|Method|Class) `([^`]+)`")
)

// Parse maps pydoclint's `file:line: DOCnnn: message` lines to findings,
// sorted by path, line and rule. Lines that do not match, and codes outside
// the rule map, are dropped. Paths are made relative to root with forward
// slashes; \r\n and backslashes are tolerated so recorded output is portable.
//
// ponytail: Subject is the function name only, so two findings of one rule on one function share a baseline fingerprint; add the argument name if that hurts.
func Parse(root string, raw []byte, sev finding.Severity) []finding.Finding {
	prefix := strings.TrimSuffix(strings.ReplaceAll(root, `\`, "/"), "/") + "/"
	var out []finding.Finding
	for _, l := range strings.Split(string(raw), "\n") {
		m := lineRE.FindStringSubmatch(strings.TrimRight(l, "\r"))
		if m == nil {
			continue
		}
		rule, ok := rules[m[3]]
		if !ok {
			continue
		}
		line, _ := strconv.Atoi(m[2])
		f := finding.Finding{
			Rule: rule, Severity: sev,
			Path: strings.TrimPrefix(strings.ReplaceAll(m[1], `\`, "/"), prefix),
			Line: line, Message: "pydoclint " + m[3] + ": " + m[4],
		}
		if s := subjectRE.FindStringSubmatch(m[4]); s != nil {
			f.Subject = s[1]
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
		return strings.Compare(a.Rule, b.Rule)
	})
	return out
}

// reported says whether stderr holds at least one violation line, mapped or
// not: exit 1 without one is a usage error, not a finding.
func reported(stderr []byte) bool {
	for _, l := range strings.Split(string(stderr), "\n") {
		if lineRE.MatchString(strings.TrimRight(l, "\r")) {
			return true
		}
	}
	return false
}

// Check runs pydoclint in ctx.Root and returns its findings. The tool must be
// on PATH and not older than cfg.MinVersion; anything unexpected is an error.
func Check(ctx runner.Context, cfg config.PydoclintConfig) ([]finding.Finding, error) {
	sev, err := extern.Severity(cfg.Severity)
	if err != nil {
		return nil, fmt.Errorf("api.pydoclint: %w", err)
	}
	path, err := extern.Lookup(toolName)
	if err != nil {
		return nil, err
	}
	version, err := extern.Version(path, "--version")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", toolName, err)
	}
	if extern.Older(version, cfg.MinVersion) {
		return nil, fmt.Errorf("%s %s is older than the required %s", toolName, version, cfg.MinVersion)
	}
	report.ToolExtensions = append(report.ToolExtensions, report.ToolExtension{Name: toolName, Version: version})

	args := append([]string{"--show-filenames-in-every-violation-message=True", "--auto-regenerate-baseline=False", "--quiet"}, cfg.Paths...)
	_, stderr, code, err := extern.Run(ctx.Root, path, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", toolName, err)
	}
	if code == 0 {
		return nil, nil
	}
	if code == 1 && reported(stderr) {
		return Parse(ctx.Root, stderr, sev), nil
	}
	return nil, fmt.Errorf("%s exited %d: %s", toolName, code, strings.TrimSpace(string(stderr)))
}
