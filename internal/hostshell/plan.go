// Package hostshell runs shell plans: external commands chained
// through pipes, in-process filters and redirects. Both executors hand it
// the plan std/shell parsed.
package hostshell

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"kigumi/internal/syntax"
)

// Plan is a shell pipeline: external commands and in-process stages.
type Plan struct {
	Stages []Stage
}

type Stage struct {
	Argv   []string
	Filter func(in io.Reader, out io.Writer) error
	Redirs []Redirect
}

type Redirect struct {
	Op     uint32
	Fd     int
	Target string
}

// GrepFilter keeps the lines matching pattern.
func GrepFilter(pattern string) func(io.Reader, io.Writer) error {
	return func(in io.Reader, out io.Writer) error {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return err
		}
		sc := bufio.NewScanner(in)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024*64)
		for sc.Scan() {
			if re.MatchString(sc.Text()) {
				fmt.Fprintln(out, sc.Text())
			}
		}
		return sc.Err()
	}
}

// fdTable is a stage's file descriptors 0, 1 and 2 while its redirects are
// applied. Entries stay io.Reader/io.Writer rather than *os.File so an
// unredirected fd can keep pointing at the pipeline's own endpoint, and so
// `>&` copies whatever the descriptor holds at that point in source order.
type fdTable [3]any

// applyRedirects walks a stage's redirects left to right, which is what makes
// `2>&1 >out` differ from `>out 2>&1`. Files it opens are returned so
// the parent can close its own copies once the child has them.
func applyRedirects(fds *fdTable, redirs []Redirect, dir string) ([]*os.File, error) {
	var opened []*os.File
	for _, r := range redirs {
		if r.Fd < 0 || r.Fd >= len(fds) {
			return opened, fmt.Errorf("only file descriptors 0, 1 and 2 can be redirected, got %d", r.Fd)
		}
		if r.Op == syntax.RedirDupIn || r.Op == syntax.RedirDupOut {
			n, err := strconv.Atoi(r.Target)
			if err != nil || n < 0 || n >= len(fds) {
				return opened, fmt.Errorf("`%s` is not a file descriptor that can be duplicated", r.Target)
			}
			fds[r.Fd] = fds[n]
			continue
		}
		f, err := openRedirect(r, dir)
		if err != nil {
			return opened, err
		}
		opened = append(opened, f)
		fds[r.Fd] = f
	}
	return opened, nil
}

func openRedirect(r Redirect, dir string) (*os.File, error) {
	path := r.Target
	if dir != "" && !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	switch r.Op {
	case syntax.RedirIn:
		return os.Open(path)
	case syntax.RedirAppend:
		return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	default:
		return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	}
}

func asReader(v any) io.Reader {
	r, _ := v.(io.Reader)
	return r
}

func asWriter(v any) io.Writer {
	w, _ := v.(io.Writer)
	return w
}
