package vm

import "kigumi/internal/mir"

// frame is one activation: its locals.
type frame struct {
	f      *mir.Func
	locals []*obj
}

// call runs a MIR function on arguments handed over (owned) or lent.
func (m *Machine) call(f *mir.Func, args []*obj) *obj {
	fr := &frame{f: f, locals: make([]*obj, len(f.Locals))}
	for i, p := range f.Params {
		if i < len(args) {
			fr.locals[p] = args[i]
		}
	}
	m.stack = append(m.stack, f)
	m.depth++
	if m.opts.Sandbox && m.depth > m.opts.DepthLimit {
		m.abort("comptime recursion too deep")
	}
	defer func() {
		m.depth--
		m.stack = m.stack[:len(m.stack)-1]
	}()
	bid := mir.BlockID(0)
	for {
		blk := &f.Blocks[bid]
		for k := range blk.Insts {
			if m.opts.Sandbox {
				m.steps++
				if m.steps > m.opts.StepLimit {
					m.abort("comptime block took too long")
				}
			}
			m.exec(fr, &blk.Insts[k])
		}
		switch blk.Term.Op {
		case mir.TermJump:
			bid = blk.Term.Targets[0]
		case mir.TermBranch:
			c := truth(fr.locals[blk.Term.Args[0]])
			if c {
				bid = blk.Term.Targets[0]
			} else {
				bid = blk.Term.Targets[1]
			}
		case mir.TermReturn:
			return fr.locals[blk.Term.Args[0]]
		default:
			m.abort("unreachable code in " + f.Name)
		}
	}
}

func truth(v *obj) bool {
	v = deref(v)
	return v != nil && v.k == kBool && v.b
}
