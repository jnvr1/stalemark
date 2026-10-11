package npm

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		cmd  string
		want invocation
	}{
		{"npm run build", invocation{script: "build"}},
		{"npm run-script build", invocation{script: "build"}},
		{"npm run build -- --watch", invocation{script: "build"}},
		{"npm run -- build", invocation{}},
		{"npm run", invocation{}},
		{"npm test -- --coverage", invocation{script: "test"}},
		{"npm t", invocation{}},
		{"npm install", invocation{}},
		{"npm -w app run build", invocation{script: "build", selector: "app"}},
		{"npm run build -w app", invocation{script: "build", selector: "app"}},
		{"npm run build --workspace=packages/app", invocation{script: "build", selector: "packages/app"}},
		{"pnpm --filter app run deploy", invocation{script: "deploy", selector: "app"}},
		{"pnpm -F app deploy", invocation{script: "deploy", selector: "app", bare: true}},
		{"pnpm -r run build", invocation{script: "build"}},
		{"yarn buidl", invocation{script: "buidl", bare: true}},
		{"yarn run buidl", invocation{script: "buidl"}},
		{"yarn run", invocation{}},
		{"yarn install", invocation{}},
		{"yarn workspace app build", invocation{}},
		{"yarn --version", invocation{}},
		{"yarn", invocation{}},
		{"bun run build", invocation{script: "build"}},
		{"bun test", invocation{}},
		{"bun build", invocation{}},
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
