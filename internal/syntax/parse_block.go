package syntax

import (
	"strings"

	"kigumi/internal/token"
)

func (p *parser) importDecl(pf prefix) NodeID {
	tok := p.expect(token.KwImport)
	var binding NodeID
	if p.at(token.LBrace) {
		var names []NodeID
		p.layoutList(token.LBrace, token.RBrace, func() {
			name := p.leaf(Ident, p.expect(token.Ident))
			if p.eatIdent("as") {
				alias := p.leaf(Ident, p.expect(token.Ident))
				names = append(names, p.node2(ImportAlias, p.tree.Nodes[alias].Tok, name, alias))
				return
			}
			names = append(names, name)
		})
		binding = p.tree.addList(List, tok, names)
	} else {
		binding = p.leaf(Ident, p.expect(token.Ident))
	}
	if !p.eatIdent("from") {
		p.errorf("expected `from` in import declaration, found %s", p.describe())
	}
	return p.tree.addRec(ImportDecl, tok, []uint32{uint32(pf.vis), uint32(binding), uint32(p.importPath())})
}

// importPath reads the rest of the line as a module path such as
// `github.com/kigumilang/exp/sgx/enclave`. The lexer splits
// `github.com` and `my-lib` into several tokens, so each segment becomes a
// synthetic Ident token appended after the real ones.
func (p *parser) importPath() NodeID {
	start := uint32(len(p.tree.Extra))
	first := p.pos
	src := p.tree.File.Src
	from := int(p.tok().Start)
	end := from
	for end < len(src) && !isPathEnd(src[end]) {
		end++
	}
	if end == from {
		p.errorf("expected an import path, found %s", p.describe())
		return p.tree.add(Node{Kind: Path, Tok: first, Lhs: start, Rhs: start})
	}
	pos := from
	for _, seg := range strings.Split(string(src[from:end]), "/") {
		span := token.Span{Start: token.Pos(pos), End: token.Pos(pos + len(seg))}
		if !validPathSegment(seg) {
			p.errorAt(span, "import path segments are letters, digits, `.`, `-` and `_`, and cannot be empty or `..`")
		}
		// Interpolation appends its tokens the same way, so both lists
		// must stay the same slice.
		p.toks = append(p.toks, token.Token{Kind: token.Ident, Span: span})
		p.tree.Toks = p.toks
		p.closer = append(p.closer, 0)
		p.tree.Extra = append(p.tree.Extra, uint32(len(p.toks)-1))
		pos += len(seg) + 1
	}
	for !p.at(token.EOF) && int(p.tok().Start) < end {
		p.pos++
	}
	p.skipComments()
	return p.tree.add(Node{Kind: Path, Tok: first, Lhs: start, Rhs: uint32(len(p.tree.Extra))})
}

func isPathEnd(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

func validPathSegment(seg string) bool {
	if seg == "" || seg == ".." || seg[0] == '.' || seg[len(seg)-1] == '.' {
		return false
	}
	for i := 0; i < len(seg); i++ {
		c := seg[i]
		if !(c == '.' || c == '-' || c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}

func (p *parser) testDecl(pf prefix) NodeID {
	p.rejectDeclPrefix(pf, "test declarations")
	tok := p.advance()
	name := p.expect(token.String)
	return p.tree.add(Node{Kind: TestDecl, Tok: tok, Lhs: name, Rhs: uint32(p.block())})
}

func (p *parser) contractBlock(pf prefix) NodeID {
	if pf.vis != 0 {
		p.errorAt(p.tree.Span(pf.vis), "`pub` cannot be applied to a contract block; write it on each declaration")
	}
	tok := p.pos
	body := p.declBlock()
	return p.nodeFlag(ContractBlock, tok, pf.mods, body)
}

func (p *parser) abiBlock(pf prefix) NodeID {
	p.rejectDeclPrefix(pf, "ABI blocks")
	tok := p.advance()
	var args []NodeID
	p.layoutList(token.LParen, token.RParen, func() {
		if p.at(token.Ident) || p.tok().Kind.IsKeyword() {
			args = append(args, p.leaf(Ident, p.advance()))
			return
		}
		p.errorf("expected ABI name, found %s", p.describe())
	})
	return p.node2(AbiBlock, tok, p.tree.addList(List, tok, args), p.declBlock())
}

func (p *parser) declBlock() NodeID {
	var decls []NodeID
	tok := p.layoutList(token.LBrace, token.RBrace, func() {
		if p.atIdent("instantiate") {
			decls = append(decls, p.instantiate())
			return
		}
		if d := p.decl(); d != 0 {
			decls = append(decls, d)
		}
	})
	return p.tree.addList(List, tok, decls)
}

func (p *parser) instantiate() NodeID {
	tok := p.advance()
	name := p.expect(token.Ident)
	p.expect(token.Assign)
	return p.tree.add(Node{Kind: InstantiateDecl, Tok: name, Lhs: tok, Rhs: uint32(p.typ())})
}
