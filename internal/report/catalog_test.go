package report_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/report"
)

var ruleConstRE = regexp.MustCompile(`=\s*"(SM-[A-Z][0-9]{3})"`)

// TestCatalogCoversEveryRule scans the rule constants declared under internal/
// and renders one SARIF finding per rule: a rule missing from the catalog
// falls back to its id as description, which this test rejects.
func TestCatalogCoversEveryRule(t *testing.T) {
	ids := map[string]bool{}
	err := filepath.WalkDir(filepath.Join("..", ".."), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")) && p != filepath.Join("..", "..") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range ruleConstRE.FindAllSubmatch(src, -1) {
			ids[string(m[1])] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) < 10 {
		t.Fatalf("found only %d rule constants, the scan is broken", len(ids))
	}
	for id := range ids {
		var buf bytes.Buffer
		f := finding.Finding{Rule: id, Severity: finding.Error, Path: "README.md", Line: 1, Message: "m", Subject: "s"}
		if err := report.SARIF(&buf, []finding.Finding{f}); err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Runs []struct {
				Tool struct {
					Driver struct {
						Rules []struct {
							ID               string
							ShortDescription struct{ Text string }
						}
					}
				}
			}
		}
		if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		if r := doc.Runs[0].Tool.Driver.Rules[0]; r.ID != id || r.ShortDescription.Text == id {
			t.Errorf("%s has no catalog entry (shortDescription = %q)", id, r.ShortDescription.Text)
		}
	}
}
