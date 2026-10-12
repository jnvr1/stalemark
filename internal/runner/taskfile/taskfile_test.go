package taskfile_test

import (
	"path/filepath"
	"testing"

	"github.com/jnvr1/stalemark/internal/runner/runnertest"
	"github.com/jnvr1/stalemark/internal/runner/taskfile"
)

func TestResolveGolden(t *testing.T) {
	runnertest.Golden(t, filepath.Join("..", "..", "..", "testdata", "task"), taskfile.New())
}
