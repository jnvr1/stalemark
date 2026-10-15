package config

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"github.com/jnvr1/stalemark/internal/finding"
)

// Entry is one suppressed finding. Only Fingerprint matters for matching; the
// other fields are there for humans reading the file.
type Entry struct {
	Fingerprint string `json:"fingerprint"`
	Rule        string `json:"rule"`
	File        string `json:"file"`
	Subject     string `json:"subject"`
}

type baselineFile struct {
	Version  int     `json:"version"`
	Findings []Entry `json:"findings"`
}

// ReadBaseline returns the entries of the baseline at path keyed by
// fingerprint. A missing file is an empty baseline.
func ReadBaseline(path string) (map[string]Entry, error) {
	src, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc baselineFile
	if err := json.Unmarshal(src, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	m := make(map[string]Entry, len(doc.Findings))
	for _, e := range doc.Findings {
		m[e.Fingerprint] = e
	}
	return m, nil
}

// WriteBaseline writes findings to path, deduplicated by fingerprint and
// sorted by it so the file is stable across runs.
func WriteBaseline(path string, findings []finding.Finding) error {
	byFP := map[string]Entry{}
	for _, f := range findings {
		fp := f.Fingerprint()
		byFP[fp] = Entry{fp, f.Rule, f.Path, f.Subject}
	}
	doc := baselineFile{Version: 1, Findings: make([]Entry, 0, len(byFP))}
	for _, e := range byFP {
		doc.Findings = append(doc.Findings, e)
	}
	slices.SortFunc(doc.Findings, func(a, b Entry) int { return cmp.Compare(a.Fingerprint, b.Fingerprint) })

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
