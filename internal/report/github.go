package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/jnvr1/stalemark/internal/finding"
)

var (
	// GitHub workflow commands: data escapes %, \r, \n; properties also : and ,.
	escapeData     = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	escapeProperty = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C")
)

// GitHub writes findings as workflow-command annotations, one per line in the
// same order as Text. More than 25,000 findings are truncated with a warning.
// Nothing is written for no findings. The caller's slice is left untouched.
func GitHub(w io.Writer, findings []finding.Finding) error {
	rows := sorted(findings)
	truncated := 0
	if len(rows) > maxResults {
		truncated = len(rows) - maxResults
		rows = rows[:maxResults]
	}
	for _, f := range rows {
		level := string(f.Severity)
		if f.Severity == finding.Note {
			level = "notice" // GitHub has no "note" command
		}
		pos := fmt.Sprintf("file=%s,line=%d", escapeProperty.Replace(slash(f.Path)), f.Line)
		if f.Col > 0 {
			pos += fmt.Sprintf(",col=%d", f.Col)
		}
		if _, err := fmt.Fprintf(w, "::%s %s,title=%s::%s\n", level, pos, escapeProperty.Replace(f.Rule), escapeData.Replace(f.Message)); err != nil {
			return err
		}
	}
	if truncated > 0 {
		_, err := fmt.Fprintf(w, "::warning title=stalemark::%d findings truncated to %d (GitHub annotations limit)\n", truncated, maxResults)
		return err
	}
	return nil
}
