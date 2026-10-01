package sem

// Send/Sync: raw pointers, erased fn values, existential boxes and executor handles
// never qualify; std opaque types use the table below.

func (r *Result) isSend(t TypeID) bool { return r.threadProp(t, false) }
func (r *Result) isSync(t TypeID) bool { return r.threadProp(t, true) }

type threadKey struct {
	t    TypeID
	sync bool
}

// a type under computation counts as qualifying, so recursion resolves via the other parts;
// such a provisional true is not memoized, since the open type may still prove false.
func (r *Result) threadProp(t TypeID, sync bool) bool {
	if t == 0 {
		return true
	}
	p := &r.Types.props[t]
	memo := &p.send
	if sync {
		memo = &p.sync
	}
	switch *memo {
	case 1:
		return true
	case -1:
		return false
	}
	key := threadKey{t, sync}
	if depth, open := r.threadOpen[key]; open {
		r.threadLow = min(r.threadLow, depth)
		return true
	}
	if r.threadOpen == nil {
		r.threadOpen = map[threadKey]int{}
	}
	depth := len(r.threadOpen)
	r.threadOpen[key] = depth
	outerLow := r.threadLow
	r.threadLow = depth
	ok := r.computeThread(t, sync)
	low := r.threadLow
	delete(r.threadOpen, key)
	r.threadLow = min(outerLow, low)
	switch {
	case !ok:
		*memo = -1
	case low >= depth:
		*memo = 1
	}
	return ok
}

// stdThread: per type, (sendable, shareable when it holds no mutable state).
var stdThread = map[string][2]bool{
	"Array": {true, true}, "Shared": {false, false}, "Weak": {false, false},
	"Path": {true, true}, "Instant": {true, true}, "Files": {true, true}, "Args": {true, true},
	"Net": {true, true}, "Http": {true, true}, "Shell": {true, true}, "Host": {true, true}, "Loader": {true, true},
	"AllocatorHandle": {true, true}, "Metadata": {true, true},
	"Stdin": {true, false},
}

func (r *Result) computeThread(t TypeID, sync bool) bool {
	tt := r.Types
	n := tt.Node(t)
	switch n.Kind {
	case KPoison, KUntyped, KPrim:
		return true
	case KRef:
		// A borrow cannot be sent (v2.5); it is shareable when its target is.
		return sync && r.threadProp(n.Elem, true)
	case KParam:
		want := CSend
		if sync {
			want = CSync
		}
		for _, c := range r.typeParam(n.Ent).Constraints {
			if c.Kind == want {
				return true
			}
		}
		return false
	case KClosure:
		info := r.closure(n.Ent)
		for _, cap := range info.Captures {
			if cap.Mode == capCell || !r.threadProp(r.Entities[cap.Local].Type, sync) {
				return false
			}
		}
		return true
	case KNamed:
	default:
		return false
	}
	for _, a := range n.Args {
		if !r.threadProp(a, sync) {
			return false
		}
	}
	e := &r.Entities[n.Ent]
	info := r.typeDecl(n.Ent)
	if info.Form == FormOpaque {
		if e.File != 0 && e.Flags&EfStd == 0 {
			return false
		}
		flags, ok := stdThread[e.Name]
		return ok && flags[boolIndex(sync)]
	}
	if sync && info.Form == FormResource {
		return false
	}
	subst := map[EntityID]TypeID{}
	for i, p := range info.Params {
		if i < len(n.Args) {
			subst[p] = n.Args[i]
		}
	}
	for _, fld := range info.Fields {
		if !r.threadProp(tt.Subst(r.Entities[fld].Type, subst), sync) {
			return false
		}
	}
	for _, v := range info.Variants {
		for _, p := range r.variant(v).Payload {
			if !r.threadProp(tt.Subst(p, subst), sync) {
				return false
			}
		}
	}
	return true
}

func boolIndex(b bool) int {
	if b {
		return 1
	}
	return 0
}
