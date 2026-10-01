package mir

import (
	"fmt"

	"kigumi/internal/hir"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// coerce applies the checker's coercion steps to a value.
func (b *builder) coerce(co hir.Coercion, n syntax.NodeID, v LocalID) LocalID {
	for _, s := range co.Coerce {
		switch s.Kind {
		case sem.CoLiteral:
			if b.fn.Locals[v].Type != s.To {
				v = b.emit(Inst{Op: OpUnary, Dst: b.temp(s.To), Str: "cast", Type: s.To, Args: []LocalID{v}, Node: n})
			}
		case sem.CoBorrow:
			v = b.emit(Inst{Op: OpBorrow, Dst: b.temp(s.To), Args: []LocalID{v}, Node: n})
		case sem.CoSome:
			// A source already at the wrapped type means this step ran
			// twice on the same value (a lowering bug), not a legitimate
			// Option[Option[T]]: the checker only wraps a bare T.
			if b.fn.Locals[v].Type == s.To {
				panic(fmt.Sprintf("mir: %s: CoSome applied to a value already of type %s (double coercion)", b.fn.Name, b.r.TypeString(s.To)))
			}
			v = b.emit(Inst{Op: OpVariant, Dst: b.temp(s.To), Ent: b.variantNamed(b.r.Types.OptionEnt(), "Some"), Type: s.To, Args: []LocalID{v}, Node: n})
		case sem.CoOk:
			if b.fn.Locals[v].Type == s.To {
				panic(fmt.Sprintf("mir: %s: CoOk applied to a value already of type %s (double coercion)", b.fn.Name, b.r.TypeString(s.To)))
			}
			v = b.emit(Inst{Op: OpVariant, Dst: b.temp(s.To), Ent: b.variantNamed(b.r.Types.ResultEnt(), "Ok"), Type: s.To, Args: []LocalID{v}, Node: n})
		case sem.CoExistential:
			v = b.emit(Inst{Op: OpBox, Dst: b.temp(s.To), Type: co.CoFrom, Args: []LocalID{v}, Node: n})
		case sem.CoCFnPtr:
			v = b.emit(Inst{Op: OpCFnPtr, Dst: b.temp(s.To), Ent: co.CoEnt, Type: s.To, Node: n})
		}
	}
	return v
}
