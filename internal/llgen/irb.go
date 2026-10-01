package llgen

import (
	"fmt"
	"strconv"
	"strings"
)

// Type is an LLVM IR scalar type used by runtime call signatures.
type Type uint8

const (
	TVoid Type = iota
	TPtr
	TI1
	TI32
	TI64
	TDouble
)

func (t Type) String() string {
	switch t {
	case TVoid:
		return "void"
	case TPtr:
		return "ptr"
	case TI1:
		return "i1"
	case TI32:
		return "i32"
	case TI64:
		return "i64"
	case TDouble:
		return "double"
	default:
		panic(fmt.Sprintf("irb: unknown type %d", t))
	}
}

// Value is one typed LLVM operand: a register, a global, a constant, or
// "null", paired with the type it is used at.
type Value struct {
	Type Type
	Text string
}

func (v Value) render() string { return v.Type.String() + " " + v.Text }

// RuntimeFn is one C runtime entry point's signature: the single source
// the `declare` list and every call's argument check draw from.
type RuntimeFn struct {
	Name   string
	Params []Type
	Ret    Type
}

// StructType is an LLVM named struct type used by a descriptor global.
type StructType struct {
	Name   string
	Fields []Type
}

// Builder emits typed LLVM IR into one function body, panicking on an
// arity or type mismatch instead of emitting bad IR.
type Builder struct {
	sb   *strings.Builder
	next func() string
}

// irb returns a Builder over sb, sharing the emitter's temporary counter.
func (e *emitter) irb(sb *strings.Builder) Builder {
	return Builder{sb: sb, next: e.newTmp}
}

// Call emits a call to fn and returns its result (TVoid, text "" for a
// void callee).
func (b Builder) Call(fn RuntimeFn, args ...Value) Value {
	if fn.Ret == TVoid {
		b.emit("", fn, args)
		return Value{Type: TVoid}
	}
	t := b.next()
	b.emit(t, fn, args)
	return Value{Type: fn.Ret, Text: t}
}

// CallAs is Call for a caller that pre-assigns its own SSA register name
// instead of taking one from the shared counter.
func (b Builder) CallAs(dst string, fn RuntimeFn, args ...Value) { b.emit(dst, fn, args) }

func (b Builder) emit(dst string, fn RuntimeFn, args []Value) {
	if len(args) != len(fn.Params) {
		panic(fmt.Sprintf("irb: %s takes %d argument(s), got %d", fn.Name, len(fn.Params), len(args)))
	}
	parts := make([]string, len(args))
	for i, a := range args {
		if a.Type != fn.Params[i] {
			panic(fmt.Sprintf("irb: %s argument %d: want %s, got %s", fn.Name, i, fn.Params[i], a.Type))
		}
		parts[i] = a.render()
	}
	if dst == "" {
		fmt.Fprintf(b.sb, "  call %s @%s(%s)\n", fn.Ret, fn.Name, strings.Join(parts, ", "))
		return
	}
	fmt.Fprintf(b.sb, "  %s = call %s @%s(%s)\n", dst, fn.Ret, fn.Name, strings.Join(parts, ", "))
}

// GlobalStruct defines a `global` of struct type st and returns name unchanged.
func (b Builder) GlobalStruct(name string, st StructType, fields ...Value) string {
	if len(fields) != len(st.Fields) {
		panic(fmt.Sprintf("irb: %s takes %d field(s), got %d", st.Name, len(st.Fields), len(fields)))
	}
	parts := make([]string, len(fields))
	for i, f := range fields {
		if f.Type != st.Fields[i] {
			panic(fmt.Sprintf("irb: %s field %d: want %s, got %s", st.Name, i, st.Fields[i], f.Type))
		}
		parts[i] = f.render()
	}
	fmt.Fprintf(b.sb, "%s = global %s { %s }\n", name, st.Name, strings.Join(parts, ", "))
	return name
}

func vptr(text string) Value     { return Value{TPtr, text} }
func vi1(text string) Value      { return Value{TI1, text} }
func vi32(n int) Value           { return Value{TI32, strconv.Itoa(n)} }
func vi32text(text string) Value { return Value{TI32, text} }
func vi64(n int) Value           { return Value{TI64, strconv.Itoa(n)} }
func vi64text(text string) Value { return Value{TI64, text} }
func vdouble(text string) Value  { return Value{TDouble, text} }
