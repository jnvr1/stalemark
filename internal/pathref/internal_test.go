package pathref

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jnvr1/stalemark/internal/markdown"
	"github.com/jnvr1/stalemark/internal/runner"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		tok    string
		inline bool
		want   string // "" = not a candidate
	}{
		{"cmd/app/main.go", true, "cmd/app/main.go"},
		{"foo/bar.go,", true, "foo/bar.go"},
		{"foo/bar.go.", true, "foo/bar.go"},
		{"(foo/bar.go)", true, ""},
		{"./scripts/x.sh", true, "./scripts/x.sh"},
		{".github/workflows/ci.yml", true, ".github/workflows/ci.yml"},
		{"internal/pkg/", true, "internal/pkg/"},
		{"internal/", true, ""},
		{"package.json", true, ""},
		{"github.com/foo/bar", true, ""},
		{"github.com/foo/bar.go", true, ""},
		{"application/json", true, ""},
		{"owner/repo", true, ""},
		{"net/http.Client", true, ""},
		{"src/*.ts", true, ""},
		{"file?.txt", true, ""},
		{"a/[b].go", true, ""},
		{"<repo>/docs.md", true, ""},
		{"a/{b}.go", true, ""},
		{"$HOME/a.go", true, ""},
		{"a b/c.go", true, ""},
		{"/etc/hosts", true, ""},
		{"~/.ssh/config", true, ""},
		{`C:\Users\x.txt`, true, ""},
		{"https://a/b.md", true, ""},
		{"a/.../b.md", true, ""},
		{"path/to/file.go", true, ""},
		{"x/path/to/file.go", true, ""},
		{"your-repo/a.go", true, ""},
		{"my-app/a.go", true, ""},
		{"dist/a.js", true, ""},
		{"node_modules/a/b.js", true, ""},
		{"./build/a.js", true, ""},
		{"target/debug/app", false, ""},
		{"./run.sh", false, ""},
		{"./bin/app", false, ""},
		{"venv/bin/activate", false, ""},
		{"../x/y.md", false, "../x/y.md"},
		{"vendor/x/y.go", true, ""},
		{"github/codeql-action/upload-sarif@v4", false, ""},
		{"docs/adr", false, "docs/adr"},
		{"owner/repo", false, "owner/repo"},
		{"internal/", false, ""},
	}
	for _, tt := range tests {
		got, ok := normalize(tt.tok, tt.inline)
		if (tt.want != "") != ok || got != tt.want {
			t.Errorf("normalize(%q, inline=%v) = %q, %v; want %q", tt.tok, tt.inline, got, ok, tt.want)
		}
	}
}

// check parses src as the doc at rel under a temp root holding files and
// returns the subjects of the findings.
func check(t *testing.T, rel, src string, files ...string) []string {
	t.Helper()
	root := t.TempDir()
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	d, err := markdown.Parse(filepath.Join(root, filepath.FromSlash(rel)), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range Check(runner.Context{Root: root}, d) {
		got = append(got, f.Subject)
	}
	return got
}

func TestCheckResolution(t *testing.T) {
	span := "see `img/pic.png`\n"
	if got := check(t, "docs/README.md", span, "docs/img/pic.png"); got != nil {
		t.Errorf("doc-relative file should resolve, got %v", got)
	}
	if got := check(t, "docs/README.md", span, "img/pic.png"); got != nil {
		t.Errorf("root-relative file should resolve from a subfolder doc, got %v", got)
	}
	if got := check(t, "README.md", span, "docs/img/pic.png"); !slices.Equal(got, []string{"img/pic.png"}) {
		t.Errorf("a file only under docs/ is missing from a root doc, got %v", got)
	}
	if got := check(t, "README.md", "see `img/`\n[d](img/pic)\n", "img/pic/x.png"); got != nil {
		t.Errorf("a directory is fine, got %v", got)
	}
	if got := check(t, "docs/README.md", "[x](../../outside/x.md)\n"); got != nil {
		t.Errorf("a path escaping the root is skipped, got %v", got)
	}
	if got := check(t, "docs/README.md", "[x](../LICENSE.md)\n[y](../gone.md)\n", "LICENSE.md"); !slices.Equal(got, []string{"../gone.md"}) {
		t.Errorf("../ inside the root is checked, got %v", got)
	}
	if got := check(t, "README.md", "[x](docs/a%20b.md#frag)\n[y](docs/a.md?x=1#f)\n", "docs/a.md"); got != nil {
		t.Errorf("query and fragment are stripped, got %v", got)
	}
}

func TestCheckCommands(t *testing.T) {
	tests := []struct {
		name, block string
		want        []string
	}{
		{"relative script", "./scripts/run.sh", []string{"./scripts/run.sh"}},
		{"parent script escaping the root", "../scripts/run.sh", nil},
		{"python", "python scripts/x.py --flag", []string{"scripts/x.py"}},
		{"python options skipped", "python3 -u scripts/x.py", []string{"scripts/x.py"}},
		{"node", "node server/x.js", []string{"server/x.js"}},
		{"source", "source cfg/activate", []string{"cfg/activate"}},
		{"python -m", "python -m pkg/mod", nil},
		{"bash -c", "bash -c scripts/x", nil},
		{"docker compose -f", "docker compose -f deploy/c.yml up", []string{"deploy/c.yml"}},
		{"docker compose --file", "docker compose --file deploy/c.yml up", []string{"deploy/c.yml"}},
		{"docker compose --file=", "docker compose --file=deploy/c.yml up", []string{"deploy/c.yml"}},
		{"docker-compose -f twice", "docker-compose -f a/b.yml -f a/c.yml up", []string{"a/b.yml", "a/c.yml"}},
		{"docker run is not compose", "docker run -f a/b.yml x", nil},
		{"kubectl apply", "kubectl apply -f k8s/a.yaml", []string{"k8s/a.yaml"}},
		{"kubectl delete", "kubectl delete -f k8s/a.yaml", []string{"k8s/a.yaml"}},
		{"kubectl get", "kubectl get -f k8s/a.yaml", nil},
		{"other commands", "cat docs/x.md\ncp a.txt docs/y.md\nvim src/a.go", nil},
		{"placeholder command", "./scripts/<name>.sh\npython scripts/$X.py", nil},
		{"absolute", "python /opt/x/y.py\n./run.sh", nil},
		{"mkdir", "mkdir -p lib/a\npython lib/a/x.py", nil},
		{"touch", "touch notes/a.txt\nnode notes/a.txt", nil},
		{"mv last arg", "mv a.go lib/b.go\nnode lib/b.go", nil},
		{"cp last arg", "cp a.go lib/b.go\nnode lib/b.go", nil},
		{"cd then mkdir", "cd app\nmkdir out/x\nnode app/out/x/i.js", nil},
		{"git clone dir", "git clone https://h/o/r.git work\nnode work/a.js", nil},
		{"git clone basename", "git clone https://h/o/r.git\nnode r/a.js", nil},
		{"cargo new", "cargo new proj\nnode proj/a.js", nil},
		{"npx create", "npx create-react-app web\nnode web/a.js", nil},
		{"npm create", "npm create vite@latest web -- --template x\nnode web/a.js", nil},
		{"unrelated clone stays missing", "mkdir a\nnode b/a.js", []string{"b/a.js"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := check(t, "README.md", "```bash\n"+tt.block+"\n```\n")
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckIgnoredBlockAndDedup(t *testing.T) {
	if got := check(t, "README.md", "<!-- stalemark-ignore -->\n```bash\n./a/b.sh\n```\n"); got != nil {
		t.Errorf("ignored block, got %v", got)
	}
	if got := check(t, "README.md", "`a/b.go` and `a/b.go` again\n"); !slices.Equal(got, []string{"a/b.go"}) {
		t.Errorf("same token twice on one line is one finding, got %v", got)
	}
}
