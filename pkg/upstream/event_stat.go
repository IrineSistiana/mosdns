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
	"net"
	"sync/atomic"
)

type Event int

const (
	EventConnOpen Event = iota
	EventConnClose
)

type EventObserver interface {
	OnEvent(typ Event)
}

type nopEO struct{}

func (n nopEO) OnEvent(_ Event) {}

type connWrapper struct {
	net.Conn
	closed atomic.Bool
	ob     EventObserver
}

// wrapConn wraps c into a connWrapper so that we can observe the connection close.
// For convenient, if c is nil, wrapConn returns nil as well. If ob is nopEO, wrapConn
// returns c.
func wrapConn(c net.Conn, ob EventObserver) net.Conn {
	if c == nil {
		return nil
	}
	if _, ok := ob.(nopEO); ok {
		return c
	}
	ob.OnEvent(EventConnOpen)
	return &connWrapper{
		Conn: c,
		ob:   ob,
	}
}

func (c *connWrapper) Close() error {
	if c.closed.CompareAndSwap(false, true) {
		c.ob.OnEvent(EventConnClose)
	}
	return c.Conn.Close()
}
