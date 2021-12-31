//     Copyright (C) 2020-2021, IrineSistiana
//
//     This file is part of mosdns.
//
//     mosdns is free software: you can redistribute it and/or modify
//     it under the terms of the GNU General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) any later version.
//
//     mosdns is distributed in the hope that it will be useful,
//     but WITHOUT ANY WARRANTY; without even the implied warranty of
//     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//     GNU General Public License for more details.
//
//     You should have received a copy of the GNU General Public License
//     along with this program.  If not, see <https://www.gnu.org/licenses/>.

package server

import (
	"net"
	"sync"
)

type onceCloseListener struct {
	net.Listener
	once     sync.Once
	closeErr error
}

func (oc *onceCloseListener) Close() error {
	oc.once.Do(func() {
		oc.closeErr = oc.Listener.Close()
	})
	return oc.closeErr
}

type onceClosePackageConn struct {
	net.PacketConn
	once     sync.Once
	closeErr error
}

func (oc *onceClosePackageConn) Close() error {
	oc.once.Do(func() {
		oc.closeErr = oc.PacketConn.Close()
	})
	return oc.closeErr
}
