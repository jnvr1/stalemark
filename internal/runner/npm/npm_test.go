package npm_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jnvr1/stalemark/internal/runner"
	"github.com/jnvr1/stalemark/internal/runner/npm"
	"github.com/jnvr1/stalemark/internal/runner/runnertest"
	"github.com/jnvr1/stalemark/internal/shellx"
)

func TestResolveGolden(t *testing.T) {
	runnertest.Golden(t, filepath.Join("..", "..", "..", "testdata", "npm"), npm.New())
}

func TestResolveFallThrough(t *testing.T) {
	// the extra scripts share a prefix with the bins below: typo evidence, so
	// only the declared dependency keeps those silent.
	const scripts = `"scripts": {"build": "tsc", "test": "vitest", "tsc:w": "", "biome:ci": "", "playwright:ui": "", "eslint:fix": "", "vite:dev": ""}`
	pkg := func(deps string) map[string]string {
		return map[string]string{"package.json": `{` + deps + scripts + `}`}
	}
	tests := []struct {
		name  string
		files map[string]string // slash path -> content, relative to the root
		cmd   string
		dir   string // working directory, slash path relative to the root
		want  bool   // a finding is expected
	}{
		{"bun tsc with typescript", pkg(`"devDependencies": {"typescript": "5"}, `), "bun run tsc", "", false},
		{"yarn tsc with typescript", pkg(`"devDependencies": {"typescript": "5"}, `), "yarn tsc", "", false},
		{"pnpm biome with scoped dep", pkg(`"devDependencies": {"@biomejs/biome": "1"}, `), "pnpm biome", "", false},
		{"yarn playwright with scoped dep", pkg(`"devDependencies": {"@playwright/test": "1"}, `), "yarn run playwright", "", false},
		{"bun eslint in optionalDependencies", pkg(`"optionalDependencies": {"eslint": "9"}, `), "bun run eslint", "", false},
		{"yarn eslint in peerDependencies", pkg(`"peerDependencies": {"eslint": "9"}, `), "yarn eslint", "", false},
		{"bun vite via -cli dep", pkg(`"devDependencies": {"vite-cli": "1"}, `), "bun run vite", "", false},
		{"hoisted root dep", map[string]string{
			"package.json":              `{"devDependencies": {"eslint": "9"}}`,
			"packages/app/package.json": `{` + scripts + `}`,
		}, "bun run eslint", "packages/app", false},
		{"yarn typo with no dep", pkg(``), "yarn buidl", "", true},
		{"bun typo with no dep", pkg(``), "bun run buidl", "", true},
		{"yarn typo that is also a dep", pkg(`"devDependencies": {"buidl": "1"}, `), "yarn buidl", "", false},
		{"yarn unknown without suggestion", pkg(``), "yarn deploy", "", false},
		{"pnpm shorthand unknown without suggestion", pkg(``), "pnpm deploy", "", false},
		{"pnpm run is strict", pkg(``), "pnpm run deploy", "", true},
		{"pnpm run is strict even with the dep", pkg(`"devDependencies": {"typescript": "5"}, `), "pnpm run tsc", "", true},
		{"npm run is strict", pkg(``), "npm run deploy", "", true},
		{"npm run is strict even with the dep", pkg(`"devDependencies": {"typescript": "5"}, `), "npm run tsc", "", true},
		{"npm run-script is strict", pkg(``), "npm run-script deploy", "", true},
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
			f := strings.Fields(tt.cmd)
			cmd := shellx.Command{Name: f[0], Args: f[1:], Dir: tt.dir, FileLine: 1}
			got := npm.New().Resolve(runner.Context{Root: root}, "README.md", cmd)
			if (len(got) > 0) != tt.want {
				t.Errorf("%q: got %+v, want finding=%v", tt.cmd, got, tt.want)
			}
		})
	}
}
