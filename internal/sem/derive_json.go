package sem

import (
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// Derived methods (toJson/fromJson/equals/hash) are
// generated as Kigumi source so both backends run ordinary code with no
// type metadata at runtime; std types implement them by hand instead.

type deriveRequest struct {
	decl  EntityID
	iface string // "Encode", "Decode", "Eq" or "Hash"
}

// deriveMethods names the method each derivable interface requires.
var deriveMethods = map[string]string{"Encode": "toJson", "Decode": "fromJson", "Eq": "equals", "Hash": "hash"}

type generatedFile struct {
	pkg  int
	name string
	text string
}

// deriveIface names a derivable interface (std/json's Encode / Decode,
// the prelude's Eq / Hash), or "".
func (r *Result) deriveIface(iface TypeID) string {
	if r.Types.Kind(iface) != KIface {
		return ""
	}
	e := r.Entities[r.Types.Node(iface).Ent]
	switch r.Packages[e.Pkg].Path {
	case "std/json":
		if e.Name == "Encode" || e.Name == "Decode" {
			return e.Name
		}
	case "std/prelude":
		if e.Name == "Eq" || e.Name == "Hash" {
			return e.Name
		}
	}
	return ""
}

// requestDerive records that t needs a generated method for iface and
// reports whether one can be generated.
func (r *Result) requestDerive(t, iface TypeID) bool {
	name := r.deriveIface(iface)
	if name == "" || r.mod.derived {
		return false
	}
	n := r.Types.Node(t)
	if n.Kind != KNamed || (r.Packages[r.Entities[n.Ent].Pkg].Std && !isProto(name)) {
		return false
	}
	if form := r.typeDecl(n.Ent).Form; form != FormRecord && form != FormAdt && !(form == FormResource && !isProto(name)) {
		return false
	}
	for _, d := range r.derives {
		if d.decl == n.Ent && d.iface == name {
			return true
		}
	}
	r.derives = append(r.derives, deriveRequest{decl: n.Ent, iface: name})
	return true
}

func isProto(iface string) bool { return iface == "Eq" || iface == "Hash" }

// deriveSources generates one JSON file and one protocol file per package,
// covering the requested types and their fields transitively.
func (r *Result) deriveSources() []generatedFile {
	g := &deriver{r: r, done: map[deriveRequest]bool{}, texts: map[PackageID]string{}, protos: map[PackageID]string{}}
	for _, d := range r.derives {
		g.derive(d)
	}
	var out []generatedFile
	for pkg := 1; pkg < len(r.Packages); pkg++ {
		dir := ""
		for _, f := range r.Packages[pkg].Files {
			dir = dirOf(r.Files[f].Tree.File.Name)
			break
		}
		if text, ok := g.texts[PackageID(pkg)]; ok {
			out = append(out, generatedFile{pkg: r.moduleIndex(PackageID(pkg)), name: dir + "json.derived.kg", text: "import json from std/json\n" + text})
		}
		if text, ok := g.protos[PackageID(pkg)]; ok {
			out = append(out, generatedFile{pkg: r.moduleIndex(PackageID(pkg)), name: dir + "proto.derived.kg", text: text})
		}
	}
	return out
}

func (r *Result) moduleIndex(pkg PackageID) int {
	for i, p := range r.mod.Packages {
		if p.Path == r.Packages[pkg].Path {
			return i
		}
	}
	return 0
}

func dirOf(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' {
			return name[:i+1]
		}
	}
	return ""
}

// Check runs the passes, then re-runs them with generated derive sources
// added if any type needed one.
func Check(mod *Module) *Result {
	r := check(mod)
	if len(r.derives) == 0 || mod.derived {
		return r
	}
	mod.derived = true
	for _, g := range r.deriveSources() {
		f := token.NewFile(g.name, []byte(g.text))
		f.Generated = true
		mod.Packages[g.pkg].Files = append(mod.Packages[g.pkg].Files, syntax.Parse(f))
	}
	return check(mod)
}
