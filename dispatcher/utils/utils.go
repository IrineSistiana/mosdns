//     Copyright (C) 2020, IrineSistiana
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

package utils

import (
	"net"
	"strconv"
	"strings"
)

// GetIPFromAddr returns net.IP from net.Addr.
// Will return nil if no ip address can be parsed.
func GetIPFromAddr(addr net.Addr) (ip net.IP) {
	switch v := addr.(type) {
	case *net.TCPAddr:
		return v.IP
	case *net.UDPAddr:
		return v.IP
	case *net.IPNet:
		return v.IP
	default:
		ipStr, _, err := net.SplitHostPort(addr.String())
		if err != nil {
			return nil
		}
		return net.ParseIP(ipStr)
	}
}

// ParseAddr splits addr to protocol and host.
func ParseAddr(addr string) (protocol, host string) {
	if s := strings.SplitN(addr, "://", 2); len(s) == 2 {
		protocol = s[0]
		host = s[1]
	} else {
		host = addr
	}

	return
}

// TryAddPort add port to host if host does not has an port suffix.
func TryAddPort(host string, port uint16) string {
	if _, p, _ := net.SplitHostPort(host); len(p) == 0 {
		return host + ":" + strconv.Itoa(int(port))
	}
	return host
}
