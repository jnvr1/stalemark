package gocargo

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestParseGo(t *testing.T) {
	tests := []struct {
		cmd  string
		want goCall
	}{
		{"go run ./cmd/app", goCall{sub: "run", targets: []string{"./cmd/app"}}},
		{"go run ./cmd/app --port 8080", goCall{sub: "run", targets: []string{"./cmd/app"}}},
		{"go run ./cmd/app -v ./other", goCall{sub: "run", targets: []string{"./cmd/app"}}},
		{"go run main.go", goCall{sub: "run", targets: []string{"main.go"}}},
		{"go run main.go util.go arg", goCall{sub: "run", targets: []string{"main.go", "util.go"}}},
		{"go run main.go ./cmd", goCall{sub: "run", targets: []string{"main.go"}}},
		{"go run -race ./cmd/app", goCall{sub: "run", targets: []string{"./cmd/app"}}},
		{"go run -tags dev ./cmd/app", goCall{sub: "run", targets: []string{"./cmd/app"}}},
		{"go run -tags=dev ./cmd/app", goCall{sub: "run", targets: []string{"./cmd/app"}}},
		{"go run -ldflags -s ./cmd/app", goCall{sub: "run", targets: []string{"./cmd/app"}}},
		{"go run -exec sudo ./cmd/app", goCall{sub: "run", targets: []string{"./cmd/app"}}},
		{"go run -C sub ./cmd/app", goCall{sub: "run", dir: "sub", targets: []string{"./cmd/app"}}},
		{"go run -C=sub ./cmd/app", goCall{sub: "run", dir: "sub", targets: []string{"./cmd/app"}}},
		{"go -C sub run ./cmd/app", goCall{sub: "run", dir: "sub", targets: []string{"./cmd/app"}}},
		{"go run -- ./cmd/app", goCall{sub: "run", targets: []string{"./cmd/app"}}},
		{"go run example.com/tool@latest", goCall{sub: "run", targets: []string{"example.com/tool@latest"}}},
		{"go run", goCall{sub: "run"}},
		{"go build ./...", goCall{sub: "build", targets: []string{"./..."}}},
		{"go build -o ./bin/app ./cmd/app", goCall{sub: "build", targets: []string{"./cmd/app"}}},
		{"go build -o=./bin/app ./cmd/app ./cmd/other", goCall{sub: "build", targets: []string{"./cmd/app", "./cmd/other"}}},
		{"go build -v ./cmd/app -trimpath ./cmd/other", goCall{sub: "build", targets: []string{"./cmd/app", "./cmd/other"}}},
		{"go build --tags dev ./x", goCall{sub: "build", targets: []string{"./x"}}},
		{"go test ./...", goCall{sub: "test", targets: []string{"./..."}}},
		{"go test -race -run TestX -count=1 ./internal/...", goCall{sub: "test", targets: []string{"./internal/..."}}},
		{"go test -run TestX -count 1 ./a ./b", goCall{sub: "test", targets: []string{"./a", "./b"}}},
		{"go test -coverprofile cover.out ./...", goCall{sub: "test", targets: []string{"./..."}}},
		{"go test -test.run TestX ./a", goCall{sub: "test", targets: []string{"./a"}}},
		{"go test -vet off ./a", goCall{sub: "test", targets: []string{"./a"}}},
		{"go test -c -o t.test ./a", goCall{sub: "test", targets: []string{"./a"}}},
		{"go test ./a -args -run ./b", goCall{sub: "test", targets: []string{"./a"}}},
		{"go test -timeout 30s", goCall{sub: "test"}},
		{"go mod tidy", goCall{}},
		{"go get ./x", goCall{}},
		{"go install ./cmd/app", goCall{}},
		{"go vet ./nope", goCall{}},
		{"go generate ./...", goCall{}},
		{"go version", goCall{}},
		{"go -h", goCall{}},
		{"go", goCall{}},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			f := strings.Fields(tt.cmd)
			if got := parseGo(f[1:]); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseGo(%q) = %+v, want %+v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestLocal(t *testing.T) {
	tests := map[string]bool{
		".":                              true,
		"..":                             true,
		"./cmd/app":                      true,
		"../x":                           true,
		"./...":                          true,
		"main.go":                        true,
		"cmd/app/main.go":                true,
		"example.com/m/pkg":              false,
		"std":                            false,
		"cmd":                            false,
		"all":                            false,
		"cmd/app":                        false,
		"example.com/tool@latest":        false,
		"./tool.go@v1":                   false,
		"...":                            false,
		filepath.Join(os.TempDir(), "x"): false,
	}
	for p, want := range tests {
		if got := local(p); got != want {
			t.Errorf("local(%q) = %v, want %v", p, got, want)
		}
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIsMain(t *testing.T) {
	tests := []struct {
		name          string
		files         map[string]string
		isMain, anyGo bool
	}{
		{"main", map[string]string{"a.go": "package main\n"}, true, true},
		{"lib", map[string]string{"a.go": "package lib\n"}, false, true},
		{"comment then main", map[string]string{"a.go": "// Command x.\npackage main\n"}, true, true},
		{"main among files", map[string]string{"a.go": "package lib\n", "b.go": "package main\n"}, true, true},
		{"main only in test", map[string]string{"a.go": "package lib\n", "a_test.go": "package main\n"}, false, true},
		{"only tests", map[string]string{"a_test.go": "package lib\n"}, false, false},
		{"no go files", map[string]string{"README.md": "x"}, false, false},
		{"mainly is not main", map[string]string{"a.go": "package mainly\n"}, false, true},
		{"empty dir", nil, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for n, c := range tt.files {
				write(t, filepath.Join(dir, n), c)
			}
			if m, a := isMain(dir); m != tt.isMain || a != tt.anyGo {
				t.Errorf("isMain = %v, %v; want %v, %v", m, a, tt.isMain, tt.anyGo)
			}
		})
	}
}

func TestFindGo(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	sub := filepath.Join(root, "a", "b")
	write(t, filepath.Join(sub, "x"), "")
	write(t, filepath.Join(base, "go.mod"), "module above\n")

	if findGo(sub, root) {
		t.Error("findGo looked above root")
	}
	write(t, filepath.Join(root, "go.work"), "go 1.22\n")
	if !findGo(sub, root) {
		t.Error("findGo missed go.work at root")
	}
	if findGo(filepath.Join(sub, "missing"), root) {
		t.Error("findGo accepted a start dir that does not exist")
	}
	if findGo(base, root) {
		t.Error("findGo accepted a start dir above root")
	}
	if findGo(filepath.Join(root, "..", "elsewhere"), root) {
		t.Error("findGo accepted a start dir outside root")
	}
}

func TestParseCargo(t *testing.T) {
	tests := []struct {
		cmd  string
		want cargoCall
	}{
		{"cargo run --bin app", cargoCall{bins: []string{"app"}}},
		{"cargo run --bin=app", cargoCall{bins: []string{"app"}}},
		{"cargo run --bin a --bin b", cargoCall{bins: []string{"a", "b"}}},
		{"cargo build --example demo", cargoCall{examples: []string{"demo"}}},
		{"cargo build --example=demo", cargoCall{examples: []string{"demo"}}},
		{"cargo run -p cli", cargoCall{pkgs: []string{"cli"}}},
		{"cargo run --package cli", cargoCall{pkgs: []string{"cli"}}},
		{"cargo run --package=cli", cargoCall{pkgs: []string{"cli"}}},
		{"cargo test -p a -p b", cargoCall{pkgs: []string{"a", "b"}}},
		{"cargo test -p core@0.1.0", cargoCall{pkgs: []string{"core"}}},
		{"cargo run -p cli --bin extra", cargoCall{pkgs: []string{"cli"}, bins: []string{"extra"}}},
		{"cargo run -p ../x", cargoCall{}},
		{"cargo run -p path+file:///x", cargoCall{}},
		{"cargo run -p https://x/y", cargoCall{}},
		{`cargo run -p crates\x`, cargoCall{}},
		{"cargo run -p serde:1.0.0", cargoCall{}},
		{"cargo run -p foo*", cargoCall{}},
		{"cargo run --bin to*", cargoCall{}},
		{"cargo run --example d?mo", cargoCall{}},
		{"cargo run --manifest-path sub/Cargo.toml --bin x", cargoCall{manifest: "sub/Cargo.toml", bins: []string{"x"}}},
		{"cargo run --manifest-path=sub/Cargo.toml -p x", cargoCall{manifest: "sub/Cargo.toml", pkgs: []string{"x"}}},
		{"cargo run --release --bin x", cargoCall{bins: []string{"x"}}},
		{"cargo build --features full --bin x", cargoCall{bins: []string{"x"}}},
		{"cargo build -F full -j 4 --target-dir t --bin x", cargoCall{bins: []string{"x"}}},
		{"cargo build --profile dev --target wasm32 --bin x", cargoCall{bins: []string{"x"}}},
		{"cargo test --test it --bench b -p x", cargoCall{pkgs: []string{"x"}}},
		{"cargo build --exclude y --workspace --bin x", cargoCall{bins: []string{"x"}}},
		{"cargo run --bin x -- --bin y -p z", cargoCall{bins: []string{"x"}}},
		{"cargo run -- --bin y", cargoCall{}},
		{"cargo +nightly run --bin x", cargoCall{bins: []string{"x"}}},
		{"cargo -Z unstable-options run --bin x", cargoCall{bins: []string{"x"}}},
		{"cargo --config k=v run --bin x", cargoCall{bins: []string{"x"}}},
		{"cargo --locked run --bin x", cargoCall{bins: []string{"x"}}},
		{"cargo check --bin x", cargoCall{bins: []string{"x"}}},
		{"cargo clippy -p x", cargoCall{pkgs: []string{"x"}}},
		{"cargo doc -p x", cargoCall{pkgs: []string{"x"}}},
		{"cargo bench --bin x", cargoCall{bins: []string{"x"}}},
		{"cargo run", cargoCall{}},
		{"cargo test", cargoCall{}},
		{"cargo install --bin x", cargoCall{}},
		{"cargo install -p x", cargoCall{}},
		{"cargo xtask ci", cargoCall{}},
		{"cargo fmt -p x", cargoCall{}},
		{"cargo", cargoCall{}},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			f := strings.Fields(tt.cmd)
			if got := parseCargo(f[1:]); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseCargo(%q) = %+v, want %+v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestPackages(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string // slash paths under the temp dir; Cargo.toml is the manifest
		want  []string
	}{
		{"single package", map[string]string{"Cargo.toml": "[package]\nname = \"app\"\n"}, []string{"app"}},
		{"virtual workspace globs", map[string]string{
			"Cargo.toml":             "[workspace]\nmembers = [\"crates/*\", \"tools/cli\"]\nexclude = [\"crates/skip\"]\n",
			"crates/a/Cargo.toml":    "[package]\nname = \"a\"\n",
			"crates/skip/Cargo.toml": "[package]\nname = \"skip\"\n",
			"crates/notes.txt":       "not a crate",
			"tools/cli/Cargo.toml":   "[package]\nname = \"cli\"\n",
		}, []string{"a", "cli"}},
		{"root package plus members", map[string]string{
			"Cargo.toml":     "[package]\nname = \"root\"\n[workspace]\nmembers = [\"sub\", \".\"]\n",
			"sub/Cargo.toml": "[package]\nname = \"sub\"\n",
		}, []string{"root", "sub"}},
		{"member without a name is skipped", map[string]string{
			"Cargo.toml":   "[workspace]\nmembers = [\"x\", \"y\"]\n",
			"x/Cargo.toml": "[package]\nversion = \"1\"\n",
			"y/Cargo.toml": "not toml [",
		}, nil},
		{"virtual workspace with missing members", map[string]string{"Cargo.toml": "[workspace]\nmembers = [\"gone\"]\n"}, nil},
		{"unparsable root", map[string]string{"Cargo.toml": "[package\n"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for n, c := range tt.files {
				write(t, filepath.Join(dir, filepath.FromSlash(n)), c)
			}
			var got []string
			for _, p := range packages(filepath.Join(dir, "Cargo.toml")) {
				got = append(got, p.name)
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.want) {
				t.Errorf("packages = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBinsAndExamples(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Cargo.toml"), `[package]
name = "app"

[[bin]]
name = "explicit"
path = "src/explicit.rs"

[[example]]
name = "custom"
path = "examples/c.rs"
`)
	write(t, filepath.Join(dir, "src", "main.rs"), "")
	write(t, filepath.Join(dir, "src", "bin", "tool.rs"), "")
	write(t, filepath.Join(dir, "src", "bin", "nested", "main.rs"), "")
	write(t, filepath.Join(dir, "src", "bin", "empty", "other.rs"), "")
	write(t, filepath.Join(dir, "src", "bin", "notes.txt"), "")
	write(t, filepath.Join(dir, "examples", "demo.rs"), "")
	write(t, filepath.Join(dir, "examples", "multi", "main.rs"), "")
	write(t, filepath.Join(dir, "examples", "readme.md"), "")

	p := pkg{name: "app", dir: dir}
	if got, want := bins(p), []string{"app", "explicit", "nested", "tool"}; !slices.Equal(got, want) {
		t.Errorf("bins = %v, want %v", got, want)
	}
	if got, want := examples(p), []string{"custom", "demo", "multi"}; !slices.Equal(got, want) {
		t.Errorf("examples = %v, want %v", got, want)
	}

	lib := t.TempDir()
	write(t, filepath.Join(lib, "Cargo.toml"), "[package]\nname = \"lib\"\n")
	write(t, filepath.Join(lib, "src", "lib.rs"), "")
	if got := bins(pkg{name: "lib", dir: lib}); len(got) != 0 {
		t.Errorf("a library has no bins, got %v", got)
	}
}

func TestFindCargo(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	member := filepath.Join(root, "crates", "m")
	write(t, filepath.Join(base, "Cargo.toml"), "[workspace]\n")
	write(t, filepath.Join(root, "Cargo.toml"), "[workspace]\nmembers = [\"crates/m\"]\n")
	write(t, filepath.Join(member, "Cargo.toml"), "[package]\nname = \"m\"\n")
	write(t, filepath.Join(root, "plain", "x"), "")

	if got, want := findCargo(member, root), filepath.Join(root, "Cargo.toml"); got != want {
		t.Errorf("from a member the workspace root wins: got %q, want %q", got, want)
	}
	if got, want := findCargo(filepath.Join(root, "plain"), root), filepath.Join(root, "Cargo.toml"); got != want {
		t.Errorf("walks up to the root manifest: got %q, want %q", got, want)
	}
	// without a workspace above, the nearest manifest wins
	write(t, filepath.Join(root, "Cargo.toml"), "[package]\nname = \"r\"\n")
	if got, want := findCargo(member, root), filepath.Join(member, "Cargo.toml"); got != want {
		t.Errorf("nearest manifest: got %q, want %q", got, want)
	}
	if got := findCargo(base, root); got != "" {
		t.Errorf("start above root: got %q", got)
	}
}

func TestDepNames(t *testing.T) {
	src := `[package]
name = "p"

[dependencies]
anyhow = "1"
my-log = { package = "log", version = "0.4" }
local = { path = "../local" }

[dev-dependencies]
pretty = "1"

[build-dependencies]
cc = "1"

[target.'cfg(unix)'.dependencies]
libc = "0.2"

[target.'cfg(windows)'.dev-dependencies]
winapi = { package = "winapi-real", version = "1" }

[target.'cfg(windows)'.build-dependencies]
embed = "1"

[workspace.dependencies]
serde = "1"
`
	var m map[string]any
	if err := toml.Unmarshal([]byte(src), &m); err != nil {
		t.Fatal(err)
	}
	got := depNames(m)
	slices.Sort(got)
	want := []string{"anyhow", "cc", "embed", "libc", "local", "log", "my-log", "pretty", "serde", "winapi", "winapi-real"}
	if !slices.Equal(got, want) {
		t.Errorf("depNames = %v, want %v", got, want)
	}
	if got := depNames(nil); len(got) != 0 {
		t.Errorf("depNames(nil) = %v", got)
	}
}

func TestKnownPackages(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "Cargo.toml")
	write(t, manifest, "[workspace]\nmembers = [\"m\"]\n[workspace.dependencies]\nserde = \"1\"\n")
	write(t, filepath.Join(dir, "m", "Cargo.toml"), "[package]\nname = \"m\"\n[dependencies]\nanyhow = \"1\"\n")

	got := knownPackages(manifest, packages(manifest))
	slices.Sort(got)
	if want := []string{"anyhow", "m", "serde"}; !slices.Equal(got, want) {
		t.Errorf("without a lock file: %v, want %v", got, want)
	}

	write(t, filepath.Join(dir, "Cargo.lock"), "version = 3\n\n[[package]]\nname = \"tokio\"\nversion = \"1.0.0\"\n\n[[package]]\nname = \"m\"\nversion = \"0.1.0\"\n")
	got = knownPackages(manifest, packages(manifest))
	slices.Sort(got)
	if want := []string{"anyhow", "m", "serde", "tokio"}; !slices.Equal(got, want) {
		t.Errorf("with a lock file: %v, want %v", got, want)
	}

	write(t, filepath.Join(dir, "Cargo.lock"), "not toml [")
	if got := knownPackages(manifest, packages(manifest)); !slices.Contains(got, "serde") {
		t.Errorf("a broken lock file must not drop the rest: %v", got)
	}
}
