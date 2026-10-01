package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// block runs the statements of a block and yields the tail value; defers
// registered inside run at exit, errdefers only on an Err exit.
func (fr *frame) block(n syntax.NodeID) (Value, *ctrl) {
	start := len(fr.defers)
	fr.pushScope()
	stmts := fr.t.Children(n)
	var result Value = Unit{}
	var c *ctrl
	for i, s := range stmts {
		last := i == len(stmts)-1
		if last && fr.t.Kind(s) == syntax.ExprStmt {
			e := syntax.NodeID(fr.t.Nodes[s].Lhs)
			result, c = fr.expr(e)
			if c == nil {
				result = fr.transfer(e, result)
			}
		} else {
			result, c = fr.stmt(s)
		}
		if c != nil {
			break
		}
	}
	if len(fr.defers) > start {
		c = fr.runDefers(start, c, result)
	}
	fr.popScope()
	if c != nil {
		return nil, c
	}
	// The checker records a coercion on the block's own node only when its
	// last statement does not stand for the block's value (no tail, or an
	// `if`/`for` in tail position without an else); fr.coerce is a no-op
	// otherwise, so this always runs.
	return fr.coerce(n, result), nil
}

func (fr *frame) runDefers(start int, c *ctrl, result Value) *ctrl {
	failing := c != nil && c.kind == ctrlFail || isErr(c, result, fr.in)
	pending := fr.defers[start:]
	fr.defers = fr.defers[:start]
	for i := len(pending) - 1; i >= 0; i-- {
		d := pending[i]
		if d.err && !failing {
			continue
		}
		if _, dc := fr.stmtOrBlock(d.node); dc != nil && c == nil {
			c = dc
		}
	}
	return c
}

func (fr *frame) stmtOrBlock(n syntax.NodeID) (Value, *ctrl) {
	if fr.t.Kind(n) == syntax.Block {
		return fr.block(n)
	}
	return fr.stmt(n)
}

// exitScope runs the frame's remaining defers at function exit.
func (fr *frame) exitScope(c *ctrl) *ctrl {
	if len(fr.defers) > 0 {
		c = fr.runDefers(0, c, nil)
	}
	return c
}

func (fr *frame) stmt(n syntax.NodeID) (Value, *ctrl) {
	node := fr.t.Nodes[n]
	switch node.Kind {
	case syntax.LetStmt:
		return Unit{}, fr.letStmt(n)
	case syntax.AssignStmt:
		return Unit{}, fr.assignStmt(n)
	case syntax.ExprStmt:
		v, c := fr.expr(syntax.NodeID(node.Lhs))
		if c == nil && fr.in.moveOnly(v) && fr.t.Kind(syntax.NodeID(node.Lhs)) != syntax.Ident {
			fr.dropDeep(v)
		}
		return Unit{}, c
	case syntax.DeferStmt:
		fr.defers = append(fr.defers, deferEntry{node: syntax.NodeID(node.Lhs)})
		return Unit{}, nil
	case syntax.ErrdeferStmt:
		fr.defers = append(fr.defers, deferEntry{node: syntax.NodeID(node.Lhs), err: true})
		return Unit{}, nil
	}
	return fr.expr(n)
}

func (fr *frame) letStmt(n syntax.NodeID) *ctrl {
	slots := fr.t.Slots(n)
	names := syntax.SlotNames(syntax.LetStmt)
	var pattern, init, els syntax.NodeID
	for i, name := range names {
		switch name {
		case "pattern":
			pattern = syntax.NodeID(slots[i])
		case "init":
			init = syntax.NodeID(slots[i])
		case "else":
			els = syntax.NodeID(slots[i])
		}
	}
	v, c := fr.expr(init)
	if c != nil {
		return c
	}
	v = fr.transferValue(init, v)
	if fr.match(pattern, v) {
		// A refutable pattern may bind by `&`, aliasing rather than taking
		// init's value (patternBindsOwned, bind in pattern.go).
		fr.markPatternMoved(init, pattern, v)
		return nil
	}
	if els == 0 {
		fr.panicAt(pattern, "refutable pattern did not match")
	}
	_, c = fr.block(els)
	return c
}

// transfer moves a move-only value out of a local source and copies a
// structural one; a non-borrow source is dereferenced first, since `mut
// self` is itself stored as a Ref to the caller's cell.
func (fr *frame) transfer(src syntax.NodeID, v Value) Value {
	v = fr.transferValue(src, v)
	if fr.in.moveOnly(v) {
		fr.markMoved(src, v)
	}
	return v
}

// transferValue is transfer without marking src moved, for a caller (like
// letStmt) that must first see whether its pattern actually takes src's
// value by ownership.
func (fr *frame) transferValue(src syntax.NodeID, v Value) Value {
	if !isBorrowExpr(fr.t, src) {
		v = deref(v)
	}
	if !fr.in.moveOnly(v) {
		return cloneValue(v)
	}
	return v
}

func (fr *frame) assignStmt(n syntax.NodeID) *ctrl {
	node := fr.t.Nodes[n]
	lhs, rhs := syntax.NodeID(node.Lhs), syntax.NodeID(node.Rhs)
	if fr.t.Kind(lhs) == syntax.Ident && fr.t.TokText(fr.t.Nodes[lhs].Tok) == "_" {
		v, c := fr.expr(rhs)
		if c == nil && fr.in.moveOnly(v) {
			// The discarded value may wrap a resource (an Option out of a map).
			fr.dropDeep(v)
		}
		return c
	}
	if call, ok := fr.info.Calls[n]; ok && (call.Kind == sem.CallBuiltinOp || call.Kind == sem.CallOperator) {
		cell, path, err := fr.resolvePlace(lhs)
		if err != nil {
			return err
		}
		cur := walkPath(cell.V, path)
		r, c := fr.expr(rhs)
		if c != nil {
			return c
		}
		var out Value
		if call.Kind == sem.CallOperator {
			out, c = fr.callUser(call.Callee, n, []Value{cur, r}, nil)
		} else {
			out = fr.binaryOp(n, call.Op, cur, r)
		}
		if c != nil {
			return c
		}
		fr.writePlace(cell, path, out)
		return nil
	}
	cell, path, err := fr.resolvePlace(lhs)
	if err != nil {
		return err
	}
	v, c := fr.expr(rhs)
	if c != nil {
		return c
	}
	fr.writePlace(cell, path, fr.transfer(rhs, v))
	return nil
}
