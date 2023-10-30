package server_utils

import "syscall"

type ControlFunc func(network, address string, c syscall.RawConn) error

func NopControlFunc(network, address string, c syscall.RawConn) error {
	return nil
}

type ListenerSocketOpts struct {
	SO_REUSEPORT bool
	SO_RCVBUF    int
	SO_SNDBUF    int
}
