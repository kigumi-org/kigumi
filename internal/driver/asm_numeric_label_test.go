package driver_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

const asmNumericLabelSrc = `type LoopOut = {
    pub acc i64
    pub left i64
}

fn sumTo(n: i64) -> i64 {
    unsafe {
        let r: LoopOut = asm {
            "1:"
            "add {acc}, {left}"
            "dec {left}"
            "jnz 1b"
            acc: inout(reg) 0
            left: inout(reg) n
        }
        r.acc
    }
}

print "start"
print "${sumTo(5)}"
`

// TestAsmNumericLocalLabel: an inline asm template's
// GNU-style numeric local label (`1:`, `jnc 1b`) used to make LLVM's x86
// Intel-syntax parser misread the backward reference as a binary integer
// literal ("invalid operand for instruction"). interp and VM keep their
// existing asm treatment (a panic and a refusal respectively); only native
// actually runs the loop, so it is the only engine that has to add to 15.
func TestAsmNumericLocalLabel(t *testing.T) {
	std, _ := filepath.Abs("../../std")
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"asmnumericlabel\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte(asmNumericLabelSrc), 0o644)

	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("native", func(t *testing.T) {
		if runtime.GOARCH != "amd64" || runtime.GOOS != "linux" {
			t.Skip("x86-64 Linux only")
		}
		if driver.CCompiler() == nil {
			t.Skip("no C compiler")
		}
		exe := filepath.Join(t.TempDir(), "prog")
		var buildErr bytes.Buffer
		if ok, err := testBuild(m, exe, &buildErr); err != nil || !ok {
			t.Fatalf("build: %v\n%s", err, buildErr.String())
		}
		cmd := exec.Command(exe)
		var out, errOut bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errOut
		if err := cmd.Run(); err != nil {
			t.Fatalf("run: %v\nstderr: %s", err, errOut.String())
		}
		if want := "start\n15\n"; out.String() != want {
			t.Errorf("stdout = %q, want %q", out.String(), want)
		}
	})

	t.Run("vm", func(t *testing.T) {
		var out, errOut bytes.Buffer
		_, err := driver.RunWith(m, driver.RunOptions{Engine: "vm"}, &out, &errOut, nil)
		if err == nil || !strings.Contains(err.Error(), "not supported by the VM") {
			t.Fatalf("want a VM refusal, got %v", err)
		}
	})

	t.Run("interp", func(t *testing.T) {
		t.Chdir(t.TempDir())
		var out, errOut bytes.Buffer
		code, err := driver.RunWith(m, driver.RunOptions{Engine: "interp"}, &out, &errOut, nil)
		if err != nil {
			t.Fatal(err)
		}
		if code != 2 || !strings.Contains(errOut.String(), "inline assembly needs `kigumi build`") {
			t.Errorf("exit %d, stderr %q", code, errOut.String())
		}
		if out.String() != "start\n" {
			t.Errorf("stdout = %q, want %q", out.String(), "start\n")
		}
	})
}
