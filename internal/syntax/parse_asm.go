package syntax

import "kigumi/internal/token"

// asmBlock parses `asm { ... }`: each entry is a template
// string, an operand (`[label:] in(reg) expr`, `const expr`, `sym path`),
// a `clobber` line or an `options` line.
func (p *parser) asmBlock() NodeID {
	tok := p.advance()
	var lines []NodeID
	open := p.layoutList(token.LBrace, token.RBrace, func() {
		if n := p.asmLine(); n != 0 {
			lines = append(lines, n)
		}
	})
	return p.node1(AsmExpr, tok, p.tree.addList(List, open, lines))
}

func (p *parser) asmLine() NodeID {
	var label uint32
	switch {
	case p.at(token.String):
		tok := p.pos
		return p.node1(AsmTemplate, tok, p.stringLit())
	case p.at(token.Ident) && p.peek(1) == token.Colon:
		label = p.advance()
		p.advance()
	case p.atIdent("clobber"):
		return p.asmItems(AsmClobber, true)
	case p.atIdent("options"):
		return p.asmItems(AsmOptions, false)
	}
	kind := p.pos
	switch {
	case p.at(token.KwIn), p.at(token.KwConst), p.atIdent("out"), p.atIdent("inout"), p.atIdent("lateout"), p.atIdent("sym"):
		p.advance()
	default:
		p.errorf("expected an asm template string, an operand (in/out/inout/lateout/const/sym), `clobber` or `options`, found %s", p.describe())
		return 0
	}
	var reg uint32
	var expr NodeID
	switch p.toks[kind].Kind {
	case token.KwConst:
		expr = p.expr()
	case token.Ident:
		if p.tree.TokText(kind) == "sym" {
			expr = p.expr()
			break
		}
		fallthrough
	default:
		p.expect(token.LParen)
		reg = p.expect(token.Ident)
		p.expect(token.RParen)
		expr = p.optionalOperand()
	}
	return p.tree.addRec(AsmOperand, kind, []uint32{label, kind, reg, uint32(expr)})
}

// asmItems parses `clobber a, b, abi(C)` or `options a, b`.
func (p *parser) asmItems(kind NodeKind, abi bool) NodeID {
	tok := p.advance()
	var items []NodeID
	for {
		switch {
		case abi && p.atIdent("abi") && p.peek(1) == token.LParen:
			p.advance()
			p.expect(token.LParen)
			items = append(items, p.leaf(AsmAbi, p.expect(token.Ident)))
			p.expect(token.RParen)
		case p.at(token.Ident), p.at(token.KwPure):
			// `pure` is a keyword elsewhere but an option name here.
			items = append(items, p.leaf(Ident, p.advance()))
		default:
			p.errorf("expected a name after `%s`, found %s", p.tree.TokText(tok), p.describe())
		}
		if !p.eat(token.Comma) {
			break
		}
	}
	return p.node1(kind, tok, p.tree.addList(List, tok, items))
}

// startsAsmBlock tells `asm { ... }` apart from a record literal of a type
// named asm: an asm block starts with a template string, an operand kind,
// `clobber`, `options`, a `label: kind` pair or nothing at all.
func (p *parser) startsAsmBlock() bool {
	i := int(p.pos) + 2
	for i < len(p.toks) && (p.toks[i].Kind == token.Newline || p.toks[i].Kind == token.Comment || p.toks[i].Kind == token.BlockComment) {
		i++
	}
	if i >= len(p.toks) {
		return true
	}
	switch p.toks[i].Kind {
	case token.RBrace, token.String, token.KwIn, token.KwConst:
		return true
	case token.Ident:
		text := p.tree.TokText(uint32(i))
		if i+1 < len(p.toks) && p.toks[i+1].Kind == token.Colon {
			if i+2 >= len(p.toks) {
				return false
			}
			switch p.toks[i+2].Kind {
			case token.KwIn, token.KwConst:
				return true
			case token.Ident:
				return asmKinds[p.tree.TokText(uint32(i+2))]
			}
			return false
		}
		return asmKinds[text] || text == "clobber" || text == "options"
	}
	return false
}

var asmKinds = map[string]bool{"in": true, "out": true, "inout": true, "lateout": true, "const": true, "sym": true}
