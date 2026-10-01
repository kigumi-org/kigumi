package vm

import (
	"os"
	"os/signal"
	"syscall"

	"kigumi/internal/sem"
)

// registerSignal gives the VM its own std/os.Signals: the Kigumi body
// reaches libc's sigaction through extern(C), which the VM statically
// refuses (OpCallC), so this drains Go's os/signal instead.
func (m *Machine) registerSignal() {
	h := m.host
	h["os.Host.signals"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj { return mkOpaque("Signals", nil) }
	h["os.Signals.watch"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		if m.opts.DenySignals {
			m.abort("`Signals.watch` is not available under `kigumi test`")
		}
		m.signalChan(m.signalName(a[1]))
		return m.ok(unitObj)
	}
	h["os.Signals.pending"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		ch, ok := m.sigWatch[m.signalName(a[1])]
		if !ok {
			return mkBool(false)
		}
		return mkBool(drainSignal(ch))
	}
}

// signalName reads the Signal variant's tag (Interrupt, Terminate, ...).
func (m *Machine) signalName(a *obj) string { return m.r.Entity(deref(a).ent).Name }

// signalChan installs os/signal.Notify on first watch; an unwatched signal
// has no channel, so pending() just reports false.
func (m *Machine) signalChan(name string) chan os.Signal {
	if m.sigWatch == nil {
		m.sigWatch = map[string]chan os.Signal{}
	}
	ch, ok := m.sigWatch[name]
	if !ok {
		ch = make(chan os.Signal, 1)
		signal.Notify(ch, signalOf(name))
		m.sigWatch[name] = ch
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
