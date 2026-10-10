// Command stalemark checks that documentation still matches the code it describes.
package main

import (
	"os"

	"github.com/jnvr1/stalemark/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
