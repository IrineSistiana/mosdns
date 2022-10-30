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

type ipv4cmc struct {
	c *ipv4.PacketConn
}

func newIpv4cmc(c *ipv4.PacketConn) *ipv4cmc {
	return &ipv4cmc{c: c}
}

func (i *ipv4cmc) readFrom(b []byte) (n int, dst net.IP, IfIndex int, src net.Addr, err error) {
	n, cm, src, err := i.c.ReadFrom(b)
	if cm != nil {
		dst, IfIndex = cm.Dst, cm.IfIndex
	}
	return
}

func (i *ipv4cmc) writeTo(b []byte, src net.IP, IfIndex int, dst net.Addr) (n int, err error) {
	cm := &ipv4.ControlMessage{
		Src:     src,
		IfIndex: IfIndex,
	}
	return i.c.WriteTo(b, cm, dst)
}

type ipv6cmc struct {
	c4 *ipv4.PacketConn // ipv4 entrypoint for sending ipv4 packages.
	c6 *ipv6.PacketConn
}

func newIpv6PacketConn(c4 *ipv4.PacketConn, c6 *ipv6.PacketConn) *ipv6cmc {
	return &ipv6cmc{c4: c4, c6: c6}
}

func (i *ipv6cmc) readFrom(b []byte) (n int, dst net.IP, IfIndex int, src net.Addr, err error) {
	n, cm, src, err := i.c6.ReadFrom(b)
	if cm != nil {
		dst, IfIndex = cm.Dst, cm.IfIndex
	}
	return
}

func (i *ipv6cmc) writeTo(b []byte, src net.IP, IfIndex int, dst net.Addr) (n int, err error) {
	if src != nil {
		// If src is ipv4, use IP_PKTINFO instead of IPV6_PKTINFO.
		// Otherwise, sendmsg will raise "invalid argument" error.
		// No official doc found.
		if src4 := src.To4(); src4 != nil {
			cm4 := &ipv4.ControlMessage{
				Src:     src4,
				IfIndex: IfIndex,
			}
			return i.c4.WriteTo(b, cm4, dst)
		}
	}
	cm6 := &ipv6.ControlMessage{
		Src:     src,
		IfIndex: IfIndex,
	}
	return i.c6.WriteTo(b, cm6, dst)
}

func newCmc(c *net.UDPConn) (cmcUDPConn, error) {
	sc, err := c.SyscallConn()
	if err != nil {
		return nil, err
	}

	var controlErr error
	var cmc cmcUDPConn

	if err := sc.Control(func(fd uintptr) {
		v, err := unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_DOMAIN)
		if err != nil {
			controlErr = os.NewSyscallError("failed to get SO_PROTOCOL", err)
			return
		}
		switch v {
		case unix.AF_INET:
			c4 := ipv4.NewPacketConn(c)
			if err := c4.SetControlMessage(ipv4.FlagDst|ipv4.FlagInterface, true); err != nil {
				controlErr = fmt.Errorf("failed to set ipv4 cmsg flags, %w", err)
			}
			cmc = newIpv4cmc(c4)
			return
		case unix.AF_INET6:
			c6 := ipv6.NewPacketConn(c)
			if err := c6.SetControlMessage(ipv6.FlagDst|ipv6.FlagInterface, true); err != nil {
				controlErr = fmt.Errorf("failed to set ipv6 cmsg flags, %w", err)
			}
			cmc = newIpv6PacketConn(ipv4.NewPacketConn(c), c6)
			return
		default:
			controlErr = fmt.Errorf("socket protocol %d is not supported", v)
		}
	}); err != nil {
		return nil, fmt.Errorf("control fd err, %w", controlErr)
	}

	if controlErr != nil {
		return nil, fmt.Errorf("failed to set up socket, %w", controlErr)
	}
	return cmc, nil
}
