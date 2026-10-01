package hir

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

func (l *lowerer) binary(n syntax.NodeID) *Expr {
	node := l.t.Nodes[n]
	lhs, rhs := syntax.NodeID(node.Lhs), syntax.NodeID(node.Rhs)
	if lit, ok := l.r.LiteralOf(l.t, n); ok && (lit.Kind == sem.LitInt || lit.Kind == sem.LitFloat) {
		return l.literal(n)
	}
	call := l.info.Calls[n]
	op := l.t.TokText(node.Tok)
	switch {
	case call.Kind == sem.CallFallback:
		out := l.node(n, Fallback)
		out.Ent = call.Callee
		out.Args = []*Expr{l.expr(lhs)}
		// The miss side of `a || (x) => b` binds the miss value.
		if l.t.Kind(rhs) == syntax.Lambda {
			ln := l.t.Nodes[rhs]
			out.Param = l.decl(l.info.Defs[l.t.Children(syntax.NodeID(ln.Lhs))[0]])
			out.Args = append(out.Args, l.expr(syntax.NodeID(ln.Rhs)))
		} else {
			out.Args = append(out.Args, l.expr(rhs))
		}
		return out
	case call.Kind == sem.CallPipe:
		// A call on the right already takes the left operand as its first
		// argument; only a lambda is applied here.
		if l.t.Kind(rhs) != syntax.Lambda {
			out := l.node(n, Wrap)
			out.Args = []*Expr{l.expr(rhs)}
			return out
		}
		out := l.node(n, Pipe)
		out.Args = []*Expr{l.expr(lhs), l.expr(rhs)}
		return out
	case call.Piped != 0:
		// `a |> f`: the name on the right called with the left operand.
		return l.callWith(n, rhs, nil, call)
	case op == "&&":
		out := l.node(n, And)
		out.Args = []*Expr{l.expr(lhs), l.expr(rhs)}
		return out
	}
	if call.Kind == sem.CallMethod {
		return l.protoBinary(n, op, call, lhs, rhs)
	}
	out := l.node(n, Binary)
	out.Name, out.Args = op, []*Expr{l.expr(lhs), l.expr(rhs)}
	if call.Kind == sem.CallOperator {
		out.Ent = call.Callee
	}
	return out
}

func (l *lowerer) call(n syntax.NodeID) *Expr {
	node := l.t.Nodes[n]
	call := l.info.Calls[n]
	// A named argument moved sem's checked args out of
	// source order; ArgOrder carries that order back for lowering.
	argNodes := call.ArgOrder
	if argNodes == nil {
		argNodes = l.t.Children(syntax.NodeID(node.Rhs))
	}
	return l.callWith(n, syntax.NodeID(node.Lhs), argNodes, call)
}

// callWith prepends a piped left operand unless ArgOrder already placed it:
// a named-argument call's order already includes the piped node.
func (l *lowerer) callWith(n, callee syntax.NodeID, argNodes []syntax.NodeID, call sem.CallInfo) *Expr {
	var args []*Expr
	if call.Piped != 0 && call.ArgOrder == nil {
		args = append(args, l.expr(call.Piped))
	}
	for _, a := range argNodes {
		args = append(args, l.expr(a))
	}
	out := l.node(n, Call)
	out.Ent, out.Args, out.Variadic = call.Callee, args, call.Variadic == sem.VariadicElements
	switch call.Kind {
	case sem.CallIntrinsic:
		out.Kind, out.Name = Intrinsic, l.r.Entity(call.Callee).Name
	case sem.CallVariant:
		out.Kind = Variant
	case sem.CallMethod:
		base := syntax.NodeID(l.t.Nodes[memberNode(l.t, callee)].Lhs)
		out.Kind, out.Witness, out.Passes, out.ConstArgs = MethodCall, call.Witness, call.Passes, call.ConstArgs
		out.Args = append([]*Expr{l.expr(base)}, args...)
		out.Async = l.r.Fn(call.Callee).Declared&sem.EffAsync != 0
	case sem.CallStatic:
		out.Kind, out.Witness = StaticCall, call.Witness
	case sem.CallFn, sem.CallAssoc, sem.CallOperator:
		out.Passes, out.ConstArgs = call.Passes, call.ConstArgs
		out.Async = l.r.Fn(call.Callee).Declared&sem.EffAsync != 0
	default:
		out.Kind = ValueCall
		out.Args = append([]*Expr{l.expr(callee)}, args...)
	}
	return out
}

// memberNode is the member expression of a method callee, stepping past
// explicit type arguments (`recv.get[T](...)`).
func memberNode(t *syntax.Tree, callee syntax.NodeID) syntax.NodeID {
	if t.Kind(callee) == syntax.BracketExpr {
		return syntax.NodeID(t.Nodes[callee].Lhs)
	}
	return callee
}

func (l *lowerer) control(n syntax.NodeID) *Expr {
	node := l.t.Nodes[n]
	slot := func(name string) syntax.NodeID { return syntax.NodeID(l.t.Slot(n, name)) }
	switch node.Kind {
	case syntax.IfLetExpr:
		out := l.node(n, IfLet)
		out.Cond = l.expr(slot("init"))
		out.Pat = l.pattern(slot("pattern"))
		if g := slot("guard"); g != 0 {
			out.Guard = l.expr(g)
		}
		out.Then = l.block(slot("then"))
		if els := slot("else"); els != 0 {
			out.Else = l.expr(els)
		}
		return out
	case syntax.MatchExpr:
		out := l.node(n, Match)
		out.Args = []*Expr{l.expr(syntax.NodeID(node.Lhs))}
		for _, arm := range l.t.Children(syntax.NodeID(node.Rhs)) {
			a := &Arm{Pat: l.pattern(syntax.NodeID(l.t.Slot(arm, "pattern")))}
			if g := syntax.NodeID(l.t.Slot(arm, "guard")); g != 0 {
				a.Guard = l.expr(g)
			}
			a.Body = l.expr(syntax.NodeID(l.t.Slot(arm, "body")))
			out.Arms = append(out.Arms, a)
		}
		return out
	}
	out := l.node(n, Loop)
	if p := slot("pattern"); p != 0 {
		out.Pat = l.pattern(p)
	}
	if head := slot("head"); head != 0 {
		out.Cond = l.expr(head)
	}
	out.Block = l.block(slot("body"))
	return out
}
