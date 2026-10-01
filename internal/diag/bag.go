package diag

import (
	"cmp"
	"slices"
)

// Bag collects diagnostics in source order. Limit caps how many are kept so a
// broken file cannot flood the output; zero means unlimited.
type Bag struct {
	Limit int
	items []Diagnostic
	drop  int
}

func (b *Bag) Add(d Diagnostic) {
	if b.Limit > 0 && len(b.items) >= b.Limit {
		b.drop++
		return
	}
	b.items = append(b.items, d)
}

func (b *Bag) Len() int { return len(b.items) }

func (b *Bag) Dropped() int { return b.drop }

func (b *Bag) HasErrors() bool {
	return slices.ContainsFunc(b.items, func(d Diagnostic) bool { return d.Severity == Error })
}

// Sorted returns diagnostics ordered by position, errors before warnings at
// the same position.
func (b *Bag) Sorted() []Diagnostic {
	out := slices.Clone(b.items)
	slices.SortStableFunc(out, func(x, y Diagnostic) int {
		return cmp.Or(
			cmp.Compare(x.Loc.Span.Start, y.Loc.Span.Start),
			cmp.Compare(x.Severity, y.Severity),
		)
	})
	return out
}
