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

package utils

import (
	"net"
	"net/netip"
)

// GetIPFromAddr returns a net.IP from the given net.Addr.
// addr can be *net.TCPAddr, *net.UDPAddr, *net.IPNet, *net.IPAddr
// Will return nil otherwise.
func GetIPFromAddr(addr net.Addr) (ip net.IP) {
	switch v := addr.(type) {
	case *net.TCPAddr:
		return v.IP
	case *net.UDPAddr:
		return v.IP
	case *net.IPNet:
		return v.IP
	case *net.IPAddr:
		return v.IP
	}
	return nil
}

// GetAddrFromAddr returns netip.Addr from net.Addr.
// See also: GetIPFromAddr.
func GetAddrFromAddr(addr net.Addr) netip.Addr {
	a, _ := netip.AddrFromSlice(GetIPFromAddr(addr))
	return a
}

// SplitSchemeAndHost splits addr to protocol and host.
func SplitSchemeAndHost(addr string) (protocol, host string) {
	if protocol, host, ok := SplitString2(addr, "://"); ok {
		return protocol, host
	} else {
		return "", addr
	}
}
