package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jnvr1/stalemark/internal/config"
	"github.com/jnvr1/stalemark/internal/finding"
)

func write(t *testing.T, files map[string]string) string {
	t.Helper()
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
	return dir
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := config.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"README.md", "CONTRIBUTING.md", "docs/**/*.md"}; !reflect.DeepEqual(cfg.Include, want) {
		t.Errorf("Include = %v, want %v", cfg.Include, want)
	}
	if cfg.Baseline != ".stalemark-baseline.json" {
		t.Errorf("Baseline = %q", cfg.Baseline)
	}
	if len(cfg.Exclude) != 0 || len(cfg.Disable) != 0 {
		t.Errorf("Exclude/Disable = %v/%v, want empty", cfg.Exclude, cfg.Disable)
	}
}

func TestLoadParsesKeys(t *testing.T) {
	dir := write(t, map[string]string{".stalemark.yml": "include: [a.md]\nexclude: [b/**]\ndisable: [SM-A001]\nbaseline: base.json\n"})
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := config.Config{Include: []string{"a.md"}, Exclude: []string{"b/**"}, Disable: []string{"SM-A001"}, Baseline: "base.json"}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}
}

func TestLoadEnvSection(t *testing.T) {
	cfg, err := config.Load(write(t, map[string]string{".stalemark.yml": "env:\n  documented-unused: true\n"}))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Env.DocumentedUnused {
		t.Errorf("cfg.Env = %+v, want DocumentedUnused", cfg.Env)
	}
}

func TestLoadADRDirs(t *testing.T) {
	dir := write(t, map[string]string{".stalemark.yml": "adr:\n  dirs: [decisions]\n"})
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"decisions"}; !reflect.DeepEqual(cfg.ADR.Dirs, want) {
		t.Errorf("ADR.Dirs = %v, want %v", cfg.ADR.Dirs, want)
	}
}

func TestLoadADRHistorical(t *testing.T) {
	for _, v := range []string{"warn", "skip"} {
		cfg, err := config.Load(write(t, map[string]string{".stalemark.yml": "adr:\n  historical: " + v + "\n"}))
		if err != nil || cfg.ADR.Historical != v {
			t.Errorf("historical %s: got %q, %v", v, cfg.ADR.Historical, err)
		}
	}
	_, err := config.Load(write(t, map[string]string{".stalemark.yml": "adr:\n  historical: nope\n"}))
	if err == nil || err.Error() != ".stalemark.yml: adr.historical must be warn or skip" {
		t.Errorf("err = %v", err)
	}
}

func TestLoadADRFreetext(t *testing.T) {
	cfg, err := config.Load(write(t, map[string]string{".stalemark.yml": "adr:\n  freetext: true\n"}))
	if err != nil || !cfg.ADR.Freetext {
		t.Errorf("Freetext = %v, err = %v, want true", cfg.ADR.Freetext, err)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := map[string]struct{ yml, want string }{
		"unknown key":     {"includes: [a.md]\n", "includes"},
		"unknown env key": {"env:\n  nope: true\n", "nope"},
		"invalid yaml":    {"include: [a.md\n", ".stalemark.yml"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := config.Load(write(t, map[string]string{".stalemark.yml": tt.yml}))
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.HasPrefix(err.Error(), ".stalemark.yml: ") {
				t.Errorf("err = %v, want prefix .stalemark.yml: and %q", err, tt.want)
			}
		})
	}
}

func TestDocs(t *testing.T) {
	dir := write(t, map[string]string{
		"README.md":                  "",
		"CONTRIBUTING.md":            "",
		"docs/a.md":                  "",
		"docs/deep/b.md":             "",
		"docs/internal/c.md":         "",
		"docs/notes.txt":             "",
		"other/x.md":                 "",
		"node_modules/pkg/README.md": "",
		"docs/node_modules/y.md":     "",
		".git/docs/z.md":             "",
	})
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Exclude = []string{"docs/internal/**"}
	got, err := cfg.Docs(dir)
	if err != nil {
		t.Fatal(err)
	}
	var rel []string
	for _, p := range got {
		r, _ := filepath.Rel(dir, p)
		rel = append(rel, filepath.ToSlash(r))
	}
	want := []string{"CONTRIBUTING.md", "README.md", "docs/a.md", "docs/deep/b.md"}
	if !reflect.DeepEqual(rel, want) {
		t.Errorf("docs = %v, want %v", rel, want)
	}
}

func TestDocsBadPattern(t *testing.T) {
	dir := write(t, map[string]string{"README.md": ""})
	cfg := config.Config{Include: []string{"[a"}}
	if _, err := cfg.Docs(dir); err == nil {
		t.Error("want error for bad pattern")
	}
}

func TestBaselineRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.json")
	m, err := config.ReadBaseline(path)
	if err != nil || len(m) != 0 {
		t.Fatalf("missing file: %v, %v", m, err)
	}
	b := finding.Finding{Rule: "SM-A001", Path: "README.md", Subject: "buidl", Line: 4}
	a := finding.Finding{Rule: "SM-P001", Path: "README.md", Subject: "x/y.go"}
	if err := config.WriteBaseline(path, []finding.Finding{b, a, b}); err != nil {
		t.Fatal(err)
	}
	m, err = config.ReadBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 {
		t.Fatalf("entries = %v, want 2", m)
	}
	e := m[b.Fingerprint()]
	if e.Rule != "SM-A001" || e.File != "README.md" || e.Subject != "buidl" {
		t.Errorf("entry = %+v", e)
	}
	raw, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(raw), "{\n  \"version\": 1,\n  \"findings\": [\n") || !strings.HasSuffix(string(raw), "}\n") {
		t.Errorf("format:\n%s", raw)
	}
}

func TestBaselineMalformed(t *testing.T) {
	dir := write(t, map[string]string{"b.json": "{nope"})
	if _, err := config.ReadBaseline(filepath.Join(dir, "b.json")); err == nil {
		t.Error("want error")
	}
}

func TestLoadAPIPydoclint(t *testing.T) {
	cfg, err := config.Load(write(t, map[string]string{".stalemark.yml": "api:\n  pydoclint: {}\n"}))
	if err != nil {
		t.Fatal(err)
	}
	want := config.PydoclintConfig{Paths: []string{"."}, MinVersion: "0.11.1", Severity: "warning"}
	if cfg.API.Pydoclint == nil || !reflect.DeepEqual(*cfg.API.Pydoclint, want) {
		t.Errorf("Pydoclint = %+v, want %+v", cfg.API.Pydoclint, want)
	}
	cfg, err = config.Load(write(t, map[string]string{".stalemark.yml": "api:\n  pydoclint: {paths: [src], min-version: \"0.12.0\", severity: note}\n"}))
	if err != nil {
		t.Fatal(err)
	}
	want = config.PydoclintConfig{Paths: []string{"src"}, MinVersion: "0.12.0", Severity: "note"}
	if !reflect.DeepEqual(*cfg.API.Pydoclint, want) {
		t.Errorf("Pydoclint = %+v, want %+v", *cfg.API.Pydoclint, want)
	}
	if cfg, _ = config.Load(t.TempDir()); cfg.API.Pydoclint != nil {
		t.Error("an absent api key must leave the wrapper off")
	}
	if _, err := config.Load(write(t, map[string]string{".stalemark.yml": "api:\n  pydoclint: {severity: fatal}\n"})); err == nil {
		t.Error("an invalid severity must fail")
	}
}

func TestLoadAPIOasdiff(t *testing.T) {
	cfg, err := config.Load(write(t, map[string]string{".stalemark.yml": "api:\n  oasdiff: {spec: openapi.yaml, against: build/gen.yaml}\n"}))
	if err != nil {
		t.Fatal(err)
	}
	want := config.OasdiffConfig{Spec: "openapi.yaml", Against: "build/gen.yaml", MinVersion: "1.33.0", Severity: "warning"}
	if cfg.API.Oasdiff == nil || *cfg.API.Oasdiff != want {
		t.Errorf("Oasdiff = %+v, want %+v", cfg.API.Oasdiff, want)
	}
	cfg, err = config.Load(write(t, map[string]string{".stalemark.yml": "api:\n  oasdiff: {spec: a.json, against: b.json, min-version: \"1.40.0\", severity: error}\n"}))
	if err != nil {
		t.Fatal(err)
	}
	want = config.OasdiffConfig{Spec: "a.json", Against: "b.json", MinVersion: "1.40.0", Severity: "error"}
	if *cfg.API.Oasdiff != want {
		t.Errorf("Oasdiff = %+v, want %+v", *cfg.API.Oasdiff, want)
	}
	if cfg, _ = config.Load(t.TempDir()); cfg.API.Oasdiff != nil {
		t.Error("an absent api key must leave the wrapper off")
	}
	for name, body := range map[string]string{
		"missing both":    "api:\n  oasdiff: {}\n",
		"missing against": "api:\n  oasdiff: {spec: a.yaml}\n",
		"url":             "api:\n  oasdiff: {spec: a.yaml, against: \"https://example.com/b.yaml\"}\n",
		"bad severity":    "api:\n  oasdiff: {spec: a.yaml, against: b.yaml, severity: fatal}\n",
	} {
		if _, err := config.Load(write(t, map[string]string{".stalemark.yml": body})); err == nil {
			t.Errorf("%s must fail", name)
		}
	}
	_, err = config.Load(write(t, map[string]string{".stalemark.yml": "api:\n  oasdiff: {spec: a.yaml}\n"}))
	if err == nil || !strings.Contains(err.Error(), ".stalemark.yml: api.oasdiff.spec and against are required") {
		t.Errorf("required error = %v", err)
	}
}

func TestLoadAPIJsdoc(t *testing.T) {
	cfg, err := config.Load(write(t, map[string]string{".stalemark.yml": "api:\n  jsdoc: {}\n"}))
	if err != nil {
		t.Fatal(err)
	}
	want := config.JsdocConfig{Paths: []string{"."}, MinVersion: "65.0.0", EslintMinVersion: "9.0.0", Severity: "warning"}
	if cfg.API.Jsdoc == nil || !reflect.DeepEqual(*cfg.API.Jsdoc, want) {
		t.Errorf("Jsdoc = %+v, want %+v", cfg.API.Jsdoc, want)
	}
	cfg, err = config.Load(write(t, map[string]string{".stalemark.yml": "api:\n  jsdoc: {paths: [src], min-version: \"65.3.0\", eslint-min-version: \"9.10.0\", severity: error}\n"}))
	if err != nil {
		t.Fatal(err)
	}
	want = config.JsdocConfig{Paths: []string{"src"}, MinVersion: "65.3.0", EslintMinVersion: "9.10.0", Severity: "error"}
	if !reflect.DeepEqual(*cfg.API.Jsdoc, want) {
		t.Errorf("Jsdoc = %+v, want %+v", *cfg.API.Jsdoc, want)
	}
	if cfg, _ = config.Load(t.TempDir()); cfg.API.Jsdoc != nil {
		t.Error("an absent api key must leave the wrapper off")
	}
	if _, err := config.Load(write(t, map[string]string{".stalemark.yml": "api:\n  jsdoc: {severity: fatal}\n"})); err == nil {
		t.Error("an invalid severity must fail")
	}
}
