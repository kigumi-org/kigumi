package hostshell

import (
	"bytes"
	"io"
	"os"
	"os/exec"
)

// stageRun tracks one running pipeline stage so a startup failure can stop
// and reap every stage already started, in the same order they ran in.
type stageRun struct {
	cmd    *exec.Cmd
	stderr *bytes.Buffer
	done   chan error // filter stages only
	out    io.Closer  // this stage's output pipe reader
}

// Run starts every stage concurrently, chained through pipes, and
// collects the final stdout, all stderr and the exit statuses, in stage
// order. A startup failure stops and reaps the stages already started
// instead of leaking them.
func Run(p *Plan, cwd string) (stdout, stderr []byte, statuses []int, err error) {
	var input io.Reader = bytes.NewReader(nil)
	var upstream *os.File
	var runs []*stageRun
	for _, st := range p.Stages {
		if st.Filter != nil {
			upstream = nil
			pr, pw := io.Pipe()
			fin := input
			done := make(chan error, 1)
			f := st.Filter
			go func() {
				err := f(fin, pw)
				pw.CloseWithError(err)
				done <- err
			}()
			runs = append(runs, &stageRun{done: done, out: pr})
			input = pr
			continue
		}
		if len(st.Argv) == 0 {
			continue
		}
		cmd := exec.Command(st.Argv[0], st.Argv[1:]...)
		cmd.Dir = cwd
		var stderrBuf bytes.Buffer
		pr, pw, perr := os.Pipe()
		if perr != nil {
			return abortPlan(runs, perr)
		}
		fds := fdTable{input, pw, &stderrBuf}
		opened, rerr := applyRedirects(&fds, st.Redirs, cwd)
		if rerr != nil {
			pr.Close()
			pw.Close()
			closeAll(opened)
			return abortPlan(runs, rerr)
		}
		cmd.Stdin, cmd.Stdout, cmd.Stderr = asReader(fds[0]), asWriter(fds[1]), asWriter(fds[2])
		serr := cmd.Start()
		pw.Close()
		closeAll(opened)
		// The child has its own descriptor now. Holding a second reader on the
		// upstream pipe would keep the writer alive after this stage exits, so
		// `producer | head` would never finish.
		if upstream != nil {
			upstream.Close()
			upstream = nil
		}
		if serr != nil {
			pr.Close()
			return abortPlan(runs, serr)
		}
		runs = append(runs, &stageRun{cmd: cmd, stderr: &stderrBuf, out: pr})
		input, upstream = pr, pr
	}
	out, rerr := io.ReadAll(input)
	var errBuf bytes.Buffer
	for _, r := range runs {
		if r.cmd != nil {
			werr := r.cmd.Wait()
			errBuf.Write(r.stderr.Bytes())
			code := 0
			if ee, ok := werr.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else if werr != nil && rerr == nil {
				rerr = werr
			}
			statuses = append(statuses, code)
			continue
		}
		if ferr := <-r.done; ferr != nil && rerr == nil {
			rerr = ferr
		}
		statuses = append(statuses, 0)
	}
	if rerr != nil {
		return nil, nil, nil, rerr
	}
	return out, errBuf.Bytes(), statuses, nil
}

func closeAll(fs []*os.File) {
	for _, f := range fs {
		f.Close()
	}
}

// abortPlan stops and reaps every stage already started: it closes each
// stage's output pipe (unblocking a filter goroutine stuck writing to a
// downstream that will never start) and kills and waits any started
// external command.
func abortPlan(runs []*stageRun, rerr error) (stdout, stderr []byte, statuses []int, err error) {
	for _, r := range runs {
		if r.out != nil {
			r.out.Close()
		}
		if r.cmd != nil {
			r.cmd.Process.Kill()
			r.cmd.Wait()
		}
		if r.done != nil {
			<-r.done
		}
	}
	return nil, nil, nil, rerr
}
