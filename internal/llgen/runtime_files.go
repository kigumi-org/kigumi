package llgen

import _ "embed"

//go:embed runtime/rt.h
var runtimeHeader string

//go:embed runtime/rt_core.c
var runtimeCore string

//go:embed runtime/rt_float.c
var runtimeFloat string

//go:embed runtime/rt_sys.c
var runtimeSys string

//go:embed runtime/rt_signal.c
var runtimeSignal string

// RuntimeFile is one C source or header of the runtime.
type RuntimeFile struct {
	Name, Src string
}

// posixSignalOS are the driver.Target.OS values with POSIX <signal.h>.
// Windows and WASI are hosted (sys != "none") but have neither, so
// rt_signal.c must be gated on this instead of on sys alone.
var posixSignalOS = map[string]bool{"linux": true, "darwin": true, "freebsd": true, "netbsd": true, "openbsd": true}

// RuntimeFiles includes rt_signal.c only for hosted POSIX OSes: sigaction
// needs a POSIX <signal.h>, which freestanding/"--sys none" and
// Windows/WASI lack.
func RuntimeFiles(sys, os string) []RuntimeFile {
	files := []RuntimeFile{{"rt.h", runtimeHeader}, {"rt_core.c", runtimeCore}, {"rt_float.c", runtimeFloat}, {"rt_sys.c", runtimeSys}}
	if sys != "none" && posixSignalOS[os] {
		files = append(files, RuntimeFile{"rt_signal.c", runtimeSignal})
	}
	return files
}

// RuntimeSource is the whole runtime as one text, for tests that scan it.
func RuntimeSource() string { return runtimeHeader + runtimeCore + runtimeFloat + runtimeSys }
