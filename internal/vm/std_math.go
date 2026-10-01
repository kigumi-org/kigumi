package vm

import "math"

// mathFn implements the std/math primitives; nil for other keys.
func (m *Machine) mathFn(key string, a0, a1 *obj) *obj {
	switch key {
	case "math.sqrt":
		return mkFloat(math.Sqrt(a0.f), 64|512)
	case "math.floor":
		return mkFloat(math.Floor(a0.f), 64|512)
	case "math.ceil":
		return mkFloat(math.Ceil(a0.f), 64|512)
	case "math.round":
		return mkFloat(math.Round(a0.f), 64|512)
	case "math.pow":
		return mkFloat(math.Pow(a0.f, a1.f), 64|512)
	}
	return nil
}
