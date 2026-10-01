package syntax_test

import (
	"testing"
	"time"

	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// FuzzParse checks the resilience contract only: any input yields a tree and
// diagnostics without panicking, and every node span lies inside the source.
func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"let x = 1\n", "fn f(a: Int) -> Int { a + 1 }\n", "type S = A(x: Int) | B\n",
		"let p = $\"cat ${path} | grep E > out 2>&1\"\n", "print \"x=${1 + 2}\"\n",
		"match x { Some(v) -> v, None -> 0 }\n",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, src string) {
		file := token.NewFile("fuzz", []byte(src))
		var tree *syntax.Tree
		done := make(chan struct{})
		go func() {
			tree = syntax.Parse(file)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("parser did not terminate on %q", src)
		}
		for id := range tree.Nodes[1:] {
			sp := tree.Span(syntax.NodeID(id + 1))
			if int(sp.End) > len(src) || sp.Start > sp.End {
				t.Fatalf("node %d span %v out of range", id+1, sp)
			}
		}
	})
}
