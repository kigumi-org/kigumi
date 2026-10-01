// Package hashkey is the one definition of the primitive Hash algorithm,
// shared by internal/interp and internal/vm so both engines (and, ported
// by hand, internal/llgen/runtime/rt_core.c) hash identical values to the
// identical uint64. It also holds the Float total order the Eq/Ord/Hash
// protocol methods use, kept apart from the IEEE `==`/`<` operators.
package hashkey

import "math"

const offset64 = 14695981039346656037
const prime64 = 1099511628211

// Sum64 is FNV-1a-64 over data, using the standard offset basis
// (0xcbf29ce484222325); a prior copy of this constant was mistyped one
// digit short in two of the three engines.
func Sum64(data []byte) uint64 {
	h := uint64(offset64)
	for _, b := range data {
		h ^= uint64(b)
		h *= prime64
	}
	return h
}

// CanonicalFloat64 maps -0.0 to 0.0 so a Float's hash agrees with its Eq
// (0.0 == -0.0); NaN is left alone, since Eq already treats it as unequal
// to everything, including itself.
func CanonicalFloat64(f float64) float64 {
	if f == 0 {
		return 0
	}
	return f
}

// TotalEqualFloat64 is the Eq protocol's `f64`/`f32` equality: every
// NaN equals every other NaN, and -0.0 equals 0.0. `==` stays IEEE and does
// not call this.
func TotalEqualFloat64(a, b float64) bool {
	an, bn := math.IsNaN(a), math.IsNaN(b)
	if an || bn {
		return an && bn
	}
	return CanonicalFloat64(a) == CanonicalFloat64(b)
}

// TotalCompareFloat64 is the Ord protocol's `f64`/`f32` total order:
// every NaN sorts above every non-NaN value and equals every other NaN;
// -0.0 and 0.0 compare equal. Returns -1, 0 or 1.
func TotalCompareFloat64(a, b float64) int {
	an, bn := math.IsNaN(a), math.IsNaN(b)
	switch {
	case an && bn:
		return 0
	case an:
		return 1
	case bn:
		return -1
	}
	a, b = CanonicalFloat64(a), CanonicalFloat64(b)
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
