package makefile_test

import (
	"path/filepath"
	"testing"

	"github.com/jnvr1/stalemark/internal/runner/makefile"
	"github.com/jnvr1/stalemark/internal/runner/runnertest"
)

func TestResolveGolden(t *testing.T) {
	runnertest.Golden(t, filepath.Join("..", "..", "..", "testdata", "make"), makefile.New())
}
