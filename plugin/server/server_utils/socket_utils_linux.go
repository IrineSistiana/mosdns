//go:build linux

package server_utils

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func ListenerControl(opt ListenerSocketOpts) ControlFunc {
	return func(network, address string, c syscall.RawConn) error {
		var (
			errControl error
			errSyscall error
		)

		errControl = c.Control(func(fd uintptr) {
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
