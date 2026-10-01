package sem

import "kigumi/internal/syntax"

// unsatKey is a (call node, argument, interface) triple: witnessArg looks it
// up to skip a constraint failure checkConstraints already reported.
type unsatKey struct {
	node       syntax.NodeID
	arg, iface TypeID
}

// markUnsatisfied records that arg was already reported against iface at
// node, so witnessArg does not report the same failure again.
func (c *checker) markUnsatisfied(node syntax.NodeID, arg, iface TypeID) {
	if c.unsatIface == nil {
		c.unsatIface = map[unsatKey]bool{}
	}
	c.unsatIface[unsatKey{node, arg, iface}] = true
}

func (c *checker) alreadyUnsatisfied(node syntax.NodeID, arg, iface TypeID) bool {
	return c.unsatIface[unsatKey{node, arg, iface}]
}
