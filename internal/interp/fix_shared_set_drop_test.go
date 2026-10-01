package interp_test

import (
	"bytes"
	"testing"

	"kigumi/internal/interp"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/testkit"
	"kigumi/internal/token"
)

// TestSharedSetDropsOldValue guards std/alloc.kg's documented contract for
// Shared[T].set ("the old value is dropped") and the drop of the content
// when the last Shared goes away at program exit.
func TestSharedSetDropsOldValue(t *testing.T) {
	const src = `import {Shared} from std/alloc

type Guard = resource {
    pub id Int
}

fn Guard.drop(move self) -> Unit {
    print "guard drop ${self.id}"
}

let s = Shared.new(Guard { id: 1 })
print "before set"
s.set(Guard { id: 2 })
print "after set"
`
	main := &sem.Package{Path: "main"}
	main.Entry = syntax.Parse(token.NewFile("main/main.kg", []byte(src)))
	main.Files = []*syntax.Tree{main.Entry}
	res := sem.Check(&sem.Module{Packages: append(testkit.LoadStd(t, "../../std"), main)})
	if res.HasErrors() {
		t.Fatalf("check errors:\n%s", res.Render())
	}
	var out, errOut bytes.Buffer
	in := interp.New(res, &out, &errOut, nil)
	in.RunMain()
	const want = "before set\nguard drop 1\nafter set\nguard drop 2\n"
	if got := out.String(); got != want {
		t.Errorf("stdout = %q, want %q (stderr: %s)", got, want, errOut.String())
	}
}
