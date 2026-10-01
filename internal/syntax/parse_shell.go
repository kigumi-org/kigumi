package syntax

import (
	"kigumi/internal/diag"
	"kigumi/internal/token"
)

// shellLit parses `$"..."` into a ShellPlan. The literal body is read
// byte by byte from the source so quoting and interpolation follow the
// two-stage rule: language escapes stay raw, shell structure is decided here.
func (p *parser) shellLit() NodeID {
	dollar := p.expect(token.Dollar)
	if !p.at(token.String) {
		p.errorf("expected string literal after `$`")
		return 0
	}
	if p.toks[p.pos-1].Kind == token.Newline || p.tree.File.Line(p.tok().Start) != p.tree.File.Line(p.toks[dollar].Start) {
		p.errorf("`$` and its string must be on the same line")
	}
	str := p.advance()
	t := p.toks[str]
	sh := &shellParser{p: p, src: p.tree.File.Src, pos: int(t.Start) + 1, end: int(t.End) - 1}
	plan := sh.plan(str)
	return p.tree.add(Node{Kind: ShellLit, Tok: dollar, Lhs: str, Rhs: uint32(plan)})
}

type shellParser struct {
	p        *parser
	src      []byte
	pos, end int
}

func (s *shellParser) errorf(start int, msg string) {
	end := max(s.pos, start+1)
	s.p.bag.Add(diag.Errorf(diag.Location{Span: token.Span{Start: token.Pos(start), End: token.Pos(end)}}, msg))
}

func (s *shellParser) plan(tok uint32) NodeID {
	var cmds []NodeID
	for {
		cmds = append(cmds, s.command(tok))
		s.skipSpace()
		if s.pos < s.end && s.src[s.pos] == '|' {
			if s.pos+1 < s.end && s.src[s.pos+1] == '|' {
				s.errorf(s.pos, "`||` is not part of the portable shell dialect (§10.7)")
				s.pos += 2
				continue
			}
			s.pos++
			continue
		}
		break
	}
	if s.pos < s.end {
		s.errorf(s.pos, "unsupported shell syntax")
	}
	return s.p.tree.addList(ShellPlan, tok, cmds)
}

func (s *shellParser) skipSpace() {
	for s.pos < s.end && (s.src[s.pos] == ' ' || s.src[s.pos] == '\t') {
		s.pos++
	}
}

func (s *shellParser) command(tok uint32) NodeID {
	var parts []NodeID
	for {
		s.skipSpace()
		if s.pos >= s.end {
			break
		}
		c := s.src[s.pos]
		switch {
		case c == '|':
			return s.p.tree.addList(ShellCmd, tok, parts)
		case c == '&' && s.pos+1 < s.end && s.src[s.pos+1] == '&':
			s.errorf(s.pos, "`&&` is not part of the portable shell dialect (§10.7)")
			s.pos += 2
		case c == '&' || c == ';' || c == '(' || c == ')' || c == '`':
			s.errorf(s.pos, "`"+string(c)+"` is not part of the portable shell dialect (§10.7)")
			s.pos++
		case c == '<' || c == '>' || (c >= '0' && c <= '9' && s.isRedirectAfterDigits()):
			parts = append(parts, s.redirect(tok))
		default:
			before := s.pos
			parts = append(parts, s.word(tok))
			if s.pos == before {
				s.errorf(s.pos, "unexpected character in shell literal")
				s.pos++
			}
		}
	}
	return s.p.tree.addList(ShellCmd, tok, parts)
}

func (s *shellParser) isRedirectAfterDigits() bool {
	i := s.pos
	for i < s.end && s.src[i] >= '0' && s.src[i] <= '9' {
		i++
	}
	return i < s.end && (s.src[i] == '<' || s.src[i] == '>')
}

// maxRedirectFd is far above any file descriptor a real process can have
// open; digits past it stop accumulating so the uint32 fd field can never
// wrap around.
const maxRedirectFd = 1_000_000

func (s *shellParser) redirect(tok uint32) NodeID {
	start := s.pos
	fd := uint32(0)
	hasFd := false
	tooBig := false
	for s.pos < s.end && s.src[s.pos] >= '0' && s.src[s.pos] <= '9' {
		if fd > maxRedirectFd {
			tooBig = true
		} else {
			fd = fd*10 + uint32(s.src[s.pos]-'0')
		}
		s.pos++
		hasFd = true
	}
	if tooBig {
		s.errorf(start, sprintf("redirect file descriptor is too large (max is %d)", maxRedirectFd))
	}
	op := RedirIn
	if s.src[s.pos] == '>' {
		op = RedirOut
		if !hasFd {
			fd = 1
		}
	}
	s.pos++
	if s.pos < s.end && s.src[s.pos] == '>' && op == RedirOut {
		op = RedirAppend
		s.pos++
	} else if s.pos < s.end && s.src[s.pos] == '<' && op == RedirIn {
		s.errorf(start, "here documents are not supported (§10.7)")
		s.pos++
	}
	if s.pos < s.end && s.src[s.pos] == '&' {
		s.pos++
		if op == RedirIn {
			op = RedirDupIn
		} else {
			op = RedirDupOut
		}
	}
	s.skipSpace()
	if s.pos >= s.end || s.src[s.pos] == '|' {
		s.errorf(start, "redirect needs a target")
		return s.p.tree.addRec(ShellRedirect, tok, []uint32{op, fd, 0})
	}
	target := s.word(tok)
	return s.p.tree.addRec(ShellRedirect, tok, []uint32{op, fd, uint32(target)})
}
