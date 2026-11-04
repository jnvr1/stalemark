package pathref

import (
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Index is the set of root-relative paths (files and directories, slash form)
// of a repository, used to resolve abbreviated paths by their tail.
type Index struct{ paths []string }

// NewIndex walks root once, skipping generated directories and testdata
// (fixture repos must not satisfy the paths of the checked repo).
func NewIndex(root string) *Index {
	ix := &Index{}
	_ = filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if p == root {
			return nil
		}
		if e.IsDir() && (slices.Contains(generated, e.Name()) || e.Name() == "testdata") {
			return filepath.SkipDir
		}
		if rel, err := filepath.Rel(root, p); err == nil {
			ix.paths = append(ix.paths, filepath.ToSlash(rel))
		}
		return nil
	})
	return ix
}

// HasTail reports whether an indexed path equals tok or ends with "/"+tok.
//
// ponytail: O(n) per candidate and only reached for tokens that are missing
// otherwise. Upgrade = a map keyed by the last segment if a repo is huge.
func (ix *Index) HasTail(tok string) bool {
	tok = path.Clean(tok)
	for _, p := range ix.paths {
		if p == tok || strings.HasSuffix(p, "/"+tok) {
			return true
		}
	}
	return false
}
