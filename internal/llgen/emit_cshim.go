package llgen

import (
	"fmt"
	"strings"

	"kigumi/internal/sem"
)

// cshim is C source generated so a real C compiler, not us, classifies
// layout(C) records moved by value; field names are positional only.
type cshim struct {
	types, fns strings.Builder
	structs    map[sem.EntityID]bool
	sigs       map[string]string
}

func newCshim() *cshim { return &cshim{structs: map[sem.EntityID]bool{}, sigs: map[string]string{}} }

func (s *cshim) text() string {
	if s.fns.Len() == 0 {
		return ""
	}
	return "#include <stdint.h>\n#include <stddef.h>\n\n" + s.types.String() + "\n" + s.fns.String()
}

func (e *emitter) cStruct(ent sem.EntityID) string {
	x := e.r.Entity(ent)
	name := "kg_" + sanitize(e.r.Packages[x.Pkg].Path) + "_" + x.Name
	if e.shim.structs[ent] {
		return name
	}
	e.shim.structs[ent] = true
	info := e.r.TypeDecl(ent)
	var fields strings.Builder
	for i, f := range info.Fields {
		fmt.Fprintf(&fields, "    %s f%d;\n", e.cLang(e.r.Entity(f).Type), i)
	}
	attr := ""
	if info.Layout == "packed" {
		attr = " __attribute__((packed))"
	}
	fmt.Fprintf(&e.shim.types, "struct %s {\n%s}%s;\n", name, fields.String(), attr)
	return name
}

func (e *emitter) cLang(t sem.TypeID) string {
	tt := e.r.Types
	switch {
	case t == sem.TyUnit, t == sem.TyNever:
		return "void"
	case t == sem.TyUsize:
		return "size_t"
	case t == sem.TyIsize:
		return "ptrdiff_t"
	case tt.IsFloat(t):
		if tt.Width(t) == 32 {
			return "float"
		}
		return "double"
	case tt.IsInteger(t) && tt.IsSigned(t):
		return fmt.Sprintf("int%d_t", tt.Width(t))
	case tt.IsInteger(t):
		return fmt.Sprintf("uint%d_t", tt.Width(t))
	case e.isCRecord(t):
		return "struct " + e.cStruct(tt.Node(t).Ent)
	}
	return "void *"
}

// shimFn defines shim(fp?, out?, a0, a1, ...): records pass by pointer,
// and viaPtr calls through the fp parameter instead of callee.
func (e *emitter) shimFn(shim string, sig sem.TypeID, callee string, viaPtr bool) {
	sn := e.r.Types.Node(sig)
	var params, llParams, callArgs, ptypes []string
	if viaPtr {
		params, llParams = append(params, "void *fp"), append(llParams, "ptr")
	}
	ret := e.cLang(sn.Elem)
	outRet := e.isCRecord(sn.Elem)
	if outRet {
		params, llParams = append(params, ret+" *out"), append(llParams, "ptr")
	}
	for i, a := range sn.Args {
		p := fmt.Sprintf("a%d", i)
		ptypes = append(ptypes, e.cLang(a))
		if e.isCRecord(a) {
			params, llParams, callArgs = append(params, "const "+e.cLang(a)+" *"+p), append(llParams, "ptr"), append(callArgs, "*"+p)
		} else {
			params, llParams, callArgs = append(params, e.cLang(a)+" "+p), append(llParams, e.cType(a)), append(callArgs, p)
		}
	}
	body := ""
	if viaPtr {
		body = fmt.Sprintf("    %s (*f)(%s) = fp;\n", ret, orVoid(ptypes))
		callee = "f"
	}
	call := fmt.Sprintf("%s(%s)", callee, strings.Join(callArgs, ", "))
	switch {
	case outRet:
		body += "    *out = " + call + ";\n"
		ret = "void"
	case ret == "void":
		body += "    " + call + ";\n"
	default:
		body += "    return " + call + ";\n"
	}
	fmt.Fprintf(&e.shim.fns, "%s %s(%s) {\n%s}\n", ret, shim, orVoid(params), body)
	llRet := "void"
	if !outRet {
		llRet = e.cType(sn.Elem)
	}
	e.foreign[shim] = fmt.Sprintf("declare %s @%s(%s)", llRet, shim, strings.Join(llParams, ", "))
}

func (e *emitter) byvalShim(ent sem.EntityID) string {
	name := e.r.Entity(ent).Name
	shim := "kg_byval_" + name
	if _, done := e.shim.sigs[shim]; done {
		return shim
	}
	e.shim.sigs[shim] = shim
	sn := e.r.Types.Node(e.r.Fn(ent).Sig)
	var ptypes []string
	for _, a := range sn.Args {
		ptypes = append(ptypes, e.cLang(a))
	}
	fmt.Fprintf(&e.shim.fns, "extern %s %s(%s);\n", e.cLang(sn.Elem), name, orVoid(ptypes))
	e.shimFn(shim, e.r.Fn(ent).Sig, name, false)
	return shim
}

func (e *emitter) callShim(sig sem.TypeID) string {
	key := e.r.TypeString(sig)
	if s, ok := e.shim.sigs[key]; ok {
		return s
	}
	shim := fmt.Sprintf("kg_call_%d", len(e.shim.sigs))
	e.shim.sigs[key] = shim
	e.shimFn(shim, sig, "", true)
	return shim
}
