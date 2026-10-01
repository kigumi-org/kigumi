package vm

import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// readSome waits up to timeoutMs for stdin to have data, then makes one
// Read call and returns what it got: nil with no error on timeout, io.EOF
// when stdin is closed. It waits with poll(2) on the raw descriptor
// instead of a goroutine race, so a timeout never strands a blocked read
// that a later call would collide with; a source that isn't a file (only
// possible in tests) skips the wait and reads directly.
func (m *Machine) readSome(timeoutMs int64) ([]byte, error) {
	r := m.stdinReader()
	if r.Buffered() == 0 {
		if f, ok := m.stdinSrc.(*os.File); ok {
			ready, err := pollReadable(int(f.Fd()), timeoutMs)
			if err != nil {
				return nil, err
			}
			if !ready {
				return nil, nil
			}
		}
	}
	buf := make([]byte, 65536)
	n, err := r.Read(buf)
	if err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, err
	}
	return buf[:n], nil
}

// pollReadable is poll(2) on one descriptor, retried across EINTR.
func pollReadable(fd int, timeoutMs int64) (bool, error) {
	if timeoutMs < 0 {
		timeoutMs = 0
	}
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	for {
		n, err := unix.Poll(fds, int(timeoutMs))
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return false, err
		}
		return n > 0, nil
	}
}
