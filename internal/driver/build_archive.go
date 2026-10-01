package driver

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// splitArchiveCFlags keeps linker-shaped flags (-Wl,*, -l*, -L*, -T) out of
// buildArchive's compile-only step: it never links, and zig cc silently
// corrupts the object when --gc-sections reaches a -c invocation together
// with -ffunction-sections.
func splitArchiveCFlags(flags []string) (compile, linkerLike []string) {
	for _, f := range flags {
		if strings.HasPrefix(f, "-Wl,") || strings.HasPrefix(f, "-l") || strings.HasPrefix(f, "-L") || strings.HasPrefix(f, "-T") {
			linkerLike = append(linkerLike, f)
			continue
		}
		compile = append(compile, f)
	}
	return compile, linkerLike
}

// buildArchive compiles every input to an object and packs them into a
// static archive: a freestanding target has no startup code or libc of
// its own, so linking is the embedder's step.
// Compiles run in dir with relative inputs and a pinned debug
// compilation dir: clang otherwise bakes the temp path into type names
// and DW_AT_comp_dir, and two builds of one source diverge.
// -ffunction-sections/-fdata-sections split each object's code into one
// section per function: without them the embedder's link-time
// `--gc-sections` has nothing to drop.
func buildArchive(cc, args, inputs []string, dir, out string, env []string, stderr io.Writer) (bool, error) {
	var objs []string
	for i, in := range inputs {
		// in may still be relative (e.g. a Source under a relative module
		// root); filepath.Rel errors if only one of its two arguments is
		// absolute, and dir always is.
		absIn, err := filepath.Abs(in)
		if err != nil {
			return false, err
		}
		rel, err := filepath.Rel(dir, absIn)
		if err != nil {
			return false, err
		}
		obj := fmt.Sprintf("obj%d.o", i)
		cargs := append(append([]string{}, args...), "-ffreestanding", "-fno-stack-protector", "-fPIE", "-ffunction-sections", "-fdata-sections", "-fdebug-compilation-dir=.", "-c", "-o", obj, rel)
		cmd := exec.Command(cc[0], cargs...)
		cmd.Dir = dir
		cmd.Stderr = stderr
		if env != nil {
			cmd.Env = env
		}
		if err := cmd.Run(); err != nil {
			return false, fmt.Errorf("compile %s: %w", filepath.Base(in), err)
		}
		objs = append(objs, obj)
	}
	os.Remove(out)
	ar := []string{"ar"}
	if strings.HasSuffix(cc[0], "zig") || filepath.Base(cc[0]) == "zig" {
		ar = []string{cc[0], "ar"}
	}
	absOut, err := filepath.Abs(out)
	if err != nil {
		return false, err
	}
	// rcsD: D zeroes member timestamp/uid/gid, so archives of the same
	// objects are byte-identical across runs and machines.
	cmd := exec.Command(ar[0], append(append(ar[1:], "rcsD", absOut), objs...)...)
	cmd.Dir = dir
	cmd.Stderr = stderr
	if env != nil {
		cmd.Env = env
	}
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("archive failed: %w", err)
	}
	return true, nil
}
