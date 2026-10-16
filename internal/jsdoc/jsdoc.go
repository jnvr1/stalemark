// Package jsdoc wraps ESLint with eslint-plugin-jsdoc (ADR 0003): it runs
// eslint from PATH with the repository's own flat config and maps the json
// formatter's messages of three rule groups to findings.
//
// Recorded fixtures (testdata/jsdoc/*/eslint.json) hold ESLint's real output
// with the absolute case directory rewritten to the placeholder "<root>/" so
// they are identical on every OS; Parse accepts that placeholder as well as
// absolute paths under the root.
//
// For a project-local ESLint, run stalemark through `npm exec`, which puts
// node_modules/.bin on PATH.
package jsdoc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
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

// Rules, one per group of eslint-plugin-jsdoc rules. Other ruleIds are dropped.
const (
	RuleParamNames = "SM-B020"
	RuleParamReq   = "SM-B021"
	RuleTypes      = "SM-B022"
	toolName       = "eslint"
	pluginName     = "eslint-plugin-jsdoc"
	rootMarker     = "<root>/"
)

var rules = map[string]string{
	"jsdoc/check-param-names":  RuleParamNames,
	"jsdoc/require-param":      RuleParamReq,
	"jsdoc/check-types":        RuleTypes,
	"jsdoc/no-undefined-types": RuleTypes,
}

// quoted picks the first double-quoted name of a message, the @param it is about.
var quoted = regexp.MustCompile(`"([^"]+)"`)

type result struct {
	FilePath string `json:"filePath"`
	Messages []struct {
		RuleID      string `json:"ruleId"`
		Message     string `json:"message"`
		Line        int    `json:"line"`
		Column      int    `json:"column"`
		Suggestions []struct {
			Desc string `json:"desc"`
		} `json:"suggestions"`
	} `json:"messages"`
}

// Parse maps ESLint's json formatter output to findings, sorted by path, line,
// column, rule and subject. Messages whose ruleId is not in the rule map are
// dropped. Paths are made relative to root with forward slashes; absolute
// paths under root and the "<root>/" placeholder of the fixtures are accepted.
//
// ponytail: Subject is the first double-quoted name of the message ("" when there is none, e.g. no-undefined-types), so two findings of one rule on one file and name share a baseline fingerprint; add the function name if that hurts.
func Parse(root string, raw []byte, sev finding.Severity) ([]finding.Finding, error) {
	var files []result
	if err := json.Unmarshal(raw, &files); err != nil {
		return nil, fmt.Errorf("%s: output is not the json formatter's: %w", toolName, err)
	}
	prefix := strings.TrimSuffix(strings.ReplaceAll(root, `\`, "/"), "/") + "/"
	var out []finding.Finding
	for _, f := range files {
		p := strings.ReplaceAll(f.FilePath, `\`, "/")
		p = strings.TrimPrefix(strings.TrimPrefix(p, rootMarker), prefix)
		for _, m := range f.Messages {
			rule, ok := rules[m.RuleID]
			if !ok {
				continue
			}
			fd := finding.Finding{
				Rule: rule, Severity: sev, Path: p, Line: m.Line, Col: m.Column,
				Message: pluginName + " " + m.RuleID + ": " + m.Message,
			}
			if q := quoted.FindStringSubmatch(m.Message); q != nil {
				fd.Subject = q[1]
			}
			if len(m.Suggestions) > 0 {
				fd.Suggestion = m.Suggestions[0].Desc
			}
			out = append(out, fd)
		}
	}
	slices.SortStableFunc(out, func(a, b finding.Finding) int {
		return cmpAll(strings.Compare(a.Path, b.Path), a.Line-b.Line, a.Col-b.Col, strings.Compare(a.Rule, b.Rule), strings.Compare(a.Subject, b.Subject))
	})
	return out, nil
}

func cmpAll(cs ...int) int {
	for _, c := range cs {
		if c != 0 {
			return c
		}
	}
	return 0
}

// pluginVersion reads the installed plugin's version from the nearest
// node_modules/eslint-plugin-jsdoc/package.json at or above root.
//
// ponytail: walks up like Node's module resolution but ignores NODE_PATH and symlinked workspaces; the plugin a config really loads is not asked of Node.
func pluginVersion(root string) (string, error) {
	for dir := root; ; {
		b, err := os.ReadFile(filepath.Join(dir, "node_modules", pluginName, "package.json"))
		if err == nil {
			var pkg struct{ Version string }
			if err := json.Unmarshal(b, &pkg); err != nil || pkg.Version == "" {
				return "", fmt.Errorf("%s: unreadable package.json in node_modules", pluginName)
			}
			return pkg.Version, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("%s: %w", pluginName, err)
		}
		up := filepath.Dir(dir)
		if up == dir {
			return "", fmt.Errorf("%s not found in node_modules; run npm install", pluginName)
		}
		dir = up
	}
}

// Check runs eslint in ctx.Root and returns its findings. eslint must be on
// PATH and the plugin installed, neither older than the configured floors;
// anything unexpected is an error.
func Check(ctx runner.Context, cfg config.JsdocConfig) ([]finding.Finding, error) {
	sev, err := extern.Severity(cfg.Severity)
	if err != nil {
		return nil, fmt.Errorf("api.jsdoc: %w", err)
	}
	path, err := extern.Lookup(toolName)
	if err != nil {
		return nil, err
	}
	version, err := extern.Version(path, "--version")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", toolName, err)
	}
	if extern.Older(version, cfg.EslintMinVersion) {
		return nil, fmt.Errorf("%s %s is older than the required %s", toolName, version, cfg.EslintMinVersion)
	}
	plugin, err := pluginVersion(ctx.Root)
	if err != nil {
		return nil, err
	}
	if extern.Older(plugin, cfg.MinVersion) {
		return nil, fmt.Errorf("%s %s is older than the required %s", pluginName, plugin, cfg.MinVersion)
	}
	report.ToolExtensions = append(report.ToolExtensions,
		report.ToolExtension{Name: toolName, Version: version},
		report.ToolExtension{Name: pluginName, Version: plugin})

	args := append([]string{"--format", "json"}, cfg.Paths...)
	stdout, stderr, code, err := extern.Run(ctx.Root, path, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", toolName, err)
	}
	if code > 1 {
		return nil, fmt.Errorf("%s exited %d: %s", toolName, code, strings.TrimSpace(string(stderr)))
	}
	out, err := Parse(ctx.Root, stdout, sev)
	if err != nil {
		return nil, fmt.Errorf("%w (stderr: %s)", err, strings.TrimSpace(string(stderr)))
	}
	return out, nil
}
