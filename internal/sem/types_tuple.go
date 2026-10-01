package sem

import "kigumi/internal/syntax"

var tupleParamNames = []string{"A", "B", "C", "D", "E", "F", "G", "H"}

// declareTupleTypes predeclares Tuple2..Tuple8 as ordinary records so later passes need no tuple-specific case.
func (r *Result) declareTupleTypes(scope ScopeID) {
	for n := 2; n <= 8; n++ {
		r.Types.tupleEnt[n] = r.declareTupleRecord(scope, n)
	}
}

func (r *Result) declareTupleRecord(scope ScopeID, n int) EntityID {
	id := r.declareGenericType(scope, tupleName(n), tupleParamNames[:n], FormRecord)
	info := r.typeDecl(id)
	for i, prm := range info.Params {
		typ := r.Types.Param(prm)
		fid := r.newEntity(Entity{Kind: EntField, Name: tupleFieldName(i), Parent: id, Type: typ, Vis: Visibility{Level: VisPub}})
		r.Entities[fid].Detail = r.addField(FieldInfo{Index: i, Type: typ})
		info.Fields = append(info.Fields, fid)
	}
	return id
}

func tupleName(n int) string {
	return "Tuple" + itoa(n)
}

func tupleFieldName(i int) string {
	return "_" + itoa(i)
}

// resolveTupleType resolves a syntax.TypeTuple node to `TupleN[A, B, ...]`.
func (r *Result) resolveTupleType(f FileID, scope ScopeID, n syntax.NodeID, pos typePos) TypeID {
	t := r.tree(f)
	items := t.Children(n)
	ent, ok := r.Types.tupleEntity(len(items))
	if !ok {
		r.errAt(f, n, cTupleArity, len(items))
		for _, it := range items {
			r.resolveType(f, scope, it, posTypeArg)
		}
		return TyPoison
	}
	args := make([]TypeID, len(items))
	for i, it := range items {
		args[i] = r.resolveType(f, scope, it, posTypeArg)
	}
	return r.Types.Named(ent, args)
}
