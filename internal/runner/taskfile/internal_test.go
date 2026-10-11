package taskfile

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		cmd  string
		want invocation
	}{
		{"task", invocation{}},
		{"task build", invocation{names: []string{"build"}}},
		{"task build test", invocation{names: []string{"build", "test"}}},
		{"task -t alt.yml x", invocation{file: "alt.yml", names: []string{"x"}}},
		{"task -t=alt.yml x", invocation{file: "alt.yml", names: []string{"x"}}},
		{"task --taskfile alt.yml x", invocation{file: "alt.yml", names: []string{"x"}}},
		{"task --taskfile=alt.yml x", invocation{file: "alt.yml", names: []string{"x"}}},
		{"task -d sub build", invocation{dir: "sub", names: []string{"build"}}},
		{"task --dir sub build", invocation{dir: "sub", names: []string{"build"}}},
		{"task --dir=sub build", invocation{dir: "sub", names: []string{"build"}}},
		{"task -d sub -t alt.yml x", invocation{dir: "sub", file: "alt.yml", names: []string{"x"}}},
		{"task -o group build", invocation{names: []string{"build"}}},
		{"task --output group build", invocation{names: []string{"build"}}},
		{"task --output=group build", invocation{names: []string{"build"}}},
		{"task -C 4 build", invocation{names: []string{"build"}}},
		{"task --concurrency 4 build", invocation{names: []string{"build"}}},
		{"task -I 5s build", invocation{names: []string{"build"}}},
		{"task --interval 5s build", invocation{names: []string{"build"}}},
		{"task --output-group-begin ::group:: build", invocation{names: []string{"build"}}},
		{"task --output-group-end ::endgroup:: build", invocation{names: []string{"build"}}},
		{"task --sort alphanumeric build", invocation{names: []string{"build"}}},
		{"task -f -s -v -w -y -x -n -p build", invocation{names: []string{"build"}}},
		{"task --dry --json --silent --verbose --force --watch --yes --exit-code --parallel --color --no-status build", invocation{names: []string{"build"}}},
		{"task -l", invocation{silent: true}},
		{"task -a", invocation{silent: true}},
		{"task --list", invocation{silent: true}},
		{"task --list-all", invocation{silent: true}},
		{"task -g build", invocation{silent: true, names: []string{"build"}}},
		{"task --global build", invocation{silent: true, names: []string{"build"}}},
		{"task --init", invocation{silent: true}},
		{"task --version", invocation{silent: true}},
		{"task -h", invocation{silent: true}},
		{"task --help", invocation{silent: true}},
		{"task --summary build", invocation{silent: true, names: []string{"build"}}},
		{"task --status build", invocation{silent: true, names: []string{"build"}}},
		{"task -- build args", invocation{}},
		{"task build -- args", invocation{names: []string{"build"}}},
		{"task build FOO=bar", invocation{names: []string{"build"}}},
		{"task FOO=bar", invocation{}},
		{"task -t", invocation{}},
		{"task -d", invocation{}},
		{"task docs:build", invocation{names: []string{"docs:build"}}},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			f := strings.Fields(tt.cmd)
			if got := parseArgs(f[1:]); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseArgs(%q) = %+v, want %+v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestParseSource(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		tasks  map[string]bool // nil means the file is rejected
		opaque bool
	}{
		{"shorthand string task", "version: '3'\ntasks:\n  build: go build ./...\n", map[string]bool{"build": false}, false},
		{"list task", "version: '3'\ntasks:\n  build:\n    - go build\n    - go test\n", map[string]bool{"build": false}, false},
		{"map task", "version: '3'\ntasks:\n  build:\n    desc: x\n    cmds: [go build]\n", map[string]bool{"build": false}, false},
		{"internal", "version: '3'\ntasks:\n  setup:\n    internal: true\n    cmds: [x]\n", map[string]bool{"setup": true}, false},
		{"aliases", "version: '3'\ntasks:\n  build:\n    aliases: [b, bld]\n    cmds: [x]\n", map[string]bool{"build": false, "b": false, "bld": false}, false},
		{"internal aliases", "version: '3'\ntasks:\n  setup:\n    internal: true\n    aliases: [s]\n", map[string]bool{"setup": true, "s": true}, false},
		{"version number", "version: 3\ntasks:\n  build: x\n", map[string]bool{"build": false}, false},
		{"version float", "version: 3.0\ntasks:\n  build: x\n", map[string]bool{"build": false}, false},
		{"version minor", "version: '3.38'\ntasks:\n  build: x\n", map[string]bool{"build": false}, false},
		{"version 2", "version: '2'\ntasks:\n  build: x\n", nil, false},
		{"missing version", "tasks:\n  build: x\n", nil, false},
		{"invalid yaml", "version: '3'\ntasks: [\n", nil, false},
		{"no tasks", "version: '3'\n", map[string]bool{}, false},
		{"templated name is opaque", "version: '3'\ntasks:\n  '{{.NAME}}': x\n  build: y\n", map[string]bool{"build": false}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, _ := parseSource([]byte(tt.src))
			if tt.tasks == nil {
				if p != nil {
					t.Fatalf("got %+v, want rejected", p)
				}
				return
			}
			if p == nil {
				t.Fatal("rejected, want parsed")
			}
			if !maps.Equal(p.tasks, tt.tasks) || p.opaque != tt.opaque {
				t.Errorf("got tasks=%v opaque=%v, want tasks=%v opaque=%v", p.tasks, p.opaque, tt.tasks, tt.opaque)
			}
		})
	}
}

func TestLoadIncludes(t *testing.T) {
	const docs = "version: '3'\ntasks:\n  build: x\n  hidden:\n    internal: true\n"
	tests := []struct {
		name    string
		files   map[string]string // relative to the temp root; Taskfile.yml is loaded
		tasks   map[string]bool
		opaques []string
	}{
		{
			"string include",
			map[string]string{"Taskfile.yml": "version: '3'\nincludes:\n  docs: ./docs/Taskfile.yml\ntasks:\n  build: x\n", "docs/Taskfile.yml": docs},
			map[string]bool{"build": false, "docs:": false, "docs:build": false, "docs:hidden": true},
			nil,
		},
		{
			"map include with aliases",
			map[string]string{"Taskfile.yml": "version: '3'\nincludes:\n  docs:\n    taskfile: ./docs/Taskfile.yml\n    aliases: [d]\n", "docs/Taskfile.yml": docs},
			map[string]bool{"docs:": false, "docs:build": false, "docs:hidden": true, "d:": false, "d:build": false, "d:hidden": true},
			nil,
		},
		{
			"directory include",
			map[string]string{"Taskfile.yml": "version: '3'\nincludes:\n  docs: ./docs\n", "docs/Taskfile.yaml": docs},
			map[string]bool{"docs:": false, "docs:build": false, "docs:hidden": true},
			nil,
		},
		{
			"internal include",
			map[string]string{"Taskfile.yml": "version: '3'\nincludes:\n  docs:\n    taskfile: ./docs/Taskfile.yml\n    internal: true\n", "docs/Taskfile.yml": docs},
			map[string]bool{"docs:": false, "docs:build": true, "docs:hidden": true},
			nil,
		},
		{
			"optional missing include",
			map[string]string{"Taskfile.yml": "version: '3'\nincludes:\n  local:\n    taskfile: ./local.yml\n    optional: true\ntasks:\n  build: x\n"},
			map[string]bool{"build": false},
			nil,
		},
		{
			"missing include is opaque",
			map[string]string{"Taskfile.yml": "version: '3'\nincludes:\n  local: ./local.yml\ntasks:\n  build: x\n"},
			map[string]bool{"build": false},
			[]string{"local:"},
		},
		{
			"include with templated task is opaque",
			map[string]string{"Taskfile.yml": "version: '3'\nincludes:\n  gen: ./gen.yml\n", "gen.yml": "version: '3'\ntasks:\n  '{{.X}}': y\n"},
			map[string]bool{},
			[]string{"gen:"},
		},
		{
			"include with wrong version is opaque",
			map[string]string{"Taskfile.yml": "version: '3'\nincludes:\n  old: ./old.yml\n", "old.yml": "version: '2'\ntasks:\n  x: y\n"},
			map[string]bool{},
			[]string{"old:"},
		},
		{
			"nested include",
			map[string]string{"Taskfile.yml": "version: '3'\nincludes:\n  a: ./a/Taskfile.yml\n", "a/Taskfile.yml": "version: '3'\nincludes:\n  b: ./b.yml\ntasks:\n  x: y\n", "a/b.yml": "version: '3'\ntasks:\n  y: z\n"},
			map[string]bool{"a:": false, "a:x": false, "a:b:": false, "a:b:y": false},
			nil,
		},
		{
			"nested remote include",
			map[string]string{"Taskfile.yml": "version: '3'\nincludes:\n  a: ./a.yml\n", "a.yml": "version: '3'\nincludes:\n  r: https://example.com/Taskfile.yml\ntasks:\n  x: y\n"},
			map[string]bool{"a:": false, "a:x": false},
			[]string{"a:r:"},
		},
		{
			"cycle",
			map[string]string{"Taskfile.yml": "version: '3'\nincludes:\n  a: ./a.yml\ntasks:\n  root: x\n", "a.yml": "version: '3'\nincludes:\n  back: ./Taskfile.yml\ntasks:\n  x: y\n"},
			map[string]bool{"root": false, "a:": false, "a:x": false},
			[]string{"a:back:"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for p, content := range tt.files {
				full := filepath.Join(root, filepath.FromSlash(p))
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			p := load(filepath.Join(root, "Taskfile.yml"), 0, map[string]bool{})
			if p == nil {
				t.Fatal("rejected, want parsed")
			}
			slices.Sort(p.opaques)
			if !maps.Equal(p.tasks, tt.tasks) || !slices.Equal(p.opaques, tt.opaques) {
				t.Errorf("got tasks=%v opaques=%v, want tasks=%v opaques=%v", p.tasks, p.opaques, tt.tasks, tt.opaques)
			}
		})
	}
}

func TestRemote(t *testing.T) {
	for _, path := range []string{
		"https://example.com/Taskfile.yml", "http://example.com/t.yml", "git://host/repo.git",
		"ssh://git@host/repo.git", "git+https://host/repo.git", "git@github.com:org/repo.git",
		"https://github.com/org/repo.git//Taskfile.yml?ref=main", "{{.ROOT}}/Taskfile.yml",
	} {
		if !(include{path: path}).remote() {
			t.Errorf("%q should be remote", path)
		}
	}
	for _, path := range []string{"./docs/Taskfile.yml", "docs", "../shared.yml", "C:/proj/Taskfile.yml", "/abs/Taskfile.yml"} {
		if (include{path: path}).remote() {
			t.Errorf("%q should be local", path)
		}
	}
}

func TestCallableAndOpaqueNS(t *testing.T) {
	p := &parsed{
		tasks:   map[string]bool{"build": false, "setup": true, "docs:": false, "docs:build": false},
		opaques: []string{"shared:"},
	}
	if got, want := p.callable(), []string{"build", "docs:build"}; !slices.Equal(got, want) {
		t.Errorf("callable = %v, want %v", got, want)
	}
	if !p.opaqueNS("shared:anything") || p.opaqueNS("docs:x") || p.opaqueNS("shared") {
		t.Error("opaqueNS should match only names under an opaque prefix")
	}
}

func TestDiscover(t *testing.T) {
	root := t.TempDir()
	if discover(root, "") != "" {
		t.Error("empty dir should find nothing")
	}
	// ponytail: Taskfile.yml vs taskfile.yml is one file on Windows, so only
	// names that differ everywhere are exercised here.
	for _, n := range []string{"Taskfile.dist.yaml", "Taskfile.yaml", "Taskfile.yml"} {
		if err := os.WriteFile(filepath.Join(root, n), []byte("version: '3'\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got, want := discover(root, ""), filepath.Join(root, n); got != want {
			t.Errorf("after adding %s: got %q, want %q", n, got, want)
		}
	}
	if got, want := discover(root, "Taskfile.yaml"), filepath.Join(root, "Taskfile.yaml"); got != want {
		t.Errorf("-t file: got %q, want %q", got, want)
	}
	if got, want := discover(root, "."), filepath.Join(root, "Taskfile.yml"); got != want {
		t.Errorf("-t dir: got %q, want %q", got, want)
	}
	if discover(root, "nope.yml") != "" {
		t.Error("-t missing file should find nothing")
	}
}
