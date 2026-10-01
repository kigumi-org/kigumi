// Package netsock is the interp and vm accelerators' shared IPv4 TCP
// implementation of std/net's socket layer. It works directly on raw file
// descriptors, the same currency std/net/net_hosted.kg's extern(C) body
// uses, so a Conn/Listener's `fd` field means the same thing under every
// backend.
package netsock

import (
	"fmt"
	"net"
	"strconv"
	"syscall"
)

// Dial resolves host:port over IPv4 and returns a connected socket's fd.
func Dial(host string, port int64) (int, error) {
	addr, err := net.ResolveTCPAddr("tcp4", net.JoinHostPort(host, strconv.FormatInt(port, 10)))
	if err != nil {
		return -1, err
	}
	ip4 := addr.IP.To4()
	if ip4 == nil {
		return -1, fmt.Errorf("%s does not resolve to an IPv4 address", host)
	}
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		return -1, err
	}
	sa := &syscall.SockaddrInet4{Port: addr.Port}
	copy(sa.Addr[:], ip4)
	if err := syscall.Connect(fd, sa); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	return fd, nil
}

// Listen binds host:port (SO_REUSEADDR set) over IPv4 and starts listening;
// an empty host binds every interface.
func Listen(host string, port int64) (int, error) {
	if host == "" {
		host = "0.0.0.0"
	}
	ip := net.ParseIP(host)
	if ip == nil {
		addr, err := net.ResolveIPAddr("ip4", host)
		if err != nil {
			return -1, err
		}
		ip = addr.IP
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return -1, fmt.Errorf("%s does not resolve to an IPv4 address", host)
	}
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		return -1, err
	}
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	sa := &syscall.SockaddrInet4{Port: int(port)}
	copy(sa.Addr[:], ip4)
	if err := syscall.Bind(fd, sa); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	if err := syscall.Listen(fd, 16); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	return fd, nil
}

// Accept blocks for the next connection on a listening fd.
func Accept(fd int) (int, error) {
	nfd, _, err := syscall.Accept(fd)
	return nfd, err
}

// Port is the local port a bound fd holds, or 0 when it cannot be read.
func Port(fd int) int {
	sa, err := syscall.Getsockname(fd)
	if err != nil {
		return 0
	}
	if sa4, ok := sa.(*syscall.SockaddrInet4); ok {
		return sa4.Port
	}
	return 0
}

// Read fills buf and reports how many bytes came in; 0, nil is EOF.
func Read(fd int, buf []byte) (int, error) { return syscall.Read(fd, buf) }

// Write may send fewer bytes than given, like the POSIX call it wraps.
func Write(fd int, data []byte) (int, error) { return syscall.Write(fd, data) }

// Close closes fd; the caller reports the error or ignores it, matching
// close(2)'s two Kigumi-side callers (an explicit close and a drop).
func Close(fd int) error { return syscall.Close(fd) }
