package interp

import (
	"strconv"

	"kigumi/internal/hashkey"
	"kigumi/internal/sem"
)

// display renders a value for interpolation and printing.
func display(v Value, in *Interp) string {
	return displayCanon(v, in, false)
}

// hashValue is the interp side of the shared Hash algorithm: it hashes the
// same rendering display would produce, except canon collapses -0.0 to 0.0
// so Float's hash agrees with its Eq.
func hashValue(v Value, in *Interp) uint64 {
	return hashkey.Sum64([]byte(displayCanon(v, in, true)))
}

func displayCanon(v Value, in *Interp, canon bool) string {
	switch x := deref(v).(type) {
	case Int:
		if in != nil && !in.r.Types.IsSigned(x.T) {
			return strconv.FormatUint(uint64(x.V), 10)
		}
		return strconv.FormatInt(x.V, 10)
	case Float:
		f := x.V
		if canon {
			f = hashkey.CanonicalFloat64(f)
		}
		if in != nil && in.r.Types.Width(x.T) == 32 {
			return strconv.FormatFloat(f, 'g', -1, 32)
		}
		return strconv.FormatFloat(f, 'g', -1, 64)
	case Bool:
		if x {
			return "true"
		}
		return "false"
	case Str:
		return string(x)
	case Bytes:
		return string(x)
	case Char:
		return string(rune(x))
	case Unit:
		return "()"
	case *Variant:
		s := in.r.Entity(x.V).Name
		if len(x.Payload) > 0 {
			s += "("
			for i, p := range x.Payload {
				if i > 0 {
					s += ", "
				}
				s += displayCanon(p, in, canon)
			}
			s += ")"
		}
		return s
	case *Record:
		s := in.r.Entity(in.r.Types.Node(x.Type).Ent).Name + " {"
		for i, f := range x.Fields {
			if i > 0 {
				s += ","
			}
			s += " " + displayCanon(f, in, canon)
		}
		return s + " }"
	case *Array:
		s := "["
		for i, e := range x.Elems {
			if i > 0 {
				s += ", "
			}
			s += displayCanon(e, in, canon)
		}
		return s + "]"
	case *Box:
		return displayCanon(x.V, in, canon)
	case *Opaque:
		if str, ok := x.Data.(string); ok {
			return str
		}
		return "<" + x.Kind + ">"
	}
	return "<value>"
}

// errorMessage calls `message()` on an Error value.
func (in *Interp) errorMessage(v Value) string {
	fr := in.newFrame(0, 1)
	req := in.r.Requirement(in.r.Types.ErrorEnt(), "message")
	if req == 0 {
		return display(v, in)
	}
	out, c := in.dispatch(fr, req, 0, nil, v)
	if c != nil {
		return display(v, in)
	}
	return display(out, in)
}

func mkErr(in *Interp, t sem.TypeID, msg string) Value {
	box := &Box{Dyn: in.r.Types.Iface(in.r.Types.ErrorEnt(), nil), V: &Opaque{Kind: "Error", Data: msg}}
	return &Variant{Type: t, V: in.variantNamed(in.r.Types.ResultEnt(), "Err"), Payload: []Value{box}}
}

func mkOk(in *Interp, t sem.TypeID, v Value) Value {
	return &Variant{Type: t, V: in.variantNamed(in.r.Types.ResultEnt(), "Ok"), Payload: []Value{v}}
}

func mkSome(in *Interp, t sem.TypeID, v Value) Value {
	return &Variant{Type: t, V: in.variantNamed(in.r.Types.OptionEnt(), "Some"), Payload: []Value{v}}
}

func mkNone(in *Interp, t sem.TypeID) Value {
	return &Variant{Type: t, V: in.variantNamed(in.r.Types.OptionEnt(), "None")}
}
