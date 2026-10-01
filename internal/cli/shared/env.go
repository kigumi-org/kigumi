package shared

import (
	"fmt"
	"os"
	"strings"
)

// The KIGUMI_* variables are read here, once, and handed to the compiler
// packages as explicit options.

// CFlags returns the extra C compiler and linker arguments of KIGUMI_CFLAGS.
func CFlags() []string { return strings.Fields(os.Getenv("KIGUMI_CFLAGS")) }

// LocalOff reports KIGUMI_LOCAL=off, which ignores mod.local.kg.
func LocalOff() bool { return os.Getenv("KIGUMI_LOCAL") == "off" }

// NoAccel reports KIGUMI_NO_ACCEL, which runs std bodies in Kigumi.
func NoAccel() bool { return os.Getenv("KIGUMI_NO_ACCEL") != "" }

// Color decides whether output to f gets ANSI colors: --color always or
// never settle it, otherwise NO_COLOR (no-color.org) turns it off,
// CLICOLOR_FORCE turns it on, and a dumb terminal or a non-terminal
// stays plain. mode must be "", "auto", "always", or "never".
func Color(mode string, f *os.File) (bool, error) {
	switch mode {
	case "", "auto":
	case "always":
		return true, nil
	case "never":
		return false, nil
	default:
		return false, fmt.Errorf("--color %q: want auto, always, or never", mode)
	}
	if _, off := os.LookupEnv("NO_COLOR"); off {
		return false, nil
	}
	if os.Getenv("CLICOLOR_FORCE") != "" && os.Getenv("CLICOLOR_FORCE") != "0" {
		return true, nil
	}
	if os.Getenv("TERM") == "dumb" || f == nil {
		return false, nil
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0, nil
}

// SecretFile returns the file named by KIGUMI_SECRET_<name>, or "".
func SecretFile(name string) string { return os.Getenv("KIGUMI_SECRET_" + name) }

// ToolEnv is the allowlisted environment a build.kg tool process gets:
// PATH, the locale and the temp directory, never the rest.
func ToolEnv() []string {
	var env []string
	for _, name := range []string{"PATH", "LANG", "LC_ALL", "LC_CTYPE", "TMPDIR", "TEMP", "TMP", "SYSTEMROOT"} {
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	return env
}

// CCEnv is the allowlisted environment a build's C compiler/archiver
// subprocess gets when --reproducible pins its own isolated compiler
// cache: internal/driver may not read the ambient environment itself
// so it starts from this instead of os.Environ().
func CCEnv() []string { return ToolEnv() }
