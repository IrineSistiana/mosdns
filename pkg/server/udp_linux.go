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

package server

import (
	"fmt"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
	"golang.org/x/sys/unix"
	"net"
	"os"
)

type protocol int

const (
	invalid protocol = iota
	v4
	v6
)

type ipv4PacketConn struct {
	c *ipv4.PacketConn
}

func (i ipv4PacketConn) readFrom(b []byte) (n int, cm any, src net.Addr, err error) {
	return i.c.ReadFrom(b)
}

func (i ipv4PacketConn) writeTo(b []byte, cm any, dst net.Addr) (n int, err error) {
	return i.c.WriteTo(b, cm.(*ipv4.ControlMessage), dst)
}

type ipv6PacketConn struct {
	c *ipv6.PacketConn
}

func (i ipv6PacketConn) readFrom(b []byte) (n int, cm any, src net.Addr, err error) {
	return i.c.ReadFrom(b)
}

func (i ipv6PacketConn) writeTo(b []byte, cm any, dst net.Addr) (n int, err error) {
	return i.c.WriteTo(b, cm.(*ipv6.ControlMessage), dst)
}

func newUDPConn(c *net.UDPConn) (cmcUDPConn, error) {
	p, err := getSocketIPProtocol(c)
	if err != nil {
		return nil, fmt.Errorf("failed to get socket ip protocol, %w", err)
	}
	switch p {
	case v4:
		c := ipv4.NewPacketConn(c)
		if err := c.SetControlMessage(ipv4.FlagSrc|ipv4.FlagDst|ipv4.FlagInterface, true); err != nil {
			return nil, fmt.Errorf("failed to set ipv4 cmsg flags, %w", err)
		}
		return ipv4PacketConn{c: c}, nil
	case v6:
		c := ipv6.NewPacketConn(c)
		if err := c.SetControlMessage(ipv6.FlagSrc|ipv6.FlagDst|ipv6.FlagInterface, true); err != nil {
			return nil, fmt.Errorf("failed to set ipv6 cmsg flags, %w", err)
		}
		return ipv6PacketConn{c: c}, nil
	default:
		return nil, fmt.Errorf("unknow protocol %d", p)
	}
}

func getSocketIPProtocol(c *net.UDPConn) (protocol, error) {
	sc, err := c.SyscallConn()
	if err != nil {
		return 0, err
	}
	proto := invalid
	var syscallErr error
	if controlErr := sc.Control(func(fd uintptr) {
		v, err := unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_DOMAIN)
		if err != nil {
			syscallErr = os.NewSyscallError("failed to get SO_PROTOCOL", err)
			return
		}
		switch v {
		case unix.AF_INET:
			proto = v4
		case unix.AF_INET6:
			proto = v6
		default:
			syscallErr = fmt.Errorf("socket protocol %d is not supported", v)
		}
	}); err != nil {
		return 0, fmt.Errorf("control fd err, %w", controlErr)
	}
	return proto, syscallErr
}
