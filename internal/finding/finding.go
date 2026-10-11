// Package finding defines the result every check reports: a rule violation
// located in a document, with a stable fingerprint for baselines.
package finding

import (
	"crypto/sha256"
	"encoding/hex"
)

// Severity uses the SARIF level names.
type Severity string

// Severity levels, from most to least severe.
const (
	Error   Severity = "error"
	Warning Severity = "warning"
	Note    Severity = "note"
)

// Finding is one rule violation. Path is the document path relative to the
// check root with forward slashes. Col 0 means unknown. Subject is the thing
// the rule is about (a script name, a path...) and Suggestion the closest
// valid alternative, "" when there is none.
type Finding struct {
	Rule       string
	Severity   Severity
	Path       string
	Line, Col  int
	Message    string
	Subject    string
	Suggestion string
}

// Fingerprint identifies the finding across runs: the first 16 hex chars of
// sha256(Rule, Path, Subject). Line is left out so baselines survive edits
// that only shift lines.
func (f Finding) Fingerprint() string {
	sum := sha256.Sum256([]byte(f.Rule + "\x00" + f.Path + "\x00" + f.Subject))
	return hex.EncodeToString(sum[:])[:16]
}
