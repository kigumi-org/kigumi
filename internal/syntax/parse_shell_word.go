package syntax

// word parses one argv element, stopping at unquoted whitespace or a shell operator.
func (s *shellParser) word(tok uint32) NodeID {
	var parts []NodeID
	for s.pos < s.end {
		c := s.src[s.pos]
		switch {
		case c == ' ' || c == '\t' || c == '|' || c == '<' || c == '>' || c == '&' || c == ';' ||
			c == '(' || c == ')' || c == '`':
			return s.p.tree.addList(ShellWord, tok, parts)
		case c == '\'':
			parts = append(parts, s.singleQuoted())
		case c == '"' || (c == '\\' && s.pos+1 < s.end && s.src[s.pos+1] == '"'):
			parts = append(parts, s.doubleQuoted(tok)...)
		case c == '$':
			if s.pos+1 < s.end && s.src[s.pos+1] == '{' {
				parts = append(parts, s.interpolation(tok))
				continue
			}
			s.errorf(s.pos, "`$name` expansion is not allowed; use `${name}` (§10.7)")
			s.pos++
		default:
			parts = append(parts, s.bareText())
		}
	}
	return s.p.tree.addList(ShellWord, tok, parts)
}

func (s *shellParser) text(start, end int) NodeID {
	return s.p.tree.add(Node{Kind: ShellText, Lhs: uint32(start), Rhs: uint32(end)})
}

func (s *shellParser) bareText() NodeID {
	start := s.pos
	for s.pos < s.end {
		switch s.src[s.pos] {
		case ' ', '\t', '|', '<', '>', '&', ';', '\'', '"', '$', '(', ')', '`':
			return s.text(start, s.pos)
		case '\\':
			if s.pos+1 < s.end && s.src[s.pos+1] == '"' {
				return s.text(start, s.pos)
			}
			// `\\x` in the source is one shell backslash escaping x; any other
			// `\x` is a language escape that decodes to one character.
			if s.pos+1 < s.end && s.src[s.pos+1] == '\\' {
				s.pos += 3
			} else {
				s.pos += 2
			}
		default:
			s.pos++
		}
	}
	return s.text(start, min(s.pos, s.end))
}

func (s *shellParser) singleQuoted() NodeID {
	start := s.pos
	s.pos++
	for s.pos < s.end && s.src[s.pos] != '\'' {
		s.pos++
	}
	if s.pos >= s.end {
		s.errorf(start, "unterminated single quote in shell literal")
		return s.text(start+1, s.end)
	}
	n := s.text(start+1, s.pos)
	s.pos++
	return n
}

// doubleQuoted handles `\"..."\"` inside the language string: the quote itself
// is escaped as `\"`, so we look for the escaped form.
func (s *shellParser) doubleQuoted(tok uint32) []NodeID {
	var parts []NodeID
	start := s.pos
	if s.src[s.pos] == '\\' {
		s.pos++
	}
	s.pos++
	segStart := s.pos
	for s.pos < s.end {
		switch s.src[s.pos] {
		case '\\':
			if s.pos+1 < s.end && s.src[s.pos+1] == '"' {
				if s.pos > segStart {
					parts = append(parts, s.text(segStart, s.pos))
				}
				s.pos += 2
				return parts
			}
			s.pos += 2
		case '"':
			if s.pos > segStart {
				parts = append(parts, s.text(segStart, s.pos))
			}
			s.pos++
			return parts
		case '$':
			if s.pos+1 < s.end && s.src[s.pos+1] == '{' {
				if s.pos > segStart {
					parts = append(parts, s.text(segStart, s.pos))
				}
				parts = append(parts, s.interpolation(tok))
				segStart = s.pos
				continue
			}
			s.pos++
		default:
			s.pos++
		}
	}
	s.errorf(start, "unterminated double quote in shell literal")
	return parts
}

func (s *shellParser) interpolation(tok uint32) NodeID {
	close := matchBrace(s.src, s.pos+2, s.end, nil)
	e := s.p.subExpr(s.pos+2, close)
	s.pos = min(close+1, s.end)
	return s.p.node1(ShellInterp, tok, e)
}
