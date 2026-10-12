package justfile_test

import (
	"path/filepath"
	"testing"

	"github.com/jnvr1/stalemark/internal/runner/justfile"
	"github.com/jnvr1/stalemark/internal/runner/runnertest"
)

func TestResolveGolden(t *testing.T) {
	runnertest.Golden(t, filepath.Join("..", "..", "..", "testdata", "just"), justfile.New())
}
