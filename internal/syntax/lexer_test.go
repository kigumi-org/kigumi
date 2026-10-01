package syntax_test

import (
	"strings"
	"testing"

	"kigumi/internal/diag"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

func kinds(src string) (string, int) {
	var bag diag.Bag
	toks := syntax.Lex([]byte(src), &bag)
	var parts []string
	for _, t := range toks {
		if t.Kind == token.EOF {
			break
		}
		if t.Kind == token.BOF {
			continue
		}
		s := t.Kind.String()
		switch t.Kind {
		case token.Ident, token.Int, token.Float, token.String, token.ByteString, token.Char, token.Operator:
			s += "(" + t.Text([]byte(src)) + ")"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " "), bag.Len()
}

func TestLex(t *testing.T) {
	tests := []struct {
		src  string
		want string
		errs int
	}{
		{"let x = 1", "let identifier(x) = integer(1)", 0},
		{"a |> b || c | d |= e", "identifier(a) |> identifier(b) || identifier(c) | identifier(d) |= identifier(e)", 0},
		{"x?.y? != z", "identifier(x) ?. identifier(y) ? != identifier(z)", 0},
		{"a..b ... c.d", "identifier(a) .. identifier(b) ... identifier(c) . identifier(d)", 0},
		{"-> => == <<= >>= &&", "-> => == <<= >>= &&", 0},
		{"a +^ b *- c", "identifier(a) operator(+^) identifier(b) operator(*-) identifier(c)", 0},
		{"x =- 1", "identifier(x) operator(=-) integer(1)", 1},
		{"0x_ff 0b1_0 0o7 1_000 1.5 2e10 1.5e-3 1..2", "integer(0x_ff) integer(0b1_0) integer(0o7) integer(1_000) float(1.5) float(2e10) float(1.5e-3) integer(1) .. integer(2)", 0},
		{"1u8", "integer(1u8)", 1},
		{"\"a ${b + \"}\"} c\" 'x' '\\n' '\\u{1F600}'", "string(\"a ${b + \"}\"} c\") char('x') char('\\n') char('\\u{1F600}')", 0},
		{"\"open", "string(\"open)", 1},
		{"// c\n/// doc\n#!/usr/bin/env kigumi\n/* a /* b */ c */ x", "comment newline doc comment newline comment newline block comment identifier(x)", 0},
		{"a\n\n\nb", "identifier(a) newline identifier(b)", 0},
		{"$\"echo hi\" @impl(A)", "$ string(\"echo hi\") @ identifier(impl) ( identifier(A) )", 0},
		{"fn Vec.+^(a: Vec)", "fn identifier(Vec) . operator(+^) ( identifier(a) : identifier(Vec) )", 0},
		{"Ast! {", "identifier(Ast) ! {", 0},
		{"Res!= 1", "identifier(Res) != integer(1)", 0},
		{"名前 = 1", "identifier(名前) = integer(1)", 0},
		{"a ` b", "identifier(a) invalid identifier(b)", 1},
		{`b"\x30\x82A" b"" let b = 1`, `byte string(b"\x30\x82A") byte string(b"") let identifier(b) = integer(1)`, 0},
		{`b"\xZZ"`, `byte string(b"\xZZ")`, 1},
		{"b\"é\"", "byte string(b\"é\")", 1},
		{`b"open`, `byte string(b"open)`, 1},
	}
	for _, tt := range tests {
		got, errs := kinds(tt.src)
		if got != tt.want {
			t.Errorf("Lex(%q)\n got: %s\nwant: %s", tt.src, got, tt.want)
		}
		if errs != tt.errs {
			t.Errorf("Lex(%q): %d diagnostics, want %d", tt.src, errs, tt.errs)
		}
	}
}

func TestSpaceBefore(t *testing.T) {
	var bag diag.Bag
	toks := syntax.Lex([]byte("f (x) g(y)"), &bag)
	if !toks[2].HasSpaceBefore() {
		t.Error("`(` after `f ` should have SpaceBefore")
	}
	if toks[6].HasSpaceBefore() {
		t.Error("`(` after `g` should not have SpaceBefore")
	}
}
