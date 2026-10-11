package runner_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/markdown"
	"github.com/jnvr1/stalemark/internal/runner"
	"github.com/jnvr1/stalemark/internal/shellx"
)

func TestClosest(t *testing.T) {
	scripts := []string{"build", "test", "lint", "build:prod", "dev"}
	tests := []struct {
		name, want string
	}{
		{"buidl", "build"},
		{"tset", "test"},
		{"lnt", "lint"},
		{"build", "build"},
		{"bui", "build"},                   // name is a prefix of a candidate
		{"build:production", "build:prod"}, // candidate is a prefix of name
		{"deploy", ""},
		{"", ""},
		{"xxxxxxxxxx", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runner.Closest(tt.name, scripts); got != tt.want {
				t.Errorf("Closest(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
	if got := runner.Closest("x", nil); got != "" {
		t.Errorf("Closest with no candidates = %q, want \"\"", got)
	}
}

func TestWorkDir(t *testing.T) {
	ctx := runner.Context{Root: filepath.Join("r", "root")}
	tests := []struct {
		dir, want string
	}{
		{"", filepath.Join("r", "root")},
		{"web", filepath.Join("r", "root", "web")},
		{"a/b", filepath.Join("r", "root", "a", "b")},
	}
	for _, tt := range tests {
		if got := runner.WorkDir(ctx, shellx.Command{Dir: tt.dir}); got != tt.want {
			t.Errorf("WorkDir(%q) = %q, want %q", tt.dir, got, tt.want)
		}
	}
}

// fake resolves "fake" and reports every call as a finding.
type fake struct {
	docs []string
}

func (f *fake) Names() []string { return []string{"fake"} }

func (f *fake) Resolve(_ runner.Context, doc string, cmd shellx.Command) []finding.Finding {
	f.docs = append(f.docs, doc)
	return []finding.Finding{{
		Rule: "T-001", Path: doc, Line: cmd.FileLine, Subject: strings.Join(append([]string{cmd.Name}, cmd.Args...), " "),
	}}
}

func TestCheckDoc(t *testing.T) {
	root := t.TempDir()
	src := strings.Join([]string{
		"# Doc",
		"",
		"```bash",
		"fake one && other x",
		"cd sub && fake two",
		"fake <name>",
		"```",
		"",
		"<!-- stalemark-ignore -->",
		"```bash",
		"fake three",
		"```",
		"",
		"```go",
		"fake four",
		"```",
		"",
		"```sh",
		"fake five",
		"```",
		"",
	}, "\n")
	path := filepath.Join(root, "docs", "README.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := markdown.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	f := &fake{}
	got := runner.New(f).CheckDoc(runner.Context{Root: root}, doc)
	want := []finding.Finding{
		{Rule: "T-001", Path: "docs/README.md", Line: 4, Subject: "fake one"},
		{Rule: "T-001", Path: "docs/README.md", Line: 5, Subject: "fake two"},
		{Rule: "T-001", Path: "docs/README.md", Line: 19, Subject: "fake five"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CheckDoc() =\n%+v\nwant\n%+v", got, want)
	}
	for _, d := range f.docs {
		if d != "docs/README.md" {
			t.Errorf("resolver got doc %q, want docs/README.md", d)
		}
	}
}

func TestCheckDocNoResolvers(t *testing.T) {
	doc, err := markdown.Parse("x.md", []byte("```sh\nnpm test\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := runner.New().CheckDoc(runner.Context{Root: "."}, doc); got != nil {
		t.Errorf("CheckDoc() = %+v, want nil", got)
	}
}
