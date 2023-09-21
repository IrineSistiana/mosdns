//go:build linux

/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package upstream

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func getSocketControlFunc(opts socketOpts) func(string, string, syscall.RawConn) error {
	return func(_, _ string, c syscall.RawConn) error {
		var sysCallErr error
		if err := c.Control(func(fd uintptr) {
			// SO_MARK
			if opts.so_mark > 0 {
				sysCallErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_MARK, opts.so_mark)
				if sysCallErr != nil {
					sysCallErr = os.NewSyscallError("failed to set SO_MARK", sysCallErr)
					return
				}
			}

			// SO_BINDTODEVICE
			if len(opts.bind_to_device) > 0 {
				sysCallErr = unix.SetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, opts.bind_to_device)
				if sysCallErr != nil {
					sysCallErr = os.NewSyscallError("failed to set SO_BINDTODEVICE", sysCallErr)
					return
				}
			}

		}); err != nil {
			return err
		}
		return sysCallErr
	}
}
