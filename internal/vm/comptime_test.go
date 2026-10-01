package vm_test

import (
	"fmt"
	"strings"
	"testing"

	"kigumi/internal/mir"
	"kigumi/internal/sem"
	"kigumi/internal/vm"
)

// evalOneComptime checks a script with exactly one comptime block and
// evaluates it in the sandbox.
func evalOneComptime(t *testing.T, src string) (sem.Literal, error) {
	t.Helper()
	res := checkSource(t, src)
	if res.HasErrors() {
		t.Fatalf("check errors:\n%s", res.Render())
	}
	prog := mir.Build(res)
	if len(prog.Comptime) != 1 {
		t.Fatalf("got %d comptime blocks, want 1", len(prog.Comptime))
	}
	if err := prog.Apply(mir.Transform{Name: "plan-rc", Run: mir.PlanRC}); err != nil {
		t.Fatalf("plan-rc: %v", err)
	}
	return vm.EvalComptime(prog, prog.Comptime[0])
}

// A non-terminating comptime block is a bounded compiler error, not a hang:
// the sandbox blocks host effects, and the step budget bounds CPU.
func TestComptimeInfiniteLoopFailsInstead(t *testing.T) {
	_, err := evalOneComptime(t, "let x = comptime {\n    let mut i = 0\n    for true {\n        i = i + 1\n    }\n    i\n}\nprint \"${x}\"\n")
	if err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("err = %v, want a step-budget diagnostic", err)
	}
}

// A comptime block that recurses too deep gets a diagnostic, not a Go
// stack overflow.
func TestComptimeDeepRecursionFailsInstead(t *testing.T) {
	_, err := evalOneComptime(t, "fn rec(n: Int) -> Int { if n <= 0 { 0 } else { 1 + rec(n - 1) } }\nlet n = comptime { rec(2000000) }\nprint \"n ${n}\"\n")
	if err == nil || !strings.Contains(err.Error(), "too deep") {
		t.Fatalf("err = %v, want a recursion-depth diagnostic", err)
	}
}

// A comptime block that keeps doubling a string is a bounded compiler
// error, not host RAM exhaustion: the memory budget is separate from the
// step budget above, and this loop is short on steps but grows without bound.
func TestComptimeMemoryLimitFailsInstead(t *testing.T) {
	_, err := evalOneComptime(t, "let x = comptime {\n    let mut s = \"x\"\n    for _ in 0..40 {\n        s = \"${s}${s}\"\n    }\n    s\n}\nprint \"${x}\"\n")
	if err == nil || !strings.Contains(err.Error(), "too much memory") {
		t.Fatalf("err = %v, want a memory-budget diagnostic", err)
	}
}

// A comptime loop that builds and immediately drops a small record each
// iteration must not trip the memory budget: the budget bounds live bytes,
// not the cumulative volume ever allocated, so this stays cheap no matter
// how many iterations run.
func TestComptimeMemoryLimitTracksLiveNotCumulative(t *testing.T) {
	_, err := evalOneComptime(t, "type Pair = {\n    pub a Int\n    pub b Int\n}\nlet x = comptime {\n    let mut count = 0\n    for i in 0..2000000 {\n        let r = Pair { a: i, b: i }\n        count = count + r.a\n    }\n    count\n}\nprint \"${x}\"\n")
	if err != nil {
		t.Fatalf("err = %v, want success (transient allocations must not accumulate)", err)
	}
}

// A comptime loop that builds and drops a fresh array each iteration via a
// pass-through primitive (array.Array.of, which just retains its argument)
// must not trip the memory budget: retaining an already-charged object must
// not double-charge it at the call-boundary site (calls.go).
func TestComptimeMemoryLimitIgnoresRetainedPassThrough(t *testing.T) {
	_, err := evalOneComptime(t, "let x = comptime {\n    let mut total = 0\n    for i in 0..2000000 {\n        let a = Array.of(i)\n        total = total + i\n    }\n    total\n}\nprint \"${x}\"\n")
	if err != nil {
		t.Fatalf("err = %v, want success (retain pass-through must not double-charge)", err)
	}
}

// A comptime loop that grows one array in place via push must trip the
// memory budget too: push mutates its receiver in place and returns Unit,
// so only its own chargeMemory(receiver) call (builtins.go) ever bills the
// growth.
func TestComptimeMemoryLimitCatchesArrayPushGrowth(t *testing.T) {
	_, err := evalOneComptime(t, "let x = comptime {\n    let mut a = Array.empty[Int]()\n    for i in 0..9000000 {\n        a.push(i)\n    }\n    a.len()\n}\nprint \"${x}\"\n")
	if err == nil || !strings.Contains(err.Error(), "too much memory") {
		t.Fatalf("err = %v, want a memory-budget diagnostic", err)
	}
}

// nestedCopySource builds Leaf, L2..L<depth> record types, each reusing one
// value 8 times for its fields (all-Int, so every level is structurally
// Copy and the reuse is an ordinary auto-copy, not a move error), then
// pushes one instance into an array. Each level multiplies the live node
// count by 8, so depth alone controls how much one m.copy() call duplicates.
func nestedCopySource(depth int) string {
	var b strings.Builder
	b.WriteString("type Leaf = { pub a Int\npub b Int\npub c Int\npub d Int\npub e Int\npub f Int\npub g Int\npub h Int\n}\n")
	prevTy := "Leaf"
	for lvl := 2; lvl <= depth; lvl++ {
		name := fmt.Sprintf("L%d", lvl)
		fmt.Fprintf(&b, "type %s = {\n", name)
		for i := 0; i < 8; i++ {
			fmt.Fprintf(&b, "pub p%d %s\n", i, prevTy)
		}
		b.WriteString("}\n")
		prevTy = name
	}
	b.WriteString("fn mkLeaf(v: Int) -> Leaf { Leaf { a: v, b: v, c: v, d: v, e: v, f: v, g: v, h: v } }\n")
	prevFn, prevTy := "mkLeaf", "Leaf"
	for lvl := 2; lvl <= depth; lvl++ {
		name := fmt.Sprintf("L%d", lvl)
		fmt.Fprintf(&b, "fn mk%s(v: Int) -> %s { let l = %s(v); %s {", name, name, prevFn, name)
		for i := 0; i < 8; i++ {
			fmt.Fprintf(&b, " p%d: l,", i)
		}
		b.WriteString(" } }\n")
		prevFn, prevTy = "mk"+name, name
	}
	fmt.Fprintf(&b, "let x = comptime {\nlet big = %s(1)\nlet mut arr = Array.empty[%s]()\narr.push(big)\narr.len()\n}\nprint \"${x}\"\n", prevFn, prevTy)
	return b.String()
}

// A comptime block that builds a deeply nested, structurally Copy record
// and reuses it 8 times per level must trip the memory budget: copy()
// (value_life.go) charges every nested record's own approxSize, not just
// the outermost value's shallow size.
func TestComptimeMemoryLimitCatchesNestedStructuralCopy(t *testing.T) {
	_, err := evalOneComptime(t, nestedCopySource(8))
	if err == nil || !strings.Contains(err.Error(), "too much memory") {
		t.Fatalf("err = %v, want a memory-budget diagnostic", err)
	}
}

// A comptime block that builds the same shape of nested Copy record, but
// shallow enough to stay well under the budget once every level is
// charged, must still succeed: charging nested copies must not make an
// ordinary small aggregate look bigger than it is.
func TestComptimeMemoryLimitAllowsSmallNestedStructuralCopy(t *testing.T) {
	_, err := evalOneComptime(t, nestedCopySource(5))
	if err != nil {
		t.Fatalf("err = %v, want success (a small nested copy must stay under budget)", err)
	}
}

// A comptime loop that slices a large array and keeps every slice alive
// must trip the memory budget too: index() (ops_index.go) allocates a new
// backing buffer for the slice, which must be charged like any other
// allocation.
func TestComptimeMemoryLimitCatchesSliceGrowth(t *testing.T) {
	_, err := evalOneComptime(t, "let x = comptime {\n    let mut big = Array.empty[Int]()\n    for i in 0..300000 {\n        big.push(i)\n    }\n    let mut store = Array.empty[Array[Int]]()\n    for _ in 0..40 {\n        let piece = big[0..300000]\n        store.push(piece)\n    }\n    store.len()\n}\nprint \"${x}\"\n")
	if err == nil || !strings.Contains(err.Error(), "too much memory") {
		t.Fatalf("err = %v, want a memory-budget diagnostic", err)
	}
}

// The sandbox refuses the clock: time.sleep is not fs/os/net but
// still reaches the host.
func TestComptimeSandboxBlocksClock(t *testing.T) {
	_, err := evalOneComptime(t, "import time from std/time\nlet leaked = comptime {\n    let epoch = time.fromUnixMillis(0)\n    time.sleep(1)\n    epoch.elapsedMillis()\n}\nprint \"${leaked}\"\n")
	if err == nil || !strings.Contains(err.Error(), "time.sleep") || !strings.Contains(err.Error(), "not available at compile time") {
		t.Fatalf("err = %v, want a `time.sleep is not available at compile time` error", err)
	}
}

// Pure std/time functions only transform a value already held and stay
// usable at compile time.
func TestComptimePureTimeStillWorks(t *testing.T) {
	lit, err := evalOneComptime(t, "import time from std/time\nlet v = comptime {\n    time.fromUnixMillis(1000).unixMillis()\n}\nprint \"${v}\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if lit.Kind != sem.LitInt || lit.Int.Int64() != 1000 {
		t.Fatalf("got %v, want 1000", lit)
	}
}
