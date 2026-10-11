package makefile

import (
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
		{"make", invocation{}},
		{"make build", invocation{targets: []string{"build"}}},
		{"make build test", invocation{targets: []string{"build", "test"}}},
		{"make -C sub build", invocation{dir: "sub", targets: []string{"build"}}},
		{"make --directory sub build", invocation{dir: "sub", targets: []string{"build"}}},
		{"make --directory=sub build", invocation{dir: "sub", targets: []string{"build"}}},
		{"make -f x.mk build", invocation{file: "x.mk", targets: []string{"build"}}},
		{"make --file x.mk build", invocation{file: "x.mk", targets: []string{"build"}}},
		{"make --file=x.mk build", invocation{file: "x.mk", targets: []string{"build"}}},
		{"make --makefile=x.mk build", invocation{file: "x.mk", targets: []string{"build"}}},
		{"make -C sub -f x.mk build", invocation{dir: "sub", file: "x.mk", targets: []string{"build"}}},
		{"make deploy ENV=prod -j4", invocation{targets: []string{"deploy"}}},
		{"make -j4 -k -s -n -B -e -i -q build", invocation{targets: []string{"build"}}},
		{"make -- build", invocation{targets: []string{"build"}}},
		{"make ENV=prod", invocation{}},
		{"make -C", invocation{}},
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
		targets []string
		dynamic bool
	}{
		{"plain rule", "build:\n\tgo build\n", []string{"build"}, false},
		{"multiple targets", "build test lint: deps\n\tgo $@\n", []string{"build", "lint", "test"}, false},
		{"continuation", "build \\\n test:\n\techo\n", []string{"build", "test"}, false},
		{"phony continuation", ".PHONY: a \\\n b\n", []string{"a", "b"}, false},
		{"phony with space before colon", ".PHONY : lint\n", []string{"lint"}, false},
		{"double colon", "clean::\n\trm -f x\n", []string{"clean"}, false},
		{"inline recipe", "build: deps ; echo hi\n", []string{"build"}, false},
		{"define body skipped", "define rule\nfake:\n\techo\nendef\nreal:\n", []string{"real"}, false},
		{"assignment with colon-equals", "foo := bar\nbuild:\n", []string{"build"}, false},
		{"assignment with colon in value", "foo ?= x:y\nbuild:\n", []string{"build"}, false},
		{"assignment with equals", "LDFLAGS = -X a.b=c\nbuild:\n", []string{"build"}, false},
		{"crlf", "build:\r\n\tgo build\r\ntest:\r\n", []string{"build", "test"}, false},
		{"phony names are targets", ".PHONY: clean lint\nbuild:\n", []string{"build", "clean", "lint"}, false},
		{"dot targets dropped", ".SUFFIXES: .c .o\n.DELETE_ON_ERROR:\nbuild:\n", []string{"build"}, false},
		{"comments and blanks", "# build: nope\n\nbuild: # yes\n", []string{"build"}, false},
		{"conditionals parse both branches", "ifeq ($(OS),Windows_NT)\nwin:\nelse\nunix:\nendif\n", []string{"unix", "win"}, false},
		{"recipe lines skipped", "build:\n\techo deploy: x\n", []string{"build"}, false},
		{"pattern rule", "%.o: %.c\n\t$(CC) $<\nbuild:\n", []string{"build"}, true},
		{"catch-all", "%:\n\techo\n", nil, true},
		{"computed target", "$(BIN): main.go\n", nil, true},
		{"default rule", ".DEFAULT:\n\techo\n", nil, true},
		{"eval", "$(eval $(call rule,build))\n", nil, true},
		{"foreach", "$(foreach t,a b,$(eval $(t):))\n", nil, true},
		{"call outside recipe", "x := $(call fn,1)\n", nil, true},
		{"non-literal include", "include $(wildcard *.mk)\n", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p parsed
			parseSource(&p, tt.src, func(string) { t.Errorf("unexpected include") })
			slices.Sort(p.targets)
			if !slices.Equal(p.targets, tt.targets) || p.dynamic != tt.dynamic {
				t.Errorf("got targets=%v dynamic=%v, want targets=%v dynamic=%v", p.targets, p.dynamic, tt.targets, tt.dynamic)
			}
		})
	}
}

func TestParseFileIncludes(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string // relative to the temp root
		targets []string
		dynamic bool
	}{
		{"include", map[string]string{"Makefile": "include rules.mk\nbuild:\n", "rules.mk": "lint:\n"}, []string{"build", "lint"}, false},
		{"include in subdir resolves from the makefile dir", map[string]string{"Makefile": "include mk/a.mk\n", "mk/a.mk": "include b.mk\n", "mk/b.mk": "deep:\n"}, []string{"deep"}, false},
		{"sinclude", map[string]string{"Makefile": "sinclude rules.mk\n", "rules.mk": "lint:\n"}, []string{"lint"}, false},
		{"-include missing", map[string]string{"Makefile": "-include local.mk\nbuild:\n"}, []string{"build"}, true},
		{"include with trailing comment", map[string]string{"Makefile": "include rules.mk # local\nbuild:\n", "rules.mk": "lint:\n"}, []string{"build", "lint"}, false},
		{"include cycle", map[string]string{"Makefile": "include a.mk\nbuild:\n", "a.mk": "include Makefile\nlint:\n"}, []string{"build", "lint"}, false},
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
			p := parseFile(filepath.Join(root, "Makefile"))
			if !slices.Equal(p.targets, tt.targets) || p.dynamic != tt.dynamic {
				t.Errorf("got targets=%v dynamic=%v, want targets=%v dynamic=%v", p.targets, p.dynamic, tt.targets, tt.dynamic)
			}
		})
	}
}

func TestDiscover(t *testing.T) {
	root := t.TempDir()
	if discover(root, "") != "" {
		t.Error("empty dir should find nothing")
	}
	// ponytail: "makefile" vs "Makefile" is one file on Windows, so only the
	// two names that differ everywhere are exercised here; the goldens cover
	// the case-sensitive listing.
	for _, n := range []string{"Makefile", "GNUmakefile"} {
		if err := os.WriteFile(filepath.Join(root, n), []byte("build:\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got, want := discover(root, ""), filepath.Join(root, n); got != want {
			t.Errorf("after adding %s: got %q, want %q", n, got, want)
		}
	}
	if got, want := discover(root, "Makefile"), filepath.Join(root, "Makefile"); got != want {
		t.Errorf("-f: got %q, want %q", got, want)
	}
	if discover(root, "nope.mk") != "" {
		t.Error("-f missing file should find nothing")
	}
}
