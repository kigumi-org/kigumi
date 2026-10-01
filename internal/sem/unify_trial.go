package sem

func (v *varStore) mark() int { return len(v.trail) }

// pendingLiteral reports whether t is a variable that will settle to a
// numeric default.
func (v *varStore) pendingLiteral(t TypeID) bool {
	i, ok := v.index(v.resolve(t))
	return ok && v.pending[i] != 0
}

func (v *varStore) count() int { return len(v.binding) }

func (v *varStore) rollback(mark int) {
	for len(v.trail) > mark {
		i := v.trail[len(v.trail)-1]
		v.trail = v.trail[:len(v.trail)-1]
		v.binding[i] = 0
	}
}

// poisonFrom retires the trial variables created since start.
func (v *varStore) poisonFrom(start int) {
	for i := start; i < len(v.binding); i++ {
		if v.binding[i] == 0 {
			v.binding[i] = TyPoison
		}
	}
}
