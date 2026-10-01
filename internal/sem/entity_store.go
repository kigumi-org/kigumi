package sem

// detail constructors; each returns the index stored in Entity.Detail.

func (r *Result) addTypeDecl(info TypeDeclInfo) uint32 {
	r.TypeDecls = append(r.TypeDecls, info)
	return uint32(len(r.TypeDecls) - 1)
}

func (r *Result) addFn(info FnInfo) uint32 {
	r.Fns = append(r.Fns, info)
	return uint32(len(r.Fns) - 1)
}

func (r *Result) addIface(info InterfaceInfo) uint32 {
	r.Ifaces = append(r.Ifaces, info)
	return uint32(len(r.Ifaces) - 1)
}

func (r *Result) addConst(info ConstInfo) uint32 {
	r.Consts = append(r.Consts, info)
	return uint32(len(r.Consts) - 1)
}

func (r *Result) addLocal(info LocalInfo) uint32 {
	r.Locals = append(r.Locals, info)
	return uint32(len(r.Locals) - 1)
}

func (r *Result) addClosure(info ClosureInfo) uint32 {
	r.Closures = append(r.Closures, info)
	return uint32(len(r.Closures) - 1)
}

func (r *Result) addField(info FieldInfo) uint32 {
	r.Fields = append(r.Fields, info)
	return uint32(len(r.Fields) - 1)
}

func (r *Result) addVariant(info VariantInfo) uint32 {
	r.Variants = append(r.Variants, info)
	return uint32(len(r.Variants) - 1)
}

func (r *Result) addTypeParam(info TypeParamInfo) uint32 {
	r.TypeParams = append(r.TypeParams, info)
	return uint32(len(r.TypeParams) - 1)
}

func (r *Result) addOverloadSet(set OverloadSet) OverloadSetID {
	r.Overloads = append(r.Overloads, set)
	return OverloadSetID(len(r.Overloads) - 1)
}
