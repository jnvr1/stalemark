package jsdoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginVersionWalksUp(t *testing.T) {
	top := t.TempDir()
	pkg := filepath.Join(top, "node_modules", "eslint-plugin-jsdoc")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "package.json"), []byte(`{"name": "eslint-plugin-jsdoc", "version": "65.3.6"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(top, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if v, err := pluginVersion(deep); err != nil || v != "65.3.6" {
		t.Errorf("pluginVersion = %q, %v", v, err)
	}
}

func TestPluginVersionMissing(t *testing.T) {
	// ponytail: a node_modules above the temp dir would make this pass wrongly; none exists on CI runners.
	_, err := pluginVersion(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "eslint-plugin-jsdoc not found in node_modules; run npm install") {
		t.Errorf("error = %v", err)
	}
}
