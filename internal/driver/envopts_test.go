package driver_test

import (
	"io"
	"os"
	"strings"

	"kigumi/internal/driver"
)

// testBuild links with KIGUMI_CFLAGS, which the driver itself no longer
// reads: sanitizers and C helpers reach the tests this way.
func testBuild(m *driver.Module, exe string, stderr io.Writer) (bool, error) {
	return driver.BuildWith(m, exe, stderr, driver.BuildOptions{CFlags: strings.Fields(os.Getenv("KIGUMI_CFLAGS"))})
}
