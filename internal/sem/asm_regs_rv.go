package sem

import "fmt"

func riscvArch() *asmArch {
	a := &asmArch{PtrBits: 64, Regs: map[string]asmReg{}, GPR: "r", FPR: "f", Flags: []string{"fflags", "vtype", "vl", "vxsat", "vxrm"}}
	abi := map[int][]string{0: {"zero"}, 1: {"ra"}, 2: {"sp"}, 3: {"gp"}, 4: {"tp"}, 8: {"s0", "fp"}, 9: {"s1"}}
	for i := 5; i <= 7; i++ {
		abi[i] = []string{fmt.Sprintf("t%d", i-5)}
	}
	for i := 10; i <= 17; i++ {
		abi[i] = []string{fmt.Sprintf("a%d", i-10)}
	}
	for i := 18; i <= 27; i++ {
		abi[i] = []string{fmt.Sprintf("s%d", i-16)}
	}
	for i := 28; i <= 31; i++ {
		abi[i] = []string{fmt.Sprintf("t%d", i-25)}
	}
	reserved := map[int]string{0: "x0 is always zero", 2: "sp is the stack pointer", 3: "gp is the global pointer", 4: "tp is the thread pointer", 8: "s0 is the frame pointer", 9: "s1 is used internally by the code generator"}
	for i := 0; i <= 31; i++ {
		for _, name := range append([]string{fmt.Sprintf("x%d", i)}, abi[i]...) {
			a.add(name, 64, false, reserved[i])
		}
	}
	fabi := map[int]string{}
	for i := 0; i <= 7; i++ {
		fabi[i] = fmt.Sprintf("ft%d", i)
	}
	fabi[8], fabi[9] = "fs0", "fs1"
	for i := 10; i <= 17; i++ {
		fabi[i] = fmt.Sprintf("fa%d", i-10)
	}
	for i := 18; i <= 27; i++ {
		fabi[i] = fmt.Sprintf("fs%d", i-16)
	}
	for i := 28; i <= 31; i++ {
		fabi[i] = fmt.Sprintf("ft%d", i-20)
	}
	for i := 0; i <= 31; i++ {
		a.add(fmt.Sprintf("f%d", i), 64, true, "")
		a.add(fabi[i], 64, true, "")
	}
	return a
}
