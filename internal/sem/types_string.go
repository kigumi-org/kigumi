package sem

import (
	"strings"

	"kigumi/internal/syntax"
)

var primNames = map[TypeID]string{
	TyPoison: "<error>", TyNever: "Never", TyUnit: "Unit", TyBool: "Bool", TyChar: "Char",
	TyString: "String", TyBytes: "Bytes", TyI8: "i8", TyI16: "i16", TyI32: "i32", TyI64: "Int",
	TyI128: "i128", TyU8: "u8", TyU16: "u16", TyU32: "u32", TyU64: "u64", TyU128: "u128",
	TyUsize: "usize", TyIsize: "isize", TyF32: "f32", TyF64: "Float",
	TyUntypedInt: "{integer}", TyUntypedFloat: "{float}",
}

// TypeString renders a type in source notation: `T?`, `T!`, `Int` for i64,
// `Float` for f64, `pure fn(A) -> R`.
func (r *Result) TypeString(t TypeID) string {
	var sb strings.Builder
	r.writeType(&sb, t)
	return sb.String()
}

func (r *Result) writeType(sb *strings.Builder, t TypeID) {
	if t == 0 {
		sb.WriteString("<none>")
		return
	}
	if name, ok := primNames[t]; ok {
		sb.WriteString(name)
		return
	}
	tt := r.Types
	n := tt.nodes[t]
	switch n.Kind {
	case KNamed, KIface:
		if elem, ok := tt.IsOption(t); ok {
			r.writeType(sb, elem)
			sb.WriteString("?")
			return
		}
		if val, err, ok := tt.IsResult(t); ok && err == tt.errorType() {
			r.writeType(sb, val)
			sb.WriteString("!")
			return
		}
		if tt.isTupleEnt(n.Ent) {
			sb.WriteString("(")
			for i, a := range n.Args {
				if i > 0 {
					sb.WriteString(", ")
				}
				r.writeType(sb, a)
			}
			sb.WriteString(")")
			return
		}
		sb.WriteString(r.entityName(n.Ent))
		r.writeArgs(sb, n.Args)
	case KParam:
		sb.WriteString(r.entityName(n.Ent))
	case KConst:
		v, ty := tt.ConstValue(t)
		if ty == TyBool {
			sb.WriteString(map[int64]string{0: "false", 1: "true"}[v])
			return
		}
		sb.WriteString(itoa64(v))
	case KVar:
		sb.WriteString("_")
	case KFn:
		r.writeFn(sb, n)
	case KClosure:
		sb.WriteString("closure ")
		if id := n.Ent; id != 0 {
			r.writeFn(sb, tt.nodes[r.closure(id).Sig])
		}
	case KRef:
		sb.WriteString("&")
		if n.Flags&flagMut != 0 {
			sb.WriteString("mut ")
		}
		r.writeType(sb, n.Elem)
	case KPtr:
		if n.Flags&flagMut != 0 {
			sb.WriteString("*mut ")
		} else {
			sb.WriteString("*const ")
		}
		r.writeType(sb, n.Elem)
	default:
		sb.WriteString("<?>")
	}
}

func (r *Result) writeArgs(sb *strings.Builder, args []TypeID) {
	if len(args) == 0 {
		return
	}
	sb.WriteString("[")
	for i, a := range args {
		if i > 0 {
			sb.WriteString(", ")
		}
		r.writeType(sb, a)
	}
	sb.WriteString("]")
}

func (tt *TypeTable) errorType() TypeID {
	if tt.errorEnt == 0 {
		return 0
	}
	return tt.Iface(tt.errorEnt, nil)
}

// TypeStringAt renders the type of node n, keeping the alias spelling the
// source used when the node refers to an alias.
func (r *Result) TypeStringAt(t *syntax.Tree, n syntax.NodeID) string {
	f := r.File(t)
	if f == nil || int(n) >= len(f.Types) {
		return ""
	}
	if use := f.Uses[n]; use != 0 && r.Entities[use].Kind == EntAlias {
		return r.Entities[use].Name
	}
	return r.TypeString(f.Types[n])
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func itoa(n int) string { return itoa64(int64(n)) }
