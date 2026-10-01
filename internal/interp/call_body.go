package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// runBody evaluates a function body and turns control transfers into the
// return value; `fail` becomes Err of the declared Result type.
func (in *Interp) runBody(fr *frame, body syntax.NodeID, ret sem.TypeID) (Value, *ctrl) {
	v, c := fr.expr(body)
	// A block body already transfers its own trailing expression (block,
	// eval_stmt.go); an expression-bodied lambda (`() => move self.r`) has
	// no block to do that for it, so it needs the same transfer here.
	if c == nil && fr.t.Kind(body) != syntax.Block {
		v = fr.transfer(body, v)
	}
	c = fr.exitScope(c)
	fr.popScope()
	if c == nil {
		return v, nil
	}
	switch c.kind {
	case ctrlReturn:
		// A bare `return` carries a plain Unit with no coercion site to
		// attach an Ok-wrap to, so it is wrapped here against ret instead.
		if _, bare := c.val.(Unit); bare {
			if val, _, ok := in.r.Types.IsResult(ret); ok && val == sem.TyUnit {
				return &Variant{Type: ret, V: in.variantNamed(in.r.Types.ResultEnt(), "Ok"), Payload: []Value{c.val}}, nil
			}
		}
		return c.val, nil
	case ctrlFail:
		return &Variant{Type: ret, V: in.variantNamed(in.r.Types.ResultEnt(), "Err"), Payload: []Value{c.val}}, nil
	}
	return Unit{}, nil
}
