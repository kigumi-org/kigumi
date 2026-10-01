// Package cheader imports a C header into Kigumi extern(C) declarations by
// running `zig translate-c` and reading its output, instead of embedding a
// C parser.
package cheader

import (
	"fmt"
	"os"
	"path/filepath"
)

// Options selects one header import.
type Options struct {
	// ZigPath is the zig binary to run (internal/driver's own lookup).
	ZigPath string
	// Target is the compile triple translate-c should see, matching the
	// current build (internal/driver Target.compileTriple()); "" for host.
	Target string
	ABI    ABI
	// HeaderPath is the header's absolute path.
	HeaderPath string
	// CacheDir is the base directory Import keys entries under; the
	// driver points it at the module cache root it already uses.
	CacheDir string
	// Run overrides how translate-c is invoked; nil uses RunZig.
	Run Runner
}

// Result is one header's generated Kigumi source.
type Result struct {
	// Source is the generated package's file content.
	Source string
	// Skipped lists what the header declared that this importer left
	// out, one entry per declaration; nil when
	// nothing was skipped.
	Skipped []string
	// FromCache is true when Source came from a prior run's cache entry
	// rather than a fresh zig translate-c invocation.
	FromCache bool
	// CachePath is the generated file's path on disk, so the driver can
	// load it like any other package file (real spans for diagnostics,
	// hover and doc).
	CachePath string
}

// Import reads and, unless a matching cache entry already exists, invokes
// zig translate-c for opts.HeaderPath, and returns the generated Kigumi
// source (cache.go, translate.go, Generate in generate.go).
func Import(opts Options) (Result, error) {
	run := opts.Run
	if run == nil {
		run = RunZig
	}
	header, err := os.ReadFile(opts.HeaderPath)
	if err != nil {
		return Result{}, err
	}
	version, err := zigFingerprint(opts.ZigPath)
	if err != nil {
		return Result{}, err
	}
	dir := filepath.Join(opts.CacheDir, cacheKey(header, version, opts.Target, opts.ABI))
	genPath := filepath.Join(dir, "gen.kg")
	if src, err := os.ReadFile(genPath); err == nil {
		return Result{Source: string(src), Skipped: readSkipped(filepath.Join(dir, "skipped.txt")), FromCache: true, CachePath: genPath}, nil
	}
	full, baseline, err := translateBoth(run, opts.ZigPath, opts.Target, opts.HeaderPath)
	if err != nil {
		return Result{}, fmt.Errorf("importing %s: %w", opts.HeaderPath, err)
	}
	source, skipped := Generate(full, baseline, opts.ABI)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{}, err
	}
	if err := writeFileAtomic(genPath, []byte(source)); err != nil {
		return Result{}, err
	}
	if err := writeSkipped(filepath.Join(dir, "skipped.txt"), skipped); err != nil {
		return Result{}, err
	}
	return Result{Source: source, Skipped: skipped, FromCache: false, CachePath: genPath}, nil
}
