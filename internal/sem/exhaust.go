package sem

// pat is the abstract pattern of the usefulness algorithm (Maranget 2007):
// a wildcard, a constructor with sub-patterns, or a literal from an
// unbounded domain.
type pat struct {
	kind uint8
	key  string
	name string
	args []pat
}

const (
	pWild uint8 = iota
	pCtor
	pLit
	pRange
	pErr
)

func hasErr(p pat) bool {
	if p.kind == pErr {
		return true
	}
	for _, a := range p.args {
		if hasErr(a) {
			return true
		}
	}
	return false
}

// ctorDesc is one constructor of a column type with its argument types.
type ctorDesc struct {
	key  string
	name string
	args []TypeID
}

func wilds(n int) []pat {
	out := make([]pat, n)
	return out
}

func (d ctorDesc) pat() pat {
	return pat{kind: pCtor, key: d.key, name: d.name, args: wilds(len(d.args))}
}

// useful reports whether row q matches some value that no row of P
// matches, over columns of the given types.
func (c *checker) useful(P [][]pat, q []pat, types []TypeID) bool {
	if len(q) == 0 {
		return len(P) == 0
	}
	h := q[0]
	if h.kind != pWild {
		args := c.argTypes(types[0], h)
		return c.useful(specialize(P, h), append(append([]pat{}, h.args...), q[1:]...), append(args, types[1:]...))
	}
	set, complete := c.ctorSet(types[0])
	if complete && allPresent(P, set) {
		for _, d := range set {
			if c.useful(specialize(P, d.pat()), append(wilds(len(d.args)), q[1:]...), append(append([]TypeID{}, d.args...), types[1:]...)) {
				return true
			}
		}
		return false
	}
	return c.useful(defaultMatrix(P), q[1:], types[1:])
}

// missing returns a witness row that P leaves uncovered, or ok when P is
// exhaustive over the column types.
func (c *checker) missing(P [][]pat, types []TypeID) ([]pat, bool) {
	if len(types) == 0 {
		return nil, len(P) > 0
	}
	set, complete := c.ctorSet(types[0])
	if complete && allPresent(P, set) {
		for _, d := range set {
			w, ok := c.missing(specialize(P, d.pat()), append(append([]TypeID{}, d.args...), types[1:]...))
			if !ok {
				head := pat{kind: pCtor, key: d.key, name: d.name, args: w[:len(d.args)]}
				return append([]pat{head}, w[len(d.args):]...), false
			}
		}
		return nil, true
	}
	w, ok := c.missing(defaultMatrix(P), types[1:])
	if ok {
		return nil, true
	}
	head := pat{kind: pWild}
	if complete {
		heads := headKeys(P)
		for _, d := range set {
			if !heads[d.key] {
				head = d.pat()
				break
			}
		}
	}
	return append([]pat{head}, w...), false
}

// specialize keeps the rows whose head is h (or a wildcard) and expands
// the head into its sub-patterns.
func specialize(P [][]pat, h pat) [][]pat {
	var out [][]pat
	for _, row := range P {
		r := row[0]
		switch {
		case r.kind == pWild:
			out = append(out, append(wilds(len(h.args)), row[1:]...))
		case r.key == h.key:
			out = append(out, append(append([]pat{}, r.args...), row[1:]...))
		}
	}
	return out
}

func defaultMatrix(P [][]pat) [][]pat {
	var out [][]pat
	for _, row := range P {
		if row[0].kind == pWild {
			out = append(out, row[1:])
		}
	}
	return out
}

func headKeys(P [][]pat) map[string]bool {
	keys := map[string]bool{}
	for _, row := range P {
		if row[0].kind != pWild {
			keys[row[0].key] = true
		}
	}
	return keys
}

func allPresent(P [][]pat, set []ctorDesc) bool {
	heads := headKeys(P)
	for _, d := range set {
		if !heads[d.key] {
			return false
		}
	}
	return true
}
