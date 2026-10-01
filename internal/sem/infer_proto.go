package sem

import (
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

func (c *checker) protoCall(n syntax.NodeID, t TypeID, proto, method string, op token.Kind) bool {
	tt := c.r.Types
	iface := c.r.langItem(c.f, n, proto)
	if iface == 0 {
		return false
	}
	req := c.r.requirement(iface, method)
	if req == 0 {
		return false
	}
	call := CallInfo{Kind: CallMethod, Callee: req, Recv: RecvSelf, Op: op, OpText: op.String()}
	switch tt.Kind(t) {
	case KParam:
		call.Witness = c.witnessLocal(n, tt.Node(t).Ent, req)
		if call.Witness == 0 {
			return false
		}
	default:
		m, _, _, ownSubst := c.r.findWitness(t, req, c.witnessIfaceSubst(iface, t), c.pkg)
		if m == 0 {
			if !c.r.requestDerive(t, tt.Iface(iface, nil)) && c.r.mod.derived {
				if why := c.r.protocolMismatch(t, proto); why != "" {
					c.errAt(n, cProtocolMismatch, t, method, proto, why)
				} else {
					c.errAt(n, cWitnessMissing, t, method, proto)
				}
			}
			return false
		}
		c.r.useEntity(c.f, n, m)
		call.Callee = m
		c.edge(EffectEdge{Kind: EdgeCall, Target: m, Node: n})
		c.info.Calls[n] = call
		c.deferWitnesses(n, m, t, c.r.ownInstArgs(m, ownSubst))
		return true
	}
	c.edge(EffectEdge{Kind: EdgeCall, Target: req, Node: n})
	c.info.Calls[n] = call
	c.touched = append(c.touched, n)
	return true
}

func (c *checker) primitiveCompare(t TypeID) bool {
	return c.r.Types.Kind(t) == KPrim
}
