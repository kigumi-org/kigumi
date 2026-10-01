package sem

import "kigumi/internal/syntax"

// conservative by design, false positives accepted: a `mut self` call from outside
// the capturing closure is rejected; a call from inside it creates the cell, so it's allowed.
func (c *checker) checkCapturedMutConflict(base syntax.NodeID, root EntityID) {
	if c.capturedHere(root) {
		c.noteCaptureWrite(base, root)
		return
	}
	if c.r.Entities[root].Flags&EfCapturedMut != 0 {
		c.errAt(base, cCapturedMutSelfConflict, c.r.Entities[root].Name)
	}
}
