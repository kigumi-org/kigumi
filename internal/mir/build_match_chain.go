package mir

import "kigumi/internal/sem"

// andThen evaluates the next test only when the previous one succeeded.
func (b *builder) andThen(cond LocalID, next func() LocalID) LocalID {
	result := b.temp(sem.TyBool)
	b.assign(result, cond)
	more, join := b.newBlockAt(), b.newBlockAt()
	b.term(Term{Op: TermBranch, Args: []LocalID{cond}, Targets: []BlockID{more, join}})
	b.cur = more
	b.assign(result, next())
	b.jump(join)
	b.cur = join
	return result
}

// orElse evaluates the next test only when the previous one failed.
func (b *builder) orElse(cond LocalID, next func() LocalID) LocalID {
	result := b.temp(sem.TyBool)
	b.assign(result, cond)
	more, join := b.newBlockAt(), b.newBlockAt()
	b.term(Term{Op: TermBranch, Args: []LocalID{cond}, Targets: []BlockID{join, more}})
	b.cur = more
	b.assign(result, next())
	b.jump(join)
	b.cur = join
	return result
}
