//go:build !linux

package server_utils

func ListenerControl(opt ListenerSocketOpts) ControlFunc {
	return NopControlFunc
}
