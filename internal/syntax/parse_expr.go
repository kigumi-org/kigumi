package syntax

import "kigumi/internal/token"

// Binding strengths; higher binds tighter.
const (
	precRange = iota + 1
	precFallback
	precAnd
	precCompare
	precPipeline
	precBitOr
	precBitXor
	precBitAnd
	precShift
	precAdd
	precMul
)

func (p *parser) expr() NodeID {
	if !p.enterDepth() {
		return 0
	}
	defer p.exitDepth()
	return p.binary(precRange)
}

func (p *parser) binaryPrec() (int, bool) {
	t := p.tok()
	switch t.Kind {
	case token.DotDot, token.DotDotEq:
		return precRange, false
	case token.OrOr:
		return precFallback, true
	case token.AndAnd:
		return precAnd, true
	case token.EqEq, token.NotEq, token.Lt, token.LtEq, token.Gt, token.GtEq, token.KwIs:
		return precCompare, false
	case token.PipeGt:
		return precPipeline, true
	case token.Pipe:
		return precBitOr, true
	case token.Caret:
		return precBitXor, true
	case token.Amp:
		return precBitAnd, true
	case token.Shl, token.Shr:
		return precShift, true
	case token.Plus, token.Minus:
		return precAdd, true
	case token.Star, token.Slash, token.Percent:
		return precMul, true
	case token.Operator:
		switch t.Text(p.tree.File.Src)[0] {
		case '*', '%':
			return precMul, true
		case '+', '-':
			return precAdd, true
		case '<', '>':
			return precCompare, false
		case '&':
			return precBitAnd, true
		case '^':
			return precBitXor, true
		}
	}
	return 0, false
}

// isLeadingContinuation reports whether k, found after a newline, continues
// the previous line's expression: `|>` sets the mechanism, extended to
// `&&` / `||`.
func isLeadingContinuation(k token.Kind) bool {
	switch k {
	case token.PipeGt, token.AndAnd, token.OrOr:
		return true
	}
	return false
}

func (p *parser) binary(minPrec int) NodeID {
	lhs := p.unary()
	if lhs == 0 {
		return 0
	}
	for {
		if p.at(token.Newline) && isLeadingContinuation(p.peekPastNewlines()) {
			p.skipNewlines()
		}
		prec, leftAssoc := p.binaryPrec()
		if prec == 0 || prec < minPrec {
			return lhs
		}
		op := p.advance()
		p.checkSpacedMinus(op, lhs)
		p.skipNewlines()
		p.checkMixing(op, lhs)
		if p.toks[op].Kind == token.KwIs {
			lhs = p.node2(IsExpr, op, lhs, p.pattern())
		} else {
			rhs := p.binary(prec + 1)
			p.checkMixing(op, rhs)
			lhs = p.node2(Binary, op, lhs, rhs)
		}
		if !leftAssoc {
			if np, _ := p.binaryPrec(); np == prec {
				p.errorf("chained %s needs parentheses", p.tok().Kind)
			}
		}
	}
}

func (p *parser) unary() NodeID {
	if !p.enterDepth() {
		return 0
	}
	defer p.exitDepth()
	switch p.tok().Kind {
	case token.Minus, token.Bang, token.Tilde:
		tok := p.advance()
		return p.node1(Unary, tok, p.unary())
	case token.Operator:
		if c := p.text()[0]; c == '!' || c == '~' {
			tok := p.advance()
			return p.node1(Unary, tok, p.unary())
		}
	case token.KwAwait:
		tok := p.advance()
		return p.node1(AwaitExpr, tok, p.unary())
	case token.KwMove:
		tok := p.advance()
		return p.node1(MoveExpr, tok, p.unary())
	case token.Amp:
		tok := p.advance()
		var flags uint32
		if p.eat(token.KwMut) {
			flags = FlagMut
		}
		return p.nodeFlag(BorrowExpr, tok, flags, p.unary())
	}
	return p.postfix()
}

func (p *parser) primary() NodeID {
	switch p.tok().Kind {
	case token.Ident:
		return p.identPrimary()
	case token.Int, token.Float, token.Char, token.KwTrue, token.KwFalse, token.String, token.ByteString:
		return p.literal()
	case token.Dollar:
		return p.shellLit()
	case token.LParen:
		return p.parenOrLambda()
	case token.LBrace:
		return p.block()
	case token.KwIf:
		return p.ifExpr()
	case token.KwMatch:
		return p.matchExpr()
	case token.KwFor:
		return p.forExpr()
	case token.KwReturn:
		return p.node1(ReturnExpr, p.advance(), p.optionalOperand())
	case token.KwFail:
		return p.node1(FailExpr, p.advance(), p.optionalOperand())
	case token.KwBreak:
		return p.node1(BreakExpr, p.advance(), p.optionalOperand())
	case token.KwContinue:
		return p.node1(ContinueExpr, p.advance(), 0)
	case token.KwUnsafe:
		return p.node1(UnsafeExpr, p.advance(), p.block())
	case token.KwComptime:
		tok := p.advance()
		if p.at(token.LBrace) {
			return p.node1(ComptimeExpr, tok, p.block())
		}
		return p.node1(ComptimeExpr, tok, p.unary())
	case token.KwPure, token.KwNoalloc:
		return p.contractExpr()
	}
	p.errorf("expected expression, found %s", p.describe())
	return 0
}
