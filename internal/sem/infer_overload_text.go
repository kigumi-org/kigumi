package sem

func preferFixed(c *checker, ms []EntityID) []EntityID {
	var fixed []EntityID
	for _, m := range ms {
		if c.r.Types.Node(c.r.Fn(m).Sig).Flags&fnVariadic == 0 {
			fixed = append(fixed, m)
		}
	}
	if len(fixed) > 0 {
		return fixed
	}
	return ms
}

func (c *checker) typesText(types []TypeID) string {
	out := ""
	for i, t := range types {
		if i > 0 {
			out += ", "
		}
		out += c.r.TypeString(c.vars.defaulted(t))
	}
	return out
}

func (c *checker) sigList(ms []EntityID) string {
	out := ""
	for i, m := range ms {
		if i > 0 {
			out += ", "
		}
		out += "`" + c.r.TypeString(c.r.Fn(m).Sig) + "`"
	}
	return out
}
