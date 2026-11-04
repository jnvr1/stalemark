package envvars_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jnvr1/stalemark/internal/config"
	"github.com/jnvr1/stalemark/internal/envvars"
	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/markdown"
	"github.com/jnvr1/stalemark/internal/runner"
)

// TestCodeGolden runs the doc checks plus the code pass over each case dir;
// a case's .stalemark.yml may switch on env.documented-unused.
func TestCodeGolden(t *testing.T) {
	casesDir := filepath.Join("..", "..", "testdata", "envcode")
	entries, err := os.ReadDir(casesDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			root, err := filepath.Abs(filepath.Join(casesDir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load(root)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := markdown.Load(filepath.Join(root, "README.md"))
			if err != nil {
				t.Fatal(err)
			}
			ctx := runner.Context{Root: root}
			reads, err := envvars.ScanCode(root)
			if err != nil {
				t.Fatal(err)
			}
			findings := envvars.Check(ctx, doc)
			findings = append(findings, envvars.CheckCode(envvars.Documented(ctx, doc), envvars.ExampleKeys(root), reads, cfg.Env.DocumentedUnused)...)
			if findings == nil {
				findings = []finding.Finding{}
			}
			got, err := json.MarshalIndent(findings, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')

			golden := filepath.Join(root, "expected.json")
			if *update {
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden: %v (run with -update to create it)", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("findings differ from %s (run with -update to refresh)\ngot:\n%s", golden, got)
			}
		})
	}
}

// TestScanCode covers what the fixtures cannot: comment shapes on one line,
// every pattern spelling, and test-only classification by path.
func TestScanCode(t *testing.T) {
	files := map[string]string{
		"a.js":                    "/* process.env.IN */ process.env.OUT; // process.env.TAIL\n",
		"b.py":                    "x = 1  # os.getenv(\"TAIL\")\ny = os.environ[ 'SP' ]\n",
		"c.rs":                    "let a = env::var_os(\"OS\"); // env::var(\"TAIL\")\n",
		"d.go":                    " * os.Getenv(\"DOC\")\n",
		"e.js":                    "const { a } = process.env['ONE'] + process.env.TWO;\n",
		"src/x.test.ts":           "process.env.T1;\n",
		"tests/y.go":              "os.Getenv(\"T2\")\n",
		"__tests__/z.js":          "process.env.T3;\n",
		"pkg/z_test.go":           "os.Getenv(\"T4\")\n",
		"test_app.py":             "os.getenv(\"T5\")\n",
		"src/spec.js":             "process.env.NOTTEST;\n",
		"notes.txt":               "process.env.TXT\n",
		"build/out.js":            "process.env.BUILT\n",
		".venv/lib.py":            "os.getenv(\"VENV\")\n",
		"testdata/fx.go":          "os.Getenv(\"FIXTURE\")\n",
		"target/debug/gen.rs":     "env::var(\"GEN\")\n",
		"dist/bundle.js":          "process.env.BUNDLED\n",
		"node_modules/m/index.js": "process.env.MOD\n",
		"vendor/v.go":             "os.Getenv(\"VEND\")\n",
		"inner/node_modules/q.js": "process.env.INNER\n",
		"x/ok.cjs":                "process.env.CJS\n",
		"e2e/x.ts":                "process.env.E1;\n",
		"cypress/x.js":            "process.env.E2;\n",
		"playwright/x.ts":         "process.env.E3;\n",
		"integration/x.go":        "os.Getenv(\"E4\")\n",
		"src/a.e2e.ts":            "process.env.E5;\n",
		"src/e2e_helper.ts":       "process.env.NOTE2E;\n",
	}
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	reads, err := envvars.ScanCode(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]envvars.Read{}
	for _, r := range reads {
		got[r.Name] = r
	}
	want := map[string]bool{ // name -> TestOnly
		"OUT": false, "SP": false, "OS": false, "ONE": false, "TWO": false, "CJS": false, "NOTTEST": false,
		"T1": true, "T2": true, "T3": true, "T4": true, "T5": true,
		"E1": true, "E2": true, "E3": true, "E4": true, "E5": true, "NOTE2E": false,
	}
	names := map[string]bool{}
	for n, r := range got {
		names[n] = r.TestOnly
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("reads (name -> test-only) = %v, want %v", names, want)
	}
	if r := got["SP"]; r.Path != "b.py" || r.Line != 2 {
		t.Errorf("SP read = %+v, want b.py line 2", r)
	}
}

func TestCheckCodeDedupesAndSkipsToolingAndTestOnly(t *testing.T) {
	reads := []envvars.Read{
		{Name: "TESTY", Path: "a.test.js", Line: 1, TestOnly: true},
		{Name: "SECRET", Path: "a.js", Line: 3},
		{Name: "SECRET", Path: "b.js", Line: 1},
		{Name: "HOME", Path: "a.js", Line: 4},
		{Name: "CARGO_HOME", Path: "a.js", Line: 5},
		{Name: "SEEN", Path: "a.js", Line: 6},
	}
	got := envvars.CheckCode(map[string]envvars.Loc{"SEEN": {Path: "README.md", Line: 2}}, nil, reads, false)
	if len(got) != 1 || got[0].Subject != "SECRET" || got[0].Path != "a.js" || got[0].Line != 3 || got[0].Severity != finding.Note {
		t.Fatalf("findings = %+v, want exactly one note for SECRET at a.js:3", got)
	}
}

func TestCheckCodeExampleKeysCountAsDocumented(t *testing.T) {
	reads := []envvars.Read{
		{Name: "DB_SERVER", Path: "app/db.py", Line: 3},
		{Name: "NOPE", Path: "app/db.py", Line: 4},
	}
	got := envvars.CheckCode(map[string]envvars.Loc{}, map[string]bool{"DB_SERVER": true}, reads, false)
	if len(got) != 1 || got[0].Subject != "NOPE" || got[0].Rule != envvars.RuleEnvReadUndocumented {
		t.Errorf("findings = %+v, want only NOPE as SM-E003", got)
	}
}

func TestExampleKeys(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env.example"), []byte("# c\nexport A=1\nB=\"x\"\n\nnot a key\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := envvars.ExampleKeys(dir)
	want := map[string]bool{"A": true, "B": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExampleKeys = %v, want %v", got, want)
	}
	if got := envvars.ExampleKeys(t.TempDir()); len(got) != 0 {
		t.Errorf("no example file must give no keys, got %v", got)
	}
}
