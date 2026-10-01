package interp

import (
	"os"
	"os/signal"
	"syscall"

	"kigumi/internal/syntax"
)

// registerSignalAccel gives the interpreter its own std/os.Signals: the
// Kigumi body reaches libc's sigaction through extern(C), which the
// interpreter cannot call, so this drains Go's os/signal instead.
func registerSignalAccel(in *Interp) {
	o := "std/os."
	in.hostAccel(o+"Signals.watch", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		if in.DenySignals {
			fr.panicAt(n, "`Signals.watch` is not available under `kigumi test`")
		}
		in.signalChan(in.signalName(a[1]))
		return mkOk(in, fr.retType(n), Unit{}), nil
	})
	in.hostAccel(o+"Signals.pending", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		ch, ok := in.sigWatch[in.signalName(a[1])]
		if !ok {
			return Bool(false), nil
		}
		return Bool(drainSignal(ch)), nil
	})
}

// signalName reads the Signal variant's tag (Interrupt, Terminate, ...).
func (in *Interp) signalName(v Value) string {
	return in.r.Entity(deref(v).(*Variant).V).Name
}

// signalChan installs os/signal.Notify on first watch; an unwatched signal
// has no channel, so pending() just reports false.
func (in *Interp) signalChan(name string) chan os.Signal {
	if in.sigWatch == nil {
		in.sigWatch = map[string]chan os.Signal{}
	}
	ch, ok := in.sigWatch[name]
	if !ok {
		ch = make(chan os.Signal, 1)
		signal.Notify(ch, signalOf(name))
		in.sigWatch[name] = ch
	}
	return ch
}

func signalOf(name string) os.Signal {
	switch name {
	case "Interrupt":
		return syscall.SIGINT
	case "Terminate":
		return syscall.SIGTERM
	case "WindowChange":
		return syscall.SIGWINCH
	default:
		return syscall.Signal(0)
	}
}

// drainSignal reports delivery since the last drain, clearing the whole
// queue so later calls see only new signals.
func drainSignal(ch chan os.Signal) bool {
	select {
	case <-ch:
	default:
		return false
	}
	for {
		select {
		case <-ch:
		default:
			return true
		}
	}
}
