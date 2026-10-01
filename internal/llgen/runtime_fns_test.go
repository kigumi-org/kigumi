package llgen

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// cDef is a parsed C function signature, parameter names stripped off.
type cDef struct {
	ret    string
	params []string
}

var defRe = regexp.MustCompile(`(?m)^([A-Za-z_][A-Za-z0-9_ ]*\*?)\s+(rt_[A-Za-z0-9_]+|display)\s*\(([^)]*)\)\s*\{`)

func parseCDefs(src string) map[string]cDef {
	out := map[string]cDef{}
	for _, m := range defRe.FindAllStringSubmatch(src, -1) {
		ret, name, rawParams := strings.TrimSpace(m[1]), m[2], strings.TrimSpace(m[3])
		var params []string
		if rawParams != "" && rawParams != "void" {
			for _, p := range strings.Split(rawParams, ",") {
				fields := strings.Fields(p)
				if len(fields) == 0 {
					continue
				}
				params = append(params, strings.Join(fields[:len(fields)-1], " "))
			}
		}
		out[name] = cDef{ret: ret, params: params}
	}
	return out
}

func cTypeMatches(t Type, c string) bool {
	switch t {
	case TPtr:
		return c == "val" || c == "adapter_fn" || strings.HasSuffix(c, "*")
	case TI32, TI1:
		return c == "int" || c == "uint32_t"
	case TI64:
		return c == "int64_t"
	case TDouble:
		return c == "double"
	case TVoid:
		return c == "void"
	}
	return false
}

// TestRuntimeFnsMatchC cross-checks the runtimeFns table against the C
// runtime it declares.
func TestRuntimeFnsMatchC(t *testing.T) {
	src, err := os.ReadFile("runtime/rt_core.c")
	if err != nil {
		t.Fatal(err)
	}
	defs := parseCDefs(string(src))
	if len(defs) < len(runtimeFns) {
		t.Fatalf("only parsed %d C definitions for %d table entries; regex drifted from rt_core.c", len(defs), len(runtimeFns))
	}
	for _, fn := range runtimeFns {
		def, ok := defs[fn.Name]
		if !ok {
			t.Errorf("%s: declared by llgen but not defined in rt_core.c", fn.Name)
			continue
		}
		if len(def.params) != len(fn.Params) {
			t.Errorf("%s: table has %d parameter(s), C definition has %d (%v)", fn.Name, len(fn.Params), len(def.params), def.params)
			continue
		}
		for i, p := range def.params {
			if !cTypeMatches(fn.Params[i], p) {
				t.Errorf("%s: parameter %d is %q in C, %s in the table", fn.Name, i, p, fn.Params[i])
			}
		}
		if !cTypeMatches(fn.Ret, def.ret) {
			t.Errorf("%s: returns %q in C, %s in the table", fn.Name, def.ret, fn.Ret)
		}
	}
}
