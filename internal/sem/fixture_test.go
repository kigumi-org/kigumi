package sem_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/testkit"
	"kigumi/internal/token"
)

// A fixture is one txtar file: an optional `-- module --`
// section of key: value lines, source sections named by their root-relative
// path, and a final `-- expect --` golden.
type section struct {
	name string
	body string
	line int
}

type fixture struct {
	path      string
	raw       string
	opts      map[string]string
	sections  []section
	expect    string
	expectOff int
}

func parseFixture(t *testing.T, path string) fixture {
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fx := fixture{path: path, raw: string(raw), opts: map[string]string{}, expectOff: -1}
	lines := strings.SplitAfter(fx.raw, "\n")
	var cur *section
	off := 0
	for i, l := range lines {
		if strings.HasPrefix(l, "-- ") && strings.HasSuffix(strings.TrimRight(l, "\n"), " --") {
			name := strings.TrimSuffix(strings.TrimPrefix(strings.TrimRight(l, "\n"), "-- "), " --")
			if name == "expect" {
				fx.expectOff = off + len(l)
				fx.expect = fx.raw[fx.expectOff:]
				cur = nil
				break
			}
			fx.sections = append(fx.sections, section{name: name, line: i + 2})
			cur = &fx.sections[len(fx.sections)-1]
		} else if cur != nil {
			cur.body += l
		}
		off += len(l)
	}
	if fx.expectOff < 0 {
		t.Fatalf("%s: missing -- expect -- section", path)
	}
	for _, s := range fx.sections {
		if s.name == "module" {
			for _, l := range strings.Split(strings.TrimSpace(s.body), "\n") {
				k, v, ok := strings.Cut(l, ":")
				if !ok {
					t.Fatalf("%s: bad module line %q", path, l)
				}
				fx.opts[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}
	for k := range fx.opts {
		switch k {
		case "entry", "test", "std", "deps", "target", "ptr", "pending":
		default:
			t.Fatalf("%s: unknown module option %q", path, k)
		}
	}
	return fx
}

func (fx fixture) opt(key, def string) string {
	if v, ok := fx.opts[key]; ok {
		return v
	}
	return def
}

func (fx fixture) deps() map[string]bool {
	out := map[string]bool{}
	for _, d := range strings.Split(fx.opt("deps", ""), ",") {
		if d = strings.TrimSpace(d); d != "" {
			out[d] = true
		}
	}
	return out
}

// buildModule assembles the sem.Module of a fixture: its own sections, the
// shared std stubs (unless std: none), and inline std sections that replace a
// stub package of the same path.
func buildModule(t *testing.T, fx fixture) *sem.Module {
	pkgs := map[string]*sem.Package{}
	deps := fx.deps()
	if fx.opt("std", "default") == "default" {
		for _, p := range testkit.LoadStd(t, "../../std") {
			pkgs[p.Path] = p
		}
	} else {
		for _, p := range testkit.LoadPrelude(t, "../../std") {
			pkgs[p.Path] = p
		}
	}
	entries := map[string]bool{}
	for _, e := range strings.Split(fx.opt("entry", ""), ",") {
		if e = strings.TrimSpace(e); e != "" {
			entries[e] = true
		}
	}
	inlineStd := map[string]bool{}
	for _, s := range fx.sections {
		if s.name == "module" {
			continue
		}
		name := s.name
		if name == "src" {
			name = "main/main.kg"
			entries[name] = true
		}
		if strings.HasSuffix(name, "_test.kg") && fx.opt("test", "true") != "true" {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(name))
		first, _, _ := strings.Cut(name, "/")
		module := ""
		if first == "std" {
			module = "std"
			if !inlineStd[dir] {
				inlineStd[dir] = true
				delete(pkgs, dir)
			}
		} else if deps[first] {
			module = first
		}
		tree := syntax.Parse(token.NewFile(name, []byte(s.body)))
		if tree.HasErrors() {
			t.Fatalf("%s: section %s has parse errors (parse errors belong in testdata/parser):\n%s", fx.path, name, tree.Dump())
		}
		p := pkgs[dir]
		if p == nil {
			p = &sem.Package{Path: dir, Module: module, Std: module == "std"}
			pkgs[dir] = p
		}
		p.Files = append(p.Files, tree)
		if entries[name] {
			p.Entry = tree
		}
	}
	mod := &sem.Module{}
	if fx.opt("target", "hosted") == "freestanding" {
		mod.Target = sem.Freestanding
	}
	paths := make([]string, 0, len(pkgs))
	for p := range pkgs {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		mod.Packages = append(mod.Packages, pkgs[p])
	}
	return mod
}
