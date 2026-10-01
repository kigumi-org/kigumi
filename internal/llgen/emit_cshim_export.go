package llgen

import (
	"fmt"
	"strings"

	"kigumi/internal/mir"
)

// exportShim defines the C symbol of an export(C) function whose records
// move by value, forwarding to kg_export_<name> (exportByval).
func (e *emitter) exportShim(f *mir.Func) {
	name := e.r.Entity(f.Ent).Name
	sn := e.r.Types.Node(e.r.Fn(f.Ent).Sig)
	ret := e.cLang(sn.Elem)
	outRet := e.isCRecord(sn.Elem)
	var params, innerParams, innerArgs []string
	if outRet {
		innerParams, innerArgs = append(innerParams, ret+" *"), append(innerArgs, "&out")
	}
	for i, a := range sn.Args {
		p := fmt.Sprintf("a%d", i)
		params = append(params, e.cLang(a)+" "+p)
		if e.isCRecord(a) {
			innerParams, innerArgs = append(innerParams, "const "+e.cLang(a)+" *"), append(innerArgs, "&"+p)
		} else {
			innerParams, innerArgs = append(innerParams, e.cLang(a)), append(innerArgs, p)
		}
	}
	innerRet := ret
	if outRet {
		innerRet = "void"
	}
	call := fmt.Sprintf("kg_export_%s(%s)", name, strings.Join(innerArgs, ", "))
	body := "    return " + call + ";\n"
	switch {
	case outRet:
		body = fmt.Sprintf("    %s out;\n    %s;\n    return out;\n", ret, call)
	case ret == "void":
		body = "    " + call + ";\n"
	}
	fmt.Fprintf(&e.shim.fns, "extern %s kg_export_%s(%s);\n%s %s(%s) {\n%s}\n", innerRet, name, orVoid(innerParams), ret, name, orVoid(params), body)
}

func orVoid(parts []string) string {
	if len(parts) == 0 {
		return "void"
	}
	return strings.Join(parts, ", ")
}

func sanitize(s string) string {
	var b strings.Builder
	for _, c := range s {
		if c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			b.WriteRune(c)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}
