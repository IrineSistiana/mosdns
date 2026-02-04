//go:build !(linux || darwin || freebsd || netbsd || openbsd)

package coremain

import (
	"errors"
	"syscall"
)

func serverSocketOption(c syscall.RawConn, cfg *ServerListenerConfig, network, address string) error {
	if cfg.ReuseAddr {
		return errors.New("reuseaddr not supported on this platform")
	}
	return nil
}
