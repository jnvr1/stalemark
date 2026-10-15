// Package config loads .stalemark.yml and the baseline file.
package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/bmatcuk/doublestar/v4"
	"go.yaml.in/yaml/v3"
)

const fileName = ".stalemark.yml"

// Config is the content of .stalemark.yml with defaults applied. Include and
// Exclude are doublestar globs relative to the check root; Disable lists rule
// IDs; Baseline is the baseline file path relative to the root.
type Config struct {
	Include  []string `yaml:"include"`
	Exclude  []string `yaml:"exclude"`
	Disable  []string `yaml:"disable"`
	Baseline string   `yaml:"baseline"`
	Env      struct {
		DocumentedUnused bool `yaml:"documented-unused"`
	} `yaml:"env"`
}

// Load reads root/.stalemark.yml; a missing file yields the defaults. Unknown
// keys and invalid YAML are errors.
//
// ponytail: only the .yml name, only in root; no .yaml variant and no parent-directory discovery (#14 out of scope), add both if users ask.
func Load(root string) (Config, error) {
	var cfg Config
	f, err := os.Open(filepath.Join(root, fileName))
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return Config{}, fmt.Errorf("%s: %w", fileName, err)
	default:
		defer func() { _ = f.Close() }()
		dec := yaml.NewDecoder(f)
		dec.KnownFields(true)
		if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
			return Config{}, fmt.Errorf("%s: %w", fileName, err)
		}
	}
	if len(cfg.Include) == 0 {
		cfg.Include = []string{"README.md", "CONTRIBUTING.md", "docs/**/*.md"}
	}
	if cfg.Baseline == "" {
		cfg.Baseline = ".stalemark-baseline.json"
	}
	return cfg, nil
}

// Docs returns the sorted absolute paths of the files under root matching an
// Include glob and no Exclude glob.
//
// ponytail: fixed skip list (.git, node_modules); make it configurable if a repo needs more.
func (c Config) Docs(root string) ([]string, error) {
	var docs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		inc, err := matchAny(c.Include, rel)
		if err != nil || !inc {
			return err
		}
		exc, err := matchAny(c.Exclude, rel)
		if err != nil || exc {
			return err
		}
		docs = append(docs, p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(docs)
	return docs, nil
}

func matchAny(patterns []string, name string) (bool, error) {
	for _, pat := range patterns {
		ok, err := doublestar.Match(pat, name)
		if err != nil {
			return false, fmt.Errorf("%s: pattern %q: %w", fileName, pat, err)
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}
