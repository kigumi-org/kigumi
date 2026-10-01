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

	"golang.org/x/sys/unix"

	"kigumi/internal/driver"
)

const httpPostProbeSrc = `let h = host.net().http()
match h.post(host.args().get(1) || "?", "hi") {
    Ok(r) -> print "ok status=${r.status}"
    Err(e) -> print "err: ${e.message()}"
}
`

func buildHTTPProbe(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"httpprobe\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte(httpPostProbeSrc), 0o644)
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "prog")
	var buildErr bytes.Buffer
	ok, err := testBuild(m, exe, &buildErr)
	if err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, buildErr.String())
	}
	return exe
}

// TestHttpPostRequestLine serves a raw TCP endpoint and checks the request
// line and Host header Http.post's native body sends, for a plain URL and
// an IPv6 literal.
func TestHttpPostRequestLine(t *testing.T) {
	t.Parallel()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	exe := buildHTTPProbe(t)
	cases := []struct {
		name, listenAddr, path, wantHost string
	}{
		{"ipv4", "127.0.0.1:0", "/foo/bar", ""},
		{"ipv6", "[::1]:0", "/v6path", "[::1]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ln, err := net.Listen("tcp", c.listenAddr)
			if err != nil {
				t.Skipf("listen %s: %v", c.listenAddr, err)
			}
			defer ln.Close()
			addr := ln.Addr().(*net.TCPAddr)
			host := addr.IP.String()
			if addr.IP.To4() == nil {
				host = "[" + host + "]"
			}
			wantHost := c.wantHost
			if wantHost == "" {
				wantHost = host
			}
			url := fmt.Sprintf("http://%s:%d%s", host, addr.Port, c.path)
			got := make(chan string, 1)
			go func() {
				conn, err := ln.Accept()
				if err != nil {
					got <- ""
					return
				}
				defer conn.Close()
				conn.SetReadDeadline(time.Now().Add(5 * time.Second))
				buf := make([]byte, 4096)
				n, _ := conn.Read(buf)
				conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"))
				got <- string(buf[:n])
			}()
			cmd := exec.Command(exe, url)
			var out, errOut bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errOut
			if err := cmd.Run(); err != nil {
				t.Fatalf("run: %v\n%s", err, errOut.String())
			}
			if strings.TrimSpace(out.String()) != "ok status=200" {
				t.Fatalf("client output: %s (stderr: %s)", out.String(), errOut.String())
			}
			wire := <-got
			wantLine := "POST " + c.path + " HTTP/1.1\r\n"
			if !strings.HasPrefix(wire, wantLine) {
				t.Errorf("request line: got %q, want prefix %q", wire, wantLine)
			}
			wantHostLine := "Host: " + wantHost + "\r\n"
			if !strings.Contains(wire, wantHostLine) {
				t.Errorf("host header: got %q, want to contain %q", wire, wantHostLine)
			}
		})
	}
}

// TestHttpPostInvalidUTF8StatusLine checks that a status line containing
// invalid UTF-8 fails Http.post with a clear message instead of readLine
// silently swallowing the bad bytes into "" (net-readline-utf8).
func TestHttpPostInvalidUTF8StatusLine(t *testing.T) {
	t.Parallel()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	exe := buildHTTPProbe(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		buf := make([]byte, 4096)
		conn.Read(buf)
		conn.Write([]byte("HTTP/1.1 200 O\xffK\r\nContent-Length: 0\r\n\r\n"))
	}()
	addr := ln.Addr().(*net.TCPAddr)
	url := fmt.Sprintf("http://127.0.0.1:%d/x", addr.Port)
	cmd := exec.Command(exe, url)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\n%s", err, errOut.String())
	}
	if got := strings.TrimSpace(out.String()); got != "err: invalid UTF-8" {
		t.Fatalf("client output: got %q, want %q (stderr: %s)", got, "err: invalid UTF-8", errOut.String())
	}
}

// TestHttpPostTimeout checks that Http.post gives up against a peer that
// accepts the connection but never answers, instead of hanging forever.
func TestHttpPostTimeout(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	t.Parallel() // ~10s of its own blocked recv(); let it overlap other tests
	exe := buildHTTPProbe(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			<-done
			conn.Close()
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	url := fmt.Sprintf("http://127.0.0.1:%d/x", addr.Port)
	cmd := exec.Command(exe, url)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)
	close(done)
	if runErr != nil {
		t.Fatalf("run: %v\n%s", runErr, errOut.String())
	}
	if elapsed > 30*time.Second {
		t.Fatalf("Http.post took %s, expected a bounded timeout well under that", elapsed)
	}
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "err:") {
		t.Fatalf("expected a timeout error, got %q", out.String())
	}
}

// TestHttpPostConnectTimeout checks that Http.post bounds the connect()
// phase itself, not just send/receive: a listener whose accept queue is
// already full silently drops the next SYN, so a plain blocking connect()
// would otherwise be bounded only by the OS's own default of well over a
// minute.
func TestHttpPostConnectTimeout(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	t.Parallel() // ~10s of its own blocked connect(); let it overlap other tests
	exe := buildHTTPProbe(t)
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
		t.Fatal(err)
	}
	if err := unix.Bind(fd, &unix.SockaddrInet4{Addr: [4]byte{127, 0, 0, 1}}); err != nil {
		t.Fatal(err)
	}
	if err := unix.Listen(fd, 1); err != nil {
		t.Fatal(err)
	}
	sa, err := unix.Getsockname(fd)
	if err != nil {
		t.Fatal(err)
	}
	port := sa.(*unix.SockaddrInet4).Port

	// Fills the one-slot accept queue without ever calling accept(2), so
	// the probe's own connect() finds no room and its SYN is dropped.
	filler, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer filler.Close()

	url := fmt.Sprintf("http://127.0.0.1:%d/x", port)
	cmd := exec.Command(exe, url)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)
	if runErr != nil {
		t.Fatalf("run: %v\n%s", runErr, errOut.String())
	}
	if elapsed > 30*time.Second {
		t.Fatalf("Http.post took %s to give up on an unacknowledged connect, expected a bounded timeout well under that", elapsed)
	}
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "err:") {
		t.Fatalf("expected a connect-timeout error, got %q", out.String())
	}
}
