package driver_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// loadRunFixture writes a testdata/run fixture's source sections under a
// fresh module root (`-- src --` is main/main.kg) and returns the expected
// stdout, stderr and exit code.
func loadRunFixture(t testing.TB, path string) (root, wantOut, wantErr string, wantExit int) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	root = t.TempDir()
	wantOut, wantErr, wantExit = "", "", 0
	name := ""
	body := ""
	flush := func() {
		if name == "" {
			return
		}
		switch name {
		case "stdout":
			wantOut = body
		case "stderr":
			wantErr = body
		case "exit":
			wantExit, _ = strconv.Atoi(strings.TrimSpace(body))
		case "engines":
			// Consumed by fixtureEngines, not written as a file.
		default:
			file := name
			if file == "src" {
				file = "main/main.kg"
			}
			full := filepath.Join(root, filepath.FromSlash(file))
			os.MkdirAll(filepath.Dir(full), 0o755)
			os.WriteFile(full, []byte(body), 0o644)
		}
	}
	for _, line := range strings.SplitAfter(string(data), "\n") {
		trimmed := strings.TrimRight(line, "\n")
		if strings.HasPrefix(trimmed, "-- ") && strings.HasSuffix(trimmed, " --") {
			flush()
			name = strings.TrimSuffix(strings.TrimPrefix(trimmed, "-- "), " --")
			body = ""
			continue
		}
		body += line
	}
	flush()
	return root, wantOut, wantErr, wantExit
}

// fixtureEngines reads a testdata/run fixture's optional `-- engines --`
// section: "" (absent) runs everywhere, "native" only under TestAotCorpus
// (a real extern(C) declaration panics on the interpreter and the VM
// statically refuses it, so TestRunCorpus and TestRunCorpusEngines would
// otherwise have nothing consistent to assert).
func fixtureEngines(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	name, engines := "", ""
	for _, line := range strings.SplitAfter(string(data), "\n") {
		trimmed := strings.TrimRight(line, "\n")
		if strings.HasPrefix(trimmed, "-- ") && strings.HasSuffix(trimmed, " --") {
			name = strings.TrimSuffix(strings.TrimPrefix(trimmed, "-- "), " --")
			continue
		}
		if name == "engines" {
			engines += line
		}
	}
	return strings.TrimSpace(engines)
}
