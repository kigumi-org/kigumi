package sem

func (r *Result) typeDecl(id EntityID) *TypeDeclInfo {
	return &r.TypeDecls[r.Entities[id].Detail]
}

func (r *Result) Fn(id EntityID) *FnInfo       { return &r.Fns[r.Entities[id].Detail] }
func (r *Result) Const(id EntityID) *ConstInfo { return r.constInfo(id) }

func (r *Result) iface(id EntityID) *InterfaceInfo { return &r.Ifaces[r.Entities[id].Detail] }

func (r *Result) constInfo(id EntityID) *ConstInfo { return &r.Consts[r.Entities[id].Detail] }

func (r *Result) local(id EntityID) *LocalInfo { return &r.Locals[r.Entities[id].Detail] }

func (r *Result) closure(id EntityID) *ClosureInfo { return &r.Closures[r.Entities[id].Detail] }

func (r *Result) field(id EntityID) *FieldInfo { return &r.Fields[r.Entities[id].Detail] }

func (r *Result) variant(id EntityID) *VariantInfo { return &r.Variants[r.Entities[id].Detail] }

func (r *Result) typeParam(id EntityID) *TypeParamInfo { return &r.TypeParams[r.Entities[id].Detail] }
