package interp

import (
	"fmt"

	"kigumi/internal/diag"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// RunMain runs the implicit main of the entry package and returns the
// process exit code: 0, 1 for an unhandled Err, 2 for a panic.
func (in *Interp) RunMain() (code int) { return in.RunEntry("") }

// RunEntry runs the entry file's main, implicit (bare top-level statements)
// or explicit (`fn main() -> Unit!` / `fn main(host: Host) -> Unit!`); ""
// takes the module's only script. This mirrors sem.Result.MainFn(), which
// mir.Build already uses for the VM and AOT entries.
func (in *Interp) RunEntry(entry string) (code int) {
	var main sem.EntityID
	explicit := false
	for _, id := range in.r.ImplicitMains() {
		if entry == "" || in.r.Tree(in.r.Entity(id).File).File.Name == entry {
			main = id
		}
	}
	if main == 0 {
		if id := in.r.MainFn(); id != 0 {
			if entry == "" || in.r.Tree(in.r.Entity(id).File).File.Name == entry {
				main, explicit = id, true
			}
		}
	}
	if main == 0 {
		fmt.Fprintln(in.stderr, "error: no entry file")
		return 1
	}
	defer func() {
		if rec := recover(); rec != nil {
			if p, ok := rec.(*Panic); ok {
				fmt.Fprintln(in.stderr, p.Error())
				code = 2
				return
			}
			panic(rec)
		}
	}()
	e := in.r.Entity(main)
	if explicit {
		var args []Value
		if len(in.r.Fn(main).Params) == 1 {
			args = []Value{hostValue()}
		}
		v, _ := in.callFnValue(nil, main, e.Node, args, nil)
		return in.finishMain(&ctrl{kind: ctrlReturn, val: v})
	}
	fr := in.newFrame(main, e.File)
	t := fr.t
	var c *ctrl
	fr.pushScope()
	for _, d := range t.Children(t.Root) {
		if !isStatement(t.Kind(d)) {
			continue
		}
		if _, c = fr.stmt(d); c != nil {
			break
		}
	}
	c = fr.exitScope(c)
	fr.popScope()
	return in.finishMain(c)
}

func (in *Interp) finishMain(c *ctrl) int {
	if c == nil || c.kind == ctrlReturn && c.val == nil {
		return 0
	}
	var errVal Value
	switch c.kind {
	case ctrlFail:
		errVal = c.val
	case ctrlReturn:
		if v, ok := c.val.(*Variant); ok && in.r.Entity(v.V).Name == "Err" {
			errVal = v.Payload[0]
		}
	}
	if errVal == nil {
		return 0
	}
	fmt.Fprintf(in.stderr, "%s: %s\n", diag.Prefix("error"), in.errorMessage(errVal))
	return 1
}

func isStatement(k syntax.NodeKind) bool {
	switch k {
	case syntax.LetStmt, syntax.AssignStmt, syntax.ExprStmt, syntax.DeferStmt, syntax.ErrdeferStmt:
		return true
	}
	return false
}
