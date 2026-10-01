package vm_test

import (
	"bytes"
	"os"
	"testing"
	"time"

	"kigumi/internal/mir"
	"kigumi/internal/vm"
)

// TestStdinReadSomeReceivesData feeds a real pipe after a short delay and
// checks readSome returns the data, then that a second call with nothing
// left times out empty. The budget is far above the delay so this is a
// correctness check, not a race against readSome's own timeout (see
// TestStdinReadSomeTimesOut for that).
func TestStdinReadSomeReceivesData(t *testing.T) {
	src := "let a = host.stdin().readSome(60000)?\n" +
		"print \"a=${a.len()} first=${a.get(0) || 255}\"\n" +
		"let b = host.stdin().readSome(150)?\n" +
		"print \"b=${b.len()}\"\n"
	res := checkSource(t, src)
	if res.HasErrors() {
		t.Fatalf("check errors:\n%s", res.Render())
	}
	prog := mir.Build(res)
	if prog.HasErrors() {
		t.Fatal("ownership errors")
	}
	if err := prog.Apply(mir.Transform{Name: "plan-rc", Run: mir.PlanRC}); err != nil {
		t.Fatalf("plan-rc: %v", err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	go func() {
		time.Sleep(150 * time.Millisecond)
		w.Write([]byte("Z"))
	}()

	start := time.Now()
	var out, errOut bytes.Buffer
	code, err := vm.Run(prog, vm.Options{Stdin: r}, &out, &errOut)
	elapsed := time.Since(start)
	w.Close()
	if err != nil {
		t.Fatalf("vm: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit %d\nstderr: %s", code, errOut.String())
	}
	if want := "a=1 first=90\nb=0\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	// Sanity bound only, not a correctness dependency: the first call's
	// 60000ms budget is far above the writer's 150ms delay, so this only
	// guards against readSome hanging past its own timeouts.
	if elapsed > 5*time.Second {
		t.Errorf("took %s; readSome should have returned well within its budgets", elapsed)
	}
}

// TestStdinReadSomeTimesOut checks readSome returns an empty read once its
// own budget elapses, with nothing ever written to the pipe so the outcome
// does not depend on any writer/timeout race.
func TestStdinReadSomeTimesOut(t *testing.T) {
	src := "let a = host.stdin().readSome(150)?\n" +
		"print \"a=${a.len()}\"\n"
	res := checkSource(t, src)
	if res.HasErrors() {
		t.Fatalf("check errors:\n%s", res.Render())
	}
	prog := mir.Build(res)
	if prog.HasErrors() {
		t.Fatal("ownership errors")
	}
	if err := prog.Apply(mir.Transform{Name: "plan-rc", Run: mir.PlanRC}); err != nil {
		t.Fatalf("plan-rc: %v", err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	start := time.Now()
	var out, errOut bytes.Buffer
	code, err := vm.Run(prog, vm.Options{Stdin: r}, &out, &errOut)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("vm: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit %d\nstderr: %s", code, errOut.String())
	}
	if want := "a=0\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	// Generous ceiling around the 150ms budget keeps this from being flaky
	// under load without masking readSome ignoring its own timeout.
	if elapsed > 1500*time.Millisecond {
		t.Errorf("took %s; readSome's timeout did not take effect", elapsed)
	}
}

// TestStdinReadSomeClosed checks that a closed pipe fails readSome instead
// of returning an empty read indistinguishable from a timeout.
func TestStdinReadSomeClosed(t *testing.T) {
	src := "let a = host.stdin().readSome(1000)?\n" +
		"print \"unreachable\"\n"
	res := checkSource(t, src)
	if res.HasErrors() {
		t.Fatalf("check errors:\n%s", res.Render())
	}
	prog := mir.Build(res)
	if prog.HasErrors() {
		t.Fatal("ownership errors")
	}
	if err := prog.Apply(mir.Transform{Name: "plan-rc", Run: mir.PlanRC}); err != nil {
		t.Fatalf("plan-rc: %v", err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	defer r.Close()

	var out, errOut bytes.Buffer
	code, err := vm.Run(prog, vm.Options{Stdin: r}, &out, &errOut)
	if err != nil {
		t.Fatalf("vm: %v", err)
	}
	if code != 1 || out.Len() != 0 {
		t.Errorf("exit %d, stdout %q; want a propagated-error exit with no output\nstderr: %s", code, out.String(), errOut.String())
	}
	if !bytes.Contains(errOut.Bytes(), []byte("stdin is closed")) {
		t.Errorf("stderr = %q, want it to name the closed stdin", errOut.String())
	}
}
