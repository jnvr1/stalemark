package report

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/jnvr1/stalemark/internal/finding"
)

// ToolVersion is reported as the SARIF driver version; cli.Run sets it from
// the ldflags-injected version.
var ToolVersion = "dev"

// maxResults is GitHub's cap on results (code scanning) and annotations per run.
const maxResults = 25000

// ruleDoc is the SARIF short description and help for a rule.
//
// ponytail: the catalog lives here; move it next to each rule constant when a reporter other than SARIF needs it.
type ruleDoc struct{ short, help string }

var catalog = map[string]ruleDoc{
	"SM-A001": {"npm script not found", "A package.json script named in the docs does not exist."},
	"SM-A002": {"make target not found", "A Makefile target named in the docs does not exist."},
	"SM-A003": {"task not found", "A Taskfile task named in the docs is not defined."},
	"SM-A004": {"task is internal", "A Taskfile task named in the docs exists but is marked internal, so it cannot be run directly."},
	"SM-A005": {"just recipe not found", "A justfile recipe named in the docs does not exist."},
	"SM-A006": {"pyproject script not found", "A pyproject.toml script named in the docs does not exist."},
	"SM-A007": {"go package or file not found", "A local Go package directory or .go file named in the docs does not exist."},
	"SM-A008": {"go run of a non-main package", "The docs run a directory with go run, but it has no main package."},
	"SM-A009": {"cargo target not found", "A cargo --bin, --example or -p target named in the docs is not in Cargo.toml."},
	"SM-D001": {"superseded-by points at a missing ADR", "An ADR status says it is superseded by an ADR number that does not exist."},
	"SM-E001": {"env var not in .env.example", "A variable the docs name is missing from the nearest .env.example"},
	"SM-N001": {"documented localhost port not published", "A localhost port the docs name is not published by any Dockerfile EXPOSE or compose ports entry."},
	"SM-V001": {"documented tool version differs from version file", "A node, go, python or rust version the docs state differs from the one .nvmrc, go.mod, pyproject.toml or another version file pins."},
	"SM-P001": {"path not found", "A file or directory referenced in the docs does not exist in the repository."},
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifRule struct {
	ID               string    `json:"id"`
	ShortDescription sarifText `json:"shortDescription"`
	Help             sarifText `json:"help"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn,omitempty"`
}

type sarifArtifact struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId"`
}

type sarifPhysical struct {
	ArtifactLocation sarifArtifact `json:"artifactLocation"`
	Region           sarifRegion   `json:"region"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysical `json:"physicalLocation"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	RuleIndex           int               `json:"ruleIndex"`
	Level               finding.Severity  `json:"level"`
	Message             sarifText         `json:"message"`
	Locations           []sarifLocation   `json:"locations"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
}

type sarifNotification struct {
	Level   string    `json:"level"`
	Message sarifText `json:"message"`
}

type sarifInvocation struct {
	ExecutionSuccessful        bool                `json:"executionSuccessful"`
	ToolExecutionNotifications []sarifNotification `json:"toolExecutionNotifications"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRun struct {
	Tool struct {
		Driver sarifDriver `json:"driver"`
	} `json:"tool"`
	Invocations []sarifInvocation `json:"invocations,omitempty"`
	Results     []sarifResult     `json:"results"`
}

// slash turns a Windows path into the repo-relative POSIX form reporters emit.
// filepath.ToSlash would be a no-op on Linux and make output OS-dependent.
func slash(p string) string { return strings.ReplaceAll(p, `\`, "/") }

// SARIF writes findings as a SARIF 2.1.0 document with one run, in the same
// order as Text. More than 25,000 findings are truncated with a notification,
// GitHub code scanning's per-run limit. The caller's slice is left untouched.
func SARIF(w io.Writer, findings []finding.Finding) error {
	rows := sorted(findings)
	var truncated int
	if len(rows) > maxResults {
		truncated = len(rows) - maxResults
		rows = rows[:maxResults]
	}

	var ids []string
	for _, f := range rows {
		ids = append(ids, f.Rule)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)

	run := sarifRun{Results: []sarifResult{}}
	run.Tool.Driver = sarifDriver{
		Name: "stalemark", Version: ToolVersion,
		InformationURI: "https://github.com/jnvr1/stalemark",
		Rules:          []sarifRule{},
	}
	for _, id := range ids {
		doc, ok := catalog[id]
		if !ok {
			doc = ruleDoc{short: id, help: id}
		}
		run.Tool.Driver.Rules = append(run.Tool.Driver.Rules, sarifRule{id, sarifText{doc.short}, sarifText{doc.help}})
	}
	for _, f := range rows {
		run.Results = append(run.Results, sarifResult{
			RuleID:    f.Rule,
			RuleIndex: slices.Index(ids, f.Rule),
			Level:     f.Severity,
			Message:   sarifText{f.Message},
			Locations: []sarifLocation{{sarifPhysical{
				sarifArtifact{slash(f.Path), "%SRCROOT%"},
				sarifRegion{f.Line, f.Col},
			}}},
			PartialFingerprints: map[string]string{"stalemark/v1": f.Fingerprint()},
		})
	}
	if truncated > 0 {
		run.Invocations = []sarifInvocation{{true, []sarifNotification{{
			"warning",
			sarifText{fmt.Sprintf("%d findings truncated to %d (GitHub code scanning limit)", truncated, maxResults)},
		}}}}
	}

	doc := struct {
		Schema  string     `json:"$schema"`
		Version string     `json:"version"`
		Runs    []sarifRun `json:"runs"`
	}{"https://docs.oasis-open.org/sarif/sarif/v2.1.0/cos02/schemas/sarif-schema-2.1.0.json", "2.1.0", []sarifRun{run}}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
