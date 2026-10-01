package driver

import "os/exec"

// CCompiler finds a C compiler able to consume LLVM IR: zig cc or clang.
func CCompiler() []string {
	if p, err := exec.LookPath("zig"); err == nil {
		return []string{p, "cc"}
	}
	if p, err := exec.LookPath("clang"); err == nil {
		return []string{p}
	}
	return nil
}
