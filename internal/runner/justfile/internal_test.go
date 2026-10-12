package justfile

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
		{"just", invocation{}},
		{"just build", invocation{args: []string{"build"}}},
		{"just build test", invocation{args: []string{"build", "test"}}},
		{"just build release", invocation{args: []string{"build", "release"}}},
		{"just -f alt.just x", invocation{file: "alt.just", args: []string{"x"}}},
		{"just -f=alt.just x", invocation{file: "alt.just", args: []string{"x"}}},
		{"just --justfile alt.just x", invocation{file: "alt.just", args: []string{"x"}}},
		{"just --justfile=alt.just x", invocation{file: "alt.just", args: []string{"x"}}},
		{"just -d sub build", invocation{dir: "sub", args: []string{"build"}}},
		{"just --working-directory sub build", invocation{dir: "sub", args: []string{"build"}}},
		{"just --working-directory=sub build", invocation{dir: "sub", args: []string{"build"}}},
		{"just -d sub -f alt.just x", invocation{dir: "sub", file: "alt.just", args: []string{"x"}}},
		{"just --set a 1 build", invocation{args: []string{"build"}}},
		{"just --shell bash --shell-arg -c build", invocation{args: []string{"build"}}},
		{"just --color always --dotenv-path .env build", invocation{args: []string{"build"}}},
		{"just -E .env build", invocation{args: []string{"build"}}},
		{"just --list-heading x --list-prefix y --timestamp-format z build", invocation{args: []string{"build"}}},
		{"just -q -v -u -n --no-deps --no-dotenv --no-aliases --one --yes --timestamp build", invocation{args: []string{"build"}}},
		{"just --dry-run build", invocation{args: []string{"build"}}},
		{"just FOO=1 build", invocation{args: []string{"build"}}},
		{"just build FOO=1", invocation{args: []string{"build", "FOO=1"}}},
		{"just FOO=1", invocation{}},
		{"just -- build args", invocation{args: []string{"build", "args"}}},
		{"just build -- args", invocation{args: []string{"build", "--", "args"}}},
		{"just -l", invocation{silent: true}},
		{"just --list", invocation{silent: true}},
		{"just --choose", invocation{silent: true}},
		{"just --dump", invocation{silent: true}},
		{"just -e", invocation{silent: true}},
		{"just --edit", invocation{silent: true}},
		{"just --evaluate", invocation{silent: true}},
		{"just --fmt --unstable", invocation{silent: true}},
		{"just --init", invocation{silent: true}},
		{"just --summary", invocation{silent: true}},
		{"just --show build", invocation{silent: true}},
		{"just -s build", invocation{silent: true}},
		{"just --variables", invocation{silent: true}},
		{"just --changelog", invocation{silent: true}},
		{"just --completions bash", invocation{silent: true}},
		{"just -h", invocation{silent: true}},
		{"just --help", invocation{silent: true}},
		{"just -V", invocation{silent: true}},
		{"just --version", invocation{silent: true}},
		{"just --groups", invocation{silent: true}},
		{"just -g build", invocation{silent: true, args: []string{"build"}}},
		{"just --global-justfile build", invocation{silent: true, args: []string{"build"}}},
		{"just sub/build", invocation{silent: true, args: []string{"sub/build"}}},
		{"just api::deploy", invocation{args: []string{"api::deploy"}}},
		{"just -f", invocation{}},
		{"just -d", invocation{}},
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
		name    string
		src     string
		recipes map[string]recipe
		aliases map[string]string
		imports []imp
		opaque  bool
	}{
		{"plain", "build:\n    echo build\n", map[string]recipe{"build": {}}, nil, nil, false},
		{"quiet prefix", "@build:\n    echo build\n", map[string]recipe{"build": {}}, nil, nil, false},
		{"dependencies", "test: build lint\n    echo test\n", map[string]recipe{"test": {}}, nil, nil, false},
		{"one param", "build target:\n", map[string]recipe{"build": {params: 1}}, nil, nil, false},
		{"param with bare default", "build target=debug:\n", map[string]recipe{"build": {params: 1}}, nil, nil, false},
		{"param with quoted default", "build target='debug':\n", map[string]recipe{"build": {params: 1}}, nil, nil, false},
		{"param with spaces in default", `build target="a b" mode="x y": deps` + "\n", map[string]recipe{"build": {params: 2}}, nil, nil, false},
		{"params and dependencies", "build a b='c': lint\n", map[string]recipe{"build": {params: 2}}, nil, nil, false},
		{"variadic plus", "release +flags:\n", map[string]recipe{"release": {params: 1, variadic: true}}, nil, nil, false},
		{"variadic star", "release *flags:\n", map[string]recipe{"release": {params: 1, variadic: true}}, nil, nil, false},
		{"variadic after params", "run target +args:\n", map[string]recipe{"run": {params: 2, variadic: true}}, nil, nil, false},
		{"env param", "build $target:\n", map[string]recipe{"build": {params: 1}}, nil, nil, false},
		{"env variadic", "build $+args:\n", map[string]recipe{"build": {params: 1, variadic: true}}, nil, nil, false},
		{"dash in name", "build-all: build\n", map[string]recipe{"build-all": {}}, nil, nil, false},
		{"underscore is private", "_setup:\n    echo x\n", map[string]recipe{"_setup": {private: true}}, nil, nil, false},
		{"private attribute", "[private]\nclean:\n    rm x\nbuild:\n", map[string]recipe{"clean": {private: true}, "build": {}}, nil, nil, false},
		{"private among attributes", "[no-cd, private]\nclean:\n", map[string]recipe{"clean": {private: true}}, nil, nil, false},
		{"other attribute", "[no-cd]\nclean:\n", map[string]recipe{"clean": {}}, nil, nil, false},
		{"attribute then comment", "[private]\n# doc\nclean:\n", map[string]recipe{"clean": {private: true}}, nil, nil, false},
		{"assignment is not a recipe", "x := 1\nbuild:\n", map[string]recipe{"build": {}}, nil, nil, false},
		{"set is not a recipe", "set shell := [\"bash\", \"-c\"]\nset positional-arguments\nbuild:\n", map[string]recipe{"build": {}}, nil, nil, false},
		{"export is not a recipe", "export FOO := \"x\"\nunexport BAR\nbuild:\n", map[string]recipe{"build": {}}, nil, nil, false},
		{"body lines are skipped", "build:\n    deploy:\n\tother:\n", map[string]recipe{"build": {}}, nil, nil, false},
		{"comments are skipped", "# build:\nbuild:\n", map[string]recipe{"build": {}}, nil, nil, false},
		{"crlf", "build:\r\n    echo x\r\ntest:\r\n", map[string]recipe{"build": {}, "test": {}}, nil, nil, false},
		{"alias", "alias b := build\nbuild:\n", map[string]recipe{"build": {}}, map[string]string{"b": "build"}, nil, false},
		{"mod is opaque", "mod api\nbuild:\n", map[string]recipe{"build": {}}, nil, nil, true},
		{"optional mod is opaque", "mod? api\n", map[string]recipe{}, nil, nil, true},
		{"mod with path is opaque", "mod api 'src/api.just'\n", map[string]recipe{}, nil, nil, true},
		{"import single quotes", "import 'shared.just'\n", map[string]recipe{}, nil, []imp{{path: "shared.just"}}, false},
		{"import double quotes", "import \"lib/shared.just\"\n", map[string]recipe{}, nil, []imp{{path: "lib/shared.just"}}, false},
		{"optional import", "import? 'local.just'\n", map[string]recipe{}, nil, []imp{{path: "local.just", optional: true}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := parseSource([]byte(tt.src))
			if tt.aliases == nil {
				tt.aliases = map[string]string{}
			}
			if !maps.Equal(p.recipes, tt.recipes) || !maps.Equal(p.aliases, tt.aliases) || !slices.Equal(p.imports, tt.imports) || p.opaque != tt.opaque {
				t.Errorf("got recipes=%v aliases=%v imports=%v opaque=%v, want recipes=%v aliases=%v imports=%v opaque=%v",
					p.recipes, p.aliases, p.imports, p.opaque, tt.recipes, tt.aliases, tt.imports, tt.opaque)
			}
		})
	}
}

func TestImportOpaque(t *testing.T) {
	for _, path := range []string{"/abs/shared.just", "C:/proj/shared.just", "~/shared.just", "{{root}}/shared.just"} {
		if !(imp{path: path}).opaque() {
			t.Errorf("%q should be opaque", path)
		}
	}
	for _, path := range []string{"shared.just", "./shared.just", "../lib/shared.just"} {
		if (imp{path: path}).opaque() {
			t.Errorf("%q should be local", path)
		}
	}
}

func TestMissing(t *testing.T) {
	p := &parsed{recipes: map[string]recipe{
		"build":   {params: 1},
		"release": {params: 1, variadic: true},
		"run":     {params: 2, variadic: true},
		"test":    {},
		"b":       {params: 1},
	}}
	tests := []struct {
		args []string
		want string
	}{
		{nil, ""},
		{[]string{"test"}, ""},
		{[]string{"build", "release"}, ""},
		{[]string{"build", "test"}, ""},
		{[]string{"build"}, ""},
		{[]string{"build", "x", "test"}, ""},
		{[]string{"build", "x", "deploy"}, "deploy"},
		{[]string{"release", "a", "b", "c"}, ""},
		{[]string{"release", "a", "deploy"}, ""},
		{[]string{"run", "deploy"}, ""},
		{[]string{"test", "build"}, ""},
		{[]string{"test", "deploy"}, "deploy"},
		{[]string{"test", "build", "x", "deploy"}, "deploy"},
		{[]string{"deploy", "release"}, "deploy"},
		{[]string{"b", "release"}, ""},
		{[]string{"api::deploy", "deploy"}, ""},
		{[]string{"test", "api::deploy", "deploy"}, ""},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			if got := p.missing(tt.args); got != tt.want {
				t.Errorf("missing(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestCallable(t *testing.T) {
	p := &parsed{recipes: map[string]recipe{"build": {}, "_setup": {private: true}, "clean": {private: true}, "b": {}}}
	if got, want := p.callable(), []string{"b", "build"}; !slices.Equal(got, want) {
		t.Errorf("callable = %v, want %v", got, want)
	}
}

func TestParseImports(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string // relative to the temp root; justfile is parsed
		recipes map[string]recipe // nil means opaque
	}{
		{
			"local import is merged unprefixed",
			map[string]string{"justfile": "import 'shared.just'\nbuild:\n", "shared.just": "publish target:\n"},
			map[string]recipe{"build": {}, "publish": {params: 1}},
		},
		{
			"alias resolves to its target arity",
			map[string]string{"justfile": "alias b := build\nalias p := publish\nimport 'shared.just'\nbuild x y:\n", "shared.just": "publish target:\n"},
			map[string]recipe{"build": {params: 2}, "b": {params: 2}, "publish": {params: 1}, "p": {params: 1}},
		},
		{
			"alias to a missing target is still callable",
			map[string]string{"justfile": "alias b := gone\n"},
			map[string]recipe{"b": {}},
		},
		{
			"alias to a private target is private",
			map[string]string{"justfile": "alias s := _setup\n_setup:\n"},
			map[string]recipe{"_setup": {private: true}, "s": {private: true}},
		},
		{
			"nested import",
			map[string]string{"justfile": "import 'a/a.just'\n", "a/a.just": "import 'b.just'\nx:\n", "a/b.just": "y:\n"},
			map[string]recipe{"x": {}, "y": {}},
		},
		{
			"missing import is opaque",
			map[string]string{"justfile": "import 'gone.just'\nbuild:\n"},
			nil,
		},
		{
			"optional missing import is ignored",
			map[string]string{"justfile": "import? 'gone.just'\nbuild:\n"},
			map[string]recipe{"build": {}},
		},
		{
			"absolute import is opaque",
			map[string]string{"justfile": "import '/etc/shared.just'\nbuild:\n"},
			nil,
		},
		{
			"import with mod is opaque",
			map[string]string{"justfile": "import 'shared.just'\nbuild:\n", "shared.just": "mod api\n"},
			nil,
		},
		{
			"diamond import",
			map[string]string{"justfile": "import 'a.just'\nimport 'b.just'\n", "a.just": "import 'shared.just'\nx:\n", "b.just": "import 'shared.just'\ny:\n", "shared.just": "z:\n"},
			map[string]recipe{"x": {}, "y": {}, "z": {}},
		},
		{
			"cycle",
			map[string]string{"justfile": "import 'a.just'\nroot:\n", "a.just": "import 'justfile'\nx:\n"},
			map[string]recipe{"root": {}, "x": {}},
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
			p := parse(filepath.Join(root, "justfile"))
			if p == nil {
				t.Fatal("unreadable, want parsed")
			}
			if tt.recipes == nil {
				if !p.opaque {
					t.Errorf("got recipes=%v, want opaque", p.recipes)
				}
				return
			}
			if p.opaque || !maps.Equal(p.recipes, tt.recipes) {
				t.Errorf("got recipes=%v opaque=%v, want recipes=%v", p.recipes, p.opaque, tt.recipes)
			}
		})
	}
}

func TestDiscover(t *testing.T) {
	root := t.TempDir()
	if discover(root, "") != "" {
		t.Error("empty dir should find nothing")
	}
	// ponytail: justfile vs Justfile is one file on Windows, so only names
	// that differ everywhere are exercised here.
	for _, n := range []string{".justfile", "justfile"} {
		if err := os.WriteFile(filepath.Join(root, n), []byte("build:\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got, want := discover(root, ""), filepath.Join(root, n); got != want {
			t.Errorf("after adding %s: got %q, want %q", n, got, want)
		}
	}
	if got, want := discover(root, ".justfile"), filepath.Join(root, ".justfile"); got != want {
		t.Errorf("-f file: got %q, want %q", got, want)
	}
	if discover(root, "nope.just") != "" {
		t.Error("-f missing file should find nothing")
	}
	if discover(root, ".") != "" {
		t.Error("-f directory should find nothing")
	}
}
