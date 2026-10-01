package mir

import "kigumi/internal/sem"

// NumKind encodes a numeric type the way llgen and the VM's runtime values
// agree on: width, with bit 8 set for a signed integer and bit 9 for a
// float.
func NumKind(tt *sem.TypeTable, t sem.TypeID) int {
	if tt.IsFloat(t) {
		return tt.Width(t) | 1<<9
	}
	if !tt.IsInteger(t) {
		return 64 | 1<<8
	}
	k := tt.Width(t)
	if tt.IsSigned(t) {
		k |= 1 << 8
	}
	return k
}
