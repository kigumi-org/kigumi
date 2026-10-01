package hir

import (
	"fmt"
	"strings"

	"kigumi/internal/sem"
)

// Dump renders a function so a test can pin what the checker decided:
// types, entities, indices, call forms and coercions.
func Dump(r *sem.Result, f *Func) string {
	d := &dumper{r: r}
	var params []string
	for _, p := range f.Params {
		params = append(params, p.Name+": "+r.TypeString(p.Type))
	}
	fmt.Fprintf(&d.sb, "fn %s(%s) -> %s\n", f.Name, strings.Join(params, ", "), r.TypeString(f.Ret))
	if f.Body != nil {
		d.expr(f.Body, 1)
	}
	if f.Main != nil {
		d.block(f.Main, 1)
	}
	return d.sb.String()
}

type dumper struct {
	r  *sem.Result
	sb strings.Builder
}

func (d *dumper) line(depth int, format string, args ...any) {
	d.sb.WriteString(strings.Repeat("  ", depth))
	fmt.Fprintf(&d.sb, format, args...)
	d.sb.WriteByte('\n')
}

func (d *dumper) name(ent sem.EntityID) string {
	if ent == 0 {
		return "?"
	}
	return d.r.Entity(ent).Name
}

func (d *dumper) block(b *Block, depth int) {
	for _, s := range b.Stmts {
		d.stmt(s, depth)
	}
	if b.Tail != nil {
		d.line(depth, "tail")
		d.expr(b.Tail, depth+1)
	}
}

func (d *dumper) stmt(s *Stmt, depth int) {
	switch s.Kind {
	case LetStmt:
		d.line(depth, "let %s", d.pat(s.Pat))
	case AssignStmt:
		op := "="
		if s.Compound {
			op = s.OpText + "="
		}
		d.line(depth, "%s %s", d.place(s.Place), op)
	case DiscardStmt:
		d.line(depth, "_ =")
	case ExprStmt, EvalStmt:
		d.line(depth, "stmt")
	case DeferStmt:
		kw := "defer"
		if s.Err {
			kw = "errdefer"
		}
		d.line(depth, "%s", kw)
		if s.Body != nil {
			d.stmt(s.Body, depth+1)
		}
	}
	if s.Expr != nil {
		d.expr(s.Expr, depth+1)
	}
	for _, b := range []*Block{s.Else, s.Block} {
		if b != nil {
			d.line(depth+1, "{")
			d.block(b, depth+2)
			d.line(depth+1, "}")
		}
	}
}

func (d *dumper) expr(e *Expr, depth int) {
	head := e.Kind.String()
	switch e.Kind {
	case Lit:
		head += " " + literalText(e.Lit)
	case Local, FnItem, Variant, Call, MethodCall, MethodValue, Record, Lambda:
		head += " " + d.name(e.Ent)
	case Unary, Binary, Intrinsic, Panic:
		head += " " + e.Name
	case Field, OptField:
		head += fmt.Sprintf(" .%d", e.Index)
	case Interp, Shell:
		head += fmt.Sprintf(" %q", strings.Join(e.Strs, "\x00"))
	case Is, IfLet:
		head += " " + d.pat(e.Pat)
	case Loop:
		if e.Pat != nil {
			head += " " + d.pat(e.Pat)
		}
	}
	if e.Witness != 0 {
		head += " witness=" + d.name(e.Witness)
	}
	head += ": " + d.r.TypeString(e.Type)
	for _, c := range e.Coerce {
		head += fmt.Sprintf(" ~%d->%s", c.Kind, d.r.TypeString(c.To))
	}
	d.line(depth, "%s", head)
	if e.Cond != nil {
		d.expr(e.Cond, depth+1)
	}
	for _, a := range e.Args {
		d.expr(a, depth+1)
	}
	for _, en := range e.Entries {
		if en.Spread {
			d.line(depth+1, "...")
		} else {
			d.line(depth+1, ".%d =", en.Index)
		}
		d.expr(en.Value, depth+2)
	}
	for _, arm := range e.Arms {
		d.line(depth+1, "arm %s", d.pat(arm.Pat))
		if arm.Guard != nil {
			d.expr(arm.Guard, depth+2)
		}
		d.expr(arm.Body, depth+2)
	}
	if e.Guard != nil {
		d.expr(e.Guard, depth+1)
	}
	for _, b := range []*Block{e.Then, e.Block} {
		if b != nil {
			d.line(depth+1, "{")
			d.block(b, depth+2)
			d.line(depth+1, "}")
		}
	}
	if e.Else != nil {
		d.line(depth+1, "else")
		d.expr(e.Else, depth+2)
	}
	if e.Fn != nil {
		d.expr(e.Fn.Body, depth+1)
	}
}

func literalText(l sem.Literal) string {
	switch l.Kind {
	case sem.LitInt:
		return l.Int.String()
	case sem.LitFloat:
		return l.Float.Text('g', -1)
	case sem.LitString:
		return fmt.Sprintf("%q", l.Str)
	case sem.LitBool:
		return fmt.Sprint(l.Bool)
	case sem.LitChar:
		return fmt.Sprintf("%q", l.Char)
	}
	return "?"
}
