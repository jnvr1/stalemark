package report_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jnvr1/stalemark/internal/report"
)

func TestSARIFToolExtensions(t *testing.T) {
	var none bytes.Buffer
	if err := report.SARIF(&none, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(none.String(), "extensions") {
		t.Error("extensions must be omitted when no wrapped tool ran")
	}

	report.ToolExtensions = []report.ToolExtension{{Name: "pydoclint", Version: "0.11.1"}}
	t.Cleanup(func() { report.ToolExtensions = nil })
	var buf bytes.Buffer
	if err := report.SARIF(&buf, nil); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Runs []struct {
			Tool struct {
				Extensions []struct{ Name, Version string }
			}
		}
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if ext := doc.Runs[0].Tool.Extensions; len(ext) != 1 || ext[0].Name != "pydoclint" || ext[0].Version != "0.11.1" {
		t.Errorf("extensions = %+v", ext)
	}
}
