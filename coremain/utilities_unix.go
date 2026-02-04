//go:build linux || darwin || freebsd || netbsd || openbsd

package coremain

import (
	"syscall"
)

func serverSocketOption(c syscall.RawConn, cfg *ServerListenerConfig, network, address string) error {
	var errSysCall error
	errControl := c.Control(func(fd uintptr) {
		if cfg.ReuseAddr {
			errSysCall = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
		}
	})
	if errSysCall != nil {
		return errSysCall
	}
	return errControl
}
