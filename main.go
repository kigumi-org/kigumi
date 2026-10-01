package main

import (
	"embed"
	"os"

	"kigumi/internal/cli"
	"kigumi/internal/cli/shared"
	"kigumi/internal/cliutil"
)

// The std stubs ship inside the executable so an installed kigumi works
// outside the repository.
//
//go:embed all:std
var stdFS embed.FS

func main() {
	shared.SetEmbeddedStd(stdFS, "std")
	root := cli.RootCmd()
	os.Exit(cliutil.Execute(root, cli.Debug(root)))
}
