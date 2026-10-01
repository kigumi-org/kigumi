package hir

import (
	"kigumi/internal/diag"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

type lowerer struct {
	r    *sem.Result
	f    sem.FileID
	t    *syntax.Tree
	info *sem.FileInfo
}

// Lower turns a checked function, implicit main or test into HIR, or nil
// when the function has no body.
func Lower(r *sem.Result, ent sem.EntityID) *Func {
	e := r.Entity(ent)
	if e.File == 0 {
		return nil
	}
	l := &lowerer{r: r, f: e.File, t: r.Tree(e.File)}
	l.info = r.File(l.t)
	out := &Func{Ent: ent, File: e.File, Name: r.Packages[e.Pkg].Path + "." + e.Name}
	failing := r.Types.Result(sem.TyUnit, r.Types.Iface(r.Types.ErrorEnt(), nil))
	switch e.Kind {
	case sem.EntFn:
		info := r.Fn(ent)
		if info.Body == 0 || r.Entity(e.Parent).Kind == sem.EntInterface {
			return nil
		}
		out.Kind = FuncFn
		if info.SelfParam != 0 {
			out.Self = l.decl(info.SelfParam)
			out.SelfMove = info.Recv == sem.RecvMove
			owner := info.Owner
			out.Destructor = owner != 0 && r.Entity(owner).Kind == sem.EntType && r.TypeDecl(owner).Drop == ent
		}
		for _, p := range info.Params {
			out.Params = append(out.Params, l.decl(p))
		}
		for _, w := range info.Witnesses {
			out.Witness = append(out.Witness, l.decl(w.Local))
		}
		for _, local := range info.ConstParams {
			out.Const = append(out.Const, l.decl(local))
		}
		out.Ret = r.Types.Node(info.Sig).Elem
		out.Body = l.expr(info.Body)
	case sem.EntImplicitMain:
		out.Kind, out.Ret, out.Name = FuncMain, failing, "main"
		out.Main = &Block{Type: sem.TyUnit, Node: l.t.Root}
		for _, d := range l.t.Children(l.t.Root) {
			if isStatement(l.t.Kind(d)) {
				out.Main.Stmts = append(out.Main.Stmts, l.stmt(d))
			}
		}
	case sem.EntTest:
		out.Kind, out.Ret, out.Name = FuncTest, failing, "test."+e.Name
		out.Body = l.expr(syntax.NodeID(l.t.Nodes[e.Node].Rhs))
	default:
		return nil
	}
	return out
}

func isStatement(k syntax.NodeKind) bool {
	switch k {
	case syntax.LetStmt, syntax.AssignStmt, syntax.ExprStmt, syntax.DeferStmt, syntax.ErrdeferStmt:
		return true
	}
	return false
}

func (l *lowerer) closure(ent sem.EntityID, body syntax.NodeID) *Func {
	info := l.r.Closure(ent)
	out := &Func{Kind: FuncClosure, Ent: ent, File: l.f, Ret: info.Ret}
	for _, cap := range info.Captures {
		d := l.decl(cap.Local)
		d.Cell = cap.Mode == sem.CapCell
		out.Captures = append(out.Captures, d)
	}
	for _, p := range info.Params {
		out.Params = append(out.Params, l.decl(p))
	}
	out.Body = l.expr(body)
	return out
}

func (l *lowerer) decl(ent sem.EntityID) *LocalDecl {
	e := l.r.Entity(ent)
	return &LocalDecl{Ent: ent, Name: e.Name, Type: e.Type, Mut: e.Flags&sem.EfMut != 0}
}

func (l *lowerer) loc(n syntax.NodeID) diag.Location { return diag.At(l.t.File, l.t.Span(n)) }

func (l *lowerer) typeOf(n syntax.NodeID) sem.TypeID { return l.info.Types[n] }

func (l *lowerer) coercion(n syntax.NodeID) Coercion {
	co, ok := l.info.Coerce[n]
	if !ok {
		return Coercion{}
	}
	out := Coercion{Coerce: co.Steps, CoFrom: co.From}
	for _, s := range co.Steps {
		if s.Kind == sem.CoCFnPtr {
			out.CoEnt = l.info.Uses[n]
		}
	}
	return out
}

// block lowers a `{ ... }`: statements, then the last expression
// statement as the value.
func (l *lowerer) block(n syntax.NodeID) *Block {
	b := &Block{Type: l.typeOf(n), Node: n, Coercion: l.coercion(n)}
	stmts := l.t.Children(n)
	for i, s := range stmts {
		if i == len(stmts)-1 && l.t.Kind(s) == syntax.ExprStmt {
			b.Tail = l.expr(syntax.NodeID(l.t.Nodes[s].Lhs))
			break
		}
		b.Stmts = append(b.Stmts, l.stmt(s))
	}
	return b
}

func (l *lowerer) stmt(n syntax.NodeID) *Stmt {
	node := l.t.Nodes[n]
	out := &Stmt{Loc: l.loc(n), Node: n}
	switch node.Kind {
	case syntax.LetStmt:
		out.Kind = LetStmt
		out.Expr = l.expr(syntax.NodeID(l.t.Slot(n, "init")))
		out.Pat = l.pattern(syntax.NodeID(l.t.Slot(n, "pattern")))
		if els := syntax.NodeID(l.t.Slot(n, "else")); els != 0 {
			out.Else = l.block(els)
		}
	case syntax.AssignStmt:
		return l.assign(n, out)
	case syntax.ExprStmt:
		out.Kind, out.Expr = ExprStmt, l.expr(syntax.NodeID(node.Lhs))
	case syntax.DeferStmt, syntax.ErrdeferStmt:
		out.Kind, out.Err = DeferStmt, node.Kind == syntax.ErrdeferStmt
		body := syntax.NodeID(node.Lhs)
		if l.t.Kind(body) == syntax.Block {
			out.Block = l.block(body)
		} else {
			out.Body = l.stmt(body)
		}
	default:
		out.Kind, out.Expr = EvalStmt, l.expr(n)
	}
	return out
}

func (l *lowerer) assign(n syntax.NodeID, out *Stmt) *Stmt {
	node := l.t.Nodes[n]
	lhs, rhs := syntax.NodeID(node.Lhs), syntax.NodeID(node.Rhs)
	if l.t.Kind(lhs) == syntax.Ident && l.t.TokText(l.t.Nodes[lhs].Tok) == "_" {
		out.Kind, out.Expr = DiscardStmt, l.expr(rhs)
		return out
	}
	out.Kind = AssignStmt
	out.Place = l.place(lhs)
	if call, ok := l.info.Calls[n]; ok && (call.Kind == sem.CallBuiltinOp || call.Kind == sem.CallOperator) {
		out.Compound, out.OpText = true, call.OpText
		if call.Kind == sem.CallOperator {
			out.OpEnt = call.Callee
		}
	}
	out.Expr = l.expr(rhs)
	return out
}

// place resolves an assignment target; its operands are lowered here so
// the builder evaluates them once, before the right-hand side.
func (l *lowerer) place(lhs syntax.NodeID) *Place {
	node := l.t.Nodes[lhs]
	out := &Place{Node: lhs, Type: l.typeOf(lhs)}
	switch node.Kind {
	case syntax.Paren:
		return l.place(syntax.NodeID(node.Lhs))
	case syntax.MemberExpr:
		out.Kind, out.Base, out.Index = PlaceField, l.expr(syntax.NodeID(node.Lhs)), l.r.Field(l.info.Uses[lhs]).Index
	case syntax.BracketExpr:
		out.Kind, out.Base, out.Idx = PlaceIndex, l.expr(syntax.NodeID(node.Lhs)), l.expr(l.t.Children(syntax.NodeID(node.Rhs))[0])
	default:
		out.Kind, out.Ent, out.Expr = PlaceLocal, l.info.Uses[lhs], l.expr(lhs)
	}
	return out
}
