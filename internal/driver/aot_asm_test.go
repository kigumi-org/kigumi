package driver_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"kigumi/internal/driver"
)

// TestAotAsm runs x86-64 inline assembly: a raw write syscall, cpuid into
// a record, and a call to an export(C) function through a sym operand.
func TestAotAsm(t *testing.T) {
	t.Parallel()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	if runtime.GOARCH != "amd64" || runtime.GOOS != "linux" {
		t.Skip("x86-64 Linux only")
	}
	root := t.TempDir()
	src := `import ffi from std/ffi

pub type Cpuid = {
    pub eax u32
    pub ebx u64
    pub ecx u32
    pub edx u32
}

export(C) {
    pub fn kg_add(a: i64, b: i64) -> i64 {
        a + b
    }
}

export(C, naked) {
    pub fn kg_naked_add(a: i64, b: i64) -> i64 {
        asm {
            "lea rax, [rdi + rsi]"
            "ret"
        }
    }
}

fn sysWrite(fd: usize, buf: *const u8, len: usize) -> Out[isize] {
    unsafe {
        asm {
            "syscall"
            in(rax) 1
            in(rdi) fd
            in(rsi) buf
            in(rdx) len
            value: out(rax)
            clobber rcx, r11
        }
    }
}

fn cpuid(leaf: u32, sub: u32) -> Cpuid {
    unsafe {
        asm {
            "mov {ebx}, rbx"
            "cpuid"
            "xchg {ebx}, rbx"
            eax: inout(eax) leaf
            ebx: out(reg)
            ecx: inout(ecx) sub
            edx: out(edx)
            options nostack, preserves_flags
        }
    }
}

fn addViaAsm(a: i64, b: i64) -> Out[i64] {
    unsafe {
        asm {
            "mov r12, rsp"
            "and rsp, -16"
            "call {f}"
            "mov rsp, r12"
            in(rdi) a
            in(rsi) b
            f: sym kg_add
            value: lateout(rax)
            clobber r12, abi(C)
        }
    }
}

fn shifted(x: u64) -> Out[u64] {
    unsafe {
        asm {
            "shl {value}, {n}"
            value: inout(reg) x
            n: const 3
            options nomem, nostack, pure, preserves_flags
        }
    }
}

let msg = "hi\n"
let s = ffi.CString.new(&msg)?
let Out { value: n } = sysWrite(1, s.ptr(), 3)
let Cpuid { eax } = cpuid(0, 0)
let Out { value: sum } = addViaAsm(20, 22)
let Out { value: sh } = shifted(5)
let nk = unsafe { kg_naked_add(40, 2) }
let fp: extern(C) fn(i64, i64) -> i64 = kg_naked_add
let nk2 = unsafe { fp(1, 2) }
print "${n} ${eax > 0} ${sum} ${sh} ${nk} ${nk2}"
`
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"asmtest\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte(src), 0o644)
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
	out, err := exec.Command(exe).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	if string(out) != "hi\n3 true 42 40 42 3\n" {
		t.Errorf("got %q", out)
	}
}
