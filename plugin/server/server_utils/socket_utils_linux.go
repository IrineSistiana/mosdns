//go:build linux

package server_utils

import (
	"syscall"

	"golang.org/x/sys/unix"
)

const tcpFastOpenBacklog = 4096

func ListenerControl(opt ListenerSocketOpts) ControlFunc {
	return func(network, address string, c syscall.RawConn) error {
		var (
			errControl error
			errSyscall error
		)

		errControl = c.Control(func(fd uintptr) {
			if opt.TCP_FAST_OPEN && isTCPNetwork(network) {
				errSyscall = unix.SetsockoptInt(int(fd), unix.IPPROTO_TCP, unix.TCP_FASTOPEN, tcpFastOpenBacklog)
				if errSyscall != nil {
					return
				}
			}

			if opt.SO_REUSEPORT {
				errSyscall = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
				if errSyscall != nil {
					return
				}
			}

			if opt.SO_RCVBUF > 0 {
				errSyscall = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_RCVBUF, opt.SO_RCVBUF)
				if errSyscall != nil {
					return
				}
			}

			if opt.SO_SNDBUF > 0 {
				errSyscall = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_SNDBUF, opt.SO_SNDBUF)
				if errSyscall != nil {
					return
				}
			}
		})

		if errControl != nil {
			return errControl
		}
		return errSyscall
	}
}

func isTCPNetwork(network string) bool {
	switch network {
	case "tcp", "tcp4", "tcp6":
		return true
	default:
		return false
	}
}
