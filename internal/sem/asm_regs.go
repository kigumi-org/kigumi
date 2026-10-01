package sem

import "fmt"

// asmReg describes one register of a target: its operand width, whether
// it belongs to the floating-point file, and why it cannot be an operand.
type asmReg struct {
	Bits     int
	Float    bool
	Reserved string
}

// asmArch is the inline-asm view of an architecture: the register table,
// the LLVM constraint letters of the `reg` and `freg` classes and the
// flag registers a block clobbers unless it preserves them.
type asmArch struct {
	PtrBits int
	Regs    map[string]asmReg
	GPR     string
	FPR     string
	Flags   []string
}

var asmArches = map[string]*asmArch{
	"amd64":   x86Arch(64),
	"386":     x86Arch(32),
	"arm64":   arm64Arch(),
	"arm":     armArch(),
	"riscv64": riscvArch(),
}

// asmModifiers maps the template modifiers a user may write to LLVM's
// operand modifier letters, per architecture.
var asmModifiers = map[string]map[string]string{
	"amd64": {"l": "b", "h": "h", "x": "w", "e": "k", "r": "q"},
	"386":   {"l": "b", "h": "h", "x": "w", "e": "k"},
	"arm64": {"w": "w", "x": "x"},
}

// AsmModifier translates a template modifier for the backend.
func AsmModifier(arch, mod string) (string, bool) {
	m, ok := asmModifiers[arch][mod]
	return m, ok
}

// AsmArch exposes the constraint letters and flag clobbers to the backend.
func AsmArch(arch string) (gpr, fpr string, flags []string, ok bool) {
	a, ok := asmArches[arch]
	if !ok {
		return "", "", nil, false
	}
	return a.GPR, a.FPR, a.Flags, true
}

func (a *asmArch) add(name string, bits int, float bool, reserved string) {
	a.Regs[name] = asmReg{Bits: bits, Float: float, Reserved: reserved}
}

func x86Arch(ptr int) *asmArch {
	a := &asmArch{PtrBits: ptr, Regs: map[string]asmReg{}, GPR: "r", FPR: "x", Flags: []string{"dirflag", "fpsr", "flags"}}
	legacy := []struct{ q, d, w, b, res string }{
		{"rax", "eax", "ax", "al", ""}, {"rcx", "ecx", "cx", "cl", ""}, {"rdx", "edx", "dx", "dl", ""},
		{"rbx", "ebx", "bx", "bl", "rbx is used internally by the code generator"},
		{"rsi", "esi", "si", "sil", ""}, {"rdi", "edi", "di", "dil", ""},
		{"rbp", "ebp", "bp", "bpl", "rbp is the frame pointer"}, {"rsp", "esp", "sp", "spl", "rsp is the stack pointer"},
	}
	for _, r := range legacy {
		res := r.res
		if ptr == 32 {
			switch r.q {
			case "rbx":
				res = ""
			case "rsi":
				res = "esi is used internally by the code generator"
			}
		} else {
			a.add(r.q, 64, false, res)
		}
		a.add(r.d, 32, false, res)
		a.add(r.w, 16, false, res)
		a.add(r.b, 8, false, res)
	}
	if ptr == 64 {
		for i := 8; i <= 15; i++ {
			a.add(fmt.Sprintf("r%d", i), 64, false, "")
			a.add(fmt.Sprintf("r%dd", i), 32, false, "")
			a.add(fmt.Sprintf("r%dw", i), 16, false, "")
			a.add(fmt.Sprintf("r%db", i), 8, false, "")
		}
	}
	for i := 0; i < ptr/4; i++ {
		a.add(fmt.Sprintf("xmm%d", i), 128, true, "")
	}
	return a
}

func arm64Arch() *asmArch {
	a := &asmArch{PtrBits: 64, Regs: map[string]asmReg{}, GPR: "r", FPR: "w", Flags: []string{"cc"}}
	reserved := map[int]string{18: "x18 is the platform register", 19: "x19 is used internally by the code generator", 29: "x29 is the frame pointer", 30: "x30 is the link register"}
	for i := 0; i <= 30; i++ {
		a.add(fmt.Sprintf("x%d", i), 64, false, reserved[i])
		a.add(fmt.Sprintf("w%d", i), 32, false, reserved[i])
	}
	a.add("fp", 64, false, reserved[29])
	a.add("lr", 64, false, reserved[30])
	a.add("sp", 64, false, "sp is the stack pointer")
	a.add("wsp", 32, false, "sp is the stack pointer")
	a.add("xzr", 64, false, "xzr is always zero")
	a.add("wzr", 32, false, "wzr is always zero")
	for i := 0; i <= 31; i++ {
		a.add(fmt.Sprintf("v%d", i), 128, true, "")
		a.add(fmt.Sprintf("d%d", i), 64, true, "")
		a.add(fmt.Sprintf("s%d", i), 32, true, "")
	}
	return a
}

func armArch() *asmArch {
	a := &asmArch{PtrBits: 32, Regs: map[string]asmReg{}, GPR: "r", FPR: "w", Flags: []string{"cc"}}
	reserved := map[int]string{6: "r6 is used internally by the code generator", 7: "r7 is the frame pointer in Thumb code", 9: "r9 is the platform register", 11: "r11 is the frame pointer", 13: "r13 is the stack pointer", 14: "r14 is the link register", 15: "r15 is the program counter"}
	for i := 0; i <= 15; i++ {
		a.add(fmt.Sprintf("r%d", i), 32, false, reserved[i])
	}
	aliases := map[string]int{"a1": 0, "a2": 1, "a3": 2, "a4": 3, "v1": 4, "v2": 5, "v3": 6, "v4": 7, "v5": 8, "v6": 9, "rfp": 9, "sl": 10, "fp": 11, "ip": 12, "sp": 13, "lr": 14, "pc": 15}
	for name, i := range aliases {
		a.add(name, 32, false, reserved[i])
	}
	for i := 0; i <= 31; i++ {
		a.add(fmt.Sprintf("s%d", i), 32, true, "")
	}
	for i := 0; i <= 15; i++ {
		a.add(fmt.Sprintf("d%d", i), 64, true, "")
	}
	return a
}
