package pyproject

import (
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestParse(t *testing.T) {
	tests := []struct {
		cmd  string
		want invocation
	}{
		// poetry: global directory options, no run options
		{"poetry run serve", invocation{script: "serve"}},
		{"poetry run serve --port 80", invocation{script: "serve"}},
		{"poetry -C sub run serve", invocation{dir: "sub", script: "serve"}},
		{"poetry --directory sub run serve", invocation{dir: "sub", script: "serve"}},
		{"poetry --directory=sub run serve", invocation{dir: "sub", script: "serve"}},
		{"poetry -P sub run serve", invocation{dir: "sub", script: "serve"}},
		{"poetry --project sub run serve", invocation{dir: "sub", script: "serve"}},
		{"poetry -vvv run serve", invocation{script: "serve"}},
		{"poetry run -- serve", invocation{script: "serve"}},
		{"poetry run --", invocation{}},
		{"poetry run", invocation{}},
		{"poetry install", invocation{}},
		{"poetry shell", invocation{}},
		{"poetry", invocation{}},
		// uv: run options with values, module and script forms, other subcommands
		{"uv run serve", invocation{script: "serve"}},
		{"uv run --with rich serve", invocation{script: "serve"}},
		{"uv run --with=rich serve", invocation{script: "serve"}},
		{"uv run --with-requirements req.txt serve", invocation{script: "serve"}},
		{"uv run --with-editable . serve", invocation{script: "serve"}},
		{"uv run -p 3.12 serve", invocation{script: "serve"}},
		{"uv run --python 3.12 serve", invocation{script: "serve"}},
		{"uv run --env-file .env serve", invocation{script: "serve"}},
		{"uv run --extra dev --group docs --only-group docs serve", invocation{script: "serve"}},
		{"uv run --package app serve", invocation{script: "serve"}},
		{"uv run --exclude-newer 2024-01-01 serve", invocation{script: "serve"}},
		{"uv run --index https://x --default-index https://y serve", invocation{script: "serve"}},
		{"uv run --frozen --no-sync serve", invocation{script: "serve"}},
		{"uv run --unknown=value serve", invocation{script: "serve"}},
		{"uv --directory sub run serve", invocation{dir: "sub", script: "serve"}},
		{"uv run --directory sub serve", invocation{dir: "sub", script: "serve"}},
		{"uv run --project sub serve", invocation{dir: "sub", script: "serve"}},
		{"uv --project=sub run serve", invocation{dir: "sub", script: "serve"}},
		{"uv run -m pkg", invocation{}},
		{"uv run --module pkg", invocation{}},
		{"uv run --script tool.py", invocation{}},
		{"uv run python main.py", invocation{script: "python"}},
		{"uv run main.py", invocation{script: "main.py"}},
		{"uv run -- serve", invocation{script: "serve"}},
		{"uv tool run ruff", invocation{}},
		{"uv sync", invocation{}},
		{"uv run", invocation{}},
		// pdm: project option before or after run, site-packages flag
		{"pdm run lint", invocation{script: "lint"}},
		{"pdm -p sub run lint", invocation{dir: "sub", script: "lint"}},
		{"pdm run -p sub lint", invocation{dir: "sub", script: "lint"}},
		{"pdm run --project sub lint", invocation{dir: "sub", script: "lint"}},
		{"pdm run --project=sub lint", invocation{dir: "sub", script: "lint"}},
		{"pdm run -s lint", invocation{script: "lint"}},
		{"pdm run --site-packages lint", invocation{script: "lint"}},
		{"pdm add requests", invocation{}},
		// hatch: env option, env: prefix, matrix selectors, built-ins
		{"hatch run test", invocation{script: "test"}},
		{"hatch run docs:build", invocation{script: "docs:build"}},
		{"hatch run -e docs build", invocation{env: "docs", script: "build"}},
		{"hatch run --env docs build", invocation{env: "docs", script: "build"}},
		{"hatch run --env=docs build", invocation{env: "docs", script: "build"}},
		{"hatch -e docs run build", invocation{env: "docs", script: "build"}},
		{"hatch run +py=3.12 test", invocation{script: "test"}},
		{"hatch run +py=3.12 +feature=a test", invocation{script: "test"}},
		{"hatch run --force-continue test", invocation{script: "test"}},
		{"hatch run fmt", invocation{script: "fmt"}}, // built-ins are filtered at lookup, not here
		{"hatch run test:all", invocation{script: "test:all"}},
		{"hatch fmt", invocation{}},
		{"hatch test", invocation{}},
		{"hatch env create", invocation{}},
		{"hatch run", invocation{}},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			f := strings.Fields(tt.cmd)
			if got := parse(f[0], f[1:]); got != tt.want {
				t.Errorf("parse(%q) = %+v, want %+v", tt.cmd, got, tt.want)
			}
		})
	}
}

const scriptTables = `
[project.scripts]
serve = "demo:main"

[project.gui-scripts]
gui = "demo:gui"

[tool.poetry.scripts]
poet = "demo:poet"

[tool.pdm.scripts]
_.env_file = ".env"
lint = "ruff check ."
test = { cmd = "pytest" }

[tool.hatch.envs.default.scripts]
cov = "pytest --cov"

[tool.hatch.envs.docs.scripts]
build = "mkdocs build"

[tool.hatch.envs.lint]
detached = true
`

const scriptTablesNoDefault = `
[tool.hatch.envs.docs.scripts]
build = "mkdocs build"

[tool.hatch.envs.test.scripts]
all = "pytest"
`

func TestScripts(t *testing.T) {
	tests := []struct {
		name, src, tool, env string
		want                 string // space-joined sorted names
		ok                   bool
	}{
		{"poetry sees every non-hatch table", scriptTables, "poetry", "", "gui lint poet serve test", true},
		{"uv sees every non-hatch table", scriptTables, "uv", "", "gui lint poet serve test", true},
		{"pdm skips the _ settings key", scriptTables, "pdm", "", "gui lint poet serve test", true},
		{"hatch bare adds the default env", scriptTables, "hatch", "", "cov gui lint poet serve test", true},
		{"hatch env adds env and default", scriptTables, "hatch", "docs", "build cov gui lint poet serve test", true},
		{"hatch env without scripts", scriptTables, "hatch", "lint", "cov gui lint poet serve test", true},
		{"hatch unknown env", scriptTables, "hatch", "nope", "", false},
		{"hatch bare without default is the union", scriptTablesNoDefault, "hatch", "", "all build", true},
		{"empty file", "", "uv", "", "", true},
		{"no envs at all for hatch", "", "hatch", "docs", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var py map[string]any
			if err := toml.Unmarshal([]byte(tt.src), &py); err != nil {
				t.Fatal(err)
			}
			names, ok := scripts(py, tt.tool, tt.env)
			if got := strings.Join(names, " "); got != tt.want || ok != tt.ok {
				t.Errorf("scripts(%s, %q) = %q, %v; want %q, %v", tt.tool, tt.env, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestRequirementName(t *testing.T) {
	tests := map[string]string{
		"Requests>=2":                      "Requests",
		"pytest[cov]":                      "pytest",
		"foo @ git+https://x/y.git":        "foo",
		"bar; python_version < '3.9'":      "bar",
		"baz (>=1.0)":                      "baz",
		"qux ~= 1.2":                       "qux",
		"quux!=1.0":                        "quux",
		"corge==1.0":                       "corge",
		"grault":                           "grault",
		"  garply >= 1 ":                   "garply",
		"python-dotenv[cli] >= 1; os_name": "python-dotenv",
	}
	for req, want := range tests {
		if got := requirementName(req); got != want {
			t.Errorf("requirementName(%q) = %q, want %q", req, got, want)
		}
	}
}

func TestNormalize(t *testing.T) {
	tests := map[string]string{
		"Requests":       "requests",
		"python_dotenv":  "python-dotenv",
		"python.dotenv":  "python-dotenv",
		"Python__Dotenv": "python-dotenv",
		"a-_.b":          "a-b",
		"ruff":           "ruff",
	}
	for name, want := range tests {
		if got := normalize(name); got != want {
			t.Errorf("normalize(%q) = %q, want %q", name, got, want)
		}
	}
}

const dependencyTables = `
[build-system]
requires = ["hatchling>=1.0"]

[project]
dependencies = ["Requests>=2", "python_dotenv[cli]; python_version < '3.9'"]

[project.optional-dependencies]
docs = ["mkdocs @ git+https://x/mkdocs.git"]

[dependency-groups]
dev = ["pytest>=8", {include-group = "lint"}]
lint = ["ruff"]

[tool.poetry.dependencies]
python = "^3.12"
Flask = "^3"

[tool.poetry.dev-dependencies]
mypy = "*"

[tool.poetry.group.test.dependencies]
Pytest_Cov = "^5"

[tool.pdm.dev-dependencies]
docs = ["sphinx>=7"]

[tool.uv]
dev-dependencies = ["black==24.1"]
`

func TestDependsOn(t *testing.T) {
	var py map[string]any
	if err := toml.Unmarshal([]byte(dependencyTables), &py); err != nil {
		t.Fatal(err)
	}
	tests := map[string]bool{
		"requests":      true, // [project.dependencies], case folded
		"python-dotenv": true, // extras and marker stripped, _ normalized
		"python_dotenv": true, // the query is normalized too
		"mkdocs":        true, // optional-dependencies, URL requirement
		"pytest":        true, // dependency-groups
		"ruff":          true, // a group reached through include-group
		"include-group": false,
		"flask":         true, // poetry table keys
		"mypy":          true,
		"pytest-cov":    true, // poetry group, mixed case and underscore
		"sphinx":        true, // pdm dev-dependencies
		"black":         true, // uv dev-dependencies
		"hatchling":     true, // build-system
		"serve":         false,
		"dotenv":        false, // ponytail: bin name differs from the package
	}
	for name, want := range tests {
		if got := dependsOn(py, name); got != want {
			t.Errorf("dependsOn(%q) = %v, want %v", name, got, want)
		}
	}
}
