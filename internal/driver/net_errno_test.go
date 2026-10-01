package driver_test

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kigumi/internal/driver"
)

// freeClosedPort hands back a port that was free a moment ago: another
// process can reclaim it before the caller connects, which is exactly what
// connectRefusedProbe below guards against.
func freeClosedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// connectRefusedProbe runs attempt against a freshly closed port and wants
// its stdout to mention "refused". On an ambiguous result it confirms the
// port was really reused (TOCTOU) before retrying with a fresh one, instead
// of flaking outright or masking a real regression as "just a port race".
func connectRefusedProbe(t *testing.T, attempt func(port int) (string, error)) {
	t.Helper()
	const attempts = 3
	for i := 0; i < attempts; i++ {
		port := freeClosedPort(t)
		out, err := attempt(port)
		if err != nil {
			t.Fatal(err)
		}
		got := strings.TrimSpace(out)
		if strings.Contains(strings.ToLower(got), "refused") {
			return
		}
		if conn, dialErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond); dialErr == nil {
			conn.Close()
			continue
		}
		t.Fatalf("stdout %q does not mention connection refused", got)
	}
	t.Fatalf("connection-refused probe stayed ambiguous after %d retries (port reused each time)", attempts)
}

// TestNetConnectRefusedReportsErrno checks that Net.connect against a
// closed port fails with the OS's real reason (ECONNREFUSED) instead of a
// generic "cannot connect", on every engine.
func TestNetConnectRefusedReportsErrno(t *testing.T) {
	const srcTmpl = `let n = host.net()
match n.connect("127.0.0.1", %d) {
    Ok(_) -> print "connected"
    Err(e) -> print "err: ${e.message()}"
}
`
	std, _ := filepath.Abs("../../std")
	writeMain := func(t *testing.T, port int) string {
		root := t.TempDir()
		os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"netconnectprobe\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
		os.WriteFile(filepath.Join(root, "main.kg"), []byte(fmt.Sprintf(srcTmpl, port)), 0o644)
		return root
	}

	t.Run("native", func(t *testing.T) {
		if driver.CCompiler() == nil {
			t.Skip("no C compiler")
		}
		connectRefusedProbe(t, func(port int) (string, error) {
			m, err := driver.LoadModule(writeMain(t, port), driver.LoadOptions{StdRoot: std})
			if err != nil {
				return "", err
			}
			exe := filepath.Join(t.TempDir(), "prog")
			var buildErr bytes.Buffer
			ok, err := testBuild(m, exe, &buildErr)
			if err != nil || !ok {
				return "", fmt.Errorf("build: %v\n%s", err, buildErr.String())
			}
			cmd := exec.Command(exe)
			var out, errOut bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errOut
			if err := cmd.Run(); err != nil {
				return "", fmt.Errorf("run: %v\n%s", err, errOut.String())
			}
			return out.String(), nil
		})
	})

	for _, engine := range []string{"interp", "vm"} {
		t.Run(engine, func(t *testing.T) {
			connectRefusedProbe(t, func(port int) (string, error) {
				m, err := driver.LoadModule(writeMain(t, port), driver.LoadOptions{StdRoot: std})
				if err != nil {
					return "", err
				}
				t.Chdir(t.TempDir())
				var out, errOut bytes.Buffer
				code, err := driver.RunWith(m, driver.RunOptions{Engine: engine}, &out, &errOut, nil)
				if err != nil {
					return "", fmt.Errorf("%s: %v", engine, err)
				}
				if code != 0 {
					return "", fmt.Errorf("%s: exit %d, stderr: %s", engine, code, errOut.String())
				}
				return out.String(), nil
			})
		})
	}
}
