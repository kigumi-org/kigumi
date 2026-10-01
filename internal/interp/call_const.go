package interp

import "kigumi/internal/sem"

// constValues produces the const-parameter arguments a call passes after
// its witness ones (sem.CallInfo.ConstArgs), mirroring witnessValues for
// interface witnesses.
func (fr *frame) constValues(args []sem.ConstArg) []Value {
	var out []Value
	for _, a := range args {
		out = append(out, fr.constValue(a))
	}
	return out
}

func (fr *frame) constValue(a sem.ConstArg) Value {
	if a.Kind == sem.ConstArgLocal {
		return fr.cell(a.Local).V
	}
	if a.Type == sem.TyBool {
		return Bool(a.Value != 0)
	}
	return Int{V: a.Value, T: a.Type}
}
