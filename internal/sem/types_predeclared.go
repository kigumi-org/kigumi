package sem

var primEntities = []struct {
	name string
	id   TypeID
}{
	{"Unit", TyUnit}, {"Never", TyNever}, {"Bool", TyBool}, {"Char", TyChar}, {"String", TyString}, {"Bytes", TyBytes},
	{"i8", TyI8}, {"i16", TyI16}, {"i32", TyI32}, {"i64", TyI64}, {"i128", TyI128},
	{"u8", TyU8}, {"u16", TyU16}, {"u32", TyU32}, {"u64", TyU64}, {"u128", TyU128},
	{"usize", TyUsize}, {"isize", TyIsize}, {"f32", TyF32}, {"f64", TyF64},
}

var primAliases = []struct {
	name string
	id   TypeID
}{{"Int", TyI64}, {"Float", TyF64}, {"byte", TyU8}}

func (r *Result) declareUniverse() {
	u := r.universe
	for _, p := range primEntities {
		id := r.newEntity(Entity{Kind: EntType, Name: p.name, Type: p.id, Vis: Visibility{Level: VisPub}})
		r.Entities[id].Detail = r.addTypeDecl(TypeDeclInfo{Form: FormOpaque, Members: map[string]OverloadSetID{}, Ops: map[string]OverloadSetID{}, Size: sizeFinite})
		r.define(u, p.name, Binding{Ent: id})
		r.primEntity[p.id] = id
	}
	for _, a := range primAliases {
		id := r.newEntity(Entity{Kind: EntAlias, Name: a.name, Type: a.id, Vis: Visibility{Level: VisPub}})
		r.define(u, a.name, Binding{Ent: id})
	}
	r.Types.optionEnt = r.declareAdt(u, "Option", []string{"T"}, []adtVariant{{"Some", []int{0}}, {"None", nil}})
	r.Types.resultEnt = r.declareAdt(u, "Result", []string{"T", "E"}, []adtVariant{{"Ok", []int{0}}, {"Err", []int{1}}})
	r.declareTupleTypes(u)
	r.Types.arrayEnt = r.declareGenericType(u, "Array", []string{"T"}, FormOpaque)
	r.Types.futureEnt = r.declareGenericType(u, "Future", []string{"T"}, FormOpaque)
	r.Types.errorEnt = r.declareIface(u, "Error", nil)
	r.langItems["Error"] = r.Types.errorEnt
	r.langItems["Index"] = r.declareIface(u, "Index", []string{"I", "O"})
	for _, name := range []string{"Copy", "Send", "Sync", "Fn", "FnMut", "FnOnce"} {
		id := r.newEntity(Entity{Kind: EntConstraint, Name: name, Vis: Visibility{Level: VisPub}})
		r.define(u, name, Binding{Ent: id})
	}
	r.declareIntrinsic(u, "panic", 0)
}

type adtVariant struct {
	name    string
	payload []int // indices into the type parameters
}

func (r *Result) declareGenericType(scope ScopeID, name string, params []string, form TypeForm) EntityID {
	id := r.newEntity(Entity{Kind: EntType, Name: name, Vis: Visibility{Level: VisPub}})
	info := TypeDeclInfo{Form: form, Members: map[string]OverloadSetID{}, Ops: map[string]OverloadSetID{}, Size: sizeFinite}
	for i, p := range params {
		pid := r.newEntity(Entity{Kind: EntTypeParam, Name: p, Parent: id, Vis: Visibility{Level: VisPub}})
		r.Entities[pid].Detail = r.addTypeParam(TypeParamInfo{Index: i})
		r.Entities[pid].Type = r.Types.Param(pid)
		info.Params = append(info.Params, pid)
	}
	r.Entities[id].Detail = r.addTypeDecl(info)
	r.define(scope, name, Binding{Ent: id})
	return id
}

func (r *Result) declareAdt(scope ScopeID, name string, params []string, variants []adtVariant) EntityID {
	id := r.declareGenericType(scope, name, params, FormAdt)
	info := r.typeDecl(id)
	for i, v := range variants {
		vid := r.newEntity(Entity{Kind: EntVariant, Name: v.name, Parent: id, Vis: Visibility{Level: VisPub}})
		vi := VariantInfo{Index: i}
		for _, p := range v.payload {
			vi.Payload = append(vi.Payload, r.Types.Param(info.Params[p]))
			vi.Names = append(vi.Names, "")
		}
		r.Entities[vid].Detail = r.addVariant(vi)
		info.Variants = append(info.Variants, vid)
		r.define(scope, v.name, Binding{Ent: vid})
	}
	return id
}

func (r *Result) declareIface(scope ScopeID, name string, params []string) EntityID {
	id := r.newEntity(Entity{Kind: EntInterface, Name: name, Vis: Visibility{Level: VisPub}})
	info := InterfaceInfo{}
	for i, p := range params {
		pid := r.newEntity(Entity{Kind: EntTypeParam, Name: p, Parent: id, Vis: Visibility{Level: VisPub}})
		r.Entities[pid].Detail = r.addTypeParam(TypeParamInfo{Index: i})
		r.Entities[pid].Type = r.Types.Param(pid)
		info.Params = append(info.Params, pid)
	}
	self := r.newEntity(Entity{Kind: EntTypeParam, Name: "Self", Parent: id, Vis: Visibility{Level: VisPub}})
	r.Entities[self].Detail = r.addTypeParam(TypeParamInfo{Index: -1})
	r.Entities[self].Type = r.Types.Param(self)
	info.SelfParam = self
	r.Entities[id].Detail = r.addIface(info)
	r.define(scope, name, Binding{Ent: id})
	return id
}

func (r *Result) declareIntrinsic(scope ScopeID, name string, flags EntityFlags) EntityID {
	id := r.newEntity(Entity{Kind: EntIntrinsic, Name: name, Vis: Visibility{Level: VisPub}, Flags: flags})
	r.define(scope, name, Binding{Ent: id})
	return id
}

func (r *Result) lookupUniverse(name string) EntityID {
	b, _, ok := r.lookup(r.universe, name)
	if !ok {
		return 0
	}
	return b.Ent
}
