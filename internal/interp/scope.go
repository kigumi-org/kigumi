package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

func (fr *frame) pushScope() { fr.scopes = append(fr.scopes, nil) }

// declare binds an entity in the innermost scope so its value is dropped
// at scope exit unless moved out.
func (fr *frame) declare(ent sem.EntityID, v Value) {
	fr.cells[ent] = &Cell{V: v}
	if len(fr.scopes) > 0 {
		fr.scopes[len(fr.scopes)-1] = append(fr.scopes[len(fr.scopes)-1], ent)
	}
}

// popScope drops the scope's unmoved resources in reverse order.
func (fr *frame) popScope() {
	if len(fr.scopes) == 0 {
		return
	}
	ents := fr.scopes[len(fr.scopes)-1]
	fr.scopes = fr.scopes[:len(fr.scopes)-1]
	for i := len(ents) - 1; i >= 0; i-- {
		c := fr.cells[ents[i]]
		if c == nil {
			continue
		}
		if _, moved := c.V.(Moved); moved {
			continue
		}
		if fr.in.r.Types.Kind(fr.in.r.Entity(ents[i]).Type) == sem.KRef {
			continue
		}
		if fr.in.moveOnly(c.V) {
			fr.dropDeep(c.V)
			c.V = Moved{}
		}
	}
}

// pushTempScope opens a scope for temporaries with no place of their own,
// e.g. the collection a `for x in &f()` head borrows.
func (fr *frame) pushTempScope() { fr.tempScopes = append(fr.tempScopes, nil) }

// registerTemp schedules v to be dropped when the innermost temp scope
// closes.
func (fr *frame) registerTemp(v Value) {
	i := len(fr.tempScopes) - 1
	fr.tempScopes[i] = append(fr.tempScopes[i], v)
}

// popTempScope drops the temp scope's values in reverse order.
func (fr *frame) popTempScope() {
	vs := fr.tempScopes[len(fr.tempScopes)-1]
	fr.tempScopes = fr.tempScopes[:len(fr.tempScopes)-1]
	for i := len(vs) - 1; i >= 0; i-- {
		fr.dropDeep(vs[i])
	}
}

// cell finds the storage of a binding, walking captured environments.
func (fr *frame) cell(ent sem.EntityID) *Cell {
	if c, ok := fr.cells[ent]; ok {
		return c
	}
	c := &Cell{V: Unit{}}
	fr.cells[ent] = c
	return c
}

func (fr *frame) typeOf(n syntax.NodeID) sem.TypeID { return fr.info.Types[n] }

// SetCwd sets the directory relative paths and commands resolve against.
func (in *Interp) SetCwd(dir string) { in.cwd = dir }
