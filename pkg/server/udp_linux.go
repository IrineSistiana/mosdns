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
	"errors"
	"fmt"
	"net"
	"os"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
	"golang.org/x/sys/unix"
)

var (
	errCmNoDstAddr = errors.New("control msg does not have dst address")
)

func getOOBFromCM4(oob []byte) (net.IP, error) {
	var cm ipv4.ControlMessage
	if err := cm.Parse(oob); err != nil {
		return nil, err
	}
	if cm.Dst == nil {
		return nil, errCmNoDstAddr
	}
	return cm.Dst, nil
}

func getOOBFromCM6(oob []byte) (net.IP, error) {
	var cm ipv6.ControlMessage
	if err := cm.Parse(oob); err != nil {
		return nil, err
	}
	if cm.Dst == nil {
		return nil, errCmNoDstAddr
	}
	return cm.Dst, nil
}

func srcIP2Cm(ip net.IP) []byte {
	if ip4 := ip.To4(); ip4 != nil {
		return (&ipv4.ControlMessage{
			Src: ip,
		}).Marshal()
	}

	if ip6 := ip.To16(); ip6 != nil {
		return (&ipv6.ControlMessage{
			Src: ip,
		}).Marshal()
	}

	return nil
}

func initOobHandler(c *net.UDPConn) (getSrcAddrFromOOB, writeSrcAddrToOOB, error) {
	if !c.LocalAddr().(*net.UDPAddr).IP.IsUnspecified() {
		return nil, nil, nil
	}

	sc, err := c.SyscallConn()
	if err != nil {
		return nil, nil, err
	}

	var getter getSrcAddrFromOOB
	var setter writeSrcAddrToOOB
	var controlErr error
	if err := sc.Control(func(fd uintptr) {
		v, err := unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_DOMAIN)
		if err != nil {
			controlErr = os.NewSyscallError("failed to get SO_PROTOCOL", err)
			return
		}
		switch v {
		case unix.AF_INET:
			c4 := ipv4.NewPacketConn(c)
			if err := c4.SetControlMessage(ipv4.FlagDst, true); err != nil {
				controlErr = fmt.Errorf("failed to set ipv4 cmsg flags, %w", err)
			}

			getter = getOOBFromCM4
			setter = srcIP2Cm
			return
		case unix.AF_INET6:
			c6 := ipv6.NewPacketConn(c)
			if err := c6.SetControlMessage(ipv6.FlagDst, true); err != nil {
				controlErr = fmt.Errorf("failed to set ipv6 cmsg flags, %w", err)
			}
			getter = getOOBFromCM6
			setter = srcIP2Cm
			return
		default:
			controlErr = fmt.Errorf("socket protocol %d is not supported", v)
		}
	}); err != nil {
		return nil, nil, fmt.Errorf("control fd err, %w", controlErr)
	}

	if controlErr != nil {
		return nil, nil, fmt.Errorf("failed to set up socket, %w", controlErr)
	}
	return getter, setter, nil
}
