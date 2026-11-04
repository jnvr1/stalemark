package pathref_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jnvr1/stalemark/internal/pathref"
)

func TestIndexHasTail(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"app/x/domain/rules.py", "node_modules/pkg/lib/a.js", "testdata/fx/only/here.py"} {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ix := pathref.NewIndex(root)
	for tok, want := range map[string]bool{
		"app/x/domain/rules.py": true, // exact
		"domain/rules.py":       true, // suffix at a boundary
		"x/domain/rules.py":     true,
		"main/rules.py":         false, // not a boundary
		"omain/rules.py":        false,
		"other/domain/rules.py": false, // whole token must match
		"x/domain/":             true,  // directory tail, trailing slash
		"domain/":               true,
		"app/x":                 true,
		"pkg/lib/a.js":          false, // generated dir skipped
		"only/here.py":          false, // testdata skipped
	} {
		if got := ix.HasTail(tok); got != want {
			t.Errorf("HasTail(%q) = %v, want %v", tok, got, want)
		}
	}
}
